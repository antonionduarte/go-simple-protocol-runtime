package transport

import (
	"bytes"
	"errors"
	"testing"
)

// TestHandshakeVersion_RoundTripCurrent verifies that an encoded
// Hello at the current version round-trips through the parser.
func TestHandshakeVersion_RoundTripCurrent(t *testing.T) {
	original := Host{IP: "127.0.0.1", Port: 9000}
	buf, err := encodeHello(original)
	if err != nil {
		t.Fatalf("encodeHello: %v", err)
	}
	ht, host, err := parseHandshakePayload(&buf)
	if err != nil {
		t.Fatalf("parseHandshakePayload: %v", err)
	}
	if ht != HandshakeHello {
		t.Errorf("got HandshakeType=%v, want HandshakeHello", ht)
	}
	if host != original {
		t.Errorf("got host=%+v, want %+v", host, original)
	}
}

// TestHandshakeVersion_Mismatch fabricates a Hello with a wrong
// version byte and verifies parseHandshakePayload returns an error
// wrapping ErrVersionMismatch.
func TestHandshakeVersion_Mismatch(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	buf.WriteByte(byte(HandshakeHello))
	buf.WriteByte(ProtocolVersion + 1) // pretend we're a future build
	_ = WriteHost(buf, Host{IP: "127.0.0.1", Port: 7000})

	_, _, err := parseHandshakePayload(buf)
	if err == nil {
		t.Fatalf("expected error for version mismatch, got nil")
	}
	if !errors.Is(err, ErrVersionMismatch) {
		t.Errorf("expected error to wrap ErrVersionMismatch, got %v", err)
	}
}

// TestHandshakeVersion_TruncatedAtVersion verifies that a Hello
// truncated immediately after the type byte (no version) is reported
// as a parse error rather than silently accepted.
func TestHandshakeVersion_TruncatedAtVersion(t *testing.T) {
	buf := bytes.NewBuffer([]byte{byte(HandshakeHello)})
	_, _, err := parseHandshakePayload(buf)
	if err == nil {
		t.Fatalf("expected truncated handshake to error")
	}
}
