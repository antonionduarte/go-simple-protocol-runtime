package runtime

import (
	"context"
	"log/slog"
	"sync"

	"github.com/antonionduarte/go-simple-protocol-runtime/pkg/runtime/net"
)

// Package runtime exposes the core protocol runtime and the primary APIs that
// protocol implementations interact with. Most user code should depend on the
// abstract interfaces here (Protocol, ProtocolContext, Message, Serializer,
// Timer) rather than on the concrete Runtime type or its globals.

type (
	// ProtocolContext is the main entry point for protocol implementations.
	// It is provided to Protocol.Start and Protocol.Init and should be used
	// to register handlers, connect/disconnect from peers, send messages,
	// access the protocol-scoped logger, and query the local Host identity.
	//
	// Protocol authors should prefer using this interface over calling
	// package-level helpers or touching the Runtime singleton directly.
	ProtocolContext interface {
		RegisterMessageHandler(messageID int, handler func(Message))
		RegisterMessageSerializer(messageID int, serializer Serializer)
		RegisterTimerHandler(timer Timer, handler func(Timer))

		Connect(host net.Host)
		Disconnect(host net.Host)
		Send(msg Message, to net.Host) error

		Logger() *slog.Logger

		Self() net.Host
	}

	// Protocol describes a user protocol that can be hosted by the runtime.
	// Implementations should use the provided ProtocolContext in Start/Init
	// to interact with the system instead of depending on concrete runtime
	// types or globals.
	Protocol interface {
		Start(ctx ProtocolContext)
		Init(ctx ProtocolContext)
		ProtocolID() int
		Self() net.Host
	}

	ProtoProtocol struct {
		protocol       Protocol
		self           net.Host
		timerChannel   chan Timer
		messageChannel chan Message
		sessionEvents  chan sessionEvent

		msgSerializers map[int]Serializer
		msgHandlers    map[int]func(msg Message)
		timerHandlers  map[int]func(timer Timer)

		ctx ProtocolContext
	}

	protocolContext struct {
		proto   *ProtoProtocol
		runtime *Runtime
		logger  *slog.Logger
	}
)

func NewProtoProtocol(protocol Protocol, self net.Host) *ProtoProtocol {
	return &ProtoProtocol{
		protocol:       protocol,
		self:           self,
		timerChannel:   make(chan Timer, 1),
		messageChannel: make(chan Message, 1),
		sessionEvents:  make(chan sessionEvent, 1),
		msgSerializers: make(map[int]Serializer),
		msgHandlers:    make(map[int]func(msg Message)),
		timerHandlers:  make(map[int]func(timer Timer)),
	}
}

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

func (p *ProtoProtocol) RegisterMessageSerializer(messageID int, serializer Serializer) {
	p.msgSerializers[messageID] = serializer
}

func (p *ProtoProtocol) RegisterMessageHandler(messageID int, handler func(Message)) {
	p.msgHandlers[messageID] = handler
}

func (p *ProtoProtocol) RegisterTimerHandler(timer Timer, handler func(Timer)) {
	p.timerHandlers[timer.TimerID()] = handler
}

func (p *ProtoProtocol) ProtocolID() int {
	return p.protocol.ProtocolID()
}

// Self returns the logical host identity for this protocol instance.
func (p *ProtoProtocol) Self() net.Host {
	return p.self
}

func (p *ProtoProtocol) TimerChannel() chan Timer {
	return p.timerChannel
}

func (p *ProtoProtocol) MessageChannel() chan Message {
	return p.messageChannel
}

// ensureContext lazily initializes the ProtocolContext for this ProtoProtocol.
func (p *ProtoProtocol) ensureContext() {
	if p.ctx != nil {
		return
	}
	runtime := GetRuntimeInstance()
	baseLogger := runtime.Logger()
	p.ctx = &protocolContext{
		proto:   p,
		runtime: runtime,
		logger: baseLogger.With(
			"component", "protocol",
			"protocolID", p.ProtocolID(),
			"self", p.self.ToString(),
		),
	}
}

// --- ProtocolContext implementation ---

func (c *protocolContext) RegisterMessageHandler(messageID int, handler func(Message)) {
	c.proto.msgHandlers[messageID] = handler
}

func (c *protocolContext) RegisterMessageSerializer(messageID int, serializer Serializer) {
	c.proto.msgSerializers[messageID] = serializer
}

func (c *protocolContext) RegisterTimerHandler(timer Timer, handler func(Timer)) {
	c.proto.timerHandlers[timer.TimerID()] = handler
}

func (c *protocolContext) Connect(host net.Host) {
	Connect(host)
}

func (c *protocolContext) Disconnect(host net.Host) {
	Disconnect(host)
}

func (c *protocolContext) Send(msg Message, to net.Host) error {
	return SendMessage(msg, to)
}

func (c *protocolContext) Self() net.Host {
	return c.proto.self
}

func (c *protocolContext) Logger() *slog.Logger {
	return c.logger
}

func (p *ProtoProtocol) eventHandler(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-p.messageChannel:
			handler := p.msgHandlers[msg.MessageID()]
			if handler != nil {
				handler(msg)
			}
		case timer := <-p.timerChannel:
			handler := p.timerHandlers[timer.TimerID()]
			if handler != nil {
				handler(timer)
			}
		case ev := <-p.sessionEvents:
			switch ev.kind {
			case sessionConnectedEvent:
				if h, ok := p.protocol.(SessionConnectedHandler); ok {
					h.OnSessionConnected(ev.host)
				}
			case sessionDisconnectedEvent:
				if h, ok := p.protocol.(SessionDisconnectedHandler); ok {
					h.OnSessionDisconnected(ev.host)
				}
			}
		}
	}
}
