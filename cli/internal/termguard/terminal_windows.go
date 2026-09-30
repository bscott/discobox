//go:build windows

package termguard

import (
	"os"
	"os/signal"
)

// openTerminal opens the console the guard shares with the program. Its input
// and output modes are separate, and the program sets both: raw input, and
// virtual terminal processing on the output.
func openTerminal() (terminal, error) {
	in, err := os.OpenFile("CONIN$", os.O_RDWR, 0)
	if err != nil {
		return terminal{}, err
	}
	out, err := os.OpenFile("CONOUT$", os.O_RDWR, 0)
	if err != nil {
		_ = in.Close()
		return terminal{}, err
	}
	return terminal{modes: []*os.File{in, out}, out: out}, nil
}

// ignoreSignals keeps the guard alive through the ^C and ^Break the console
// sends every process attached to it.
func ignoreSignals() {
	signal.Ignore(os.Interrupt)
}
