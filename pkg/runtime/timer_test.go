package runtime

import (
	"testing"
	"time"

	"github.com/antonionduarte/go-simple-protocol-runtime/pkg/runtime/net"
)

type testTimer struct {
	id  int
	pid int
}

func (t *testTimer) TimerID() int    { return t.id }
func (t *testTimer) ProtocolID() int { return t.pid }

func TestCancelTimerStopsTimer(t *testing.T) {
	rt := New(net.NewHost(7400, "127.0.0.1"))
	rt.timerChannel = make(chan Timer, 1)

	timer := &testTimer{id: 1, pid: 1}
	rt.setupTimer(timer, 10*time.Millisecond)
	rt.cancelTimer(timer.id)

	select {
	case <-rt.timerChannel:
		t.Fatalf("expected timer to be cancelled and not fire")
	case <-time.After(30 * time.Millisecond):
	}
}
