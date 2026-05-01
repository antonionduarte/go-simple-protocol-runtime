package runtime

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	rtconfig "github.com/antonionduarte/go-simple-protocol-runtime/pkg/runtime/config"
	"github.com/antonionduarte/go-simple-protocol-runtime/pkg/runtime/net"
)

type Runtime struct {
	self                  net.Host
	ctx                   context.Context
	cancelFunc            func()
	msgChannel            chan Message
	timerChannel          chan Timer
	timerMutex            sync.Mutex
	wg                    sync.WaitGroup
	ongoingTimers         map[int]*time.Timer
	ongoingPeriodicTimers map[int]context.CancelFunc
	protocols             map[int]*ProtoProtocol
	networkLayer          net.TransportLayer
	sessionLayer          *net.SessionLayer

	logger *slog.Logger
}

// SessionConnectedHandler can be implemented by a protocol that wants to be
// notified whenever a session is established with some Host.
type SessionConnectedHandler interface {
	OnSessionConnected(net.Host)
}

// SessionDisconnectedHandler can be implemented by a protocol that wants to be
// notified whenever a session is torn down with some Host.
type SessionDisconnectedHandler interface {
	OnSessionDisconnected(net.Host)
}

// Internal representation of session events delivered to each ProtoProtocol.
type sessionEventType int

const (
	sessionConnectedEvent sessionEventType = iota
	sessionDisconnectedEvent
)

type sessionEvent struct {
	kind sessionEventType
	host net.Host
}

// Option configures a Runtime at construction.
type Option func(*Runtime)

// WithLogger overrides the slog.Logger used by the runtime. The runtime
// rebinds the logger with component=runtime so users don't need to do it
// themselves.
func WithLogger(logger *slog.Logger) Option {
	return func(r *Runtime) {
		if logger == nil {
			return
		}
		r.logger = logger.With("component", "runtime")
	}
}

// WithChannelBuffer sets the buffer size for the runtime's internal message
// and timer channels. A non-positive value falls back to the package default.
func WithChannelBuffer(n int) Option {
	return func(r *Runtime) {
		buf := rtconfig.RuntimeMsgTimerBufferOr(n)
		r.msgChannel = make(chan Message, buf)
		r.timerChannel = make(chan Timer, buf)
	}
}

// New constructs a Runtime bound to the given local Host. Protocols, the
// transport layer, and the session layer must be registered before calling
// Start.
func New(self net.Host, opts ...Option) *Runtime {
	defaultBuf := rtconfig.RuntimeMsgTimerBufferOr(0)
	r := &Runtime{
		self:                  self,
		msgChannel:            make(chan Message, defaultBuf),
		timerChannel:          make(chan Timer, defaultBuf),
		ongoingTimers:         make(map[int]*time.Timer),
		ongoingPeriodicTimers: make(map[int]context.CancelFunc),
		protocols:             make(map[int]*ProtoProtocol),
		logger:                slog.Default().With("component", "runtime"),
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

func (r *Runtime) Self() net.Host { return r.self }

func (r *Runtime) Logger() *slog.Logger {
	if r.logger == nil {
		return slog.Default()
	}
	return r.logger
}

// Start sets up a fresh background context for this Runtime instance and
// launches all long-lived goroutines owned by the runtime (protocol event
// loops, session event pump, and the main dispatcher). It returns an error
// if either the network or session layer was not registered.
func (r *Runtime) Start() error {
	if r.networkLayer == nil {
		return errors.New("runtime: network layer not registered")
	}
	if r.sessionLayer == nil {
		return errors.New("runtime: session layer not registered")
	}

	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	r.ctx = ctx
	r.cancelFunc = cancel

	r.Logger().Info("runtime starting")

	r.startProtocols(ctx)
	r.initProtocols()
	r.startSessionEvents(ctx)

	r.wg.Add(1)
	go r.eventHandler(ctx)
	return nil
}

func (r *Runtime) StartWithDuration(duration time.Duration) error {
	if err := r.Start(); err != nil {
		return err
	}
	time.AfterFunc(duration, func() {
		r.Cancel()
	})
	return nil
}

func (r *Runtime) RegisterNetworkLayer(networkLayer net.TransportLayer) {
	r.networkLayer = networkLayer
}

func (r *Runtime) RegisterSessionLayer(sessionLayer *net.SessionLayer) {
	r.sessionLayer = sessionLayer
}

func (r *Runtime) RegisterProtocol(protocol *ProtoProtocol) {
	protocol.bindRuntime(r)
	r.protocols[protocol.ProtocolID()] = protocol
}

func (r *Runtime) GetProtocol(protocolID int) *ProtoProtocol {
	return r.protocols[protocolID]
}

// connect / disconnect are the runtime-internal entry points used by
// ProtocolContext implementations.
func (r *Runtime) connect(host net.Host)    { r.sessionLayer.Connect(host) }
func (r *Runtime) disconnect(host net.Host) { r.sessionLayer.Disconnect(host) }

func (r *Runtime) Cancel() {
	// Cancel tears down the runtime in the following order:
	//   1. Cancel the runtime context (used by all internal goroutines).
	//   2. Stop session and transport layers.
	//   3. Stop and clear all timers.
	//   4. Wait for all goroutines to finish via the WaitGroup.
	if r.cancelFunc != nil {
		r.cancelFunc()
	}
	if r.sessionLayer != nil {
		r.sessionLayer.Cancel()
	}
	if r.networkLayer != nil {
		r.networkLayer.Cancel()
	}
	r.timerMutex.Lock()
	for id, t := range r.ongoingTimers {
		if t != nil {
			t.Stop()
		}
		delete(r.ongoingTimers, id)
	}
	for id, cancel := range r.ongoingPeriodicTimers {
		if cancel != nil {
			cancel()
		}
		delete(r.ongoingPeriodicTimers, id)
	}
	r.timerMutex.Unlock()

	r.wg.Wait()
	r.Logger().Info("runtime stopped")
}

func (r *Runtime) eventHandler(ctx context.Context) {
	defer r.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-r.msgChannel:
			if !r.routeMessage(ctx, msg) {
				return
			}
		case timer := <-r.timerChannel:
			if !r.routeTimer(ctx, timer) {
				return
			}
		case sessionMsg := <-r.sessionLayer.OutMessages():
			r.processMessage(sessionMsg.Msg, sessionMsg.Host())
		}
	}
}

// routeMessage forwards an incoming Message to the destination protocol's
// channel. Returns false if the runtime context fired and the dispatcher
// should exit.
func (r *Runtime) routeMessage(ctx context.Context, msg Message) bool {
	protocol := r.protocols[msg.ProtocolID()]
	if protocol == nil {
		return true
	}
	select {
	case protocol.MessageChannel() <- msg:
		return true
	case <-ctx.Done():
		return false
	}
}

// routeTimer forwards a fired Timer to the destination protocol's channel.
// Returns false if the runtime context fired and the dispatcher should exit.
func (r *Runtime) routeTimer(ctx context.Context, timer Timer) bool {
	protocol := r.protocols[timer.ProtocolID()]
	if protocol == nil {
		return true
	}
	select {
	case protocol.TimerChannel() <- timer:
		return true
	case <-ctx.Done():
		return false
	}
}

func (r *Runtime) startProtocols(ctx context.Context) {
	for _, protocol := range r.protocols {
		r.Logger().Info("starting protocol", "protocolID", protocol.ProtocolID())
		protocol.Start(ctx, &r.wg)
	}
}

func (r *Runtime) initProtocols() {
	for _, protocol := range r.protocols {
		r.Logger().Info("initializing protocol", "protocolID", protocol.ProtocolID())
		protocol.Init()
	}
}

func (r *Runtime) startSessionEvents(ctx context.Context) {
	if r.sessionLayer == nil {
		return
	}

	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case ev := <-r.sessionLayer.OutChannelEvents():
				if !r.dispatchSessionEvent(ctx, ev) {
					return
				}
			}
		}
	}()
}

// dispatchSessionEvent translates a SessionLayer event into the runtime's
// internal sessionEvent kind and fans it out to every registered protocol.
// Returns false if the runtime context fired during fanout, signalling the
// caller to exit its goroutine.
func (r *Runtime) dispatchSessionEvent(ctx context.Context, ev net.SessionEvent) bool {
	switch e := ev.(type) {
	case *net.SessionConnected:
		return r.fanoutSessionEvent(ctx, sessionConnectedEvent, e.Host())
	case *net.SessionDisconnected:
		return r.fanoutSessionEvent(ctx, sessionDisconnectedEvent, e.Host())
	}
	return true
}

// fanoutSessionEvent delivers (kind, host) into every protocol's sessionEvents
// channel, ctx-guarded so a slow consumer or shutdown doesn't wedge the
// caller. Returns false on ctx cancellation.
func (r *Runtime) fanoutSessionEvent(ctx context.Context, kind sessionEventType, host net.Host) bool {
	for _, proto := range r.protocols {
		select {
		case proto.sessionEvents <- sessionEvent{kind: kind, host: host}:
		case <-ctx.Done():
			return false
		}
	}
	return true
}

type senderSetter interface {
	SetSender(net.Host)
}

// parseAppHeader pulls the [ProtocolID(uint16 LE) || MessageID(uint16 LE)]
// prefix out of an application-layer payload. The remaining bytes in buffer
// belong to the message-specific payload.
func parseAppHeader(buffer *bytes.Buffer) (protocolID, messageID uint16, err error) {
	if err = binary.Read(buffer, binary.LittleEndian, &protocolID); err != nil {
		return 0, 0, fmt.Errorf("read protocolID: %w", err)
	}
	if err = binary.Read(buffer, binary.LittleEndian, &messageID); err != nil {
		return protocolID, 0, fmt.Errorf("read messageID: %w", err)
	}
	return protocolID, messageID, nil
}

func (r *Runtime) processMessage(buffer bytes.Buffer, from net.Host) {
	logger := r.Logger()
	protocolID, messageID, err := parseAppHeader(&buffer)
	if err != nil {
		logger.Error("failed to read message header",
			"from", from.ToString(),
			"protocolID", protocolID,
			"err", err,
		)
		return
	}

	protocol, exists := r.protocols[int(protocolID)]
	if !exists {
		logger.Warn("received message for unknown protocol",
			"from", from.ToString(),
			"protocolID", protocolID,
			"messageID", messageID,
		)
		return
	}

	serializer, ok := protocol.msgSerializers[int(messageID)]
	if !ok {
		logger.Warn("received message for unknown messageID",
			"from", from.ToString(),
			"protocolID", protocolID,
			"messageID", messageID,
		)
		return
	}

	message, err := serializer.Deserialize(buffer.Bytes())
	if err != nil {
		logger.Error("failed to deserialize message",
			"from", from.ToString(),
			"protocolID", protocolID,
			"messageID", messageID,
			"err", err,
		)
		return
	}

	if m, ok := message.(senderSetter); ok {
		m.SetSender(from)
	}

	logger.Debug("dispatching message",
		"from", from.ToString(),
		"protocolID", protocolID,
		"messageID", messageID,
	)
	ctx := r.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case protocol.messageChannel <- message:
	case <-ctx.Done():
	}
}
