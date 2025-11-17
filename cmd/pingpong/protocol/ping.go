package protocol

import (
	"log/slog"

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
		peer       *net.Host
	}
)

/*----------- Constructor ----------- */

func NewPingPongProtocol(self *net.Host, peer *net.Host) *PingPongProtocol {
	return &PingPongProtocol{
		protocolID: PingPongProtocolId,
		self:       self,
		peer:       peer,
	}
}

/*----------- Mandatory Methods ----------- */

func (p *PingPongProtocol) Start() {
	runtime.RegisterMessageHandler(PingPongProtocolId, 1, p.HandlePing)
	runtime.RegisterMessageHandler(PingPongProtocolId, 2, p.HandlePong)

	// ...

	runtime.RegisterMessageSerializer(PingPongProtocolId, 1, &PingSerializer{})
	runtime.RegisterMessageSerializer(PingPongProtocolId, 2, &PongSerializer{})
}

func (p *PingPongProtocol) Init() {
	// Initial Ping is triggered from main() once the session
	// to the peer is established.
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

	from := ping.Sender()
	slog.Info("Ping received", "from", (&from).ToString())

	// Reply with a Pong to the configured peer
	runtime.SendMessage(NewPongMessage(*p.self), *p.peer)
}

func (p *PingPongProtocol) HandlePong(msg runtime.Message) {
	// when accessing the message, you need to cast it to the correct type
	pong := msg.(*PongMessage)

	from := pong.Sender()
	slog.Info("Pong received", "from", (&from).ToString())

	// Send another Ping back to the peer for continuous ping-pong.
	runtime.SendMessage(NewPingMessage(*p.self), *p.peer)
}

/*----------- Timer Handlers ----------- */
