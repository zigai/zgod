package match

import "github.com/bmatcuk/doublestar/v4"

type GlobMatcher struct{}

func (m *GlobMatcher) Match(pattern string, candidates []string) []Match {
	if pattern == "" {
		return nil
	}
	var matches []Match
	for i, c := range candidates {
		ok, _ := doublestar.Match(pattern, c)
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
