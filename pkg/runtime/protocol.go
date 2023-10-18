package protocol_runtime

type Protocol interface {
	Start()                 // Start the protocol, and register all the message handlers
	Init()                  // Init the protocol, runs after all protocols are registered, send initial messages here
	ProtocolID() ProtocolID // Returns the protocol ID
}

type ProtoProtocol struct {
	protocol    Protocol
	msgChannel  chan Message
	msgHandlers map[MessageID]chan Message
}

func NewProtoProtocol(protocol Protocol) *ProtoProtocol {
	return &ProtoProtocol{
		protocol:    protocol,
		msgChannel:  make(chan Message),
		msgHandlers: make(map[MessageID]chan Message),
	}
}

type ProtocolID int

func (p *ProtoProtocol) Start() {
	p.Start()
	go p.MessageHandler()
}

func (p *ProtoProtocol) Init() {
	p.Init()
}

func (p *ProtoProtocol) RegisterMessageHandler(message Message) {
	p.msgHandlers[message.HandlerID()] = make(chan Message)
}

func (p *ProtoProtocol) MessageHandler() {
	for {
		select {
		case msg := <-p.msgChannel:
			p.msgHandlers[msg.HandlerID()] <- msg
		}
	}
}

func (p *ProtoProtocol) MsgChannel() chan Message {
	return p.msgChannel
}

func (p *ProtoProtocol) ProtocolID() ProtocolID {
	return p.protocol.ProtocolID()
}
