package runtime

import (
	"sync"

	"github.com/antonionduarte/go-simple-protocol-runtime/pkg/runtime/net"
	"golang.org/x/net/context"
)

type (
	Protocol interface {
		Start()
		Init()
		ProtocolID() int
		Self() *net.Host
	}

	ProtoProtocol struct {
		protocol       Protocol
		self           net.Host
		timerChannel   chan Timer
		messageChannel chan Message

		msgSerializers map[int]Serializer
		msgHandlers    map[int]func(msg Message)
		timerHandlers  map[int]func(timer Timer)
	}
)

func NewProtoProtocol(protocol Protocol, self *net.Host) *ProtoProtocol {
	return &ProtoProtocol{
		protocol:       protocol,
		self:           *self,
		timerChannel:   make(chan Timer, 1),
		messageChannel: make(chan Message, 1),
		msgSerializers: make(map[int]Serializer),
		msgHandlers:    make(map[int]func(msg Message)),
		timerHandlers:  make(map[int]func(timer Timer)),
	}
}

func (p *ProtoProtocol) Start(ctx context.Context, wg *sync.WaitGroup) {
	wg.Add(1)
	p.protocol.Start()
	go p.eventHandler(ctx, wg)
}

func (p *ProtoProtocol) Init() {
	p.protocol.Init()
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

func (p *ProtoProtocol) TimerChannel() chan Timer {
	return p.timerChannel
}

func (p *ProtoProtocol) MessageChannel() chan Message {
	return p.messageChannel
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
		}
	}
}
