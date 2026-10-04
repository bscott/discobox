package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/discobox-ai/discobox/cli/internal/tui"
)

// The console keeps the header's filters — server, folder, tag — per folder,
// so the next window in a folder opens on what the last one was left on, and a
// folder never narrowed opens on everything. Like the prompt drafts it is
// derived convenience state, kept in the CLI's state directory and always
// best-effort: a missing or corrupt file opens the window on everything, which
// is where it opens without one.

// consoleViewsFile is the state file, relative to the CLI's state directory.
const consoleViewsFile = "console-views.json"

// consoleViewLimit bounds the file the way the drafts' limit does.
const consoleViewLimit = 50

type consoleView struct {
	Server       string   `json:"server,omitempty"`
	FolderKey    string   `json:"folderKey,omitempty"`
	FolderLabel  string   `json:"folderLabel,omitempty"`
	FolderSource string   `json:"folderSource,omitempty"`
	FolderLocal  bool     `json:"folderLocal,omitempty"`
	Tags         []string `json:"tags,omitempty"`
	// Tag is the one tag a view held before it could hold several. It is
	// read as Tags, and a save never sets it: a folder's entry is rewritten
	// in the new shape when its own filters move, while the others' entries
	// are written back as they were read, Tag and all, so it must stay in the
	// struct for as long as files from before may hold it.
	Tag string `json:"tag,omitempty"`
	// At is when it was written, and is what trimming sorts on.
	At time.Time `json:"at"`
}

func consoleViewsPath() string {
	return filepath.Join(cliStateDir(), consoleViewsFile)
}

func loadConsoleViews() map[string]consoleView {
	data, err := os.ReadFile(consoleViewsPath())
	if err != nil {
		return nil
	}
	var views map[string]consoleView
	if err := json.Unmarshal(data, &views); err != nil {
		return nil
	}
	return views
}

// consoleViewFor returns the filters left in folder, or the zero view — every
// server, folder and tag — when there are none.
func consoleViewFor(folder string) tui.ListView {
	if strings.TrimSpace(folder) == "" {
		return tui.ListView{}
	}
	v, ok := loadConsoleViews()[folder]
	if !ok {
		return tui.ListView{}
	}
	return tui.ListView{
		Server:       v.Server,
		FolderKey:    v.FolderKey,
		FolderLabel:  v.FolderLabel,
		FolderSource: v.FolderSource,
		FolderLocal:  v.FolderLocal,
		Tags:         v.tags(),
	}
}

// saveConsoleView records view as the filters for folder. The zero view drops
// the entry: it is the default, and storing it would be remembering nothing.
func saveConsoleView(folder string, view tui.ListView) error {
	if strings.TrimSpace(folder) == "" {
		return nil
	}
	views := loadConsoleViews()
	if view.IsZero() {
		if _, ok := views[folder]; !ok {
			return nil
		}
		delete(views, folder)
	} else {
		if views == nil {
			views = map[string]consoleView{}
		}
		views[folder] = consoleView{
			Server:       view.Server,
			FolderKey:    view.FolderKey,
			FolderLabel:  view.FolderLabel,
			FolderSource: view.FolderSource,
			FolderLocal:  view.FolderLocal,
			Tags:         view.Tags,
			At:           time.Now().UTC(),
		}
	}
	trimConsoleViews(views)
	return writeStateFile(consoleViewsPath(), views)
}

// tags are the view's tags, reading a file written when it held one.
func (v consoleView) tags() []string {
	if len(v.Tags) == 0 && v.Tag != "" {
		return []string{v.Tag}
	}
	return v.Tags
}

func trimConsoleViews(views map[string]consoleView) {
	if len(views) <= consoleViewLimit {
		return
	}
	keys := make([]string, 0, len(views))
	for key := range views {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return views[keys[j]].At.Before(views[keys[i]].At) })
	for _, key := range keys[consoleViewLimit:] {
		delete(views, key)
	}
}
