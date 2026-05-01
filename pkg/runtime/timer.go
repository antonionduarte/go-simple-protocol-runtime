package runtime

import (
	"context"
	"time"
)

// Timer represents a logical timer scheduled through the runtime. This is
// part of the public API exposed to protocol implementations.
// TimerID must be stable and unique for the lifetime of the timer
// within the runtime, as it is used as the key in Runtime.ongoingTimers.
// Reusing the same TimerID for overlapping timers will cause later
// registrations to overwrite earlier ones.
type Timer interface {
	TimerID() int
	ProtocolID() int
}

// shutdownCtx returns the runtime's shutdown context, falling back to
// context.Background() if Start has not yet been called. The fallback lets
// callers register timers during construction or in tests.
func (r *Runtime) shutdownCtx() context.Context {
	r.timerMutex.Lock()
	ctx := r.ctx
	r.timerMutex.Unlock()
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func (r *Runtime) setupTimer(timer Timer, duration time.Duration) {
	ctx := r.shutdownCtx()
	goTimer := time.AfterFunc(duration, func() {
		r.timerMutex.Lock()
		delete(r.ongoingTimers, timer.TimerID())
		r.timerMutex.Unlock()
		select {
		case r.timerChannel <- timer:
		case <-ctx.Done():
		}
	})
	r.timerMutex.Lock()
	if existing, ok := r.ongoingTimers[timer.TimerID()]; ok && existing != nil {
		existing.Stop()
	}
	r.ongoingTimers[timer.TimerID()] = goTimer
	r.timerMutex.Unlock()
}

func (r *Runtime) cancelTimer(timerID int) {
	r.timerMutex.Lock()
	if goTimer, ok := r.ongoingTimers[timerID]; ok && goTimer != nil {
		goTimer.Stop()
		delete(r.ongoingTimers, timerID)
	}
	if cancel, ok := r.ongoingPeriodicTimers[timerID]; ok && cancel != nil {
		cancel()
		delete(r.ongoingPeriodicTimers, timerID)
	}
	r.timerMutex.Unlock()
}

func (r *Runtime) setupPeriodicTimer(timer Timer, duration time.Duration) {
	parent := r.shutdownCtx()
	ctx, cancel := context.WithCancel(parent)

	r.timerMutex.Lock()
	if existing, ok := r.ongoingPeriodicTimers[timer.TimerID()]; ok && existing != nil {
		existing()
	}
	r.ongoingPeriodicTimers[timer.TimerID()] = cancel
	r.timerMutex.Unlock()

	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		defer cancel()
		ticker := time.NewTicker(duration)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				select {
				case r.timerChannel <- timer:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
}
