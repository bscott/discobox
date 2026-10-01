package tui

import "slices"

// The tag is the narrowest of the header filter's three (filter.go), after the
// server and the folder: it narrows the list to the discoboxes carrying one
// tag. A discobox's tags are its own, in the meta file inside it, and the rows
// show the copy the server last heard (ADR 0136). The filter matches the tag as the row spells it —
// `wip`, or `ticket=ENG-12` — so what you pick is what you see after the name.
//
// It is only offered once there is a tag to pick: a project nobody tags has no
// use for a choice that can only say "all tags". It is not offered over the
// harnesses and secrets screens, which list no discoboxes.

// allTags is the choice that is not a tag: every discobox, tagged or not.
const allTags = "all tags"

// tagged reports whether a discobox carries the tag the list is filtered to.
// Every discobox carries the empty one.
func (l *sandboxList) tagged(s Sandbox) bool {
	return l.tag == "" || slices.Contains(s.Tags, l.tag)
}

// tags are what the filter can narrow to: every tag the discoboxes inside the
// other two filters carry, in order, and the one it is on when none of them
// carries it any longer — the way the folders keep the one chosen, so the
// choice does not vanish from under the list it is showing.
func (l *sandboxList) tags() []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range l.all {
		if !l.onServer(s) || !l.folder.holds(s, l.session) {
			continue
		}
		if !l.showArchived && s.State == StateArchived {
			continue
		}
		for _, tag := range s.Tags {
			if !seen[tag] {
				seen[tag] = true
				out = append(out, tag)
			}
		}
	}
	slices.Sort(out)
	if l.tag != "" && !seen[l.tag] {
		out = append(out, l.tag)
	}
	return out
}

// tagLabel is how a tag reads in the header and on the filter's card: the tag
// as the rows draw it, so the eye matches the one to the other.
func tagLabel(tag string) string {
	if tag == "" {
		return allTags
	}
	return "#" + tag
}
