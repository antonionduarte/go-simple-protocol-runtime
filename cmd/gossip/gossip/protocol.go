package gossip

import (
	"math/rand/v2"
	"slices"
	"sync"

	"github.com/antonionduarte/go-simple-protocol-runtime/cmd/gossip/membership"
	"github.com/antonionduarte/go-simple-protocol-runtime/pkg/protorun"
	"github.com/antonionduarte/go-simple-protocol-runtime/pkg/transport"
)

// Protocol is the eager-push gossip protocol. Each node broadcasts a
// payload by sending it to every peer in its (cached) membership
// view; on receipt, the node delivers locally if it hasn't seen the
// ID before and forwards to every peer except the sender.
//
// State is event-loop-owned for inbound handlers and the ViewChanged
// subscription, with a mutex to guard the seen-set and view cache so
// the public Broadcast method can be called from any goroutine.
type Protocol struct {
	ctx       protorun.ProtocolContext
	onDeliver func([]byte)

	mu   sync.Mutex
	seen map[uint64]struct{}
	view []transport.Host
}

// New returns a Protocol that calls onDeliver(payload) exactly once
// per unique payload it observes — including on the originator when
// Broadcast is called locally.
func New(onDeliver func([]byte)) *Protocol {
	return &Protocol{
		onDeliver: onDeliver,
		seen:      make(map[uint64]struct{}),
	}
}

func (p *Protocol) Start(ctx protorun.ProtocolContext) {
	p.ctx = ctx
	protorun.RegisterCodec(ctx, Codec{})
	protorun.RegisterHandler(ctx, p.handleInbound)
	protorun.SubscribeNotification(ctx, p.handleViewChanged)
}

func (p *Protocol) Init(ctx protorun.ProtocolContext) {
	protorun.SendRequest(ctx, &membership.GetView{}, p.handleInitialView)
}

// Broadcast sends payload to every node in the cluster (including the
// originator). Goroutine-safe.
func (p *Protocol) Broadcast(payload []byte) {
	// Math/rand/v2 is fine here: a 64-bit value is collision-free in
	// practice and we don't care about adversarial collisions —
	// gossip IDs are not authenticated.
	id := rand.Uint64() //nolint:gosec // see comment

	p.mu.Lock()
	p.seen[id] = struct{}{}
	peers := slices.Clone(p.view)
	p.mu.Unlock()

	p.onDeliver(payload)
	p.fanout(&Message{ID: id, Payload: payload}, peers)
}

func (p *Protocol) handleInbound(msg *Message, sender transport.Host) {
	p.mu.Lock()
	if _, dup := p.seen[msg.ID]; dup {
		p.mu.Unlock()
		return
	}
	p.seen[msg.ID] = struct{}{}
	peers := excluding(p.view, sender)
	p.mu.Unlock()

	p.onDeliver(msg.Payload)
	p.fanout(msg, peers)
}

func (p *Protocol) handleInitialView(rep *membership.View, err error) {
	if err != nil {
		p.ctx.Logger().Error("initial GetView failed", "err", err)
		return
	}
	p.mu.Lock()
	p.view = rep.Peers
	p.mu.Unlock()
}

func (p *Protocol) handleViewChanged(ev membership.ViewChanged) {
	p.mu.Lock()
	defer p.mu.Unlock()
	switch {
	case ev.HasAdded:
		if !slices.ContainsFunc(p.view, sameHost(ev.Added)) {
			p.view = append(p.view, ev.Added)
		}
	case ev.HasRemoved:
		p.view = slices.DeleteFunc(p.view, sameHost(ev.Removed))
	}
}

// fanout sends msg to each peer. Errors are logged and otherwise
// ignored — gossip is best-effort, and the runtime already emits
// SessionFailed events that membership will translate into a view
// shrink, which heals subsequent broadcasts.
func (p *Protocol) fanout(msg *Message, peers []transport.Host) {
	for _, peer := range peers {
		if err := p.ctx.Send(msg, peer); err != nil {
			p.ctx.Logger().Warn("gossip send failed", "peer", (&peer).ToString(), "err", err)
		}
	}
}

func excluding(peers []transport.Host, exclude transport.Host) []transport.Host {
	out := make([]transport.Host, 0, len(peers))
	for _, h := range peers {
		if !transport.CompareHost(h, exclude) {
			out = append(out, h)
		}
	}
	return out
}

func sameHost(target transport.Host) func(transport.Host) bool {
	return func(h transport.Host) bool { return transport.CompareHost(h, target) }
}
