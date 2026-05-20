package history

import (
	"testing"

	"github.com/zigai/zgod/internal/db"
	"github.com/zigai/zgod/internal/match"
)

func TestScoreAndSortUsesRecencyOrderForTies(t *testing.T) {
	t.Parallel()

	entries := []db.HistoryEntry{
		{Command: "newer"},
		{Command: "older"},
	}
	matches := []match.Match{
		{Index: 1, Score: 100},
		{Index: 0, Score: 100},
	}

	got := ScoreAndSort(entries, matches, ScoringOpts{})

	if got[0].Entry.Command != "newer" || got[1].Entry.Command != "older" {
		t.Fatalf("ScoreAndSort() order = %q, %q; want newer, older", got[0].Entry.Command, got[1].Entry.Command)
	}
}

func TestScoreAndSortKeepsIndexOrderForEqualScores(t *testing.T) {
	t.Parallel()

	entries := []db.HistoryEntry{
		{Command: "newer"},
		{Command: "middle"},
		{Command: "older"},
	}
	matches := []match.Match{
		{Index: 0, Score: 100},
		{Index: 1, Score: 100},
		{Index: 2, Score: 100},
	}

	got := ScoreAndSort(entries, matches, ScoringOpts{})

	for i, want := range []string{"newer", "middle", "older"} {
		if got[i].Entry.Command != want {
			t.Fatalf("ScoreAndSort()[%d] = %q, want %q", i, got[i].Entry.Command, want)
		}
	}
}

func TestScoreAndSortPartitionsCWDFirstWhenBoostDominatesRecency(t *testing.T) {
	t.Parallel()

	entries := []db.HistoryEntry{
		{Command: "other-new", Directory: "/other"},
		{Command: "cwd-new", Directory: "/cwd"},
		{Command: "other-old", Directory: "/other"},
		{Command: "cwd-old", Directory: "/cwd"},
	}
	matches := []match.Match{
		{Index: 0, Score: 100},
		{Index: 1, Score: 100},
		{Index: 2, Score: 100},
		{Index: 3, Score: 100},
	}

	got := ScoreAndSort(entries, matches, ScoringOpts{CWD: "/cwd", CWDBonus: 50, RecencyBase: 10})

	for i, want := range []string{"cwd-new", "cwd-old", "other-new", "other-old"} {
		if got[i].Entry.Command != want {
			t.Fatalf("ScoreAndSort()[%d] = %q, want %q", i, got[i].Entry.Command, want)
		}
	}
}
