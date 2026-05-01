package transport

import (
	"bytes"
	"testing"
)

func TestHost_RoundTrip_IPv4(t *testing.T) {
	original := Host{IP: "192.168.1.42", Port: 6443}
	var buf bytes.Buffer
	if err := WriteHost(&buf, original); err != nil {
		t.Fatalf("WriteHost: %v", err)
	}
	got, err := ReadHost(&buf)
	if err != nil {
		t.Fatalf("ReadHost: %v", err)
	}
	if got != original {
		t.Fatalf("round-trip: got %+v, want %+v", got, original)
	}
}

func TestHost_RoundTrip_IPv6(t *testing.T) {
	original := Host{IP: "2001:db8::1", Port: 9999}
	var buf bytes.Buffer
	if err := WriteHost(&buf, original); err != nil {
		t.Fatalf("WriteHost: %v", err)
	}
	got, err := ReadHost(&buf)
	if err != nil {
		t.Fatalf("ReadHost: %v", err)
	}
	if got != original {
		t.Fatalf("round-trip: got %+v, want %+v", got, original)
	}
}

func TestHost_RoundTrip_Hostname(t *testing.T) {
	original := Host{IP: "node-3.example.com", Port: 443}
	var buf bytes.Buffer
	if err := WriteHost(&buf, original); err != nil {
		t.Fatalf("WriteHost: %v", err)
	}
	got, err := ReadHost(&buf)
	if err != nil {
		t.Fatalf("ReadHost: %v", err)
	}
	if got != original {
		t.Fatalf("round-trip: got %+v, want %+v", got, original)
	}
}
