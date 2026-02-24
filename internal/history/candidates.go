package history

import (
	"fmt"

	"github.com/zigai/zgod/internal/db"
)

type CandidateOpts struct {
	Limit     int
	Dedupe    bool
	OnlyFails bool
}

func FetchCandidates(repo *db.HistoryRepo, opts CandidateOpts) ([]db.HistoryEntry, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 10000
	}

	entries, err := repo.FetchCandidates(limit, opts.Dedupe, opts.OnlyFails)
	if err != nil {
		return nil, fmt.Errorf("fetching history candidates: %w", err)
	}

	return entries, nil
}
