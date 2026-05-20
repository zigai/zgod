package match

import (
	"unicode"
	"unicode/utf8"
)

type FuzzyMatcher struct{}

const (
	fuzzyFirstCharMatchBonus            = 10
	fuzzyMatchFollowingSeparatorBonus   = 20
	fuzzyCamelCaseMatchBonus            = 20
	fuzzyAdjacentMatchBonus             = 5
	fuzzyUnmatchedLeadingCharPenalty    = -5
	fuzzyMaxUnmatchedLeadingCharPenalty = -15
)

func (m *FuzzyMatcher) Match(pattern string, candidates []string) []Match {
	return m.MatchInto(pattern, candidates, nil)
}

func (m *FuzzyMatcher) MatchInto(pattern string, candidates []string, dst []Match) []Match {
	if pattern == "" {
		return nil
	}

	patternASCII := isASCII(pattern)

	var (
		patternASCIIBytes []byte
		patternRunes      []rune
	)

	if patternASCII {
		patternASCIIBytes = lowerASCIIBytes(pattern)
	} else {
		patternRunes = []rune(pattern)
	}

	matches := dst[:0]
	if cap(matches) < len(candidates) {
		matches = make([]Match, 0, len(candidates))
	}

	for i, candidate := range candidates {
		var (
			score int
			ok    bool
		)

		if patternASCII {
			var ascii bool

			score, ok, ascii = fuzzyScoreASCII(patternASCIIBytes, candidate)
			if !ascii {
				if patternRunes == nil {
					patternRunes = []rune(pattern)
				}

				score, ok = fuzzyScore(patternRunes, candidate)
			}
		} else {
			score, ok = fuzzyScore(patternRunes, candidate)
		}

		if !ok {
			continue
		}

		matches = append(matches, Match{
			Index:         i,
			Score:         score,
			MatchedRanges: nil,
		})
	}

	return matches
}

func (m *FuzzyMatcher) MatchIndexed(pattern string, candidates []string, indexes []int) []Match {
	return m.MatchIndexedInto(pattern, candidates, indexes, nil)
}

func (m *FuzzyMatcher) MatchIndexedInto(pattern string, candidates []string, indexes []int, dst []Match) []Match {
	if pattern == "" {
		return nil
	}

	patternASCII := isASCII(pattern)

	var (
		patternASCIIBytes []byte
		patternRunes      []rune
	)

	if patternASCII {
		patternASCIIBytes = lowerASCIIBytes(pattern)
	} else {
		patternRunes = []rune(pattern)
	}

	matches := dst[:0]
	if cap(matches) < len(indexes) {
		matches = make([]Match, 0, len(indexes))
	}

	for _, index := range indexes {
		if index < 0 || index >= len(candidates) {
			continue
		}

		var (
			score int
			ok    bool
		)

		if patternASCII {
			var ascii bool

			score, ok, ascii = fuzzyScoreASCII(patternASCIIBytes, candidates[index])
			if !ascii {
				if patternRunes == nil {
					patternRunes = []rune(pattern)
				}

				score, ok = fuzzyScore(patternRunes, candidates[index])
			}
		} else {
			score, ok = fuzzyScore(patternRunes, candidates[index])
		}

		if !ok {
			continue
		}

		matches = append(matches, Match{
			Index:         index,
			Score:         score,
			MatchedRanges: nil,
		})
	}

	return matches
}

func fuzzyScoreASCII(pattern []byte, candidate string) (int, bool, bool) {
	state := newFuzzyScoreState()

	for j := range len(candidate) {
		current := candidate[j]
		if current >= utf8.RuneSelf {
			return 0, false, false
		}

		if fuzzyEqualFoldASCII(current, pattern[state.patternIndex]) {
			score := state.asciiCharScore(current, j)
			state.recordCandidate(score, j)
		}

		nextPattern := nextASCIIPattern(pattern, state.patternIndex)

		nextCandidate, ascii := nextASCIICandidate(candidate, j)
		if !ascii {
			return 0, false, false
		}

		if fuzzyEqualFoldASCII(nextPattern, nextCandidate) || nextCandidate == 0 {
			state.commitBestMatch()

			if state.patternIndex == len(pattern) {
				break
			}
		}

		state.lastIndex = j
		state.lastASCII = current
	}

	return state.finalScore(len(candidate)), state.matchedCount == len(pattern), true
}

func lowerASCIIBytes(s string) []byte {
	result := make([]byte, len(s))
	for i := range len(s) {
		b := s[i]
		if 'A' <= b && b <= 'Z' {
			b += 'a' - 'A'
		}

		result[i] = b
	}

	return result
}

func fuzzyScore(patternRunes []rune, candidate string) (int, bool) {
	state := newFuzzyScoreState()
	nextCandidate, nextSize := utf8.DecodeRuneInString(candidate)

	var (
		current     rune
		currentSize int
	)

	for j := 0; j < len(candidate); j += currentSize {
		current, currentSize = nextCandidate, nextSize
		if fuzzyEqualFold(current, patternRunes[state.patternIndex]) {
			score := state.runeCharScore(current, j)
			state.recordCandidate(score, j)
		}

		nextPattern := nextRunePattern(patternRunes, state.patternIndex)

		if j+currentSize < len(candidate) {
			if candidate[j+currentSize] < utf8.RuneSelf {
				nextCandidate, nextSize = rune(candidate[j+currentSize]), 1
			} else {
				nextCandidate, nextSize = utf8.DecodeRuneInString(candidate[j+currentSize:])
			}
		} else {
			nextCandidate, nextSize = 0, 0
		}

		if fuzzyEqualFold(nextPattern, nextCandidate) || nextCandidate == 0 {
			state.commitBestMatch()

			if state.patternIndex == len(patternRunes) {
				break
			}
		}

		state.lastIndex = j
		state.lastRune = current
	}

	return state.finalScore(len(candidate)), state.matchedCount == len(patternRunes)
}

type fuzzyScoreState struct {
	lastRune               rune
	lastASCII              byte
	patternIndex           int
	bestScore              int
	matchedIndex           int
	currAdjacentMatchBonus int
	lastMatchedIndex       int
	matchedCount           int
	totalScore             int
	lastIndex              int
}

func newFuzzyScoreState() fuzzyScoreState {
	return fuzzyScoreState{
		lastRune:               0,
		lastASCII:              0,
		patternIndex:           0,
		bestScore:              -1,
		matchedIndex:           -1,
		currAdjacentMatchBonus: 0,
		lastMatchedIndex:       -1,
		matchedCount:           0,
		totalScore:             0,
		lastIndex:              0,
	}
}

func (s *fuzzyScoreState) asciiCharScore(current byte, index int) int {
	score := s.baseCharScore(index)
	if isLowerASCII(s.lastASCII) && isUpperASCII(current) {
		score += fuzzyCamelCaseMatchBonus
	}

	if index != 0 && fuzzyIsSeparatorASCII(s.lastASCII) {
		score += fuzzyMatchFollowingSeparatorBonus
	}

	return s.scoreWithAdjacency(score)
}

func (s *fuzzyScoreState) runeCharScore(current rune, index int) int {
	score := s.baseCharScore(index)
	if unicode.IsLower(s.lastRune) && unicode.IsUpper(current) {
		score += fuzzyCamelCaseMatchBonus
	}

	if index != 0 && fuzzyIsSeparator(s.lastRune) {
		score += fuzzyMatchFollowingSeparatorBonus
	}

	return s.scoreWithAdjacency(score)
}

func (s *fuzzyScoreState) baseCharScore(index int) int {
	if index == 0 {
		return fuzzyFirstCharMatchBonus
	}

	return 0
}

func (s *fuzzyScoreState) scoreWithAdjacency(score int) int {
	if s.matchedCount == 0 {
		return score
	}

	bonus := fuzzyAdjacentCharBonus(s.lastIndex, s.lastMatchedIndex, s.currAdjacentMatchBonus)
	s.currAdjacentMatchBonus += bonus

	return score + bonus
}

func (s *fuzzyScoreState) recordCandidate(score int, index int) {
	if score <= s.bestScore {
		return
	}

	s.bestScore = score
	s.matchedIndex = index
}

func (s *fuzzyScoreState) commitBestMatch() {
	if s.matchedIndex < 0 {
		return
	}

	if s.matchedCount == 0 {
		penalty := s.matchedIndex * fuzzyUnmatchedLeadingCharPenalty
		s.bestScore += max(penalty, fuzzyMaxUnmatchedLeadingCharPenalty)
	}

	s.totalScore += s.bestScore
	s.lastMatchedIndex = s.matchedIndex
	s.matchedCount++
	s.bestScore = -1
	s.matchedIndex = -1
	s.patternIndex++
}

func (s *fuzzyScoreState) finalScore(candidateLen int) int {
	return s.totalScore + s.matchedCount - candidateLen
}

func nextASCIIPattern(pattern []byte, patternIndex int) byte {
	if patternIndex >= len(pattern)-1 {
		return 0
	}

	return pattern[patternIndex+1]
}

func nextASCIICandidate(candidate string, index int) (byte, bool) {
	nextIndex := index + 1
	if nextIndex >= len(candidate) {
		return 0, true
	}

	next := candidate[nextIndex]
	if next >= utf8.RuneSelf {
		return 0, false
	}

	return next, true
}

func nextRunePattern(pattern []rune, patternIndex int) rune {
	if patternIndex >= len(pattern)-1 {
		return 0
	}

	return pattern[patternIndex+1]
}

func fuzzyEqualFold(a rune, b rune) bool {
	if a == b {
		return true
	}

	if a < b {
		a, b = b, a
	}

	if a < utf8.RuneSelf {
		return 'A' <= b && b <= 'Z' && a == b+'a'-'A'
	}

	r := unicode.SimpleFold(b)
	for r != b && r < a {
		r = unicode.SimpleFold(r)
	}

	return r == a
}

func fuzzyEqualFoldASCII(a byte, b byte) bool {
	if a == b {
		return true
	}

	if 'A' <= a && a <= 'Z' {
		a += 'a' - 'A'
	}

	if 'A' <= b && b <= 'Z' {
		b += 'a' - 'A'
	}

	return a == b
}

func isLowerASCII(b byte) bool {
	return 'a' <= b && b <= 'z'
}

func isUpperASCII(b byte) bool {
	return 'A' <= b && b <= 'Z'
}

func fuzzyAdjacentCharBonus(i int, lastMatch int, currentBonus int) int {
	if lastMatch == i {
		return currentBonus*2 + fuzzyAdjacentMatchBonus
	}

	return 0
}

func fuzzyIsSeparator(r rune) bool {
	switch r {
	case '/', '-', '_', ' ', '.', '\\':
		return true
	default:
		return false
	}
}

func fuzzyIsSeparatorASCII(b byte) bool {
	switch b {
	case '/', '-', '_', ' ', '.', '\\':
		return true
	default:
		return false
	}
}

func isASCII(s string) bool {
	for i := range len(s) {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}

	return true
}
