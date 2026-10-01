package tui

import (
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func taggedSandboxes() []Sandbox {
	boxes := testSandboxes()
	boxes[0].Tags = []string{"ticket=ENG-12", "wip"}
	boxes[1].Tags = []string{"wip"}
	return boxes
}

func headerLine(m *Model) string { return ansi.Strip(m.viewHeaderLeft()) }

// tagRows are the tags the filter card offers, as it reads them.
func tagRows(m *Model) []string {
	var out []string
	for _, row := range m.filterDialog().filter.rows() {
		if row.group == "Tag" {
			out = append(out, row.label)
		}
	}
	return out
}

// A project nobody tags offers no tags: a group that can only say "all tags"
// is a group spent saying nothing.
func TestThereAreNoTagsToPickUntilSomethingIsTagged(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, newFakeSource(testSandboxes()...))
	if got := tagRows(m); len(got) != 0 {
		t.Fatalf("the card offers tags %q with nothing tagged", got)
	}
}

// Once a discobox is tagged the card offers every tag after the folders, each
// with how many boxes carry it, and marking one narrows the list to the boxes
// carrying it.
func TestATagNarrowsTheList(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, newFakeSource(taggedSandboxes()...))
	filterTo(t, m, "Folder: "+m.session.folder().label)
	if got := tagRows(m); len(got) != 3 || got[0] != allTags || got[1] != "#ticket=ENG-12" || got[2] != "#wip" {
		t.Fatalf("tags = %q, want every box then each tag in order", got)
	}
	view := m.filterDialog().view(m.st, &m.zones, 120, 40)
	for _, want := range []string{allTags, "#ticket=ENG-12", "#wip", "2 boxes", "ticket set to this value"} {
		if !strings.Contains(view, want) {
			t.Errorf("the card is missing %q:\n%s", want, view)
		}
	}
	rows := func() []string {
		var ids []string
		for _, s := range m.list.rows() {
			ids = append(ids, s.ID)
		}
		return ids
	}
	if got := rows(); len(got) != 2 {
		t.Fatalf("rows = %v, want both of this folder's boxes", got)
	}

	filterTo(t, m, "#wip")
	if got := rows(); !slices.Equal(m.list.tags, []string{"wip"}) || len(got) != 2 {
		t.Fatalf("tags %q list %v, want both boxes tagged wip", m.list.tags, got)
	}
	filterTo(t, m, "#ticket=ENG-12")
	if !slices.Equal(m.list.tags, []string{"ticket=ENG-12", "wip"}) || !strings.Contains(headerLine(m), "#ticket=ENG-12 #wip") {
		t.Fatalf("tags = %q, header = %q, want both tags", m.list.tags, headerLine(m))
	}
	if got := rows(); len(got) != 1 || got[0] != "sbx_one" {
		t.Fatalf("rows = %v, want only the box carrying both tags", got)
	}
	filterTo(t, m, allTags)
	if len(m.list.tags) != 0 || strings.Contains(headerLine(m), "#") {
		t.Fatalf("tags = %q, header = %q, want every box and no tag named", m.list.tags, headerLine(m))
	}
}

// A tag that goes while the filter is on it stays the filter's choice rather
// than vanishing from under it, so what the header says is still what is shown.
func TestAChosenTagOutlivesItsLastBox(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, newFakeSource(taggedSandboxes()...))
	m.list.tags = []string{"wip"}
	untagged := testSandboxes()
	m.list.setAll(untagged)
	if got := tagRows(m); !slices.Contains(got, "#wip") || !strings.Contains(headerLine(m), "#wip") {
		t.Fatalf("header = %q, tags = %q, want the filter still naming #wip", headerLine(m), got)
	}
	if len(m.list.rows()) != 0 {
		t.Fatalf("rows = %d, want none: nothing carries the tag now", len(m.list.rows()))
	}
}

// Space turns a tag on and off again, so several can be marked; Enter only
// ever turns the row it is on on, so the key that applies the card never drops
// the tag under the cursor.
func TestSpaceTogglesTagsAndEnterOnlyMarks(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, newFakeSource(taggedSandboxes()...))
	m.dialog = m.filterDialog()
	p := m.dialog.filter
	marked := func() []string { return p.list.tags }

	markFilter(t, m, "#wip")
	markFilter(t, m, "#ticket=ENG-12")
	if got := marked(); !slices.Equal(got, []string{"ticket=ENG-12", "wip"}) {
		t.Fatalf("marked %q, want both tags", got)
	}
	if got := dialogText(m); !strings.Contains(got, "■ #wip") || !strings.Contains(got, "□ "+allTags) {
		t.Fatalf("the tags are not drawn as boxes, marked:\n%s", got)
	}
	markFilter(t, m, "#wip")
	if got := marked(); !slices.Equal(got, []string{"ticket=ENG-12"}) {
		t.Fatalf("marked %q after Space on a marked tag, want it let go", got)
	}
	markFilter(t, m, "#ticket=ENG-12")
	if got := marked(); len(got) != 0 {
		t.Fatalf("marked %q, want none", got)
	}

	markFilter(t, m, "#wip")
	send(t, m, keyPress("enter"))
	if !slices.Equal(m.list.tags, []string{"wip"}) {
		t.Fatalf("tags = %q after Enter on a marked tag, want it kept", m.list.tags)
	}

	m.dialog = m.filterDialog()
	markFilter(t, m, "#ticket=ENG-12")
	markFilter(t, m, allTags)
	if got := m.dialog.filter.list.tags; len(got) != 0 {
		t.Fatalf("marked %q after all tags, want none", got)
	}
}

// A discobox has one value for a key, so marking another value of a key
// already marked takes its place rather than narrowing to what nothing carries.
func TestATagReplacesAnotherValueOfItsKey(t *testing.T) {
	t.Parallel()
	var l sandboxList
	l.addTag("wip")
	l.addTag("ticket=ENG-12")
	l.addTag("ticket=ENG-13")
	if want := []string{"ticket=ENG-13", "wip"}; !slices.Equal(l.tags, want) {
		t.Fatalf("tags = %q, want %q", l.tags, want)
	}
	l.addTag("ticket")
	if want := []string{"ticket", "wip"}; !slices.Equal(l.tags, want) {
		t.Fatalf("tags = %q, want a bare key in place of its value: %q", l.tags, want)
	}
}
