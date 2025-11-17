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

		logger *slog.Logger

		ctx runtime.ProtocolContext
	}
)

func NewPingPongProtocol(self *net.Host, peer *net.Host) *PingPongProtocol {
	return &PingPongProtocol{
		protocolID: PingPongProtocolId,
		self:       *self,
		peer:       *peer,
	}
}

func (p *PingPongProtocol) Start(ctx runtime.ProtocolContext) {
	p.logger = ctx.Logger()
	p.ctx = ctx

	ctx.RegisterMessageHandler(PingMessageID, p.HandlePing)
	ctx.RegisterMessageHandler(PongMessageID, p.HandlePong)

	ctx.RegisterMessageSerializer(PingMessageID, &PingSerializer{})
	ctx.RegisterMessageSerializer(PongMessageID, &PongSerializer{})
}

func (p *PingPongProtocol) Init(ctx runtime.ProtocolContext) {
	ctx.Connect(p.peer)
}

func (p *PingPongProtocol) ProtocolID() int {
	return p.protocolID
}

func (p *PingPongProtocol) Self() net.Host {
	return p.self
}

func (p *PingPongProtocol) OnSessionConnected(h net.Host) {
	if net.CompareHost(h, p.peer) {
		peerStr := (&p.peer).ToString()
		p.logger.Info("session established with peer, sending initial Ping", "peer", peerStr)
		if err := p.ctx.Send(NewPingMessage(p.self), p.peer); err != nil {
			p.logger.Error("failed to send initial Ping", "peer", peerStr, "err", err)
		}
	}
}

func (p *PingPongProtocol) OnSessionDisconnected(h net.Host) {
	if net.CompareHost(h, p.peer) {
		peerStr := (&p.peer).ToString()
		p.logger.Warn("session with peer disconnected", "peer", peerStr)
	}
}

func (p *PingPongProtocol) HandlePing(msg runtime.Message) {
	ping := msg.(*PingMessage)

	from := ping.Sender()
	p.logger.Info("Ping received", "from", (&from).ToString())

	if err := p.ctx.Send(NewPongMessage(p.self), p.peer); err != nil {
		p.logger.Error("failed to send Pong", "err", err)
	}
}

func (p *PingPongProtocol) HandlePong(msg runtime.Message) {
	pong := msg.(*PongMessage)

	from := pong.Sender()
	p.logger.Info("Pong received", "from", (&from).ToString())

	if err := p.ctx.Send(NewPingMessage(p.self), p.peer); err != nil {
		p.logger.Error("failed to send Ping", "err", err)
	}
}
