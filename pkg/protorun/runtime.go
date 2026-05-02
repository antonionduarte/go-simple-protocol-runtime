package protorun

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/antonionduarte/go-simple-protocol-runtime/pkg/transport"
)

// Public sentinel errors. The runtime returns these (or wraps them with
// fmt.Errorf("...: %w", Err...)) so callers can use errors.Is to test
// for specific failure conditions instead of string-matching.
var (
	// ErrNoSessionLayer is returned by Connect/Disconnect/ConnectWithRetry
	// when no SessionLayer has been registered (typically because
	// WithTCPTransport was not supplied to New).
	ErrNoSessionLayer = errors.New("protorun: session layer not registered")

	// ErrNoNetworkLayer is returned by Run when no transport.Layer has
	// been registered.
	ErrNoNetworkLayer = errors.New("protorun: network layer not registered")

	// ErrAlreadyCancelled is returned by Run when Cancel has already
	// been called on the runtime instance.
	ErrAlreadyCancelled = errors.New("protorun: cannot start a runtime that has been cancelled")

	// ErrNoCodec is returned by Send when no Codec has been registered
	// for the message type's wire identifier.
	ErrNoCodec = errors.New("protorun: no codec registered for message type")
)

type Runtime struct {
	self                  transport.Host
	ctx                   context.Context
	cancelFunc            func()
	timerMutex            sync.Mutex
	wg                    sync.WaitGroup
	ongoingTimers         map[int]*time.Timer
	ongoingPeriodicTimers map[int]context.CancelFunc

	protocols          []*protoProtocol
	codecLookup        map[uint64]*protoProtocol // wireID -> owning protocol; built up as codecs register
	codecLookupMu      sync.RWMutex
	protoChannelBuffer int // 0 means use defaultProtoChannelBuffer

	retryPolicy       RetryPolicy
	retryMu           sync.Mutex
	connectionRetries map[transport.Host]*retryState

	// IPC routing tables. requestRoutes maps a request wireID to the
	// protocol that handles it (one handler per type, runtime-wide).
	// notificationFanout maps a notification wireID to the set of
	// subscribed protocols and their captured handler closures.
	requestRoutes        map[uint64]requestRoute
	requestRoutesMu      sync.RWMutex
	notificationFanout   map[uint64]map[*protoProtocol]func(Notification)
	notificationFanoutMu sync.RWMutex

	defaultRequestTimeout time.Duration

	networkLayer transport.Layer
	sessionLayer *transport.SessionLayer

	logger  *slog.Logger
	metrics Metrics
}

// SessionConnectedHandler can be implemented by a protocol that wants to be
// notified whenever a session is established with some Host.
type SessionConnectedHandler interface {
	OnSessionConnected(transport.Host)
}

// SessionDisconnectedHandler can be implemented by a protocol that wants to be
// notified whenever a session is torn down with some Host.
type SessionDisconnectedHandler interface {
	OnSessionDisconnected(transport.Host)
}

// Internal representation of session events delivered to each protoProtocol.
type sessionEventType int

const (
	sessionConnectedEvent sessionEventType = iota
	sessionDisconnectedEvent
	sessionGivenUpEvent
)

type sessionEvent struct {
	kind     sessionEventType
	host     transport.Host
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

// WithChannelBuffer overrides the per-protocol channel buffer size used
// by each registered protocol's message, timer, and session-event
// channels. A non-positive value is ignored; the framework default
// (defaultProtoChannelBuffer) applies.
func WithChannelBuffer(n int) Option {
	return func(r *Runtime) {
		if n > 0 {
			r.protoChannelBuffer = n
		}
	}
}

// New constructs a Runtime bound to the given local Host. Protocols, the
// transport layer, and the session layer must be registered before calling
// Start. The runtime context is created here, so Cancel works regardless
// of whether Start has been called.
func New(self transport.Host, opts ...Option) *Runtime {
	ctx, cancel := context.WithCancel(context.Background())
	r := &Runtime{
		self:                  self,
		ctx:                   ctx,
		cancelFunc:            cancel,
		ongoingTimers:         make(map[int]*time.Timer),
		ongoingPeriodicTimers: make(map[int]context.CancelFunc),
		codecLookup:           make(map[uint64]*protoProtocol),
		connectionRetries:     make(map[transport.Host]*retryState),
		requestRoutes:         make(map[uint64]requestRoute),
		notificationFanout:    make(map[uint64]map[*protoProtocol]func(Notification)),
		logger:                slog.Default().With("component", "runtime"),
		metrics:               noopMetrics{},
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

func (r *Runtime) Self() transport.Host { return r.self }

func (r *Runtime) Logger() *slog.Logger {
	if r.logger == nil {
		return slog.Default()
	}
	return r.logger
}

// start launches all long-lived goroutines owned by the runtime
// (protocol event loops, session event pump, and the main dispatcher)
// and returns immediately. It is the synchronous half of Run: tests
// inside this package use start() directly when they need "register
// then verify" without blocking; user code should call Run instead.
func (r *Runtime) start() error {
	if r.networkLayer == nil {
		return ErrNoNetworkLayer
	}
	if r.sessionLayer == nil {
		return ErrNoSessionLayer
	}
	if err := r.ctx.Err(); err != nil {
		return ErrAlreadyCancelled
	}

	r.Logger().Info("runtime starting")

	r.startProtocols(r.ctx)
	r.initProtocols()
	r.startSessionEvents(r.ctx)

	r.wg.Add(1)
	go r.eventHandler(r.ctx)
	return nil
}

// registerNetworkLayer attaches a Layer. Most users wire this
// via WithTCPTransport — only test code that needs to inject a mock
// transport reaches in here directly.
func (r *Runtime) registerNetworkLayer(networkLayer transport.Layer) {
	r.networkLayer = networkLayer
}

// registerSessionLayer attaches a SessionLayer. As with the transport
// layer, normal callers wire this via WithTCPTransport.
func (r *Runtime) registerSessionLayer(sessionLayer *transport.SessionLayer) {
	r.sessionLayer = sessionLayer
}

// Register wraps the user's Protocol implementation in a protoProtocol
// envelope and attaches it to this runtime. The protocol's per-channel
// buffer size comes from the runtime's WithChannelBuffer option (or the
// framework default if none was supplied).
func (r *Runtime) Register(impl Protocol) {
	r.registerProtocol(newProtoProtocol(impl, r.protoChannelBuffer))
}

// registerProtocol attaches an already-constructed protoProtocol to this
// runtime. Reserved for tests that want to inspect the envelope.
func (r *Runtime) registerProtocol(protocol *protoProtocol) {
	protocol.bindRuntime(r)
	r.protocols = append(r.protocols, protocol)
}

// connect / disconnect are the runtime-internal entry points used by
// ProtocolContext implementations. They validate that the runtime is
// usable and return a sync error if not; transport-level failures still
// surface asynchronously through SessionFailed events.
//
// disconnect also clears any tracked retry intent so a user-initiated
// disconnect halts further reconnect attempts.
func (r *Runtime) connect(host transport.Host) error {
	if r.sessionLayer == nil {
		return ErrNoSessionLayer
	}
	if err := r.ctx.Err(); err != nil {
		return err
	}
	r.sessionLayer.Connect(host)
	return nil
}

func (r *Runtime) disconnect(host transport.Host) error {
	if r.sessionLayer == nil {
		return ErrNoSessionLayer
	}
	r.stopRetryFor(host)
	if err := r.ctx.Err(); err != nil {
		return err
	}
	r.sessionLayer.Disconnect(host)
	return nil
}

// --- IPC routing ---
//
// These helpers are the runtime-side glue behind the public IPC API in
// ipc.go. They follow the same pattern as session-event fanout: cross-
// protocol channel writes are ctx-guarded so a slow consumer or a
// shutdown in flight can't wedge the caller.

// deliverReply pushes a reply (or terminal error) onto the requester's
// replyEvents channel. Safe to call from any goroutine — responder
// methods (Reply / Fail) and time.AfterFunc timeout callbacks both end
// up here.
func (r *Runtime) deliverReply(token replyToken, rep Reply, err error) {
	select {
	case token.requester.replyEvents <- inboundReply{requestID: token.requestID, rep: rep, err: err}:
	case <-r.ctx.Done():
	}
}

// sendRequest is the requester-side internal entry point. It allocates
// a request ID, registers a pending entry, then either routes the
// request to its handler (and arms a timeout) or delivers
// ErrNoRequestHandler immediately. Pending is registered before any
// delivery path so deliverReply can always find the entry — without
// it, the no-handler error would be dropped on the floor.
func (r *Runtime) sendRequest(
	requester *protoProtocol,
	wireID uint64,
	req Request,
	timeout time.Duration,
	onReply func(Reply, error),
) {
	requestID := requester.nextRequestID.Add(1)
	token := replyToken{requester: requester, requestID: requestID}

	wireIDAttr := Attr{Key: "wireID", Value: fmt.Sprintf("%#x", wireID)}
	r.metrics.Counter("protorun.ipc.request.sent", 1, wireIDAttr)

	now := time.Now()
	requester.pendingMu.Lock()
	requester.pending[requestID] = pendingRequest{
		cb:        onReply,
		deadline:  now.Add(timeout),
		startedAt: now,
		wireID:    wireID,
	}
	requester.pendingMu.Unlock()

	r.requestRoutesMu.RLock()
	route, ok := r.requestRoutes[wireID]
	r.requestRoutesMu.RUnlock()
	if !ok {
		r.deliverReply(token, nil, ErrNoRequestHandler)
		return
	}

	time.AfterFunc(timeout, func() {
		r.deliverReply(token, nil, ErrRequestTimeout)
	})

	select {
	case route.proto.requestEvents <- inboundRequest{req: req, token: token, handler: route.handler}:
	case <-r.ctx.Done():
	}
}

// subscribeNotification adds proto to the fan-out set for wireID and
// stores the handler closure under that protocol's key. A second call
// from the same proto for the same wireID replaces the prior closure.
func (r *Runtime) subscribeNotification(proto *protoProtocol, wireID uint64, fn func(Notification)) {
	r.notificationFanoutMu.Lock()
	defer r.notificationFanoutMu.Unlock()
	subs, ok := r.notificationFanout[wireID]
	if !ok {
		subs = make(map[*protoProtocol]func(Notification))
		r.notificationFanout[wireID] = subs
	}
	subs[proto] = fn
}

func (r *Runtime) unsubscribeNotification(proto *protoProtocol, wireID uint64) {
	r.notificationFanoutMu.Lock()
	defer r.notificationFanoutMu.Unlock()
	subs, ok := r.notificationFanout[wireID]
	if !ok {
		return
	}
	delete(subs, proto)
	if len(subs) == 0 {
		delete(r.notificationFanout, wireID)
	}
}

// publishNotification fans n out to every subscriber of wireID. Each
// subscriber's notificationEvents channel write is ctx-guarded; if the
// runtime is shutting down or a single subscriber's mailbox is full we
// stop fanning out and return — the publisher should not block on
// subscribers it doesn't know about.
func (r *Runtime) publishNotification(wireID uint64, n Notification) {
	wireIDAttr := Attr{Key: "wireID", Value: fmt.Sprintf("%#x", wireID)}
	r.metrics.Counter("protorun.notification.published", 1, wireIDAttr)

	r.notificationFanoutMu.RLock()
	subs := r.notificationFanout[wireID]
	// Snapshot the (proto, handler) pairs so we don't hold the lock
	// while pushing onto channels (which can block on slow consumers).
	pairs := make([]struct {
		proto *protoProtocol
		fn    func(Notification)
	}, 0, len(subs))
	for proto, fn := range subs {
		pairs = append(pairs, struct {
			proto *protoProtocol
			fn    func(Notification)
		}{proto, fn})
	}
	r.notificationFanoutMu.RUnlock()

	for _, p := range pairs {
		select {
		case p.proto.notificationEvents <- inboundNotification{n: n, handler: p.fn}:
			r.metrics.Counter("protorun.notification.delivered", 1, wireIDAttr)
		case <-r.ctx.Done():
			return
		}
	}
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

	r.wg.Go(func() {
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
	})
}

// dispatchSessionEvent translates a SessionLayer event into the runtime's
// internal sessionEvent kind, updates retry bookkeeping if applicable, and
// fans the event out to every registered protocol. Returns false if the
// runtime context fired during fanout.
func (r *Runtime) dispatchSessionEvent(ctx context.Context, ev transport.SessionEvent) bool {
	switch e := ev.(type) {
	case *transport.SessionConnected:
		r.metrics.Counter("protorun.session.connected", 1, Attr{Key: "host", Value: e.Host().String()})
		r.onSessionUpForRetry(e.Host())
		return r.fanoutSessionEvent(ctx, sessionEvent{kind: sessionConnectedEvent, host: e.Host()})
	case *transport.SessionDisconnected:
		r.metrics.Counter("protorun.session.disconnected", 1, Attr{Key: "host", Value: e.Host().String()})
		giveUp, attempts := r.onSessionDownForRetry(e.Host())
		if giveUp {
			r.metrics.Counter("protorun.session.given_up", 1,
				Attr{Key: "host", Value: e.Host().String()},
				Attr{Key: "attempts", Value: attempts})
			if !r.fanoutSessionEvent(ctx, sessionEvent{kind: sessionGivenUpEvent, host: e.Host(), attempts: attempts}) {
				return false
			}
		}
		return r.fanoutSessionEvent(ctx, sessionEvent{kind: sessionDisconnectedEvent, host: e.Host()})
	case *transport.SessionFailed:
		r.metrics.Counter("protorun.session.failed", 1, Attr{Key: "host", Value: e.Host().String()})
		giveUp, attempts := r.onSessionDownForRetry(e.Host())
		if giveUp {
			r.metrics.Counter("protorun.session.given_up", 1,
				Attr{Key: "host", Value: e.Host().String()},
				Attr{Key: "attempts", Value: attempts})
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

// processMessage decodes the wire id from the application-layer payload,
// looks up the owning protocol via the runtime's codecLookup map, decodes
// the message, and pushes a (msg, from) envelope onto that protocol's
// messageChannel. Sender info is delivered to handlers via the envelope's
// from field, not via fields on the message itself.
func (r *Runtime) processMessage(buffer bytes.Buffer, from transport.Host) {
	logger := r.Logger()

	var wireID uint64
	if err := binary.Read(&buffer, binary.LittleEndian, &wireID); err != nil {
		logger.Error("failed to read wireID header",
			"from", from.String(),
			"err", err,
		)
		r.metrics.Counter("protorun.message.dropped_header", 1)
		return
	}
	wireIDAttr := Attr{Key: "wireID", Value: fmt.Sprintf("%#x", wireID)}

	r.codecLookupMu.RLock()
	protocol, exists := r.codecLookup[wireID]
	r.codecLookupMu.RUnlock()
	if !exists {
		logger.Warn("received message for unknown wireID",
			"from", from.String(),
			"wireID", fmt.Sprintf("%#x", wireID),
		)
		r.metrics.Counter("protorun.message.dropped_unknown_id", 1, wireIDAttr)
		return
	}

	c, ok := protocol.codecs[wireID]
	if !ok {
		logger.Warn("codec lookup raced",
			"from", from.String(),
			"wireID", fmt.Sprintf("%#x", wireID),
		)
		r.metrics.Counter("protorun.message.dropped_codec_race", 1, wireIDAttr)
		return
	}

	message, err := c.unmarshal(buffer.Bytes())
	if err != nil {
		logger.Error("failed to decode message",
			"from", from.String(),
			"wireID", fmt.Sprintf("%#x", wireID),
			"err", err,
		)
		r.metrics.Counter("protorun.message.dropped_decode_error", 1, wireIDAttr)
		return
	}

	logger.Debug("dispatching message",
		"from", from.String(),
		"wireID", fmt.Sprintf("%#x", wireID),
	)
	r.metrics.Counter("protorun.message.dispatched", 1, wireIDAttr)
	ctx := r.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case protocol.messageChannel <- messageEnvelope{msg: message, from: from}:
	case <-ctx.Done():
	}
}

// WithTCPTransport wires the runtime's transport + session layers with
// the framework's TCP+Hello/Ack stack. It is the typical way to set up
// a runtime — most users never need to construct TCPLayer or
// SessionLayer themselves.
//
// The supplied ctx becomes the parent for both layers' internal
// goroutines.
func WithTCPTransport(ctx context.Context) Option {
	return func(r *Runtime) {
		tcp := transport.NewTCPLayer(r.self, ctx, 0)
		session := transport.NewSessionLayer(tcp, r.self, ctx, 0, 0)
		r.networkLayer = tcp
		r.sessionLayer = session
	}
}

// WithTransport injects a pre-constructed transport stack. Use this
// when you need to plug in a non-TCP transport (UDP, in-memory, mock
// for tests), tune buffer sizes / timeouts on the existing layers, or
// otherwise own the construction yourself. Either argument may be nil
// — the runtime accepts a network layer without a session layer if
// you wire sessions some other way, and vice versa.
//
// For the default TCP setup, prefer WithTCPTransport(ctx).
func WithTransport(layer transport.Layer, session *transport.SessionLayer) Option {
	return func(r *Runtime) {
		if layer != nil {
			r.networkLayer = layer
		}
		if session != nil {
			r.sessionLayer = session
		}
	}
}

// Run starts the runtime, installs SIGINT/SIGTERM handlers that call
// Cancel on receipt, and blocks until the runtime context is done.
// Returns the error from start if any. Use Run from cmd/main instead of
// orchestrating Start + signal handling + select{} yourself.
func (r *Runtime) Run() error {
	if err := r.start(); err != nil {
		return err
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		select {
		case <-sigCh:
			r.Logger().Info("signal received, cancelling runtime")
			r.Cancel()
		case <-r.ctx.Done():
		}
		signal.Stop(sigCh)
	}()

	<-r.ctx.Done()
	return nil
}
