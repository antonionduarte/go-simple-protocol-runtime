package runtime

import (
	"context"
	"testing"
	"time"

	"github.com/antonionduarte/go-simple-protocol-runtime/pkg/runtime/net"
)

// TestRuntime_RegisterAndRun verifies that Register adds the protocol and
// Run blocks until Cancel is invoked from another goroutine.
func TestRuntime_RegisterAndRun(t *testing.T) {
	self := net.NewHost(0, "127.0.0.1")
	rt := New(self)

	mock := NewMockNetworkLayer()
	rt.RegisterNetworkLayer(mock)
	sess := net.NewSessionLayer(mock, self, context.Background(), 0, 0)
	rt.RegisterSessionLayer(sess)

	impl := &MockProtocol{}
	rt.Register(impl)
	if len(rt.protocols) != 1 {
		t.Fatalf("Register did not append: got %d protocols", len(rt.protocols))
	}

	done := make(chan error, 1)
	go func() {
		done <- Run(rt)
	}()

	// Give Run a moment to enter its blocking wait, then cancel.
	time.Sleep(50 * time.Millisecond)
	rt.Cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("Run did not return within 2s of Cancel")
	}

	if !impl.StartCalled {
		t.Errorf("Start was not called on the registered protocol")
	}
	if !impl.InitCalled {
		t.Errorf("Init was not called on the registered protocol")
	}
}
