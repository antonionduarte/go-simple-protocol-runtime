package runtime

import (
	"time"
)

type (
	// Timer represents a logical timer scheduled through the runtime.
	// TimerID must be stable and unique for the lifetime of the timer
	// within the runtime, as it is used as the key in Runtime.ongoingTimers.
	// Reusing the same TimerID for overlapping timers will cause later
	// registrations to overwrite earlier ones.
	Timer interface {
		TimerID() int
		ProtocolID() int
	}
)

func SetupTimer(timer Timer, duration time.Duration) {
	runtime := GetRuntimeInstance()
	goTimer := time.AfterFunc(duration, func() {
		runtime.timerChannel <- timer
		runtime.timerMutex.Lock()
		delete(runtime.ongoingTimers, timer.TimerID())
		runtime.timerMutex.Unlock()
	})
	runtime.timerMutex.Lock()
	runtime.ongoingTimers[timer.TimerID()] = goTimer
	runtime.timerMutex.Unlock()
}

func CancelTimer(timerID int) {
	runtime := GetRuntimeInstance()
	runtime.timerMutex.Lock()
	goTimer, ok := runtime.ongoingTimers[timerID]
	if ok && goTimer != nil {
		goTimer.Stop()
		delete(runtime.ongoingTimers, timerID)
	}
	runtime.timerMutex.Unlock()
}

func SetupPeriodicTimer(timer Timer, duration time.Duration) {
	runtime := GetRuntimeInstance()
	goTimer := time.AfterFunc(duration, func() {
		runtime.timerChannel <- timer

		runtime.timerMutex.Lock()
		delete(runtime.ongoingTimers, timer.TimerID())
		runtime.timerMutex.Unlock()

		SetupPeriodicTimer(timer, duration)
	})
	runtime.timerMutex.Lock()
	runtime.ongoingTimers[timer.TimerID()] = goTimer
	runtime.timerMutex.Unlock()
}
