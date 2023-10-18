package runtime

type Protocol interface {
	Start()                 // Start the protocol, and register all the message handlers
	Init()                  // Init the protocol, runs after all protocols are registered, send initial messages here
	ProtocolID() ProtocolID // Returns the protocol ID
}

type ProtocolID int

type ProtoProtocol struct {
	protocol    Protocol
	msgChannel  chan Message
	msgHandlers map[MessageID]func(msg Message)
}

func NewProtoProtocol(protocol Protocol) *ProtoProtocol {
	return &ProtoProtocol{
		protocol:    protocol,
		msgChannel:  make(chan Message),
		msgHandlers: make(map[MessageID]func(msg Message)),
	}
}

func (p *ProtoProtocol) Start() {
	p.protocol.Start()
	go p.MessageHandler()
}

func (p *ProtoProtocol) Init() {
	p.protocol.Init()
}

func (p *ProtoProtocol) RegisterMessageHandler(message Message, handler func(Message)) chan Message {
	msgChan := make(chan Message)
	p.msgHandlers[message.HandlerID()] = handler
	return msgChan
}

func (p *ProtoProtocol) MessageHandler() {
	for {
		select {
		case msg := <-p.msgChannel:
			p.msgHandlers[msg.HandlerID()](msg)
		}
	}
}

func (p *ProtoProtocol) MsgChannel() chan Message {
	return p.msgChannel
}

func (p *ProtoProtocol) ProtocolID() ProtocolID {
	return p.protocol.ProtocolID()
}
