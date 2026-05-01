package runtime

import (
	"os"
	"os/signal"
	"syscall"
)

// Run starts the runtime, installs SIGINT/SIGTERM handlers that call
// rt.Cancel() on receipt, and blocks until the runtime context is done.
// It returns the error from rt.Start() if any. Use this from cmd/main
// instead of calling rt.Start() yourself and then blocking on select{}.
func Run(rt *Runtime) error {
	if err := rt.Start(); err != nil {
		return err
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		select {
		case <-sigCh:
			rt.Logger().Info("signal received, cancelling runtime")
			rt.Cancel()
		case <-rt.ctx.Done():
		}
		signal.Stop(sigCh)
	}()

	<-rt.ctx.Done()
	return nil
}
