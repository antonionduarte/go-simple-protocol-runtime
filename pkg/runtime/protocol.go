package runtime

import (
	"bytes"
	"github.com/antonionduarte/go-simple-protocol-runtime/pkg/runtime/net"
	"strconv"
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
		bufferChannel  chan bytes.Buffer

		msgSerializers map[int]Serializer

		msgHandlers   map[int]func(msg Message) // TODO: this is never going to receive a Message, it's going to still receive a Buffer
		timerHandlers map[TimerID]func(timer Timer)
	}
)

func NewProtoProtocol(protocol Protocol) *ProtoProtocol {
	return &ProtoProtocol{
		protocol: protocol,

		msgSerializers: make(map[int]Serializer),

		messageChannel: make(chan Message),
		timerChannel:   make(chan Timer),
		bufferChannel:  make(chan bytes.Buffer),

		msgHandlers:   make(map[int]func(msg Message)),
		timerHandlers: make(map[TimerID]func(timer Timer)),
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
		// case buffer := <-p.bufferChannel:
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

	// Add to the Buffer: [ProtocolID][MessageID][Message]
	protocolIDBytes := []byte(strconv.Itoa(msg.ProtocolID()))
	messageIDBytes := []byte(strconv.Itoa(msg.MessageID()))

	// Create a buffer and write the bytes.
	buffer := new(bytes.Buffer)
	buffer.Write(protocolIDBytes)
	buffer.Write(messageIDBytes)
	buffer.Write(msgBuffer.Bytes())

	GetRuntimeInstance().networkLayer.Send(buffer, host)
}

// ReceiveMessage receives a message from the Network Layer.
func ReceiveMessage(buffer bytes.Buffer) {
	protocolIDByte := buffer.Next(1)
	messageIDByte := buffer.Next(1)

	protocolID, err := strconv.Atoi(string(protocolIDByte))
	if err != nil {
		// TODO: Replace with decent logger event.
	}

	messageID, err := strconv.Atoi(string(messageIDByte))
	if err != nil {
		// TODO: Replace with decent logger event.
	}

	protocol := GetRuntimeInstance().protocols[protocolID]
	message, _ := protocol.msgSerializers[messageID].Deserialize(buffer)

	protocol.messageChannel <- message
}
