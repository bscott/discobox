package tui

// The folder is the place in the header's filter (filter.go): it says where
// the sandboxes on screen came from, and changing it changes which ones are on
// screen. It stands in for both a folder column — every row on screen shares
// the value, so a column repeating it says nothing — and a key toggling "only
// the ones started here", which would be the same filter with one of its
// choices missing.

// A folder is an origin key (ADR 0111): a machine, and where the discoboxes in
// it had their source from — a directory or a repository URL — or the machine
// alone for the ones with none. It is matched by key and named by its source,
// so one path on two machines is two folders, and a URL is a folder the way a
// directory is.

// folder is one of the filter's choices: the origin key the discoboxes in it
// are filed under, what it reads as, the source a create from it cuts from,
// and whether it is this machine's — in which case this machine's discoboxes
// with no source are in it too, the way `discobox ls` lists them beside the
// directory's own.
type folder struct {
	key    string
	label  string
	source string
	local  bool
}

// allFolders is the choice that is not a place: every sandbox in the project,
// wherever it was started. It is last among the folders rather than first,
// because the folder you are standing in is the one you almost always want.
const allFolders = "all folders"

// everyFolder is allFolders as a choice: no key, so nothing is filtered out.
var everyFolder = folder{label: allFolders}

// holds reports whether a discobox is in the folder: filed under its key, or —
// in a folder of this machine's — under this machine's own, where its
// discoboxes with no source are. Every discobox is in the folder with no key.
func (f folder) holds(s Sandbox, session Session) bool {
	switch {
	case f.key == "":
		return true
	case s.OriginKey == f.key:
		return true
	default:
		return f.local && session.HostKey != "" && s.OriginKey == session.HostKey
	}
}

// folder is the window's own folder: where a discobox cut from its own source
// is filed, named the way the header spells that source — which is the one
// place a branch is shown, since it belongs to the directory the window is
// running in and means nothing next to a folder somewhere else. It is this
// machine's, so its discoboxes with no source are in it.
func (s Session) folder() folder {
	return folder{key: s.OriginKey, label: s.sourceLabel(), source: s.source(), local: true}
}
