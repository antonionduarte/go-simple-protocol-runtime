package protocol

import (
	"log/slog"

	"github.com/antonionduarte/go-simple-protocol-runtime"
	"github.com/antonionduarte/go-simple-protocol-runtime/transport"
)

type PingPongProtocol struct {
	peer transport.Host
	seq  uint64

	logger *slog.Logger
	ctx    protorun.ProtocolContext
}

func NewPingPongProtocol(peer transport.Host) *PingPongProtocol {
	return &PingPongProtocol{peer: peer}
}

func (p *PingPongProtocol) Start(ctx protorun.ProtocolContext) {
	p.logger = ctx.Logger()
	p.ctx = ctx

	protorun.RegisterCodec[*PingMessage](ctx, protorun.BinaryCodec[*PingMessage]{})
	protorun.RegisterCodec[*PongMessage](ctx, protorun.BinaryCodec[*PongMessage]{})
	protorun.RegisterHandler[*PingMessage](ctx, p.HandlePing)
	protorun.RegisterHandler[*PongMessage](ctx, p.HandlePong)
}

func (p *PingPongProtocol) Init(ctx protorun.ProtocolContext) {
	if err := ctx.Connect(p.peer); err != nil {
		p.logger.Error("initial Connect failed", "peer", (&p.peer).ToString(), "err", err)
	}
}

func (p *PingPongProtocol) OnSessionConnected(h transport.Host) {
	if !transport.CompareHost(h, p.peer) {
		return
	}
	p.seq++
	p.logger.Info("session established with peer, sending initial Ping",
		"peer", (&p.peer).ToString(), "seq", p.seq)
	if err := p.ctx.Send(NewPingMessage(p.seq), p.peer); err != nil {
		p.logger.Error("failed to send initial Ping", "err", err)
	}
}

func (p *PingPongProtocol) OnSessionDisconnected(h transport.Host) {
	if transport.CompareHost(h, p.peer) {
		p.logger.Warn("session with peer disconnected", "peer", (&p.peer).ToString())
	}
}

func (p *PingPongProtocol) HandlePing(ping *PingMessage, from transport.Host) {
	p.logger.Info("Ping received", "from", (&from).ToString(), "seq", ping.Seq)
	if err := p.ctx.Send(NewPongMessage(ping.Seq), p.peer); err != nil {
		p.logger.Error("failed to send Pong", "err", err)
	}
}

func (p *PingPongProtocol) HandlePong(pong *PongMessage, from transport.Host) {
	p.logger.Info("Pong received", "from", (&from).ToString(), "seq", pong.Seq)
	p.seq++
	if err := p.ctx.Send(NewPingMessage(p.seq), p.peer); err != nil {
		p.logger.Error("failed to send Ping", "err", err)
	}
}
