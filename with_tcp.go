package protorun

import (
	"context"

	"github.com/antonionduarte/go-simple-protocol-runtime/transport"
)

// WithTCPTransport wires the runtime's transport + session layers with
// the framework's TCP+Hello/Ack stack. It is the typical way to set up
// a runtime — most users never need to construct TCPLayer or
// SessionLayer themselves.
//
// The supplied ctx becomes the parent for both layers' internal
// goroutines. Callers wanting custom channel buffer sizes can override
// individual values via the runtime.Default*Buffer constants in their
// own transport setup.
func WithTCPTransport(ctx context.Context) Option {
	return func(r *Runtime) {
		tcp := transport.NewTCPLayer(r.self, ctx, 0)
		session := transport.NewSessionLayer(tcp, r.self, ctx, 0, 0)
		r.networkLayer = tcp
		r.sessionLayer = session
	}
}
