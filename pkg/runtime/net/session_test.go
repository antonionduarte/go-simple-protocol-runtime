package net

import (
	"context"
	"testing"
	"time"
)

// waitSessionEvent waits for a SessionEvent on the given channel or fails the test
// after the provided timeout.
func waitSessionEvent(t *testing.T, ch chan SessionEvent, timeout time.Duration) SessionEvent {
	t.Helper()
	select {
	case ev := <-ch:
		return ev
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for SessionEvent")
		return nil
	}
}

// TestSessionLayerSuccessfulHandshake verifies that two SessionLayers built on top
// of TCPLayer perform the handshake and emit SessionConnected events with the
// correct logical hosts.
func TestSessionLayerSuccessfulHandshake(t *testing.T) {
	// two logical hosts
	h1 := NewHost(7001, "127.0.0.1")
	h2 := NewHost(7002, "127.0.0.1")

	ctx1, cancel1 := context.WithCancel(context.Background())
	defer cancel1()
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()

	tcp1 := NewTCPLayer(h1, ctx1)
	tcp2 := NewTCPLayer(h2, ctx2)
	defer tcp1.Cancel()
	defer tcp2.Cancel()

	s1 := NewSessionLayer(tcp1, h1, ctx1)
	s2 := NewSessionLayer(tcp2, h2, ctx2)

	// initiate session from h1 to h2
	s1.Connect(h2)

	ev1 := waitSessionEvent(t, s1.OutChannelEvents(), 5*time.Second)
	ev2 := waitSessionEvent(t, s2.OutChannelEvents(), 5*time.Second)

	c1, ok1 := ev1.(*SessionConnected)
	c2, ok2 := ev2.(*SessionConnected)
	if !ok1 || !ok2 {
		t.Fatalf("expected SessionConnected on both sides, got %T and %T", ev1, ev2)
	}

	if !CompareHost(c1.Host(), h2) {
		t.Fatalf("expected s1 to see peer %v, got %v", h2, c1.Host())
	}
	if !CompareHost(c2.Host(), h1) {
		t.Fatalf("expected s2 to see peer %v, got %v", h1, c2.Host())
	}
}

// TestSessionLayerFailedConnection verifies that a failed connect attempt
// results in a SessionFailed event with the correct logical host.
func TestSessionLayerFailedConnection(t *testing.T) {
	hClient := NewHost(7101, "127.0.0.1")
	hNoServer := NewHost(7102, "127.0.0.1")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tcp := NewTCPLayer(hClient, ctx)
	defer tcp.Cancel()

	s := NewSessionLayer(tcp, hClient, ctx)

	// Connect to a host that has no listener
	s.Connect(hNoServer)

	ev := waitSessionEvent(t, s.OutChannelEvents(), 5*time.Second)
	failed, ok := ev.(*SessionFailed)
	if !ok {
		t.Fatalf("expected SessionFailed, got %T", ev)
	}
	if !CompareHost(failed.Host(), hNoServer) {
		t.Fatalf("expected failed host %v, got %v", hNoServer, failed.Host())
	}
}

// TestSessionLayerCancelStopsEvents ensures that calling Cancel on the
// SessionLayer stops the handler loop and no further events are emitted.
func TestSessionLayerCancelStopsEvents(t *testing.T) {
	h := NewHost(7201, "127.0.0.1")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tcp := NewTCPLayer(h, ctx)
	defer tcp.Cancel()

	s := NewSessionLayer(tcp, h, ctx)

	// Immediately cancel the session layer; there should be no events after this.
	s.Cancel()

	select {
	case ev := <-s.OutChannelEvents():
		t.Fatalf("did not expect event after SessionLayer.Cancel(), got %T", ev)
	case <-time.After(20 * time.Millisecond):
		// ok
	}
}
