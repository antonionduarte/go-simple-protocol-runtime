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
// TODO: I'm using mutexes here, maybe create some kind of TimerManager that handles all the timers through channels.

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
	// TODO: Handle error
	goTimer := runtime.ongoingTimers[timerID]
	runtime.timerMutex.Unlock()
	goTimer.Stop()
	delete(runtime.ongoingTimers, timerID)
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
