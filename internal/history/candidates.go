package history

import (
	"fmt"

	"github.com/zigai/zgod/internal/db"
)

type CandidateOpts struct {
	Limit      int
	Dedupe     bool
	FailFilter db.FailFilterMode
	CWD        string
}

func FetchCandidates(repo *db.HistoryRepo, opts CandidateOpts) ([]db.HistoryEntry, error) {
	entries, err := repo.FetchCandidatesInDir(opts.Limit, opts.Dedupe, opts.FailFilter, opts.CWD)
	if err != nil {
		return nil, fmt.Errorf("fetching history candidates: %w", err)
	}

	return entries, nil
}
