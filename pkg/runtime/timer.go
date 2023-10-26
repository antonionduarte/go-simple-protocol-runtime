package runtime

import (
	"time"
)

type (
	Timer interface {
		TimerID() int
		ProtocolID() int
	}
)

// TODO: I don't know if this is enough, because I might want to have several timers for the same timerID.

func SetupTimer(timer Timer, duration time.Duration) {
	// TODO: I think i have a race condition here. CHANGED MY MIND. THESE DEFINITELY HAVE RACE CONDITIONS.
	instance := GetRuntimeInstance()
	goTimer := time.AfterFunc(duration, func() {
		instance.timerChannel <- timer
		delete(instance.ongoingTimers, timer.TimerID())
	})
	instance.ongoingTimers[timer.TimerID()] = goTimer
}

func CancelTimer(timerID int) {
	// TODO: I think i have a race condition here.
	instance := GetRuntimeInstance()
	goTimer := instance.ongoingTimers[timerID]
	goTimer.Stop()
	delete(instance.ongoingTimers, timerID)
}

func SetupPeriodicTimer(timer Timer, duration time.Duration) {
	// TODO: I think i have a race condition here.
	instance := GetRuntimeInstance()
	goTimer := time.AfterFunc(duration, func() {
		instance.timerChannel <- timer
		delete(instance.ongoingTimers, timer.TimerID())
		SetupPeriodicTimer(timer, duration)
	})
	instance.ongoingTimers[timer.TimerID()] = goTimer
}
