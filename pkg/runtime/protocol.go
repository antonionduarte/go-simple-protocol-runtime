package runtime

import (
	"bytes"
	"encoding/binary"
	"github.com/antonionduarte/go-simple-protocol-runtime/pkg/runtime/net"
)

type (
	Protocol interface {
		Start()          // Start the protocol, and register all the message handlers
		Init()           // Init the protocol, runs after all protocols are registered, send initial messages here
		ProtocolID() int // Returns the protocol ID
	}

	ProtoProtocol struct {
		protocol Protocol

		timerChannel   chan Timer
		messageChannel chan Message

		msgSerializers map[int]Serializer

		msgHandlers   map[int]func(msg Message) // TODO: this is never going to receive a Message, it's going to still receive a Buffer
		timerHandlers map[int]func(timer Timer)
	}
)

func NewProtoProtocol(protocol Protocol) *ProtoProtocol {
	return &ProtoProtocol{
		protocol: protocol,

		msgSerializers: make(map[int]Serializer),

		messageChannel: make(chan Message, 1),
		timerChannel:   make(chan Timer, 1),

		msgHandlers:   make(map[int]func(msg Message)),
		timerHandlers: make(map[int]func(timer Timer)),
	}
}

func (p *ProtoProtocol) Start() {
	p.protocol.Start()
	go p.EventHandler()
}

func (p *ProtoProtocol) Init() {
	p.protocol.Init()
}

func (p *ProtoProtocol) RegisterMessageHandler(message Message, handler func(Message)) {
	p.msgHandlers[message.MessageID()] = handler
}

func (p *ProtoProtocol) RegisterTimerHandler(timer Timer, handler func(Timer)) {
	p.timerHandlers[timer.TimerID()] = handler
}

func (p *ProtoProtocol) EventHandler() {
	for {
		select {
		case msg := <-p.messageChannel:
			handler := p.msgHandlers[msg.MessageID()]
			handler(msg)
		case timer := <-p.timerChannel:
			handler := p.timerHandlers[timer.TimerID()]
			handler(timer)
		}
	}
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

// SendMessage sends a message to a host via the Network Layer.
func SendMessage(msg Message, host *net.Host) {
	msgBuffer, err := msg.Serializer().Serialize()

	if err != nil {
		// TODO: Replace with decent logger event.
	}

	// Create a buffer and write ProtocolID, MessageID, and the message.
	buffer := new(bytes.Buffer)
	protocolID := uint16(msg.ProtocolID())
	err = binary.Write(buffer, binary.LittleEndian, protocolID)
	if err != nil {
		return
	}
	messageID := uint16(msg.MessageID())
	err = binary.Write(buffer, binary.LittleEndian, messageID)
	if err != nil {
		return
	}
	buffer.Write(msgBuffer.Bytes())

	networkMessage := net.NewNetworkMessage(buffer, host)

	GetRuntimeInstance().networkLayer.Send(networkMessage)
}
