package match

import (
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

type RegexMatcher struct{}

const (
	regexMatchScore             = 100
	defaultLiteralMatchCapacity = 64
)

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

		if isASCII(c) {
			for j, loc := range locs {
				ranges[j] = Range{Start: loc[0], End: loc[1]}
			}
		} else {
			runeStarts := buildRuneByteOffsets(c)
			for j, loc := range locs {
				ranges[j] = Range{
					Start: byteOffsetToRuneIndex(runeStarts, loc[0]),
					End:   byteOffsetToRuneIndex(runeStarts, loc[1]),
				}
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

func (m *RegexMatcher) MatchCommands(pattern string, candidates []string, dst []Match) []Match {
	if pattern == "" {
		return nil
	}

	if IsLiteralRegex(pattern) {
		return MatchLiteralCommandsFold(pattern, candidates, dst)
	}

	re, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		return nil
	}

	return MatchRegexCommands(re, candidates, dst)
}

func IsLiteralRegex(pattern string) bool {
	if pattern == "" {
		return false
	}

	for i := range len(pattern) {
		switch pattern[i] {
		case '.', '+', '*', '?', '(', ')', '|', '[', ']', '{', '}', '^', '$', '\\':
			return false
		}
	}

	return true
}

func MatchLiteralCommandsFold(pattern string, candidates []string, dst []Match) []Match {
	matches := dst[:0]
	if cap(matches) == 0 {
		matches = make([]Match, 0, min(len(candidates), defaultLiteralMatchCapacity))
	}

	patternASCII := isASCII(pattern)
	lowerPattern := strings.ToLower(pattern)

	for i, c := range candidates {
		var ok bool

		if patternASCII {
			var ascii bool

			ok, ascii = containsFoldASCII(c, lowerPattern)
			if !ascii {
				ok = strings.Contains(strings.ToLower(c), lowerPattern)
			}
		} else {
			ok = strings.Contains(strings.ToLower(c), lowerPattern)
		}

		if !ok {
			continue
		}

		matches = append(matches, Match{
			Index:         i,
			Score:         regexMatchScore,
			MatchedRanges: nil,
		})
	}

	return matches
}

func containsFoldASCII(s string, lowerSubstr string) (bool, bool) {
	if lowerSubstr == "" {
		return true, true
	}

	first := lowerSubstr[0]

	limit := len(s) - len(lowerSubstr)
	for i := 0; i <= limit; i++ {
		b := s[i]
		if b >= utf8.RuneSelf {
			return false, false
		}

		if toLowerASCII(b) != first {
			continue
		}

		matched := true

		for j := 1; j < len(lowerSubstr); j++ {
			b = s[i+j]
			if b >= utf8.RuneSelf {
				return false, false
			}

			if toLowerASCII(b) != lowerSubstr[j] {
				matched = false
				break
			}
		}

		if matched {
			return true, true
		}
	}

	for i := max(limit+1, 0); i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false, false
		}
	}

	return false, true
}

func toLowerASCII(b byte) byte {
	if 'A' <= b && b <= 'Z' {
		return b + ('a' - 'A')
	}

	return b
}

func MatchRegexCommands(re *regexp.Regexp, candidates []string, dst []Match) []Match {
	matches := dst[:0]
	if cap(matches) < len(candidates) {
		matches = make([]Match, 0, len(candidates))
	}

	for i, c := range candidates {
		if !re.MatchString(c) {
			continue
		}

		matches = append(matches, Match{
			Index:         i,
			Score:         regexMatchScore,
			MatchedRanges: nil,
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
