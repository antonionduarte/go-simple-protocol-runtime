package runtime

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
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
			protocol := r.protocols[msg.ProtocolID()]
			if protocol == nil {
				continue
			}
			select {
			case protocol.MessageChannel() <- msg:
			case <-ctx.Done():
				return
			}
		case timer := <-r.timerChannel:
			protocol := r.protocols[timer.ProtocolID()]
			if protocol == nil {
				continue
			}
			select {
			case protocol.TimerChannel() <- timer:
			case <-ctx.Done():
				return
			}
		case sessionMsg := <-r.sessionLayer.OutMessages():
			r.processMessage(sessionMsg.Msg, sessionMsg.Host())
		}
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
				switch e := ev.(type) {
				case *net.SessionConnected:
					host := e.Host()
					for _, proto := range r.protocols {
						select {
						case proto.sessionEvents <- sessionEvent{kind: sessionConnectedEvent, host: host}:
						case <-ctx.Done():
							return
						}
					}
				case *net.SessionDisconnected:
					host := e.Host()
					for _, proto := range r.protocols {
						select {
						case proto.sessionEvents <- sessionEvent{kind: sessionDisconnectedEvent, host: host}:
						case <-ctx.Done():
							return
						}
					}
				}
			}
		}
	}()
}

type senderSetter interface {
	SetSender(net.Host)
}

func (r *Runtime) processMessage(buffer bytes.Buffer, from net.Host) {
	logger := r.Logger()
	var protocolID, messageID uint16
	if err := binary.Read(&buffer, binary.LittleEndian, &protocolID); err != nil {
		logger.Error("failed to read protocolID from message header",
			"from", from.ToString(),
			"err", err,
		)
		return
	}
	if err := binary.Read(&buffer, binary.LittleEndian, &messageID); err != nil {
		logger.Error("failed to read messageID from message header",
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
