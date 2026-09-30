package cli

import (
	"os"

	"github.com/discobox-ai/discobox/cli/internal/termguard"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// consoleGuardCommand is where the guard runs: this binary, under admin.
var consoleGuardCommand = []string{"admin", "console-guard"}

// newConsoleGuardCommand is the process that puts the terminal back when the
// console dies without doing so itself. See termguard.
//
// It is hidden because nothing types it: the console starts it, with the crash
// report it is to show as its one argument.
func (a *App) newConsoleGuardCommand() *cobra.Command {
	return &cobra.Command{
		Use:    "console-guard REPORT",
		Short:  "Restore the terminal if the console dies holding it",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		// The root's hook resolves the server and starts the parent watch, and
		// a guard needs neither. Whatever the environment gets wrong, the guard
		// must still start, or the console runs without one.
		PersistentPreRunE: func(*cobra.Command, []string) error { return nil },
		RunE: func(cmd *cobra.Command, args []string) error {
			return termguard.Serve(cmd.InOrStdin(), cmd.OutOrStdout(), args[0])
		},
	}
}

// startConsoleGuard starts the guard for a console about to take the terminal,
// or returns nil when there is no terminal to guard or no guard to be had. A
// console without a guard is what there was before there was one, so failing
// to start it stops nothing.
func startConsoleGuard() *termguard.Guard {
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return nil
	}
	self, err := os.Executable()
	if err != nil {
		return nil
	}
	guard, err := termguard.Start(append([]string{self}, consoleGuardCommand...))
	if err != nil {
		return nil
	}
	return guard
}
