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
		self       net.Host
		peer       net.Host
	}
)

/*----------- Constructor ----------- */

func NewPingPongProtocol(self *net.Host, peer *net.Host) *PingPongProtocol {
	return &PingPongProtocol{
		protocolID: PingPongProtocolId,
		self:       *self,
		peer:       *peer,
	}
}

/*----------- Mandatory Methods ----------- */

func (p *PingPongProtocol) Start(ctx runtime.ProtocolContext) {
	ctx.RegisterMessageHandler(PingMessageID, p.HandlePing)
	ctx.RegisterMessageHandler(PongMessageID, p.HandlePong)

	ctx.RegisterMessageSerializer(PingMessageID, &PingSerializer{})
	ctx.RegisterMessageSerializer(PongMessageID, &PongSerializer{})
}

func (p *PingPongProtocol) Init(ctx runtime.ProtocolContext) {
	// Ask the runtime/session layer to initiate the connection to our peer.
	ctx.Connect(p.peer)
}

func (p *PingPongProtocol) ProtocolID() int {
	return p.protocolID
}

func (p *PingPongProtocol) Self() net.Host {
	return p.self
}

/*----------- Session Event Handlers (optional) ----------- */

// OnSessionConnected is called by the runtime when a session is
// established with some Host. We use it to trigger the initial Ping.
func (p *PingPongProtocol) OnSessionConnected(h net.Host) {
	if net.CompareHost(h, p.peer) {
		peerStr := (&p.peer).ToString()
		slog.Info("session established with peer, sending initial Ping", "peer", peerStr)
		runtime.SendMessage(NewPingMessage(p.self), p.peer)
	}
}

// OnSessionDisconnected is called by the runtime when a session is
// torn down with some Host. For now we just log.
func (p *PingPongProtocol) OnSessionDisconnected(h net.Host) {
	if net.CompareHost(h, p.peer) {
		peerStr := (&p.peer).ToString()
		slog.Warn("session with peer disconnected", "peer", peerStr)
	}
}

/*----------- Message Handlers ----------- */

func (p *PingPongProtocol) HandlePing(msg runtime.Message) {
	// when accessing the message, you need to cast it to the correct type
	ping := msg.(*PingMessage)

	from := ping.Sender()
	slog.Info("Ping received", "from", (&from).ToString())

	// Reply with a Pong to the configured peer
	runtime.SendMessage(NewPongMessage(p.self), p.peer)
}

func (p *PingPongProtocol) HandlePong(msg runtime.Message) {
	// when accessing the message, you need to cast it to the correct type
	pong := msg.(*PongMessage)

	from := pong.Sender()
	slog.Info("Pong received", "from", (&from).ToString())

	// Send another Ping back to the peer for continuous ping-pong.
	runtime.SendMessage(NewPingMessage(p.self), p.peer)
}

/*----------- Timer Handlers ----------- */
