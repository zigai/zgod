package history

import (
	"sort"

	"github.com/zigai/zgod/internal/db"
	"github.com/zigai/zgod/internal/match"
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
		CWDBonus:    50,
		RecencyBase: 10,
	}
}

func ScoreAndSort(entries []db.HistoryEntry, matches []match.Match, opts ScoringOpts) []ScoredEntry {
	entryMap := map[int]db.HistoryEntry{}
	for i, e := range entries {
		entryMap[i] = e
	}

	scored := make([]ScoredEntry, len(matches))
	for i, m := range matches {
		entry := entryMap[m.Index]
		score := m.Score

		if opts.CWD != "" && entry.Directory == opts.CWD {
			score += opts.CWDBonus
		}

		recency := max(opts.RecencyBase-(m.Index/100), 0)
		score += recency

		scored[i] = ScoredEntry{
			Entry:      entry,
			MatchInfo:  m,
			FinalScore: score,
		}
	}

	sort.SliceStable(scored, func(i, j int) bool {
		return scored[i].FinalScore > scored[j].FinalScore
	})

	return scored
}
