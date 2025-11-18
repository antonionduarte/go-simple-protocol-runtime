package runtime

import (
	"context"
	"encoding/binary"
	"testing"
	"time"

	"github.com/antonionduarte/go-simple-protocol-runtime/pkg/runtime/net"
)

func TestRuntime_EndToEndIntegration(t *testing.T) {
	resetRuntimeForTests()
	runtime := GetRuntimeInstance()

	hostA := net.NewHost(7301, "127.0.0.1")
	hostB := net.NewHost(7302, "127.0.0.1")

	// Contexts for both sides
	ctxA, cancelA := context.WithCancel(context.Background())
	defer cancelA()
	ctxB, cancelB := context.WithCancel(context.Background())
	defer cancelB()

	// Transport + session for A (owned by runtime)
	tcpA := net.NewTCPLayer(hostA, ctxA)
	sessionA := net.NewSessionLayer(tcpA, hostA, ctxA)
	runtime.RegisterNetworkLayer(tcpA)
	runtime.RegisterSessionLayer(sessionA)

	// Transport + session for B (no runtime)
	tcpB := net.NewTCPLayer(hostB, ctxB)
	sessionB := net.NewSessionLayer(tcpB, hostB, ctxB)

	defer tcpB.Cancel()

	// Register integration protocol
	proto := NewProtoProtocol(&IntegrationProtocol{
		ProtoID:  1001,
		SelfHost: hostA,
		Peer:     hostB,
	}, hostA)
	runtime.RegisterProtocol(proto)

	// Start the runtime
	runtime.Start()
	defer runtime.Cancel()

	// Wait for B to see SessionConnected
	evB := waitSessionEventRuntime(t, sessionB.OutChannelEvents(), 5*time.Second)
	if _, ok := evB.(*net.SessionConnected); !ok {
		t.Fatalf("expected SessionConnected on B, got %T", evB)
	}

	// Now wait for a session-level application message on B
	select {
	case sm := <-sessionB.OutMessages():
		// sm.Msg should contain [LayerID(Application) handled already] and then
		// [ProtocolID || MessageID || Payload]. We only check ProtocolID/MessageID.
		buf := sm.Msg
		if buf.Len() < 4 {
			t.Fatalf("expected at least 4 bytes for protocolID/messageID, got %d", buf.Len())
		}
		var protoID, msgID uint16
		if err := binary.Read(&buf, binary.LittleEndian, &protoID); err != nil {
			t.Fatalf("failed to read protocolID: %v", err)
		}
		if err := binary.Read(&buf, binary.LittleEndian, &msgID); err != nil {
			t.Fatalf("failed to read messageID: %v", err)
		}
		if protoID != 1001 || msgID != 1 {
			t.Fatalf("unexpected protocolID/messageID: got (%d,%d), want (1001,1)", protoID, msgID)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for SessionMessage on B")
	}
}

// TestRuntime_LifecycleShutdown starts a runtime with a transport + session
// layer, lets it run briefly, then calls Cancel and ensures shutdown
// completes without further events being emitted on the remote session.
func TestRuntime_LifecycleShutdown(t *testing.T) {
	resetRuntimeForTests()
	runtime := GetRuntimeInstance()

	hostA := net.NewHost(7303, "127.0.0.1")
	hostB := net.NewHost(7304, "127.0.0.1")

	ctxA, cancelA := context.WithCancel(context.Background())
	defer cancelA()
	ctxB, cancelB := context.WithCancel(context.Background())
	defer cancelB()

	// Transport + session for A (owned by runtime)
	tcpA := net.NewTCPLayer(hostA, ctxA)
	sessionA := net.NewSessionLayer(tcpA, hostA, ctxA)
	runtime.RegisterNetworkLayer(tcpA)
	runtime.RegisterSessionLayer(sessionA)

	// Transport + session for B (no runtime); we only keep it alive to complete
	// the handshake, but it is not owned by the runtime.
	tcpB := net.NewTCPLayer(hostB, ctxB)
	_ = net.NewSessionLayer(tcpB, hostB, ctxB)
	defer tcpB.Cancel()

	// Register a trivial protocol that just connects to B on init.
	proto := NewProtoProtocol(&IntegrationProtocol{
		ProtoID:  2001,
		SelfHost: hostA,
		Peer:     hostB,
	}, hostA)
	runtime.RegisterProtocol(proto)

	runtime.Start()

	// Allow some time for handshake and message exchange (if any).
	time.Sleep(100 * time.Millisecond)

	// Cancel the runtime and ensure it returns promptly.
	runtime.Cancel()

	// After Cancel, the runtime-owned layers (tcpA/sessionA) should be
	// quiescent. Allow a brief grace period, then assert no further events.
	time.Sleep(20 * time.Millisecond)

	select {
	case ev := <-sessionA.OutChannelEvents():
		t.Fatalf("did not expect session event on A after runtime.Cancel(), got %T", ev)
	case <-time.After(20 * time.Millisecond):
		// ok
	}
}
