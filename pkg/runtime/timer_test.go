package runtime

import (
	"testing"
	"time"
)

type testTimer struct {
	id  int
	pid int
}

func (t *testTimer) TimerID() int    { return t.id }
func (t *testTimer) ProtocolID() int { return t.pid }

func TestCancelTimerStopsTimer(t *testing.T) {
	resetRuntimeForTests()
	runtime := GetRuntimeInstance()

	timer := &testTimer{id: 1, pid: 1}

	done := make(chan struct{})

	// Replace the runtime's timerChannel with a small buffered channel we can observe.
	runtime.timerChannel = make(chan Timer, 1)

	SetupTimer(timer, 10*time.Millisecond)

	// Cancel immediately; the timer callback should not fire.
	CancelTimer(timer.id)

	select {
	case <-runtime.timerChannel:
		t.Fatalf("expected timer to be cancelled and not fire")
	case <-time.After(30 * time.Millisecond):
		close(done)
	}
}
