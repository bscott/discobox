package termguard

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServeLeavesAFinishedProgramAlone(t *testing.T) {
	var out, ready bytes.Buffer
	report := writeReport(t, "")
	if err := serve(terminal{out: &out}, strings.NewReader(string(finished)), &ready, report); err != nil {
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
	if err := serve(terminal{out: &out}, strings.NewReader(""), io.Discard, report); err != nil {
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
	if err := serve(terminal{out: &out}, strings.NewReader(""), io.Discard, report); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out.String(), resetSequence) || !strings.Contains(out.String(), "left no crash report") {
		t.Fatalf("got %q", out.String())
	}
	if _, err := os.Stat(report); !os.IsNotExist(err) {
		t.Errorf("an empty report was kept: %v", err)
	}
}

func writeReport(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "console-test.log")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
