package termpane

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// remapPort is a host's answer for a pane whose 8080 is reachable here as 8081,
// and whose bind address is not an address at all.
func remapPort(url string) string {
	for _, from := range []string{"http://localhost:8080", "http://0.0.0.0:8080", "http://127.0.0.1:8080"} {
		if strings.HasPrefix(url, from) {
			return "http://localhost:8081" + url[len(from):]
		}
	}
	return url
}

// A link the application made itself keeps its text and loses its target: the
// application knows what it is offering, and the host knows where that is.
func TestRewriteLinksRetargetsTheApplicationsOwnLinks(t *testing.T) {
	row := "open " + ansi.SetHyperlink("http://localhost:8080/admin") + "the console" + ansi.ResetHyperlink()

	got := rewriteLinks(row, remapPort)

	if want := ansi.SetHyperlink("http://localhost:8081/admin"); !strings.Contains(got, want) {
		t.Errorf("rewriteLinks = %q, want it to open %q", got, want)
	}
	if strings.Contains(got, "localhost:8080") {
		t.Errorf("rewriteLinks = %q, want the old target gone", got)
	}
	if got, want := ansi.Strip(got), "open the console"; got != want {
		t.Errorf("text = %q, want %q — the application's text is not the host's", got, want)
	}
}

// A URL in plain output becomes a link when the host moves it. The text still
// says what the program printed, because that is what the program printed.
func TestRewriteLinksLinksTextTheHostMoves(t *testing.T) {
	row := "listening on http://0.0.0.0:8080/ ..."

	got := rewriteLinks(row, remapPort)

	want := "listening on " + ansi.SetHyperlink("http://localhost:8081/") + "http://0.0.0.0:8080/" + ansi.ResetHyperlink() + " ..."
	if got != want {
		t.Errorf("rewriteLinks = %q, want %q", got, want)
	}
}

// A URL the host does not move is left as text. The terminal drawing the pane
// does its own detection on it, and a link that says the same thing would
// replace a working one with a copy.
func TestRewriteLinksLeavesTextItDoesNotMoveAlone(t *testing.T) {
	for _, row := range []string{
		"see https://github.com/discobox-ai/discobox for the rest",
		"listening on http://localhost:9999/",
		"nothing here at all",
	} {
		if got := rewriteLinks(row, remapPort); got != row {
			t.Errorf("rewriteLinks(%q) = %q, want it untouched", row, got)
		}
	}
}

// Text the application has already linked is not linked again: its target was
// rewritten where the link is, and a second link over the same cells would be
// two answers to one question.
func TestRewriteLinksDoesNotLinkInsideALink(t *testing.T) {
	row := ansi.SetHyperlink("http://localhost:8080/docs") + "http://localhost:8080/other" + ansi.ResetHyperlink()

	got := rewriteLinks(row, remapPort)

	if n := strings.Count(got, oscHyperlink); n != 2 {
		t.Errorf("rewriteLinks = %q, want one link opened and one closed, got %d sequences", got, n)
	}
	if want := ansi.SetHyperlink("http://localhost:8081/docs"); !strings.Contains(got, want) {
		t.Errorf("rewriteLinks = %q, want it to open %q", got, want)
	}
}

// A URL the application colored is still one URL. The style changes inside it
// are cells of the row, not breaks in the text.
func TestRewriteLinksSurvivesAStyleInsideTheURL(t *testing.T) {
	row := "up at http://localhost:\x1b[1m8080\x1b[0m/health"

	got := rewriteLinks(row, remapPort)

	if want := ansi.SetHyperlink("http://localhost:8081/health"); !strings.Contains(got, want) {
		t.Errorf("rewriteLinks = %q, want it to link %q", got, want)
	}
	if !strings.Contains(got, "\x1b[1m8080\x1b[0m") {
		t.Errorf("rewriteLinks = %q, want the application's style kept", got)
	}
}

// The punctuation a URL picked up from the sentence around it is not part of
// the URL; the brackets of an IPv6 authority are.
func TestRewriteLinksTrimsTheSentenceAndNotTheAddress(t *testing.T) {
	moved := ""
	rewrite := func(url string) string {
		moved = url
		return "http://localhost:8081/"
	}

	rewriteLinks("try http://localhost:8080/, then stop.", rewrite)
	if want := "http://localhost:8080/"; moved != want {
		t.Errorf("rewrote %q, want %q", moved, want)
	}

	rewriteLinks("try http://[::1]:8080 (loopback)", rewrite)
	if want := "http://[::1]:8080"; moved != want {
		t.Errorf("rewrote %q, want %q", moved, want)
	}
}

// The sequences take no cells, so a row that gained a link still measures as
// its text — the grid is exactly as wide as it was.
func TestRewriteLinksAddsNoCells(t *testing.T) {
	row := "listening on http://localhost:8080/"

	got := rewriteLinks(row, remapPort)

	if w, want := ansi.StringWidth(got), ansi.StringWidth(row); w != want {
		t.Fatalf("width = %d, want %d", w, want)
	}
	if text, want := ansi.Strip(got), row; text != want {
		t.Fatalf("text = %q, want %q", text, want)
	}
}

// Without a rewriter the pane is what it was: nothing is detected and nothing
// is retargeted.
func TestRewriteLinksIsInertWithoutARewriter(t *testing.T) {
	row := "listening on http://localhost:8080/"
	if got := rewriteLinks(row, nil); got != row {
		t.Errorf("rewriteLinks = %q, want it untouched", got)
	}
}

// End to end: what the far end prints, the pane draws with the host's link on
// it — through the scrollback too, which is where output goes to be read.
func TestPaneLinksWhatTheFarEndPrints(t *testing.T) {
	m, stream, cmd := attach(t, 40, 3, WithLinkRewrite(remapPort))

	stream.send("serving http://localhost:8080/\r\n")
	for range 4 {
		stream.send("filler\r\n")
	}
	cmd = pump(t, m, cmd, "filler")
	_ = cmd

	m.Scroll(3)
	rows := strings.Join(m.View(), "\n")
	// Its target, whatever id the pane names it by.
	if want := ";http://localhost:8081/\a"; !strings.Contains(rows, want) {
		t.Errorf("view = %q, want a link to %q", rows, want)
	}
	if !strings.Contains(ansi.Strip(rows), "serving http://localhost:8080/") {
		t.Errorf("view = %q, want the text the far end printed", ansi.Strip(rows))
	}
}

// linkIDs is the id each OSC 8 open in a row names, in order.
func linkIDs(row string) []string {
	var ids []string
	tokens, _, _ := scanRow(row)
	for _, token := range tokens {
		uri, params, _, ok := parseHyperlink(token.s)
		if !ok || uri == "" {
			continue
		}
		id := ""
		for param := range strings.SplitSeq(params, ":") {
			if v, found := strings.CutPrefix(param, "id="); found {
				id = v
			}
		}
		ids = append(ids, id)
	}
	return ids
}

func TestPaneGivesAWrappedLinkOneIDAcrossItsRows(t *testing.T) {
	m, stream, cmd := attach(t, 20, 5)
	url := "https://example.com/a/very/long/path/that/wraps"
	stream.send("see " + ansi.SetHyperlink(url) + url + ansi.ResetHyperlink() + "\r\nEND")
	_ = pump(t, m, cmd, "END")

	rows := m.View()
	var ids []string
	for _, row := range rows[:3] {
		got := linkIDs(row)
		if len(got) != 1 || got[0] == "" {
			t.Fatalf("row %q: link ids = %q, want one id", row, got)
		}
		ids = append(ids, got[0])
	}
	if ids[0] != ids[1] || ids[1] != ids[2] {
		t.Errorf("ids = %q, want one id across every row of the wrapped link", ids)
	}
	if !strings.Contains(rows[1], ";"+url) {
		t.Errorf("row %q does not point at %q", rows[1], url)
	}
}

func TestPaneNamesALinkForItsTargetAndItsPane(t *testing.T) {
	m, stream, cmd := attach(t, 40, 4)
	link := func(url string) string { return ansi.SetHyperlink(url) + "here" + ansi.ResetHyperlink() }
	stream.send(link("https://example.com/") + "\r\n" + link("https://example.com/") + "\r\n" + link("https://example.org/") + "\r\nEND")
	_ = pump(t, m, cmd, "END")

	rows := m.View()
	first, again, other := linkIDs(rows[0]), linkIDs(rows[1]), linkIDs(rows[2])
	if len(first) != 1 || first[0] == "" {
		t.Fatalf("row %q: link ids = %q, want one id", rows[0], first)
	}
	if len(again) != 1 || again[0] != first[0] {
		t.Errorf("ids = %q and %q, want one id for one target", first, again)
	}
	if len(other) != 1 || other[0] == first[0] {
		t.Errorf("id for another target = %q, want one that is not %q", other, first)
	}

	// Another pane showing the same thing names its links for itself.
	pane, paneStream, paneCmd := attach(t, 40, 4)
	paneStream.send(link("https://example.com/") + "\r\nEND")
	_ = pump(t, pane, paneCmd, "END")
	if ids := linkIDs(pane.View()[0]); len(ids) != 1 || ids[0] == first[0] {
		t.Errorf("another pane's id = %q, want one that is not %q", ids, first[0])
	}
}

func TestPaneLinkIDHoldsStillAsOutputScrollsIt(t *testing.T) {
	// Three rows and two lines of scrollback: the lines below push the link
	// into the scrollback and drop the two above it out of the far end, so
	// the link is at a different row of the view and of the kept history.
	m, stream, cmd := attach(t, 40, 3, WithScrollback(2))
	stream.send("a\r\nb\r\n" + ansi.SetHyperlink("https://example.com/") + "here" + ansi.ResetHyperlink() + "\r\nEND")
	cmd = pump(t, m, cmd, "END")
	before := linkIDs(m.View()[1])

	stream.send("\r\none\r\ntwo\r\nDONE")
	_ = pump(t, m, cmd, "DONE")
	if got := m.Scroll(2); got != 2 {
		t.Fatalf("scrolled back %d lines, want 2", got)
	}
	after := linkIDs(m.View()[0])
	if len(before) != 1 || len(after) != 1 || before[0] != after[0] {
		t.Errorf("id before scrolling = %q, after = %q, want the same", before, after)
	}
}

func TestPaneKeepsTheApplicationsOwnLinkIDUnderItsName(t *testing.T) {
	link := ansi.SetHyperlink("https://example.com/", "id=mine") + "here" + ansi.ResetHyperlink()
	ids := make([]string, 2)
	for i := range ids {
		m, stream, cmd := attach(t, 40, 3)
		stream.send(link + "\r\nEND")
		_ = pump(t, m, cmd, "END")
		got := linkIDs(m.View()[0])
		if len(got) != 1 || !strings.HasSuffix(got[0], "mine") {
			t.Fatalf("ids = %q, want the application's own %q under the pane's name", got, "mine")
		}
		ids[i] = got[0]
	}
	if ids[0] == ids[1] {
		t.Errorf("two panes both named the application's link %q", ids[0])
	}
}
