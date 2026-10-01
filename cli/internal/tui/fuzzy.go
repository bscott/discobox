package tui

import (
	"strings"
	"unicode"
)

// fuzzyQuery is a filter read the way fzf reads one: each word of it is a
// term that must appear in the text in order but not necessarily together —
// "gh200" finds "GET 200 https://api.github.com" — and every term must appear.
// It is smart-case: lower case matches either case, and a query with a capital
// in it matches case exactly.
type fuzzyQuery struct {
	terms         [][]rune
	caseSensitive bool
}

func parseFuzzy(query string) fuzzyQuery {
	q := fuzzyQuery{caseSensitive: strings.IndexFunc(query, unicode.IsUpper) >= 0}
	for _, term := range strings.Fields(query) {
		q.terms = append(q.terms, []rune(term))
	}
	return q
}

func (q fuzzyQuery) empty() bool { return len(q.terms) == 0 }

// match reports whether text holds every term, and which of its runes matched
// them, by rune index. An empty query matches everything and marks nothing.
func (q fuzzyQuery) match(text string) (hits map[int]bool, ok bool) {
	if q.empty() {
		return nil, true
	}
	runes := []rune(text)
	hits = map[int]bool{}
	for _, term := range q.terms {
		if !q.matchTerm(runes, term, hits) {
			return nil, false
		}
	}
	return hits, true
}

// matchTerm finds one term in text, marking where. It is fzf's first
// algorithm: the earliest place the term can end, then back from there to the
// latest place it can start — the tightest window at the first match, so what
// is lit reads as the thing that was found rather than letters strewn along
// the line.
func (q fuzzyQuery) matchTerm(text, term []rune, hits map[int]bool) bool {
	end, t := -1, 0
	for i, r := range text {
		if q.same(r, term[t]) {
			t++
			if t == len(term) {
				end = i
				break
			}
		}
	}
	if end < 0 {
		return false
	}
	t = len(term) - 1
	for i := end; i >= 0 && t >= 0; i-- {
		if q.same(text[i], term[t]) {
			hits[i] = true
			t--
		}
	}
	return true
}

func (q fuzzyQuery) same(a, b rune) bool {
	if q.caseSensitive {
		return a == b
	}
	return unicode.ToLower(a) == unicode.ToLower(b)
}
