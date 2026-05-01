package runtime

import (
	"context"
	"testing"
	"time"

	"github.com/antonionduarte/go-simple-protocol-runtime/pkg/runtime/net"
)

// TestRuntime_TwoRuntimes_PingPong stands up two independent runtimes on
// localhost ports and asserts that they complete a session handshake and
// exchange at least one application message in each direction. This is the
// primary end-to-end test that the singleton removal made possible — the
// previous incarnation could only have one real runtime per test.
func TestRuntime_TwoRuntimes_PingPong(t *testing.T) {
	hostA := net.NewHost(7301, "127.0.0.1")
	hostB := net.NewHost(7302, "127.0.0.1")

	rtA := buildRuntime(t, hostA, hostB, 1001)
	rtB := buildRuntime(t, hostB, hostA, 1001)

	if err := rtA.Start(); err != nil {
		t.Fatalf("rtA.Start: %v", err)
	}
	defer rtA.Cancel()
	if err := rtB.Start(); err != nil {
		t.Fatalf("rtB.Start: %v", err)
	}
	defer rtB.Cancel()

	// Both protocols mark themselves "received the peer message" via the
	// MockProtocol.HandledMessages slice. Wait for both to register at least
	// one inbound message.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if hadInbound(rtA, 1001) && hadInbound(rtB, 1001) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for ping-pong message exchange between rtA/rtB")
}

func hadInbound(rt *Runtime, protocolID int) bool {
	proto := rt.GetProtocol(protocolID)
	if proto == nil {
		return false
	}
	mp, ok := proto.protocol.(*twoSidedProtocol)
	if !ok {
		return false
	}
	return mp.received.Load()
}

// buildRuntime wires a fresh Runtime + TCPLayer + SessionLayer + integration
// protocol that connects to peer on init and replies once on inbound.
func buildRuntime(t *testing.T, self, peer net.Host, protoID int) *Runtime {
	t.Helper()
	rt := New(self)
	ctx := context.Background()
	tcp := net.NewTCPLayer(self, ctx, 0)
	session := net.NewSessionLayer(tcp, self, ctx, 0, 0)
	rt.RegisterNetworkLayer(tcp)
	rt.RegisterSessionLayer(session)
	rt.RegisterProtocol(NewProtoProtocol(&twoSidedProtocol{
		ProtoID:  protoID,
		SelfHost: self,
		Peer:     peer,
	}, self))
	return rt
}

// TestRuntime_LifecycleShutdown starts a runtime, lets it run briefly, then
// calls Cancel and ensures shutdown completes without further events being
// emitted on the local session layer.
func TestRuntime_LifecycleShutdown(t *testing.T) {
	hostA := net.NewHost(7303, "127.0.0.1")
	hostB := net.NewHost(7304, "127.0.0.1")

	rtA := New(hostA)
	ctxA := context.Background()
	tcpA := net.NewTCPLayer(hostA, ctxA, 0)
	sessionA := net.NewSessionLayer(tcpA, hostA, ctxA, 0, 0)
	rtA.RegisterNetworkLayer(tcpA)
	rtA.RegisterSessionLayer(sessionA)
	rtA.RegisterProtocol(NewProtoProtocol(&twoSidedProtocol{
		ProtoID:  2001,
		SelfHost: hostA,
		Peer:     hostB,
	}, hostA))

	// Stand up B as a real runtime too so the handshake can complete.
	rtB := New(hostB)
	ctxB := context.Background()
	tcpB := net.NewTCPLayer(hostB, ctxB, 0)
	sessionB := net.NewSessionLayer(tcpB, hostB, ctxB, 0, 0)
	rtB.RegisterNetworkLayer(tcpB)
	rtB.RegisterSessionLayer(sessionB)
	rtB.RegisterProtocol(NewProtoProtocol(&twoSidedProtocol{
		ProtoID:  2001,
		SelfHost: hostB,
		Peer:     hostA,
	}, hostB))

	if err := rtA.Start(); err != nil {
		t.Fatalf("rtA.Start: %v", err)
	}
	if err := rtB.Start(); err != nil {
		t.Fatalf("rtB.Start: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	rtA.Cancel()
	rtB.Cancel()

	time.Sleep(20 * time.Millisecond)

	select {
	case ev := <-sessionA.OutChannelEvents():
		t.Fatalf("did not expect session event on A after Cancel, got %T", ev)
	case <-time.After(20 * time.Millisecond):
	}
}

// TestRuntime_RapidStartCancel runs many tight Start->short-sleep->Cancel
// cycles with a 1ms periodic timer arming/disarming each iteration. This is
// the primary regression test for the timer-rearm and dispatcher-shutdown
// races; it should be run under -race to catch any send-on-closed-channel
// or goroutine-leak regressions.
func TestRuntime_RapidStartCancel(t *testing.T) {
	const iterations = 50
	for i := 0; i < iterations; i++ {
		self := net.NewHost(0, "127.0.0.1")
		rt := New(self)

		mock := NewMockNetworkLayer()
		rt.RegisterNetworkLayer(mock)
		sess := net.NewSessionLayer(mock, self, context.Background(), 0, 0)
		rt.RegisterSessionLayer(sess)

		proto := NewProtoProtocol(&MockProtocol{ProtoID: 9000, MockSelf: self}, self)
		rt.RegisterProtocol(proto)

		if err := rt.Start(); err != nil {
			t.Fatalf("iteration %d: Start failed: %v", i, err)
		}
		rt.setupPeriodicTimer(&rapidTickTimer{id: 1, pid: 9000}, time.Millisecond)
		time.Sleep(5 * time.Millisecond)

		done := make(chan struct{})
		go func() {
			rt.Cancel()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatalf("iteration %d: Cancel did not return within 2s", i)
		}
	}
}

type rapidTickTimer struct{ id, pid int }

func (t *rapidTickTimer) TimerID() int    { return t.id }
func (t *rapidTickTimer) ProtocolID() int { return t.pid }
