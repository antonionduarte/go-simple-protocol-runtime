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
