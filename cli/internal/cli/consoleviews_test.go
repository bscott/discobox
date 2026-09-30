package cli

import (
	"strconv"
	"testing"

	"github.com/discobox-ai/discobox/cli/internal/tui"
)

func TestConsoleViewRoundTrips(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	if got := consoleViewFor("/src/foo"); got != (tui.ListView{}) {
		t.Fatalf("a fresh state dir has a view: %+v", got)
	}
	foo := tui.ListView{Server: "beta", FolderKey: "k", FolderLabel: "/src/foo @ main", FolderSource: "/src/foo", FolderLocal: true, Tag: "wip"}
	if err := saveConsoleView("/src/foo", foo); err != nil {
		t.Fatalf("saveConsoleView: %v", err)
	}
	// Every folder keeps its own, and one never narrowed opens on everything.
	if got := consoleViewFor("/src/foo"); got != foo {
		t.Fatalf("view = %+v, want %+v", got, foo)
	}
	if got := consoleViewFor("/src/bar"); got != (tui.ListView{}) {
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
		if err := saveConsoleView("/src/"+strconv.Itoa(i), tui.ListView{Tag: "t"}); err != nil {
			t.Fatalf("saveConsoleView: %v", err)
		}
	}
	if got := len(loadConsoleViews()); got != consoleViewLimit {
		t.Fatalf("views = %d, want %d", got, consoleViewLimit)
	}
}
