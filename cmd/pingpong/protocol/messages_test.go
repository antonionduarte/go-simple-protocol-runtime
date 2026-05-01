package protocol

import (
	"testing"

	"github.com/antonionduarte/go-simple-protocol-runtime/pkg/runtime/net"
)

func TestPingSerializer_RoundTrip(t *testing.T) {
	sender := net.NewHost(5001, "127.0.0.1")
	original := NewPingMessage(sender, 42)

	payload, err := (&PingSerializer{}).Serialize(original)
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	roundtripped, err := (&PingSerializer{}).Deserialize(payload)
	if err != nil {
		t.Fatalf("Deserialize: %v", err)
	}
	got := roundtripped.(*PingMessage)
	if got.Seq() != 42 {
		t.Fatalf("Seq round-trip: got %d, want 42", got.Seq())
	}
	if got.MessageID() != PingMessageID || got.ProtocolID() != PingPongProtocolId {
		t.Fatalf("IDs not preserved: messageID=%d protocolID=%d", got.MessageID(), got.ProtocolID())
	}
}

func TestPongSerializer_RoundTrip(t *testing.T) {
	sender := net.NewHost(5002, "127.0.0.1")
	original := NewPongMessage(sender, 99)

	payload, err := (&PongSerializer{}).Serialize(original)
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	roundtripped, err := (&PongSerializer{}).Deserialize(payload)
	if err != nil {
		t.Fatalf("Deserialize: %v", err)
	}
	got := roundtripped.(*PongMessage)
	if got.Seq() != 99 {
		t.Fatalf("Seq round-trip: got %d, want 99", got.Seq())
	}
}

func TestPingSerializer_DeserializeShortPayload(t *testing.T) {
	if _, err := (&PingSerializer{}).Deserialize([]byte{0x01, 0x02}); err == nil {
		t.Fatalf("expected error for short payload")
	}
}
