package runtime

type Runtime struct {
	msgChannel   chan Message
	timerChannel chan Timer
	protocols    map[ProtocolID]ProtoProtocol
}

// NewRuntime creates a new runtime.
func NewRuntime() *Runtime {
	return &Runtime{
		msgChannel:   make(chan Message),
		timerChannel: make(chan Timer),
		protocols:    make(map[ProtocolID]ProtoProtocol),
	}
}

// Start starts the runtime, and runs the start and init function for all the protocols.
func (r *Runtime) Start() {
	r.startProtocols()
	r.initProtocols()

	for {
		select {
		case msg := <-r.msgChannel:
			protocol := r.protocols[msg.ProtocolID()]
			protocol.MessageChannel() <- msg
		case timer := <-r.timerChannel:
			protocol := r.protocols[timer.ProtocolID()]
			protocol.TimerChannel() <- timer
		}
	}
}

// RegisterProtocol registers a protocol to the runtime.
// It must take in a ProtoProtocol, which should encapsulate the protocol that you yourself develop.
func (r *Runtime) RegisterProtocol(protocol ProtoProtocol) {
	r.protocols[protocol.ProtocolID()] = protocol
}

func (r *Runtime) startProtocols() {
	for _, protocol := range r.protocols {
		protocol.Start()
	}
}

func (r *Runtime) initProtocols() {
	for _, protocol := range r.protocols {
		protocol.Init()
	}
}
