package runtime

type Runtime struct {
	msgChannel chan Message
	protocols  map[ProtocolID]ProtoProtocol
}

func NewRuntime() *Runtime {
	return &Runtime{
		msgChannel: make(chan Message),
		protocols:  make(map[ProtocolID]ProtoProtocol),
	}
}

func (r *Runtime) Start() {
	r.startProtocols()
	r.initProtocols()

	for {
		select {
		case msg := <-r.msgChannel:
			protocol := r.protocols[msg.ProtocolID()]
			protocol.MsgChannel() <- msg
		}
	}
}

func (r *Runtime) RegisterProtocol(protocol ProtoProtocol) {
	r.protocols[protocol.ProtocolID()] = protocol
}

func (r *Runtime) startProtocols() {
	for _, protocol := range r.protocols {
		go protocol.Start()
	}
}

func (r *Runtime) initProtocols() {
	for _, protocol := range r.protocols {
		go protocol.Init()
	}
}
