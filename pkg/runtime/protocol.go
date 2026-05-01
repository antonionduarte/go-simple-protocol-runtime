package runtime

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/antonionduarte/go-simple-protocol-runtime/pkg/runtime/net"
)

// Package runtime exposes the core protocol runtime and the primary APIs
// that protocol implementations interact with. Most user code should
// depend on the abstract interfaces here (Protocol, ProtocolContext,
// Message, Codec, Timer) rather than on the concrete Runtime type.

type (
	// ProtocolContext is the main entry point for protocol implementations.
	// It is provided to Protocol.Start and Protocol.Init and is used to
	// connect/disconnect from peers, send messages, schedule timers,
	// access the protocol-scoped logger, and query the local Host identity.
	//
	// Codec and message-handler registration is done via the typed
	// generic helpers RegisterCodec[M] and RegisterHandler[M], which
	// reach the framework through the unexported methods on this
	// interface.
	ProtocolContext interface {
		Connect(host net.Host)
		ConnectWithRetry(host net.Host)
		Disconnect(host net.Host)
		Send(msg Message, to net.Host) error

		SetupTimer(timer Timer, duration time.Duration)
		SetupPeriodicTimer(timer Timer, duration time.Duration)
		CancelTimer(timerID int)
		RegisterTimerHandler(timer Timer, handler func(Timer))

		Logger() *slog.Logger
		Self() net.Host

		// Internal hooks used by RegisterCodec[M] and RegisterHandler[M].
		// Defined as unexported methods so only the runtime package can
		// implement ProtocolContext.
		registerCodec(wireID uint64, c codec)
		registerHandler(wireID uint64, fn func(Message))
	}

	// Protocol describes a user protocol that can be hosted by the
	// runtime. Implementations should use the provided ProtocolContext
	// in Start/Init to interact with the system.
	Protocol interface {
		Start(ctx ProtocolContext)
		Init(ctx ProtocolContext)
	}

	ProtoProtocol struct {
		protocol       Protocol
		runtime        *Runtime
		timerChannel   chan Timer
		messageChannel chan Message
		sessionEvents  chan sessionEvent

		codecs        map[uint64]codec
		handlers      map[uint64]func(Message)
		timerHandlers map[int]func(timer Timer)

		ctx ProtocolContext
	}

	protocolContext struct {
		proto   *ProtoProtocol
		runtime *Runtime
		logger  *slog.Logger
	}
)

// NewProtoProtocol wraps a user Protocol implementation in the framework's
// ProtoProtocol envelope. Most callers should use Runtime.Register instead;
// this lower-level constructor exists for tests that want to inspect or
// manipulate the envelope directly.
func NewProtoProtocol(protocol Protocol) *ProtoProtocol {
	return &ProtoProtocol{
		protocol:       protocol,
		timerChannel:   make(chan Timer, protoProtocolChannelBuffer),
		messageChannel: make(chan Message, protoProtocolChannelBuffer),
		sessionEvents:  make(chan sessionEvent, protoProtocolChannelBuffer),
		codecs:         make(map[uint64]codec),
		handlers:       make(map[uint64]func(Message)),
		timerHandlers:  make(map[int]func(timer Timer)),
	}
}

const protoProtocolChannelBuffer = 16

// bindRuntime is called by Runtime.RegisterProtocol so the protocol can
// resolve its hosting runtime when ensureContext fires.
func (p *ProtoProtocol) bindRuntime(r *Runtime) { p.runtime = r }

func (p *ProtoProtocol) Start(ctx context.Context, wg *sync.WaitGroup) {
	wg.Add(1)
	p.ensureContext()
	p.protocol.Start(p.ctx)
	go p.eventHandler(ctx, wg)
}

func (p *ProtoProtocol) Init() {
	p.ensureContext()
	p.protocol.Init(p.ctx)
}

func (p *ProtoProtocol) RegisterTimerHandler(timer Timer, handler func(Timer)) {
	p.timerHandlers[timer.TimerID()] = handler
}

func (p *ProtoProtocol) TimerChannel() chan Timer { return p.timerChannel }

func (p *ProtoProtocol) MessageChannel() chan Message { return p.messageChannel }

// ensureContext lazily initializes the ProtocolContext. The protocol must
// have been registered with a Runtime via RegisterProtocol or Register
// before Start or Init is called.
func (p *ProtoProtocol) ensureContext() {
	if p.ctx != nil {
		return
	}
	if p.runtime == nil {
		panic("runtime: protocol used before being registered with a Runtime")
	}
	baseLogger := p.runtime.Logger()
	p.ctx = &protocolContext{
		proto:   p,
		runtime: p.runtime,
		logger: baseLogger.With(
			"component", "protocol",
			"self", p.runtime.self.ToString(),
		),
	}
}

// --- ProtocolContext implementation ---

func (c *protocolContext) Connect(host net.Host)          { c.runtime.connect(host) }
func (c *protocolContext) ConnectWithRetry(host net.Host) { c.runtime.connectWithRetry(host) }
func (c *protocolContext) Disconnect(host net.Host)       { c.runtime.disconnect(host) }

func (c *protocolContext) Send(msg Message, to net.Host) error {
	return c.runtime.sendMessage(msg, to)
}

func (c *protocolContext) SetupTimer(timer Timer, duration time.Duration) {
	c.runtime.setupTimer(c.proto, timer, duration)
}
func (c *protocolContext) SetupPeriodicTimer(timer Timer, duration time.Duration) {
	c.runtime.setupPeriodicTimer(c.proto, timer, duration)
}
func (c *protocolContext) CancelTimer(timerID int) { c.runtime.cancelTimer(timerID) }

func (c *protocolContext) RegisterTimerHandler(timer Timer, handler func(Timer)) {
	c.proto.timerHandlers[timer.TimerID()] = handler
}

func (c *protocolContext) Self() net.Host       { return c.runtime.self }
func (c *protocolContext) Logger() *slog.Logger { return c.logger }

func (c *protocolContext) registerCodec(wireID uint64, codec codec) {
	c.proto.codecs[wireID] = codec
	// Make this protocol the routing target for the wire id on the
	// runtime-level lookup table. RegisterCodec should be called once
	// per (protocol, message-type) pair; if two protocols register the
	// same wire id, the last one wins (and the operator should investigate).
	c.runtime.codecLookupMu.Lock()
	c.runtime.codecLookup[wireID] = c.proto
	c.runtime.codecLookupMu.Unlock()
}

func (c *protocolContext) registerHandler(wireID uint64, fn func(Message)) {
	c.proto.handlers[wireID] = fn
}

func (p *ProtoProtocol) eventHandler(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-p.messageChannel:
			if h := p.handlers[wireIDOf(msg)]; h != nil {
				h(msg)
			}
		case timer := <-p.timerChannel:
			if h := p.timerHandlers[timer.TimerID()]; h != nil {
				h(timer)
			}
		case ev := <-p.sessionEvents:
			p.deliverSessionEvent(ev)
		}
	}
}

// deliverSessionEvent invokes the protocol's optional session-event
// handlers (OnSessionConnected / OnSessionDisconnected / OnSessionGivenUp)
// when implemented.
func (p *ProtoProtocol) deliverSessionEvent(ev sessionEvent) {
	switch ev.kind {
	case sessionConnectedEvent:
		if h, ok := p.protocol.(SessionConnectedHandler); ok {
			h.OnSessionConnected(ev.host)
		}
	case sessionDisconnectedEvent:
		if h, ok := p.protocol.(SessionDisconnectedHandler); ok {
			h.OnSessionDisconnected(ev.host)
		}
	case sessionGivenUpEvent:
		if h, ok := p.protocol.(SessionGivenUpHandler); ok {
			h.OnSessionGivenUp(ev.host, ev.attempts)
		}
	}
}
