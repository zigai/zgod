package match

import (
	"sort"

	"github.com/sahilm/fuzzy"
)

type FuzzyMatcher struct{}

func (m *FuzzyMatcher) Match(pattern string, candidates []string) []Match {
	results := fuzzy.Find(pattern, candidates)
	sort.Stable(results)

	matches := make([]Match, len(results))
	for i, r := range results {
		ranges := make([]Range, 0)

		if len(r.MatchedIndexes) > 0 {
			start := r.MatchedIndexes[0]

			end := start + 1
			for _, idx := range r.MatchedIndexes[1:] {
				if idx == end {
					end++
				} else {
					ranges = append(ranges, Range{Start: start, End: end})
					start = idx
					end = idx + 1
				}
			}

			ranges = append(ranges, Range{Start: start, End: end})
		}

		matches[i] = Match{
			Index:         r.Index,
			Score:         r.Score,
			MatchedRanges: ranges,
		}
	}

	return matches
}
