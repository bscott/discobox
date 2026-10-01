package termguard

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOOMKillsReadsTheUnifiedCgroup(t *testing.T) {
	dir := t.TempDir()
	proc := filepath.Join(dir, "cgroup")
	root := filepath.Join(dir, "sys")
	write := func(path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(proc, "1:name=systemd:/old\n0::/user.slice/term.scope\n")
	write(filepath.Join(root, "user.slice/term.scope/memory.events"), "low 0\nhigh 0\nmax 2\noom 2\noom_kill 5\noom_group_kill 0\n")

	cgroup, kills, ok := oomKillsIn(proc, root)
	if !ok || cgroup != "/user.slice/term.scope" || kills != 5 {
		t.Fatalf("got %q %d %v", cgroup, kills, ok)
	}

	// No memory controller in this cgroup: nothing to count.
	write(proc, "0::/elsewhere\n")
	if _, _, ok := oomKillsIn(proc, root); ok {
		t.Fatal("counted a cgroup with no memory.events")
	}
}
