package runtime

type Protocol interface {
	Start()                 // Start the protocol, and register all the message handlers
	Init()                  // Init the protocol, runs after all protocols are registered, send initial messages here
	ProtocolID() ProtocolID // Returns the protocol ID
}

type ProtocolID int

type ProtoProtocol struct {
	protocol Protocol

	timerChannel   chan Timer
	messageChannel chan Message

	msgHandlers   map[MessageID]func(msg Message)
	timerHandlers map[TimerID]func(timer Timer)
}

func NewProtoProtocol(protocol Protocol) *ProtoProtocol {
	return &ProtoProtocol{
		protocol:       protocol,
		messageChannel: make(chan Message),
		timerChannel:   make(chan Timer),
		msgHandlers:    make(map[MessageID]func(msg Message)),
		timerHandlers:  make(map[TimerID]func(timer Timer)),
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

func (p *ProtoProtocol) ProtocolID() ProtocolID {
	return p.protocol.ProtocolID()
}

func (p *ProtoProtocol) TimerChannel() chan Timer {
	return p.timerChannel
}

func (p *ProtoProtocol) MessageChannel() chan Message {
	return p.messageChannel
}
