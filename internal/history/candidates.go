package history

import "github.com/zigai/zgod/internal/db"

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
	return repo.FetchCandidates(limit, opts.Dedupe, opts.OnlyFails)
}
