package tui

import (
	"sort"
	"testing"
)

// A term matches as a subsequence, and lights the tightest window it can at
// its first match; every term must match; a capital makes the query exact.
func TestFuzzyQueryMatchesTheWayFzfDoes(t *testing.T) {
	t.Parallel()
	positions := func(hits map[int]bool) []int {
		var out []int
		for i := range hits {
			out = append(out, i)
		}
		sort.Ints(out)
		return out
	}
	for _, tc := range []struct {
		query, text string
		ok          bool
		lit         []int
	}{
		// The first match, pulled tight: g of GET would start it, but the
		// window ends at github's h and is walked back to github's g.
		{"gith", "GET 200 https://api.github.com", true, []int{20, 21, 22, 23}},
		// First, not best: fzf's first algorithm, since the timeline is not
		// ranked. G of GET and h of https.
		{"gh", "GET 200 https://api.github.com", true, []int{0, 8}},
		{"get gith", "GET 200 https://api.github.com", true, []int{0, 1, 2, 20, 21, 22, 23}},
		{"get gitlab", "GET 200 https://api.github.com", false, nil},
		{"GET", "get 200", false, nil},
		{"GET", "GET 200", true, []int{0, 1, 2}},
		{"post", "POST 201 https://x", true, []int{0, 1, 2, 3}},
		// The window is pulled tight: "ab" in "a_xab" lights the second a.
		{"ab", "a_xab", true, []int{3, 4}},
		{"", "anything", true, nil},
	} {
		hits, ok := parseFuzzy(tc.query).match(tc.text)
		if ok != tc.ok {
			t.Errorf("%q in %q: ok = %v, want %v", tc.query, tc.text, ok, tc.ok)
			continue
		}
		if got := positions(hits); ok && len(got)+len(tc.lit) > 0 && !equalInts(got, tc.lit) {
			t.Errorf("%q in %q: lit %v, want %v", tc.query, tc.text, got, tc.lit)
		}
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
