package cli

import (
	"os"
	"strconv"
	"testing"

	"github.com/discobox-ai/discobox/cli/internal/tui"
)

func TestConsoleViewRoundTrips(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	if got := consoleViewFor("/src/foo"); !got.IsZero() {
		t.Fatalf("a fresh state dir has a view: %+v", got)
	}
	foo := tui.ListView{Server: "beta", FolderKey: "k", FolderLabel: "/src/foo @ main", FolderSource: "/src/foo", FolderLocal: true, Tags: []string{"ticket=ENG-12", "wip"}}
	if err := saveConsoleView("/src/foo", foo); err != nil {
		t.Fatalf("saveConsoleView: %v", err)
	}
	// Every folder keeps its own, and one never narrowed opens on everything.
	if got := consoleViewFor("/src/foo"); !got.Equal(foo) {
		t.Fatalf("view = %+v, want %+v", got, foo)
	}
	if got := consoleViewFor("/src/bar"); !got.IsZero() {
		t.Fatalf("view for another folder = %+v, want the default", got)
	}

	// Going back to the default drops the entry.
	if err := saveConsoleView("/src/foo", tui.ListView{}); err != nil {
		t.Fatalf("saveConsoleView: %v", err)
	}
	if _, ok := loadConsoleViews()["/src/foo"]; ok {
		t.Fatal("the default view was stored rather than dropping the entry")
	}
}

func TestConsoleViewsAreTrimmed(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	for i := range consoleViewLimit + 5 {
		if err := saveConsoleView("/src/"+strconv.Itoa(i), tui.ListView{Tags: []string{"t"}}); err != nil {
			t.Fatalf("saveConsoleView: %v", err)
		}
	}
	if got := len(loadConsoleViews()); got != consoleViewLimit {
		t.Fatalf("views = %d, want %d", got, consoleViewLimit)
	}
}

// A file written when a view held one tag opens on that tag.
func TestAConsoleViewWithOneTagStillOpens(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	if err := os.MkdirAll(cliStateDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	old := `{"/src/foo":{"server":"beta","tag":"wip","at":"2026-09-30T00:00:00Z"}}`
	if err := os.WriteFile(consoleViewsPath(), []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	want := tui.ListView{Server: "beta", Tags: []string{"wip"}}
	if got := consoleViewFor("/src/foo"); !got.Equal(want) {
		t.Fatalf("view = %+v, want %+v", got, want)
	}
}
