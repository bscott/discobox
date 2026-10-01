//go:build !linux

package termguard

// killSenderHint is empty where there is no tracing a user can be pointed at
// without first turning off what the system protects itself with.
const killSenderHint = ""

// oomKills has nothing to count outside Linux's cgroups.
func oomKills() (string, uint64, bool) {
	return "", 0, false
}
