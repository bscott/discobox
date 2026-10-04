//go:build !windows

package termguard

import (
	"os"
	"os/signal"
	"syscall"
)

// watchHangup has the program say it was hung up on before it dies of it, and
// returns what stops the watch.
//
// Catching SIGHUP is what stops the runtime from exiting on it, so the watch
// then puts the default back and raises it again: the program still dies of
// it, with the exit status a shell reports as a hangup. One that arrives after
// the stop, while the watch is coming down, is raised the same way rather than
// swallowed. A SIGHUP the program was started ignoring (nohup) stays ignored:
// catching it would be what stopped ignoring it.
func watchHangup(say func(string)) func() {
	if signal.Ignored(syscall.SIGHUP) {
		return func() {}
	}
	hup := make(chan os.Signal, 1)
	done := make(chan struct{})
	signal.Notify(hup, syscall.SIGHUP)
	go func() {
		select {
		case <-hup:
		case <-done:
			select {
			case <-hup:
			default:
				return
			}
		}
		say("SIGHUP")
		signal.Reset(syscall.SIGHUP)
		_ = syscall.Kill(os.Getpid(), syscall.SIGHUP)
	}()
	return func() {
		signal.Stop(hup)
		close(done)
	}
}
