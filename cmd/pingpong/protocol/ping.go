package protocol

import (
	"github.com/antonionduarte/go-simple-protocol-runtime/pkg/runtime"
	"github.com/antonionduarte/go-simple-protocol-runtime/pkg/runtime/net"
)

const (
	PingPongProtocolId = 1
)

type (
	PingPongProtocol struct {
		protocolID int
		self       *net.Host
	}
)

/*----------- Constructor ----------- */

func NewPingPongProtocol(self *net.Host) *PingPongProtocol {
	return &PingPongProtocol{
		protocolID: PingPongProtocolId,
		self:       self,
	}
}

/*----------- Mandatory Methods ----------- */

func (p *PingPongProtocol) Start() {
	instance := runtime.GetRuntimeInstance()
	proto := instance.GetProtocol(PingPongProtocolId)

	proto.RegisterMessageHandler(1, p.HandlePing)
	proto.RegisterMessageHandler(2, p.HandlePong)

	// ...

	proto.RegisterMessageSerializer(1, &PingSerializer{})
	proto.RegisterMessageSerializer(2, &PongSerializer{})
}

func (p *PingPongProtocol) Init() {
	// send initial messages here
}

func (p *PingPongProtocol) ProtocolID() int {
	return p.protocolID
}

func (p *PingPongProtocol) Self() *net.Host {
	return p.self
}

/*----------- Message Handlers ----------- */

func (p *PingPongProtocol) HandlePing(msg runtime.Message) {
	// when accessing the message, you need to cast it to the correct type
	ping := msg.(*PingMessage)

	// do something with the message
	// ...

	print(ping) // placeholder
}

func (p *PingPongProtocol) HandlePong(msg runtime.Message) {
	// when accessing the message, you need to cast it to the correct type
	pong := msg.(*PongMessage)

	// do something with the message
	// ...

	print(pong) // placeholder
}

/*----------- Timer Handlers ----------- */
