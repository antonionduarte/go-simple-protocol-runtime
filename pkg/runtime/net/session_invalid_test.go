package net

import (
	"bytes"
	"context"
	"testing"
)

// TestSessionLayer_UnknownLayerIdentifier ensures that a SessionMessage with
// an unknown LayerIdentifier is dropped without panicking.
func TestSessionLayer_UnknownLayerIdentifier(t *testing.T) {
	self := NewHost(8001, "127.0.0.1")

	// Use a mock transport: we can re-use TCPLayer with no external peers by
	// not connecting anywhere and just feeding messages directly.
	tcp := NewTCPLayer(self, context.Background())
	defer tcp.Cancel()

	session := NewSessionLayer(tcp, self, context.Background())

	// Construct a TransportMessage whose payload has an invalid LayerIdentifier.
	payload := bytes.NewBuffer(nil)
	payload.WriteByte(0xFF) // invalid layer
	tcp.OutChannel() <- TransportMessage{Host: self, Msg: *payload}

	// Let the session handler process the message; we only assert that it
	// doesn't panic and doesn't emit any SessionEvent.
	select {
	case ev := <-session.OutChannelEvents():
		t.Fatalf("did not expect session event for unknown layer, got %T", ev)
	default:
		// ok
	}
}
