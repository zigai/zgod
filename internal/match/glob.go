package match

import (
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

type GlobMatcher struct{}

func (m *GlobMatcher) Match(pattern string, candidates []string) []Match {
	return m.MatchInto(pattern, candidates, nil)
}

func (m *GlobMatcher) MatchInto(pattern string, candidates []string, dst []Match) []Match {
	if pattern == "" {
		return nil
	}

	if !doublestar.ValidatePattern(pattern) {
		return nil
	}

	if isSimpleStarGlob(pattern) {
		return matchSimpleStarGlob(pattern, candidates, dst)
	}

	matches := dst[:0]
	if dst != nil && cap(matches) < len(candidates) {
		matches = make([]Match, 0, len(candidates))
	}

	for i, c := range candidates {
		ok := doublestar.MatchUnvalidated(pattern, c)
		if !ok {
			continue
		}

		matches = append(matches, Match{
			Index:         i,
			Score:         1,
			MatchedRanges: nil,
		})
	}

	return matches
}

func isSimpleStarGlob(pattern string) bool {
	if pattern == "" || strings.Contains(pattern, "/") {
		return false
	}

	for i := range len(pattern) {
		switch pattern[i] {
		case '*':
		case '?', '[', ']', '{', '}', '\\':
			return false
		}
	}

	return strings.Contains(pattern, "*")
}

func matchSimpleStarGlob(pattern string, candidates []string, dst []Match) []Match {
	var parts []string

	matches := dst[:0]
	if dst != nil && cap(matches) < len(candidates) {
		matches = make([]Match, 0, len(candidates))
	}

	for i, c := range candidates {
		if strings.Contains(c, "/") {
			ok := doublestar.MatchUnvalidated(pattern, c)
			if ok {
				matches = append(matches, Match{Index: i, Score: 1, MatchedRanges: nil})
			}

			continue
		}

		if parts == nil {
			parts = strings.FieldsFunc(pattern, func(r rune) bool { return r == '*' })
		}

		if simpleStarGlobMatch(pattern, parts, c) {
			matches = append(matches, Match{Index: i, Score: 1, MatchedRanges: nil})
		}
	}

	return matches
}

func simpleStarGlobMatch(pattern string, parts []string, candidate string) bool {
	if len(parts) == 0 {
		return true
	}

	pos := 0

	if !strings.HasPrefix(pattern, "*") {
		first := parts[0]
		if !strings.HasPrefix(candidate, first) {
			return false
		}

		pos = len(first)
		parts = parts[1:]
	}

	for len(parts) > 0 {
		part := parts[0]
		parts = parts[1:]

		idx := strings.Index(candidate[pos:], part)
		if idx < 0 {
			return false
		}

		pos += idx + len(part)
	}

	if !strings.HasSuffix(pattern, "*") && pos != len(candidate) {
		return false
	}

	return true
}
