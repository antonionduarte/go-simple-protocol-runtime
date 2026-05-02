package protorun

import "sync"

// codecRegistry owns the runtime-wide wireID-to-owning-protocol map
// used by Send to find the right codec and by processMessage to
// route inbound payloads to the right protocol's mailbox.
//
// Built up at Start time as protocols call ProtocolContext.RegisterCodec.
// Read on every send and every receive, write only at registration:
// hence sync.RWMutex.
type codecRegistry struct {
	mu     sync.RWMutex
	lookup map[uint64]*protoProtocol
}

func newCodecRegistry() *codecRegistry {
	return &codecRegistry{lookup: make(map[uint64]*protoProtocol)}
}

// Set records that wireID is owned by proto. Called from
// protocolContext.registerCodec while a protocol is in its Start
// phase. Last-writer-wins if two protocols claim the same wireID
// (strict mode catches that as a panic; non-strict logs a warning).
func (r *codecRegistry) Set(wireID uint64, proto *protoProtocol) {
	r.mu.Lock()
	r.lookup[wireID] = proto
	r.mu.Unlock()
}

// Get returns the protocol that owns wireID, or (nil, false) if no
// codec is registered.
func (r *codecRegistry) Get(wireID uint64) (*protoProtocol, bool) {
	r.mu.RLock()
	p, ok := r.lookup[wireID]
	r.mu.RUnlock()
	return p, ok
}
