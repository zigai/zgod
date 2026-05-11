package history

import (
	"fmt"

	"github.com/zigai/zgod/internal/db"
)

type CandidateOpts struct {
	Limit      int
	Dedupe     bool
	FailFilter db.FailFilterMode
}

func FetchCandidates(repo *db.HistoryRepo, opts CandidateOpts) ([]db.HistoryEntry, error) {
	entries, err := repo.FetchCandidates(opts.Limit, opts.Dedupe, opts.FailFilter)
	if err != nil {
		return nil, fmt.Errorf("fetching history candidates: %w", err)
	}

	return entries, nil
}
