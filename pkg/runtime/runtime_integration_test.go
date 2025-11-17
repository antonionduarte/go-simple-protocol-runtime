package runtime

import (
	"context"
	"encoding/binary"
	"testing"
	"time"

	"github.com/antonionduarte/go-simple-protocol-runtime/pkg/runtime/net"
)

/* ------------------------------------------------------------------
   Integration-style end-to-end test (runtime + session + TCP)
   ------------------------------------------------------------------ */

// TestRuntime_EndToEndIntegration sets up a runtime with real TCPLayer and
// SessionLayer, plus a remote SessionLayer, and verifies that when the
// protocol sends a message on OnSessionConnected, the remote side receives
// an application-level SessionMessage with the expected protocol/message IDs.
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
