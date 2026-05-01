package runtime

import (
	"os"
	"os/signal"
	"syscall"
)

// Run starts the runtime, installs SIGINT/SIGTERM handlers that call
// Cancel on receipt, and blocks until the runtime context is done.
// Returns the error from start if any. Use Run from cmd/main instead of
// orchestrating Start + signal handling + select{} yourself.
func (r *Runtime) Run() error {
	if err := r.start(); err != nil {
		return err
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		select {
		case <-sigCh:
			r.Logger().Info("signal received, cancelling runtime")
			r.Cancel()
		case <-r.ctx.Done():
		}
		signal.Stop(sigCh)
	}()

	<-r.ctx.Done()
	return nil
}
