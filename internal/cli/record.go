package cli

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/zigai/zgod/internal/config"
	"github.com/zigai/zgod/internal/db"
	"github.com/zigai/zgod/internal/history"
	"github.com/zigai/zgod/internal/paths"
)

var recordCmd = &cobra.Command{
	Use:    "record",
	Short:  "Record a command to history",
	Hidden: true,
	RunE:   runRecord,
}

const (
	recordMsPerSecond           int64 = 1000
	recordUnixMillisCutoffValue int64 = 1_000_000_000_000
)

func registerRecordCommand() {
	recordCmd.Flags().String("ts", "", "start timestamp: milliseconds, seconds (with 's' suffix), or 'now'")
	recordCmd.Flags().Int64("duration", -1, "duration in milliseconds (-1 to auto-compute from ts)")
	recordCmd.Flags().Int("exit-code", 0, "exit code")
	recordCmd.Flags().String("command", "", "command string")
	recordCmd.Flags().String("directory", "", "working directory")
	recordCmd.Flags().String("session", "", "session ID")
	rootCmd.AddCommand(recordCmd)
}

func runRecord(cmd *cobra.Command, args []string) error {
	command, _ := cmd.Flags().GetString("command")
	if command == "" {
		return nil
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	exitCode, _ := cmd.Flags().GetInt("exit-code")
	directory, _ := cmd.Flags().GetString("directory")

	shouldRecord, err := shouldRecordCommand(cfg, command, exitCode, directory)
	if err != nil {
		return err
	}

	if !shouldRecord {
		return nil
	}

	if err = paths.EnsureDirs(); err != nil {
		return fmt.Errorf("ensuring directories: %w", err)
	}

	dbPath, err := cfg.DatabasePath()
	if err != nil {
		return fmt.Errorf("resolving database path: %w", err)
	}

	database, err := db.Open(dbPath)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}

	defer func() { _ = database.Close() }()

	nowMs := time.Now().UnixMilli()
	ts, duration := parseRecordTiming(cmd, nowMs)
	sessionID, _ := cmd.Flags().GetString("session")
	hostname := getHostname()

	repo := db.NewHistoryRepo(database)

	_, err = repo.Insert(db.HistoryEntry{
		ID:        0,
		TsMs:      ts,
		Duration:  duration,
		ExitCode:  exitCode,
		Command:   command,
		Directory: directory,
		SessionID: sessionID,
		Hostname:  hostname,
	})
	if err != nil {
		return fmt.Errorf("inserting history entry: %w", err)
	}

	return nil
}

func shouldRecordCommand(cfg config.Config, command string, exitCode int, directory string) (bool, error) {
	filter, err := history.NewFilter(cfg.Filters)
	if err != nil {
		return false, fmt.Errorf("building filter: %w", err)
	}

	return filter.ShouldRecord(command, exitCode, directory), nil
}

func parseRecordTiming(cmd *cobra.Command, nowMs int64) (int64, int64) {
	tsStr, _ := cmd.Flags().GetString("ts")
	ts := parseTimestamp(tsStr, nowMs)

	duration, _ := cmd.Flags().GetInt64("duration")
	if duration < 0 && ts > 0 && ts < nowMs {
		duration = nowMs - ts
	}

	if duration < 0 {
		duration = 0
	}

	return ts, duration
}

func getHostname() string {
	h, _ := os.Hostname()
	return h
}

// parseTimestamp parses a timestamp string into milliseconds.
// Accepts: "now", milliseconds (13 digits), seconds (10 digits), or seconds with "s" suffix.
func parseTimestamp(s string, nowMs int64) int64 {
	if s == "" || s == "now" {
		return nowMs
	}

	if len(s) > 1 && s[len(s)-1] == 's' {
		sec, err := strconv.ParseInt(s[:len(s)-1], 10, 64)
		if err != nil {
			return nowMs
		}

		return sec * recordMsPerSecond
	}

	val, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return nowMs
	}

	if val < recordUnixMillisCutoffValue {
		return val * recordMsPerSecond
	}

	return val
}
