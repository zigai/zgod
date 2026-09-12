package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zigai/zgod/internal/config"
	"github.com/zigai/zgod/internal/db"
	"github.com/zigai/zgod/internal/history"
	"github.com/zigai/zgod/internal/match"
)

const defaultSearchLimit = 1000

type historyRecord struct {
	ID          int64  `json:"id"`
	TimestampMS int64  `json:"timestampMs"`
	DurationMS  int64  `json:"durationMs"`
	ExitCode    int    `json:"exitCode"`
	Command     string `json:"command"`
	Directory   string `json:"directory"`
}

func writeJSON(output io.Writer, value any) error {
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)

	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("writing JSON: %w", err)
	}

	return nil
}

func runSearchOutput(cmd *cobra.Command, cfg config.Config) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting current directory: %w", err)
	}

	entries, err := fetchSearchOutput(cfg, cwd)
	if err != nil {
		return err
	}

	mode, _ := match.ParseMode(cfg.Display.DefaultMode)
	matches := outputMatches(mode, flagString(cmd, "query"), entries)
	scoring := history.DefaultScoringOpts(cwd)
	scoring.CWDBonus = cfg.Display.CWDBoost
	ranked := history.ScoreAndSort(entries, matches, scoring)

	limit, _ := cmd.Flags().GetInt("limit")
	if !cmd.Flags().Changed("limit") {
		limit = defaultSearchLimit
	}

	filtered := make([]db.HistoryEntry, 0, min(len(ranked), limit))
	for _, result := range ranked {
		if cfg.Display.HideMultiline && strings.ContainsAny(result.Entry.Command, "\r\n") {
			continue
		}

		filtered = append(filtered, result.Entry)
		if len(filtered) == limit {
			break
		}
	}

	return printHistory(cmd, filtered)
}

func fetchSearchOutput(cfg config.Config, cwd string) ([]db.HistoryEntry, error) {
	dbPath, err := cfg.DatabasePath()
	if err != nil {
		return nil, fmt.Errorf("resolving database: %w", err)
	}

	if _, err = os.Stat(dbPath); os.IsNotExist(err) {
		return nil, nil
	}

	if err = drainPendingRecordsForSearch(dbPath); err != nil {
		return nil, err
	}

	database, err := openSearchDatabase(dbPath)
	if err != nil {
		return nil, err
	}

	defer func() { _ = database.Close() }()

	if cfg.Display.DefaultScope != "cwd" {
		cwd = ""
	}

	failFilter, _ := db.ParseFailFilterMode(cfg.Display.DefaultFailFilter)

	entries, err := history.FetchCandidates(db.NewHistoryRepo(database), history.CandidateOpts{
		Limit: 0, Dedupe: true, FailFilter: failFilter, CWD: cwd,
	})
	if err != nil {
		return nil, fmt.Errorf("reading history: %w", err)
	}

	return entries, nil
}

func outputMatches(mode match.Mode, query string, entries []db.HistoryEntry) []match.Match {
	if query == "" {
		matches := make([]match.Match, len(entries))
		for index := range entries {
			matches[index] = match.Match{Index: index, Score: 0, MatchedRanges: nil}
		}

		return matches
	}

	candidates := make([]string, len(entries))
	for index, entry := range entries {
		candidates[index] = entry.Command
	}

	return match.New(mode).Match(query, candidates)
}

func printHistory(cmd *cobra.Command, entries []db.HistoryEntry) error {
	if !flagBool(cmd, "json") {
		for _, entry := range entries {
			if _, err := fmt.Fprintln(cmd.OutOrStdout(), entry.Command); err != nil {
				return fmt.Errorf("writing history: %w", err)
			}
		}

		return nil
	}

	records := make([]historyRecord, 0, len(entries))
	for _, entry := range entries {
		records = append(records, historyRecord{
			ID: entry.ID, TimestampMS: entry.TimestampMS, DurationMS: entry.DurationMS,
			ExitCode: entry.ExitCode, Command: entry.Command, Directory: entry.Directory,
		})
	}

	return writeJSON(cmd.OutOrStdout(), records)
}
