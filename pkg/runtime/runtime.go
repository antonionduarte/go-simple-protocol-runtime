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

	"github.com/antonionduarte/go-simple-protocol-runtime/pkg/runtime/net"
)

type Runtime struct {
	self                  net.Host
	ctx                   context.Context
	cancelFunc            func()
	timerMutex            sync.Mutex
	wg                    sync.WaitGroup
	ongoingTimers         map[int]*time.Timer
	ongoingPeriodicTimers map[int]context.CancelFunc

	protocols     []*ProtoProtocol
	codecLookup   map[uint64]*ProtoProtocol // wireID -> owning protocol; built up as codecs register
	codecLookupMu sync.RWMutex

	retryPolicy       RetryPolicy
	retryMu           sync.Mutex
	connectionRetries map[net.Host]*retryState

	networkLayer net.TransportLayer
	sessionLayer *net.SessionLayer

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
	sessionGivenUpEvent
)

type sessionEvent struct {
	kind     sessionEventType
	host     net.Host
	attempts int // populated for sessionGivenUpEvent
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

// WithChannelBuffer is reserved for future use (per-protocol channel
// buffer overrides). It is currently a no-op; per-protocol buffer sizes
// are set via the protoProtocolChannelBuffer constant in protocol.go.
func WithChannelBuffer(_ int) Option {
	return func(_ *Runtime) {}
}

// New constructs a Runtime bound to the given local Host. Protocols, the
// transport layer, and the session layer must be registered before calling
// Start. The runtime context is created here, so Cancel works regardless
// of whether Start has been called.
func New(self net.Host, opts ...Option) *Runtime {
	ctx, cancel := context.WithCancel(context.Background())
	r := &Runtime{
		self:                  self,
		ctx:                   ctx,
		cancelFunc:            cancel,
		ongoingTimers:         make(map[int]*time.Timer),
		ongoingPeriodicTimers: make(map[int]context.CancelFunc),
		codecLookup:           make(map[uint64]*ProtoProtocol),
		connectionRetries:     make(map[net.Host]*retryState),
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

// Start launches all long-lived goroutines owned by the runtime
// (protocol event loops, session event pump, and the main dispatcher).
// It returns an error if either the network or session layer was not
// registered, or if Cancel has already been called on this runtime.
func (r *Runtime) Start() error {
	if r.networkLayer == nil {
		return errors.New("runtime: network layer not registered")
	}
	if r.sessionLayer == nil {
		return errors.New("runtime: session layer not registered")
	}
	if err := r.ctx.Err(); err != nil {
		return errors.New("runtime: cannot Start a runtime that has been cancelled")
	}

	r.Logger().Info("runtime starting")

	r.startProtocols(r.ctx)
	r.initProtocols()
	r.startSessionEvents(r.ctx)

	r.wg.Add(1)
	go r.eventHandler(r.ctx)
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

// Register wraps the user's Protocol implementation in a ProtoProtocol
// envelope and attaches it to this runtime. This is the recommended way
// to add a protocol; the lower-level RegisterProtocol(*ProtoProtocol)
// exists for tests.
func (r *Runtime) Register(impl Protocol) {
	r.RegisterProtocol(NewProtoProtocol(impl))
}

// RegisterProtocol attaches a ProtoProtocol envelope to this runtime.
// Most callers should use Register(impl) instead; this lower-level entry
// point exists for tests that want to inspect the envelope.
func (r *Runtime) RegisterProtocol(protocol *ProtoProtocol) {
	protocol.bindRuntime(r)
	r.protocols = append(r.protocols, protocol)
}

// connect / disconnect are the runtime-internal entry points used by
// ProtocolContext implementations. disconnect also clears any tracked
// retry intent so a user-initiated disconnect halts further reconnect
// attempts.
func (r *Runtime) connect(host net.Host) { r.sessionLayer.Connect(host) }
func (r *Runtime) disconnect(host net.Host) {
	r.stopRetryFor(host)
	r.sessionLayer.Disconnect(host)
}

func (r *Runtime) Cancel() {
	// Cancel tears down the runtime in the following order:
	//   1. Cancel the runtime context (used by all internal goroutines).
	//   2. Stop session and transport layers.
	//   3. Stop and clear all timers.
	//   4. Wait for all goroutines to finish via the WaitGroup.
	r.cancelFunc()
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

	r.retryTeardown()

	r.wg.Wait()
	r.Logger().Info("runtime stopped")
}

func (r *Runtime) eventHandler(ctx context.Context) {
	defer r.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case sessionMsg := <-r.sessionLayer.OutMessages():
			r.processMessage(sessionMsg.Msg, sessionMsg.Host())
		}
	}
}

func (r *Runtime) startProtocols(ctx context.Context) {
	for _, protocol := range r.protocols {
		r.Logger().Info("starting protocol", "protocols", len(r.protocols))
		protocol.Start(ctx, &r.wg)
	}
}

func (r *Runtime) initProtocols() {
	for _, protocol := range r.protocols {
		r.Logger().Info("initializing protocol")
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
// internal sessionEvent kind, updates retry bookkeeping if applicable, and
// fans the event out to every registered protocol. Returns false if the
// runtime context fired during fanout.
func (r *Runtime) dispatchSessionEvent(ctx context.Context, ev net.SessionEvent) bool {
	switch e := ev.(type) {
	case *net.SessionConnected:
		r.onSessionUpForRetry(e.Host())
		return r.fanoutSessionEvent(ctx, sessionEvent{kind: sessionConnectedEvent, host: e.Host()})
	case *net.SessionDisconnected:
		giveUp, attempts := r.onSessionDownForRetry(e.Host())
		if giveUp {
			if !r.fanoutSessionEvent(ctx, sessionEvent{kind: sessionGivenUpEvent, host: e.Host(), attempts: attempts}) {
				return false
			}
		}
		return r.fanoutSessionEvent(ctx, sessionEvent{kind: sessionDisconnectedEvent, host: e.Host()})
	case *net.SessionFailed:
		giveUp, attempts := r.onSessionDownForRetry(e.Host())
		if giveUp {
			return r.fanoutSessionEvent(ctx, sessionEvent{kind: sessionGivenUpEvent, host: e.Host(), attempts: attempts})
		}
		// SessionFailed with retry-in-progress is suppressed from fanout —
		// protocols only see the eventual SessionConnected (success) or
		// SessionGivenUp (terminal failure). Without a retry policy in
		// effect this branch is unreachable because no retry state existed.
		return true
	}
	return true
}

// fanoutSessionEvent delivers ev into every protocol's sessionEvents
// channel, ctx-guarded so a slow consumer or shutdown doesn't wedge the
// caller. Returns false on ctx cancellation.
func (r *Runtime) fanoutSessionEvent(ctx context.Context, ev sessionEvent) bool {
	for _, proto := range r.protocols {
		select {
		case proto.sessionEvents <- ev:
		case <-ctx.Done():
			return false
		}
	}
	return true
}

type senderSetter interface {
	SetSender(net.Host)
}

// processMessage decodes the wire id from the application-layer payload,
// looks up the owning protocol via the runtime's codecLookup map, decodes
// the message, and pushes it onto that protocol's messageChannel.
func (r *Runtime) processMessage(buffer bytes.Buffer, from net.Host) {
	logger := r.Logger()

	var wireID uint64
	if err := binary.Read(&buffer, binary.LittleEndian, &wireID); err != nil {
		logger.Error("failed to read wireID header",
			"from", from.ToString(),
			"err", err,
		)
		return
	}

	r.codecLookupMu.RLock()
	protocol, exists := r.codecLookup[wireID]
	r.codecLookupMu.RUnlock()
	if !exists {
		logger.Warn("received message for unknown wireID",
			"from", from.ToString(),
			"wireID", fmt.Sprintf("%#x", wireID),
		)
		return
	}

	c, ok := protocol.codecs[wireID]
	if !ok {
		logger.Warn("codec lookup raced",
			"from", from.ToString(),
			"wireID", fmt.Sprintf("%#x", wireID),
		)
		return
	}

	message, err := c.decode(buffer.Bytes())
	if err != nil {
		logger.Error("failed to decode message",
			"from", from.ToString(),
			"wireID", fmt.Sprintf("%#x", wireID),
			"err", err,
		)
		return
	}

	if m, ok := message.(senderSetter); ok {
		m.SetSender(from)
	}

	logger.Debug("dispatching message",
		"from", from.ToString(),
		"wireID", fmt.Sprintf("%#x", wireID),
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
