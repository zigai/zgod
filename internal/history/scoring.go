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
	scored := make([]ScoredEntry, len(matches))
	for i, m := range matches {
		entry := entries[m.Index]
		score := m.Score

		if opts.CWD != "" && entry.Directory == opts.CWD {
			score += opts.CWDBonus
		}

		recency := max(opts.RecencyBase-(m.Index/scoringRecencyIndexStep), 0)
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
