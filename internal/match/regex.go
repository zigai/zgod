package match

import (
	"regexp"
	"sort"
)

type RegexMatcher struct{}

const regexMatchScore = 100

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

		runeStarts := buildRuneByteOffsets(c)
		for j, loc := range locs {
			ranges[j] = Range{
				Start: byteOffsetToRuneIndex(runeStarts, loc[0]),
				End:   byteOffsetToRuneIndex(runeStarts, loc[1]),
			}
		}

		matches = append(matches, Match{
			Index:         i,
			Score:         regexMatchScore,
			MatchedRanges: ranges,
		})
	}

	return matches
}

func buildRuneByteOffsets(s string) []int {
	offsets := make([]int, 0, len([]rune(s))+1)
	for i := range s {
		offsets = append(offsets, i)
	}

	offsets = append(offsets, len(s))

	return offsets
}

func byteOffsetToRuneIndex(offsets []int, byteOffset int) int {
	return sort.SearchInts(offsets, byteOffset)
}
