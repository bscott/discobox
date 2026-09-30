// Package termguard puts a terminal back the way it was found when the process
// that took it over dies without doing so itself.
//
// A full-screen program leaves the terminal in raw mode, on the alternate
// screen, and reporting the mouse, and only the program can undo that. Bubble
// Tea undoes it on every exit it sees — a quit, a signal it handles, a panic in
// Update, View or a command — but a process can end without Bubble Tea seeing
// it: a panic in any other goroutine (its own renderer and input reader
// included), a runtime fatal error such as a concurrent map write, os.Exit, or
// SIGKILL. What that leaves behind is the last frame, no echo or a shell that
// has adopted raw mode, and a mouse whose every movement is typed at the
// prompt as an escape sequence.
//
// Nothing inside the process can cover all of those, so the guard is a second
// process: the same binary, re-run as a hidden command, holding the read end of
// a pipe whose write end only the program holds. The kernel closes that pipe
// however the program ends, so the guard always hears about it. A program that
// finished properly writes one byte first; a pipe that closes without it is a
// program that died, and the guard restores the terminal and says why.
//
// The program also points the runtime's crash output (debug.SetCrashOutput) at
// a temporary file. A trace printed to the alternate screen is gone the moment
// the screen is switched back, so the file is where it survives; the guard
// names it in one line rather than reprinting it, since a trace of every
// goroutine runs to a thousand lines and would scroll everything else away.
package termguard

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime/debug"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
	"golang.org/x/term"
)

// readyTimeout bounds how long the program waits for the guard to have taken
// the terminal's state. A guard that has not started by then is not one worth
// holding the window for: the program runs unguarded, as it did before there
// was a guard.
const readyTimeout = 5 * time.Second

// ready is the line the guard writes once it holds the terminal's state, and
// finished the byte the program writes before it closes the lifeline on a
// proper exit.
const (
	ready    = "ready"
	finished = 'x'
)

// Guard is a running guard, as seen from the program it guards.
type Guard struct {
	cmd      *exec.Cmd
	lifeline io.WriteCloser
	report   *os.File
}

// Start runs the guard as command, which must end up in Serve with the report
// path appended to its arguments, and returns once the guard holds the
// terminal's state — so it must be called before the program changes it.
//
// From here until Release, a crash of this process writes its report to a
// temporary file, which Release removes again.
//
// An error means there is no guard, and nothing has been changed: the caller
// runs without one.
func Start(command []string) (*Guard, error) {
	report, err := os.CreateTemp("", "discobox-console-crash-*.log")
	if err != nil {
		return nil, err
	}
	g, err := start(command, report)
	if err != nil {
		_ = report.Close()
		_ = os.Remove(report.Name())
		return nil, err
	}
	if err := debug.SetCrashOutput(report, debug.CrashOptions{}); err != nil {
		g.stop()
		_ = report.Close()
		_ = os.Remove(report.Name())
		return nil, fmt.Errorf("send crash output to %s: %w", report.Name(), err)
	}
	// Every goroutine, not only the one that failed: a race or a deadlock is
	// explained by what the others were doing. It only ever raises the level,
	// so GOTRACEBACK set higher still wins.
	debug.SetTraceback("all")
	return g, nil
}

func start(command []string, report *os.File) (*Guard, error) {
	// No context of the console's: the guard is what has to outlive whatever
	// ends the console, cancellation included. Release is what ends it.
	//nolint:gosec // the program is this one and the arguments are its own
	cmd := exec.CommandContext(context.Background(), command[0], append(slices.Clone(command[1:]), report.Name())...)
	lifeline, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start the terminal guard: %w", err)
	}
	g := &Guard{cmd: cmd, lifeline: lifeline, report: report}

	line := make(chan string, 1)
	go func() {
		s, _ := bufio.NewReader(out).ReadString('\n')
		line <- strings.TrimSpace(s)
	}()
	select {
	case s := <-line:
		if s == ready {
			return g, nil
		}
		g.stop()
		return nil, errors.New("the terminal guard could not take the terminal's state")
	case <-time.After(readyTimeout):
		g.stop()
		return nil, errors.New("the terminal guard did not start")
	}
}

// Release tells the guard the program ended properly and waits for it to go.
// Call it once the terminal has been handed back; the crash report is removed,
// since there is nothing in it.
func (g *Guard) Release() {
	if g == nil {
		return
	}
	_ = debug.SetCrashOutput(nil, debug.CrashOptions{})
	_, _ = g.lifeline.Write([]byte{finished})
	_ = g.lifeline.Close()
	_ = g.cmd.Wait()
	_ = g.report.Close()
	_ = os.Remove(g.report.Name())
}

// stop ends a guard that never became one.
func (g *Guard) stop() {
	_ = g.lifeline.Close()
	_ = g.cmd.Process.Kill()
	_ = g.cmd.Wait()
}

// Serve is the guard: it takes the terminal's state, says so on ready, and
// waits on lifeline. A lifeline that ends without the finished byte is a
// program that died holding the terminal; Serve puts the terminal back and
// says where the report at reportPath is, if the runtime wrote one.
func Serve(lifeline io.Reader, readyOut io.Writer, reportPath string) error {
	ignoreSignals()
	tty, err := openTerminal()
	if err != nil {
		return err
	}
	defer tty.close()
	return serve(tty, lifeline, readyOut, reportPath)
}

func serve(tty terminal, lifeline io.Reader, readyOut io.Writer, reportPath string) error {
	states, err := tty.save()
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintln(readyOut, ready); err != nil {
		return err
	}

	var b [1]byte
	if n, _ := io.ReadFull(lifeline, b[:]); n == 1 && b[0] == finished {
		return nil
	}

	// The escape sequences first, while the output still interprets them the
	// way the program left it; the modes after. The message goes in the same
	// write: the shell takes the terminal back as the program dies and prints
	// its prompt as soon as it has, and one write is the least there is for
	// that prompt to land in the middle of.
	_, _ = io.WriteString(tty.out, resetSequence+explanation(reportPath))
	tty.restore(states)
	return nil
}

// resetSequence undoes everything a Bubble Tea program, and the renderer under
// it, may have left switched on. It is every mode rather than the ones this
// particular frame used, since the guard never saw the frame — and turning off
// a mode that was never on is harmless.
var resetSequence = strings.Join([]string{
	// A frame the program died in the middle of: until this, a terminal that
	// honors synchronized output shows nothing new at all.
	ansi.ResetModeSynchronizedOutput,
	ansi.ResetModeMouseNormal,
	ansi.ResetModeMouseHighlight,
	ansi.ResetModeMouseButtonEvent,
	ansi.ResetModeMouseAnyEvent,
	ansi.ResetModeMouseExtSgr,
	ansi.ResetModeMouseExtUrxvt,
	ansi.ResetModeMouseExtSgrPixel,
	ansi.ResetModeFocusEvent,
	ansi.ResetModeBracketedPaste,
	ansi.ResetModifyOtherKeys,
	ansi.KittyKeyboard(0, 1),
	ansi.ResetModeUnicodeCore,
	ansi.ResetModeInsertReplace,
	ansi.ResetModeOrigin,
	ansi.SetModeAutoWrap,
	ansi.ResetModeCursorKeys,
	ansi.ResetModeNumericKeypad,
	ansi.ResetStyle,
	ansi.ResetProgressBar,
	ansi.ResetCursorColor,
	ansi.ResetForegroundColor,
	ansi.ResetBackgroundColor,
	ansi.SetCursorStyle(0),
	ansi.SetWindowTitle(""),
	// The scroll region is the terminal's, not the screen's, so one the
	// renderer set would follow the user back to the primary screen. Resetting
	// it homes the cursor, which is why it is done on the alternate screen,
	// before leaving it restores the primary screen's cursor.
	ansi.SetTopBottomMargins(0, 0),
	ansi.ResetModeAltScreenSaveCursor,
	ansi.ShowCursor,
}, "")

// explanation is what the guard says once the terminal is back: one line, and
// where the report is when the runtime wrote one. The written \r\n is because
// the terminal is still in the mode the program left it in when this is
// written.
func explanation(reportPath string) string {
	if info, err := os.Stat(reportPath); err != nil || info.Size() == 0 {
		_ = os.Remove(reportPath)
		return "\r\ndiscobox: the console ended without restoring the terminal, and left no crash report; the terminal has been restored.\r\n"
	}
	return fmt.Sprintf("\r\ndiscobox: the console crashed; the terminal has been restored. The crash report is in %s\r\n", reportPath)
}

// terminal is the guard's own handle on the terminal the program is drawing on.
// It is opened by name rather than inherited: the guard's standard input is
// the lifeline, and the program's output may not be its standard error.
type terminal struct {
	// modes are the handles whose mode is saved and restored: the one tty on
	// Unix, the console's input and output on Windows.
	modes []*os.File
	out   io.Writer
}

func (t terminal) save() ([]*term.State, error) {
	states := make([]*term.State, len(t.modes))
	for i, f := range t.modes {
		s, err := term.GetState(int(f.Fd()))
		if err != nil {
			return nil, fmt.Errorf("read the terminal's state: %w", err)
		}
		states[i] = s
	}
	return states, nil
}

func (t terminal) restore(states []*term.State) {
	for i, f := range t.modes {
		_ = term.Restore(int(f.Fd()), states[i])
	}
}

func (t terminal) close() {
	for _, f := range t.modes {
		_ = f.Close()
	}
}
