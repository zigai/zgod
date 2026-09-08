package match

import (
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
