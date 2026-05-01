package runtime

import (
	"testing"

	"github.com/antonionduarte/go-simple-protocol-runtime/pkg/runtime/net"
)

// fixedSizePing is a test message used to exercise BinaryCodec — every
// field is fixed-size so encoding/binary can size the struct.
//
// It implements Sender() inline (no embedded BaseMessage, since
// BaseMessage carries a string-typed net.Host that encoding/binary
// rejects).
type fixedSizePing struct {
	Seq   uint64
	Round uint32
}

func (fixedSizePing) Sender() net.Host { return net.Host{} }

func TestBinaryCodec_FixedSize_RoundTrip(t *testing.T) {
	codec := BinaryCodec[*fixedSizePing]{}
	original := &fixedSizePing{Seq: 0xdeadbeef, Round: 7}

	payload, err := codec.Encode(original)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if len(payload) != 12 {
		t.Fatalf("expected 12 bytes (uint64+uint32), got %d", len(payload))
	}

	got, err := codec.Decode(payload)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if *got != *original {
		t.Fatalf("round-trip: got %+v, want %+v", got, original)
	}
}

func TestBinaryCodec_DecodeShortPayload(t *testing.T) {
	codec := BinaryCodec[*fixedSizePing]{}
	if _, err := codec.Decode([]byte{0x01, 0x02}); err == nil {
		t.Fatalf("expected error for short payload")
	}
}
