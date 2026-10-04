package termguard

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestServeLeavesAFinishedProgramAlone(t *testing.T) {
	var out, ready bytes.Buffer
	report := writeReport(t, "")
	if err := serve(terminal{out: &out}, strings.NewReader(string(finished)), &ready, report, 42, noOOMKiller); err != nil {
		t.Fatal(err)
	}
	if ready.String() != "ready\n" {
		t.Fatalf("ready = %q", ready.String())
	}
	if out.Len() != 0 {
		t.Fatalf("a program that finished had its terminal written to: %q", out.String())
	}
}

func TestServeRestoresAndNamesTheReport(t *testing.T) {
	var out bytes.Buffer
	report := writeReport(t, "panic: boom\n\ngoroutine 7 [running]:\nmain.main()\n")
	// A lifeline that closes without the finished byte: the program died.
	if err := serve(terminal{out: &out}, strings.NewReader(""), io.Discard, report, 42, noOOMKiller); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.HasPrefix(got, resetSequence) {
		t.Fatalf("want the reset first, got %q", got)
	}
	for _, want := range []string{"\x1b[?1003l", "\x1b[?1049l", "\x1b[?2026l", "\x1b[?25h", "crashed", report} {
		if !strings.Contains(got, want) {
			t.Errorf("output is missing %q: %q", want, got)
		}
	}
	if strings.Contains(got, "boom") {
		t.Errorf("the report was printed rather than named: %q", got)
	}
	if _, err := os.Stat(report); err != nil {
		t.Errorf("the report was not kept: %v", err)
	}
}

func TestServeSaysWhenThereIsNoReport(t *testing.T) {
	var out bytes.Buffer
	report := writeReport(t, "")
	if err := serve(terminal{out: &out}, strings.NewReader(""), io.Discard, report, 42, noOOMKiller); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.HasPrefix(got, resetSequence) || !strings.Contains(got, "(pid 42) was killed without warning") {
		t.Fatalf("got %q", got)
	}
	if !strings.Contains(got, killSenderHint) {
		t.Errorf("output is missing how to catch the sender: %q", got)
	}
	if _, err := os.Stat(report); !os.IsNotExist(err) {
		t.Errorf("an empty report was kept: %v", err)
	}
}

func TestServeSaysWhatTheProgramDiedOf(t *testing.T) {
	var out bytes.Buffer
	report := writeReport(t, "")
	if err := serve(terminal{out: &out}, strings.NewReader("SIGHUP"), io.Discard, report, 42, noOOMKiller); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); !strings.Contains(got, "(pid 42) was ended by SIGHUP;") || strings.Contains(got, "without warning") {
		t.Fatalf("got %q", got)
	}
}

func TestServeBlamesTheOOMKiller(t *testing.T) {
	var out bytes.Buffer
	report := writeReport(t, "")
	// The count rises between the guard starting and the program dying.
	kills := uint64(3)
	oom := func() (string, uint64, bool) {
		kills++
		return "/user.slice/term.scope", kills, true
	}
	if err := serve(terminal{out: &out}, strings.NewReader(""), io.Discard, report, 42, oom); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); !strings.Contains(got, "(pid 42) was most likely killed by the kernel's out-of-memory killer") || !strings.Contains(got, "/user.slice/term.scope") {
		t.Fatalf("got %q", got)
	}
}

func TestServeDoesNotBlameAnEarlierOOMKill(t *testing.T) {
	defer func(d time.Duration) { oomPollInterval = d }(oomPollInterval)
	oomPollInterval = time.Millisecond
	var out bytes.Buffer
	report := writeReport(t, "")
	// Another pane in the cgroup is killed long before the console dies: the
	// count rises once, and a poll sees it.
	var mu sync.Mutex
	var reads int
	polled := make(chan struct{})
	oom := func() (string, uint64, bool) {
		mu.Lock()
		defer mu.Unlock()
		reads++
		if reads == 3 {
			close(polled)
		}
		if reads == 1 {
			return "/tmux.scope", 3, true
		}
		return "/tmux.scope", 4, true
	}
	lifeline, program := io.Pipe()
	go func() {
		<-polled
		_ = program.Close()
	}()
	if err := serve(terminal{out: &out}, lifeline, io.Discard, report, 42, oom); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); strings.Contains(got, "out-of-memory") || !strings.Contains(got, "SIGKILL") {
		t.Fatalf("got %q", got)
	}
}

func TestServeBlamesAnOOMKillPolledBeforeTheLifelineCloses(t *testing.T) {
	defer func(d time.Duration) { oomPollInterval = d }(oomPollInterval)
	oomPollInterval = 20 * time.Millisecond
	var out bytes.Buffer
	report := writeReport(t, "")
	// The kernel counts the console's kill, a poll sees it, and only then does
	// the dying console let go of the lifeline. Later reads count one more,
	// so a poll that slips in before the guard hears the lifeline close does
	// not move the baseline past the kill.
	lifeline, program := io.Pipe()
	var mu sync.Mutex
	var reads int
	oom := func() (string, uint64, bool) {
		mu.Lock()
		defer mu.Unlock()
		reads++
		switch reads {
		case 1:
			return "/term.scope", 3, true
		case 2:
			_ = program.Close()
			return "/term.scope", 4, true
		default:
			return "/term.scope", 5, true
		}
	}
	if err := serve(terminal{out: &out}, lifeline, io.Discard, report, 42, oom); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); !strings.Contains(got, "out-of-memory killer") {
		t.Fatalf("got %q", got)
	}
}

func TestServeClaimsNoCauseItCouldNotRuleOut(t *testing.T) {
	var out bytes.Buffer
	report := writeReport(t, "")
	uncounted := func() (string, uint64, bool) { return "", 0, false }
	if err := serve(terminal{out: &out}, strings.NewReader(""), io.Discard, report, 42, uncounted); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "(pid 42) was killed without warning: by the system") || strings.Contains(got, "SIGKILL") {
		t.Fatalf("got %q", got)
	}
}

func noOOMKiller() (string, uint64, bool) { return "/term.scope", 7, true }

func writeReport(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "console-test.log")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
