// Package membership implements a minimal membership layer for the
// gossip example: it tracks the set of peers we are currently
// session-connected to, and exposes that view to other protocols on
// the same runtime via protorun's IPC primitives.
//
// Two access patterns are supported:
//
//   - Synchronous: a peer protocol issues a GetView request and
//     receives a snapshot of the current view in the reply.
//   - Asynchronous: a peer protocol subscribes to ViewChanged
//     notifications and receives one event per add or remove.
//
// State is mutated only from the protocol's event loop (in the
// session-event handlers), and the view is guarded by a mutex so the
// View() helper used in tests is safe to call from any goroutine.
package membership

import (
	"slices"
	"sync"

	"github.com/antonionduarte/go-simple-protocol-runtime/pkg/protorun"
	"github.com/antonionduarte/go-simple-protocol-runtime/pkg/transport"
)

// GetView is sent by a peer protocol to fetch a snapshot of the
// current active view.
type GetView struct{ protorun.BaseRequest }

// View is the reply to GetView. Peers is a fresh slice owned by the
// caller — safe to mutate.
type View struct {
	protorun.BaseReply
	Peers []transport.Host
}

// ViewChanged is published every time the active view gains or loses
// a peer. Exactly one of HasAdded / HasRemoved is true per event.
type ViewChanged struct {
	protorun.BaseNotification
	Added, Removed         transport.Host
	HasAdded, HasRemoved   bool
}

// Protocol is the membership protocol. Construct with New, then
// Register with a Runtime.
type Protocol struct {
	contacts []transport.Host
	ctx      protorun.ProtocolContext

	mu   sync.Mutex
	view map[transport.Host]struct{}
}

// New returns a Protocol bootstrapped with the given contact peers.
// On Init the protocol will ConnectWithRetry to each contact.
func New(contacts []transport.Host) *Protocol {
	return &Protocol{
		contacts: contacts,
		view:     make(map[transport.Host]struct{}),
	}
}

func (p *Protocol) Start(ctx protorun.ProtocolContext) {
	p.ctx = ctx
	protorun.RegisterRequestHandler(ctx, p.handleGetView)
}

func (p *Protocol) Init(ctx protorun.ProtocolContext) {
	for _, c := range p.contacts {
		if err := ctx.ConnectWithRetry(c); err != nil {
			ctx.Logger().Error("ConnectWithRetry failed", "contact", (&c).ToString(), "err", err)
		}
	}
}

func (p *Protocol) OnSessionConnected(h transport.Host) {
	p.mu.Lock()
	if _, exists := p.view[h]; exists {
		p.mu.Unlock()
		return
	}
	p.view[h] = struct{}{}
	p.mu.Unlock()

	// Emit the notification *after* unlocking. PublishNotification
	// fans out to subscribers' channels, which can block under load —
	// holding the mutex across that would let a slow subscriber stall
	// other readers of the view.
	protorun.PublishNotification(p.ctx, ViewChanged{Added: h, HasAdded: true})
}

func (p *Protocol) OnSessionDisconnected(h transport.Host) {
	p.mu.Lock()
	if _, exists := p.view[h]; !exists {
		p.mu.Unlock()
		return
	}
	delete(p.view, h)
	p.mu.Unlock()

	protorun.PublishNotification(p.ctx, ViewChanged{Removed: h, HasRemoved: true})
}

// View returns a snapshot of the currently-known peers. Goroutine-safe.
// Used by tests for convergence detection; protocols on the same
// runtime should prefer the GetView IPC request or the ViewChanged
// notification.
func (p *Protocol) View() []transport.Host {
	p.mu.Lock()
	defer p.mu.Unlock()
	peers := make([]transport.Host, 0, len(p.view))
	for h := range p.view {
		peers = append(peers, h)
	}
	return peers
}

func (p *Protocol) handleGetView(_ *GetView, r protorun.Responder[*View]) {
	r.Reply(&View{Peers: slices.Clone(p.View())})
}
