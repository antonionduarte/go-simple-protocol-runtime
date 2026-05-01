package protocol

import (
	"testing"

	"github.com/antonionduarte/go-simple-protocol-runtime/pkg/runtime/net"
)

func TestPingCodec_RoundTrip(t *testing.T) {
	original := NewPingMessage(net.NewHost(5001, "127.0.0.1"), 42)
	payload, err := PingCodec{}.Encode(original)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	got, err := PingCodec{}.Decode(payload)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got.Seq != 42 {
		t.Fatalf("Seq round-trip: got %d, want 42", got.Seq)
	}
}

func TestPongCodec_RoundTrip(t *testing.T) {
	original := NewPongMessage(net.NewHost(5002, "127.0.0.1"), 99)
	payload, err := PongCodec{}.Encode(original)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	got, err := PongCodec{}.Decode(payload)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got.Seq != 99 {
		t.Fatalf("Seq round-trip: got %d, want 99", got.Seq)
	}
}

func TestPingCodec_DecodeShortPayload(t *testing.T) {
	if _, err := (PingCodec{}).Decode([]byte{0x01, 0x02}); err == nil {
		t.Fatalf("expected error for short payload")
	}
}
