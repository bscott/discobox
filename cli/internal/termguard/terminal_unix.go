//go:build !windows

package termguard

import (
	"os"
	"os/signal"
	"syscall"
)

func openTerminal() (terminal, error) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return terminal{}, err
	}
	return terminal{modes: []*os.File{tty}, out: tty}, nil
}

// ignoreSignals keeps the guard alive through what is sent to the whole
// foreground job: ^C and ^\ while the program has stepped aside for an editor
// or a harness, and a kill of the process group. The guard is the thing that
// must outlive all of them.
//
// SIGTTOU and SIGTTIN are what makes the restore work at all once the shell has
// the terminal back — which it takes the moment the program is gone. A process
// outside the foreground group that sets the terminal's mode or writes to it is
// stopped by them unless it ignores them.
//
// SIGHUP too, because the program dying can be what sends it: a program that
// is its session's leader — run straight from ssh -t, or as a tmux window's
// command — takes the terminal with it, and the kernel hangs up everything
// left in the foreground group, the guard included, before it has read the
// lifeline. Where the terminal really has gone, the guard's writes fail and it
// exits as soon as the lifeline closes, which the hangup ends the program to do.
//
// SIGTSTP is left alone, so the guard stops and continues with its job.
func ignoreSignals() {
	signal.Ignore(syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGTTOU, syscall.SIGTTIN)
}
