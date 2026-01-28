package match

import "regexp"

type RegexMatcher struct{}

func (m *RegexMatcher) Match(pattern string, candidates []string) []Match {
	if pattern == "" {
		return nil
	}
	re, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		return nil
	}

	var matches []Match
	for i, c := range candidates {
		locs := re.FindAllStringIndex(c, -1)
		if locs == nil {
			continue
		}
		ranges := make([]Range, len(locs))
		for j, loc := range locs {
			ranges[j] = Range{Start: loc[0], End: loc[1]}
		}
		matches = append(matches, Match{
			Index:         i,
			Score:         100,
			MatchedRanges: ranges,
		})
	}
	return matches
}
