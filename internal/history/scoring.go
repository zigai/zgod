package history

import (
	"sort"

	"github.com/zigai/zgod/internal/db"
	"github.com/zigai/zgod/internal/match"
)

const (
	defaultScoringCWDBonus    = 50
	defaultScoringRecencyBase = 10
	scoringRecencyIndexStep   = 100
	minMatchesForOrderCheck   = 2
)

type ScoredEntry struct {
	Entry      db.HistoryEntry
	MatchInfo  match.Match
	FinalScore int
}

type ScoringOpts struct {
	CWD         string
	CWDBonus    int
	RecencyBase int
}

func DefaultScoringOpts(cwd string) ScoringOpts {
	return ScoringOpts{
		CWD:         cwd,
		CWDBonus:    defaultScoringCWDBonus,
		RecencyBase: defaultScoringRecencyBase,
	}
}

func ScoreAndSort(entries []db.HistoryEntry, matches []match.Match, opts ScoringOpts) []ScoredEntry {
	return ScoreAndSortInto(nil, entries, matches, opts)
}

func ScoreAndSortInto(dst []ScoredEntry, entries []db.HistoryEntry, matches []match.Match, opts ScoringOpts) []ScoredEntry {
	scored := dst[:0]
	if cap(scored) < len(matches) {
		scored = make([]ScoredEntry, 0, len(matches))
	}

	scored = scored[:len(matches)]
	if canUseMatchIndexOrder(matches) {
		if scoreInMatchOrder(scored, entries, matches, opts) {
			return scored
		}
	}

	for i, m := range matches {
		entry := entries[m.Index]
		scored[i] = scoredEntryForMatch(entry, m, opts)
	}

	sort.Sort(ScoredEntriesByScore(scored))

	return scored
}

func scoreInMatchOrder(scored []ScoredEntry, entries []db.HistoryEntry, matches []match.Match, opts ScoringOpts) bool {
	partitionCWD := opts.CWD != "" && opts.CWDBonus > opts.RecencyBase
	cwdWrite := 0
	otherWrite := cwdPartitionStart(entries, matches, opts, partitionCWD)

	for i, m := range matches {
		entry := entries[m.Index]
		scoredEntry := scoredEntryForMatch(entry, m, opts)

		switch {
		case partitionCWD && entry.Directory == opts.CWD:
			scored[cwdWrite] = scoredEntry
			cwdWrite++
		case partitionCWD:
			scored[otherWrite] = scoredEntry
			otherWrite++
		default:
			scored[i] = scoredEntry
		}
	}

	return partitionCWD || opts.CWD == "" || opts.CWDBonus == 0
}

func cwdPartitionStart(entries []db.HistoryEntry, matches []match.Match, opts ScoringOpts, partitionCWD bool) int {
	if !partitionCWD {
		return 0
	}

	count := 0

	for _, m := range matches {
		if entries[m.Index].Directory == opts.CWD {
			count++
		}
	}

	return count
}

func canUseMatchIndexOrder(matches []match.Match) bool {
	if len(matches) < minMatchesForOrderCheck {
		return true
	}

	score := matches[0].Score

	previousIndex := matches[0].Index
	for _, m := range matches[1:] {
		if m.Score != score || m.Index < previousIndex {
			return false
		}

		previousIndex = m.Index
	}

	return true
}

func scoredEntryForMatch(entry db.HistoryEntry, m match.Match, opts ScoringOpts) ScoredEntry {
	score := m.Score

	if opts.CWD != "" && entry.Directory == opts.CWD {
		score += opts.CWDBonus
	}

	recency := max(opts.RecencyBase-(m.Index/scoringRecencyIndexStep), 0)
	score += recency

	return ScoredEntry{
		Entry:      entry,
		MatchInfo:  m,
		FinalScore: score,
	}
}

type ScoredEntriesByScore []ScoredEntry

func (s ScoredEntriesByScore) Len() int {
	return len(s)
}

func (s ScoredEntriesByScore) Less(i int, j int) bool {
	if s[i].FinalScore == s[j].FinalScore {
		return s[i].MatchInfo.Index < s[j].MatchInfo.Index
	}

	return s[i].FinalScore > s[j].FinalScore
}

func (s ScoredEntriesByScore) Swap(i int, j int) {
	s[i], s[j] = s[j], s[i]
}
