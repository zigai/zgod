package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/zigai/zgod/internal/config"
	"github.com/zigai/zgod/internal/db"
	"github.com/zigai/zgod/internal/history"
	"github.com/zigai/zgod/internal/paths"
)

var recordCmd = &cobra.Command{
	Use:          "record",
	Short:        "Record a command to history",
	Hidden:       true,
	SilenceUsage: true,
	RunE:         runRecord,
}

const (
	recordMillisecondsPerSecond       int64 = 1000
	recordUnixMillisecondsCutoffValue int64 = 1_000_000_000_000
	recordBusyRetryAttempts                 = 5
	recordBusyRetryBackoff                  = 250 * time.Millisecond
	recordPendingNameAttempts               = 100
	recordPendingFileExtension              = ".json"
	recordPendingTempPrefix                 = ".tmp-"
)

var (
	errPendingRecordNameExhausted = errors.New("exhausted pending history record file names")
	recordWriteLockTimeout        = 30 * time.Second
)

type pendingHistoryRecord struct {
	TimestampMS int64  `json:"tsMs"`
	DurationMS  int64  `json:"duration"`
	ExitCode    int    `json:"exitCode"`
	Command     string `json:"command"`
	Directory   string `json:"directory"`
	SessionID   string `json:"sessionId"`
	Hostname    string `json:"hostname"`
}

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

	nowMS := time.Now().UnixMilli()
	timestampMS, durationMS := parseRecordTimingMS(cmd, nowMS)
	sessionID, _ := cmd.Flags().GetString("session")
	hostname := getHostname()

	entry := db.HistoryEntry{
		ID:          0,
		TimestampMS: timestampMS,
		DurationMS:  durationMS,
		ExitCode:    exitCode,
		Command:     command,
		Directory:   directory,
		SessionID:   sessionID,
		Hostname:    hostname,
	}

	if err = insertRecordWithRetry(dbPath, entry); err != nil {
		return fmt.Errorf("recording command history: %w", err)
	}

	return nil
}

func insertRecordWithRetry(dbPath string, entry db.HistoryEntry) error {
	ctx, cancel := context.WithTimeout(context.Background(), recordWriteLockTimeout)
	defer cancel()

	err := db.WithDatabaseWriteLock(ctx, dbPath, func() error {
		if err := drainPendingRecordsLocked(dbPath); err != nil {
			return err
		}

		return insertRecordWithRetryLocked(dbPath, entry)
	})
	if isRecordWriterContention(err) {
		if err := queuePendingRecord(dbPath, entry); err != nil {
			return fmt.Errorf("queueing history record after writer contention: %w", err)
		}

		return nil
	}

	if err != nil {
		return fmt.Errorf("locking database and writing history record: %w", err)
	}

	return nil
}

func isRecordWriterContention(err error) bool {
	return db.IsBusyError(err) || errors.Is(err, db.ErrDatabaseWriteLockTimeout)
}

func insertRecordWithRetryLocked(dbPath string, entry db.HistoryEntry) error {
	var lastBusyErr error

	for attempt := 0; attempt <= recordBusyRetryAttempts; attempt++ {
		database, err := db.Open(dbPath)
		if err != nil {
			if db.IsBusyError(err) {
				lastBusyErr = err

				sleepBeforeRecordRetry(attempt)

				continue
			}

			return fmt.Errorf("opening database: %w", err)
		}

		repo := db.NewHistoryRepo(database)
		_, err = repo.Insert(entry)

		closeErr := database.Close()
		if err == nil {
			if closeErr != nil {
				return fmt.Errorf("closing database: %w", closeErr)
			}

			return nil
		}

		if !db.IsBusyError(err) {
			return fmt.Errorf("writing history record: %w", err)
		}

		lastBusyErr = err

		sleepBeforeRecordRetry(attempt)
	}

	return lastBusyErr
}

func pendingRecordsDir(dbPath string) string {
	return dbPath + ".pending"
}

func queuePendingRecord(dbPath string, entry db.HistoryEntry) error {
	dir := pendingRecordsDir(dbPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating pending history directory %q: %w", dir, err)
	}

	data, err := json.Marshal(newPendingHistoryRecord(entry))
	if err != nil {
		return fmt.Errorf("encoding pending history record: %w", err)
	}

	data = append(data, '\n')

	for attempt := range recordPendingNameAttempts {
		name := pendingRecordFileName(entry, attempt)
		tmpPath := filepath.Join(dir, recordPendingTempPrefix+name)
		finalPath := filepath.Join(dir, name)

		if err = writePendingRecordFile(tmpPath, data); errors.Is(err, os.ErrExist) {
			continue
		}

		if err != nil {
			return err
		}

		if err = os.Rename(tmpPath, finalPath); err != nil {
			_ = os.Remove(tmpPath)
			return fmt.Errorf("publishing pending history record %q: %w", finalPath, err)
		}

		return nil
	}

	return errPendingRecordNameExhausted
}

func pendingRecordFileName(entry db.HistoryEntry, attempt int) string {
	return fmt.Sprintf(
		"%020d-%d-%d-%02d%s",
		entry.TimestampMS,
		os.Getpid(),
		time.Now().UnixNano(),
		attempt,
		recordPendingFileExtension,
	)
}

func writePendingRecordFile(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("creating pending history record %q: %w", path, err)
	}

	if _, err = file.Write(data); err != nil {
		_ = file.Close()
		_ = os.Remove(path)

		return fmt.Errorf("writing pending history record %q: %w", path, err)
	}

	if err = file.Close(); err != nil {
		_ = os.Remove(path)

		return fmt.Errorf("closing pending history record %q: %w", path, err)
	}

	return nil
}

func drainPendingRecordsLocked(dbPath string) error {
	dir := pendingRecordsDir(dbPath)

	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	if err != nil {
		return fmt.Errorf("reading pending history directory %q: %w", dir, err)
	}

	sort.Slice(entries, func(i int, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() ||
			strings.HasPrefix(name, recordPendingTempPrefix) ||
			!strings.HasSuffix(name, recordPendingFileExtension) {
			continue
		}

		if err = flushPendingRecordLocked(dbPath, filepath.Join(dir, name)); err != nil {
			return err
		}
	}

	return nil
}

func flushPendingRecordLocked(dbPath string, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading pending history record %q: %w", path, err)
	}

	var pending pendingHistoryRecord
	if err = json.Unmarshal(data, &pending); err != nil {
		return fmt.Errorf("decoding pending history record %q: %w", path, err)
	}

	if err = insertRecordWithRetryLocked(dbPath, pending.historyEntry()); err != nil {
		return fmt.Errorf("writing pending history record %q: %w", path, err)
	}

	if err = os.Remove(path); err != nil {
		return fmt.Errorf("removing pending history record %q: %w", path, err)
	}

	return nil
}

func newPendingHistoryRecord(entry db.HistoryEntry) pendingHistoryRecord {
	return pendingHistoryRecord{
		TimestampMS: entry.TimestampMS,
		DurationMS:  entry.DurationMS,
		ExitCode:    entry.ExitCode,
		Command:     entry.Command,
		Directory:   entry.Directory,
		SessionID:   entry.SessionID,
		Hostname:    entry.Hostname,
	}
}

func (r pendingHistoryRecord) historyEntry() db.HistoryEntry {
	return db.HistoryEntry{
		ID:          0,
		TimestampMS: r.TimestampMS,
		DurationMS:  r.DurationMS,
		ExitCode:    r.ExitCode,
		Command:     r.Command,
		Directory:   r.Directory,
		SessionID:   r.SessionID,
		Hostname:    r.Hostname,
	}
}

func sleepBeforeRecordRetry(attempt int) {
	if attempt < recordBusyRetryAttempts {
		time.Sleep(recordBusyRetryBackoff)
	}
}

func shouldRecordCommand(cfg config.Config, command string, exitCode int, directory string) (bool, error) {
	filter, err := history.NewFilter(cfg.Filters)
	if err != nil {
		return false, fmt.Errorf("building filter: %w", err)
	}

	return filter.ShouldRecord(command, exitCode, directory), nil
}

func parseRecordTimingMS(cmd *cobra.Command, nowMS int64) (int64, int64) {
	tsStr, _ := cmd.Flags().GetString("ts")
	timestampMS := parseTimestampMS(tsStr, nowMS)

	durationMS, _ := cmd.Flags().GetInt64("duration")
	if durationMS < 0 && timestampMS > 0 && timestampMS < nowMS {
		durationMS = nowMS - timestampMS
	}

	if durationMS < 0 {
		durationMS = 0
	}

	return timestampMS, durationMS
}

func getHostname() string {
	h, _ := os.Hostname()
	return h
}

// parseTimestampMS parses a timestamp string into milliseconds.
// Accepts: "now", milliseconds (13 digits), seconds (10 digits), or seconds with "s" suffix.
func parseTimestampMS(s string, nowMS int64) int64 {
	if s == "" || s == "now" {
		return nowMS
	}

	if len(s) > 1 && s[len(s)-1] == 's' {
		sec, err := strconv.ParseInt(s[:len(s)-1], 10, 64)
		if err != nil {
			return nowMS
		}

		return sec * recordMillisecondsPerSecond
	}

	val, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return nowMS
	}

	if val < recordUnixMillisecondsCutoffValue {
		return val * recordMillisecondsPerSecond
	}

	return val
}
