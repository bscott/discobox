//go:build linux

package termguard

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// killSenderHint is how to find who sends the next SIGKILL, which nothing
// short of the kernel can see: a tracepoint on every signal generated for a
// process named discobox, printing the sender. It needs root, so the guard
// only prints it.
const killSenderHint = "discobox: to see who sends the next SIGKILL, keep this running as root while the console runs: " +
	`bpftrace -e 'tracepoint:signal:signal_generate /args->sig == 9 && args->comm == "discobox"/ { printf("%s (pid %d) sent SIGKILL to pid %d\n", comm, pid, args->pid); }'` +
	"\r\n"

// oomKills reports this process's cgroup and how many processes the kernel's
// out-of-memory killer has killed in it, or false where cgroup v2 or its
// memory controller is not there to say.
//
// The count is the cgroup's, not the process's, and the cgroup may be a tmux
// server's or an ssh session's, shared with every other pane; serve counts only
// a rise within a poll of the death for that reason.
func oomKills() (string, uint64, bool) {
	return oomKillsIn("/proc/self/cgroup", "/sys/fs/cgroup")
}

func oomKillsIn(procCgroup, root string) (string, uint64, bool) {
	membership, err := os.ReadFile(procCgroup)
	if err != nil {
		return "", 0, false
	}
	var cgroup string
	for line := range strings.Lines(string(membership)) {
		// The unified hierarchy is the one with ID 0 and no controllers.
		if path, ok := strings.CutPrefix(strings.TrimSpace(line), "0::"); ok {
			cgroup = path
		}
	}
	if cgroup == "" {
		return "", 0, false
	}
	events, err := os.Open(filepath.Join(root, cgroup, "memory.events"))
	if err != nil {
		return "", 0, false
	}
	defer events.Close()
	for s := bufio.NewScanner(events); s.Scan(); {
		if count, ok := strings.CutPrefix(s.Text(), "oom_kill "); ok {
			n, err := strconv.ParseUint(count, 10, 64)
			return cgroup, n, err == nil
		}
	}
	return "", 0, false
}
