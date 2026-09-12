package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/zigai/zgod/internal/config"
	"github.com/zigai/zgod/internal/db"
	"github.com/zigai/zgod/internal/history"
)

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

func registerRecordCommand(root *cobra.Command) {
	recordCmd := &cobra.Command{Use: "record", Short: "Record a command to history", Hidden: true, GroupID: "core", RunE: runRecord}
	recordCmd.Flags().String("ts", "", "start timestamp: milliseconds, seconds (with 's' suffix), or 'now'")
	recordCmd.Flags().Int64("duration", 0, "Duration in milliseconds (-1 or omitted: compute from ts; otherwise >=0)")
	recordCmd.Flags().Int("exit-code", 0, "Exit status (0–4294967295; Windows also accepts signed 32-bit statuses; default 0)")
	recordCmd.Flags().String("command", "", "command string")
	recordCmd.Flags().String("directory", "", "working directory")
	recordCmd.Flags().String("session", "", "session ID")
	recordCmd.Flags().Bool("command-stdin", false, "Read the command from stdin instead of process arguments (maximum 1 MiB)")
	root.AddCommand(recordCmd)
}

func runRecord(cmd *cobra.Command, args []string) error {
	command, _ := cmd.Flags().GetString("command")
	if flagBool(cmd, "command-stdin") {
		data, err := io.ReadAll(io.LimitReader(cmd.InOrStdin(), maxCommandBytes+1))
		if err != nil {
			return fmt.Errorf("reading command from stdin: %w", err)
		}

		if len(data) > maxCommandBytes {
			return fmt.Errorf("%w: command exceeds 1 MiB", errUsage)
		}

		command = string(data)
	}

	if command == "" {
		return nil
	}

	cfg, err := loadConfig(cmd)
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

	dbPath, err := cfg.DatabasePath()
	if err != nil {
		return fmt.Errorf("resolving database path: %w", err)
	}

	nowMS := time.Now().UnixMilli()

	timestampMS, durationMS, err := parseRecordTimingMS(cmd, nowMS)
	if err != nil {
		return fmt.Errorf("%w: %w", errUsage, err)
	}

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

func parseRecordTimingMS(cmd *cobra.Command, nowMS int64) (int64, int64, error) {
	timestampMS, err := parseTimestampMS(flagString(cmd, "ts"), nowMS)
	if err != nil {
		return 0, 0, err
	}

	durationMS, _ := cmd.Flags().GetInt64("duration")
	if !cmd.Flags().Changed("duration") {
		durationMS = -1
	}

	if durationMS < -1 {
		return 0, 0, fmt.Errorf("%w: --duration must be -1 or nonnegative", errUsage)
	}

	if durationMS == -1 {
		durationMS = max(nowMS-timestampMS, 0)
	}

	return timestampMS, durationMS, nil
}

func getHostname() string {
	h, _ := os.Hostname()
	return h
}

// parseTimestampMS accepts now, Unix seconds (optional s suffix), or milliseconds.
func parseTimestampMS(value string, nowMS int64) (int64, error) {
	if value == "" || value == "now" {
		return nowMS, nil
	}

	seconds := strings.HasSuffix(value, "s")
	number := strings.TrimSuffix(value, "s")

	parsed, err := strconv.ParseInt(number, 10, 64)
	if err != nil || parsed < 0 {
		return 0, fmt.Errorf("%w: --ts requires now or a nonnegative Unix timestamp", errUsage)
	}

	if seconds || parsed < recordUnixMillisecondsCutoffValue {
		if parsed > math.MaxInt64/recordMillisecondsPerSecond {
			return 0, fmt.Errorf("%w: --ts seconds overflow milliseconds", errUsage)
		}

		parsed *= recordMillisecondsPerSecond
	}

	return parsed, nil
}
