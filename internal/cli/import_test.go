package cli

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"pgregory.net/rapid"

	"github.com/zigai/zgod/internal/db"
)

func TestImportHistoryEntriesImportsValidSedCommandWithExistingInputFile(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "history.db")

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}

	defer func() { _ = database.Close() }()

	workingDirectory := t.TempDir()

	inputPath := filepath.Join(workingDirectory, "file.txt")
	if err := os.WriteFile(inputPath, []byte("a\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	entry := db.HistoryEntry{
		TimestampMS: 1,
		DurationMS:  10,
		ExitCode:    0,
		Command:     `sed 's/a/b/' file.txt`,
		Directory:   workingDirectory,
		SessionID:   "session-1",
		Hostname:    "host-1",
	}

	summary, err := importHistoryEntries(database, []db.HistoryEntry{entry})
	if err != nil {
		t.Fatalf("importHistoryEntries() error: %v", err)
	}

	if summary.total != 1 {
		t.Fatalf("summary.total = %d, want 1", summary.total)
	}

	if summary.imported != 1 {
		t.Fatalf("summary.imported = %d, want 1", summary.imported)
	}

	if summary.skippedMissingPath != 0 {
		t.Fatalf("summary.skippedMissingPath = %d, want 0", summary.skippedMissingPath)
	}

	repo := db.NewHistoryRepo(database)

	entries, err := repo.ListAll()
	if err != nil {
		t.Fatalf("ListAll() error: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("ListAll() returned %d entries, want 1", len(entries))
	}
}

func TestImportHistoryEntriesAllowsBareCreatorTargets(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "history.db")

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}

	defer func() { _ = database.Close() }()

	workingDirectory := t.TempDir()

	entries := []db.HistoryEntry{
		{TimestampMS: 1, Command: "touch new.txt", Directory: workingDirectory},
		{TimestampMS: 2, Command: "mkdir out", Directory: workingDirectory},
		{TimestampMS: 3, Command: "echo README.md", Directory: workingDirectory},
	}

	summary, err := importHistoryEntries(database, entries)
	if err != nil {
		t.Fatalf("importHistoryEntries() error: %v", err)
	}

	if summary.total != len(entries) {
		t.Fatalf("summary.total = %d, want %d", summary.total, len(entries))
	}

	if summary.imported != len(entries) {
		t.Fatalf("summary.imported = %d, want %d", summary.imported, len(entries))
	}

	if summary.skippedMissingPath != 0 {
		t.Fatalf("summary.skippedMissingPath = %d, want 0", summary.skippedMissingPath)
	}

	repo := db.NewHistoryRepo(database)

	importedEntries, err := repo.ListAll()
	if err != nil {
		t.Fatalf("ListAll() error: %v", err)
	}

	if len(importedEntries) != len(entries) {
		t.Fatalf("ListAll() returned %d entries, want %d", len(importedEntries), len(entries))
	}
}

func TestImportHistoryEntriesSkipsMissingRequiredPaths(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "history.db")

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}

	defer func() { _ = database.Close() }()

	workingDirectory := t.TempDir()

	entries := []db.HistoryEntry{
		{TimestampMS: 1, Command: "cd missing", Directory: workingDirectory},
		{TimestampMS: 2, Command: `sed 's/a/b/' missing.txt`, Directory: workingDirectory},
	}

	summary, err := importHistoryEntries(database, entries)
	if err != nil {
		t.Fatalf("importHistoryEntries() error: %v", err)
	}

	if summary.total != len(entries) {
		t.Fatalf("summary.total = %d, want %d", summary.total, len(entries))
	}

	if summary.imported != 0 {
		t.Fatalf("summary.imported = %d, want 0", summary.imported)
	}

	if summary.skippedMissingPath != len(entries) {
		t.Fatalf("summary.skippedMissingPath = %d, want %d", summary.skippedMissingPath, len(entries))
	}
}

func TestImportHistoryEntriesCountsDuplicateSourceRows(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "history.db")

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}

	defer func() { _ = database.Close() }()

	entry := db.HistoryEntry{
		TimestampMS: 1000,
		DurationMS:  10,
		ExitCode:    0,
		Command:     "echo duplicate",
		Directory:   "/tmp",
		SessionID:   "session-1",
		Hostname:    "host-1",
	}

	summary, err := importHistoryEntries(database, []db.HistoryEntry{entry, entry})
	if err != nil {
		t.Fatalf("importHistoryEntries() error: %v", err)
	}

	if summary.total != 2 {
		t.Fatalf("summary.total = %d, want 2", summary.total)
	}

	if summary.imported != 1 {
		t.Fatalf("summary.imported = %d, want 1", summary.imported)
	}

	if summary.skippedDuplicate != 1 {
		t.Fatalf("summary.skippedDuplicate = %d, want 1", summary.skippedDuplicate)
	}

	repo := db.NewHistoryRepo(database)

	entries, err := repo.ListAll()
	if err != nil {
		t.Fatalf("ListAll() error: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("ListAll() returned %d entries, want 1", len(entries))
	}
}

func TestImportHistoryEntriesCountsExistingTargetRowsAsDuplicates(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "history.db")

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}

	defer func() { _ = database.Close() }()

	repo := db.NewHistoryRepo(database)
	existing := db.HistoryEntry{
		TimestampMS: 1000,
		DurationMS:  10,
		ExitCode:    0,
		Command:     "echo already imported",
		Directory:   "/tmp",
		SessionID:   "session-1",
		Hostname:    "host-1",
	}

	if _, err = repo.Insert(existing); err != nil {
		t.Fatalf("Insert(existing) error: %v", err)
	}

	summary, err := importHistoryEntries(database, []db.HistoryEntry{existing})
	if err != nil {
		t.Fatalf("importHistoryEntries() error: %v", err)
	}

	if summary.imported != 0 {
		t.Fatalf("summary.imported = %d, want 0", summary.imported)
	}

	if summary.skippedDuplicate != 1 {
		t.Fatalf("summary.skippedDuplicate = %d, want 1", summary.skippedDuplicate)
	}

	entries, err := repo.ListAll()
	if err != nil {
		t.Fatalf("ListAll() error: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("ListAll() returned %d entries, want 1", len(entries))
	}
}

func TestImportHistoryEntriesUpdatesLatestCommandForNewestStagedRow(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "history.db")

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}

	defer func() { _ = database.Close() }()

	entries := []db.HistoryEntry{
		{TimestampMS: 1000, Command: "repeat", Directory: "/old"},
		{TimestampMS: 2000, Command: "repeat", Directory: "/new"},
	}

	summary, err := importHistoryEntries(database, entries)
	if err != nil {
		t.Fatalf("importHistoryEntries() error: %v", err)
	}

	if summary.imported != len(entries) {
		t.Fatalf("summary.imported = %d, want %d", summary.imported, len(entries))
	}

	var (
		timestampMS int64
		directory   string
	)

	row := database.QueryRowContext(context.Background(), `SELECT ts_ms, directory FROM latest_command WHERE command = ?`, "repeat")
	if err = row.Scan(&timestampMS, &directory); err != nil {
		t.Fatalf("reading latest_command: %v", err)
	}

	if timestampMS != 2000 || directory != "/new" {
		t.Fatalf("latest_command = (%d, %q), want (2000, /new)", timestampMS, directory)
	}
}

func TestImportHistoryEntriesFiltersBeforeStaging(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "history.db")

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}

	defer func() { _ = database.Close() }()

	workingDirectory := t.TempDir()
	entries := []db.HistoryEntry{
		{TimestampMS: 1, ExitCode: 1, Command: "echo failed", Directory: workingDirectory},
		{TimestampMS: 2, Command: "cat missing.txt", Directory: workingDirectory},
		{TimestampMS: 3, Command: "echo imported", Directory: workingDirectory},
	}

	summary, err := importHistoryEntries(database, entries)
	if err != nil {
		t.Fatalf("importHistoryEntries() error: %v", err)
	}

	if summary.total != len(entries) {
		t.Fatalf("summary.total = %d, want %d", summary.total, len(entries))
	}

	if summary.imported != 1 {
		t.Fatalf("summary.imported = %d, want 1", summary.imported)
	}

	if summary.skippedFailed != 1 {
		t.Fatalf("summary.skippedFailed = %d, want 1", summary.skippedFailed)
	}

	if summary.skippedMissingPath != 1 {
		t.Fatalf("summary.skippedMissingPath = %d, want 1", summary.skippedMissingPath)
	}
}

func generatedImportSource(t *testing.T, check *rapid.T) ([]db.HistoryEntry, map[db.HistoryEntry]importPathCheckStatus) {
	t.Helper()
	dir := t.TempDir()

	name := "input-" + rapid.StringMatching("[a-z]{1,16}").Draw(check, "filename")
	if err := os.WriteFile(filepath.Join(dir, name), []byte("input"), 0o600); err != nil {
		check.Fatal(err)
	}

	pool := rapid.SliceOfN(historyRecordGenerator(), 1, 10).Draw(check, "pool")
	categories := make(map[db.HistoryEntry]importPathCheckStatus)

	for index := range pool {
		entry := &pool[index]
		entry.Directory = dir

		entry.ExitCode = rapid.IntRange(0, 2).Draw(check, "exit")
		switch rapid.IntRange(0, 3).Draw(check, "path-kind") {
		case 0:
			entry.Command = "cat " + name
			categories[*entry] = importPathExists
		case 1:
			entry.Command = "cat missing-" + name
			categories[*entry] = importPathMissing
		case 2:
			entry.Command = "cat ./" + strings.Repeat(name, 4096)
			categories[*entry] = importPathError
		case 3:
			entry.Command = "touch new-" + name
			categories[*entry] = importPathExists
		}
	}

	entries := rapid.SliceOfN(rapid.SampledFrom(pool), 1, 20).Draw(check, "source")
	entries = append(entries, entries[0])

	return entries, categories
}

func expectedImport(original, source []db.HistoryEntry, categories map[db.HistoryEntry]importPathCheckStatus, opts importOptions) (importSummary, []db.HistoryEntry) {
	summary := importSummary{total: len(source)}

	entries := append([]db.HistoryEntry(nil), original...)
	seen := historyRecordCounts(original)

	for _, entry := range source {
		switch {
		case entry.ExitCode != 0 && !opts.includeFailed:
			summary.skippedFailed++
		case !opts.includeMissingPaths && categories[entry] == importPathMissing:
			summary.skippedMissingPath++
		case !opts.includeMissingPaths && categories[entry] == importPathError:
			summary.skippedPathError++
		case seen[entry] > 0:
			summary.skippedDuplicate++
		default:
			summary.imported++
			seen[entry]++
			entries = append(entries, entry)
		}
	}

	return summary, entries
}

func assertImportedRecords(check *rapid.T, stored, original, expected []db.HistoryEntry) {
	check.Helper()

	if !reflect.DeepEqual(historyRecordCounts(stored), historyRecordCounts(expected)) {
		check.Fatalf("destination = %#v, want %#v", stored, expected)
	}

	byID := make(map[int64]db.HistoryEntry)
	for _, entry := range stored {
		byID[entry.ID] = entry
	}

	for _, entry := range original {
		if byID[entry.ID] != entry {
			check.Fatalf("original record changed: %#v", entry)
		}
	}
}

func TestImportConservationProperty(t *testing.T) {
	rapid.Check(t, func(check *rapid.T) {
		sourceEntries, categories := generatedImportSource(t, check)

		original := rapid.SliceOfN(historyRecordGenerator(), 0, 4).Draw(check, "original")
		if rapid.Bool().Draw(check, "existing-duplicate") {
			original = append(original, sourceEntries[0])
		}

		opts := importOptions{
			includeFailed:       rapid.Bool().Draw(check, "include-failed"),
			includeMissingPaths: rapid.Bool().Draw(check, "include-missing"),
		}
		want, wantEntries := expectedImport(original, sourceEntries, categories, opts)
		dir := t.TempDir()
		target := openPropertyHistory(check, filepath.Join(dir, "target.db"))
		original = storePropertyHistory(check, target, original)
		sourcePath := filepath.Join(dir, "source.db")
		source := openPropertyHistory(check, sourcePath)
		storePropertyHistory(check, source, sourceEntries)

		if err := source.Close(); err != nil {
			check.Fatal(err)
		}

		before, err := os.ReadFile(sourcePath)
		if err != nil {
			check.Fatal(err)
		}

		source, err = openImportSourceDatabase(sourcePath)
		if err != nil {
			check.Fatal(err)
		}

		got, err := importSourceHistoryEntries(target, source, opts)
		if closeErr := source.Close(); closeErr != nil {
			check.Fatal(closeErr)
		}

		if err != nil {
			check.Fatal(err)
		}

		if got != want || got.total != got.imported+got.skippedDuplicate+got.skippedFailed+got.skippedMissingPath+got.skippedPathError {
			check.Fatalf("summary = %#v, want %#v", got, want)
		}

		assertImportedRecords(check, readPropertyHistory(check, target), original, wantEntries)

		after, err := os.ReadFile(sourcePath)
		if err != nil || !bytes.Equal(before, after) {
			check.Fatalf("source database changed, error: %v", err)
		}
	})
}

func failingImportStream(check *rapid.T, database *sql.DB, entries []db.HistoryEntry, phase string) historyEntryStreamer {
	check.Helper()

	if phase == "history" {
		execPropertySQL(check, database, fmt.Sprintf(`CREATE TRIGGER reject_import BEFORE INSERT ON history WHEN NEW.command LIKE 'incoming-%d %%' BEGIN SELECT RAISE(ABORT, 'unavailable'); END`, len(entries)-1))
	}

	if phase == "cache" {
		execPropertySQL(check, database, `CREATE TRIGGER reject_cache BEFORE INSERT ON latest_command BEGIN SELECT RAISE(ABORT, 'unavailable'); END`)
	}

	failAt := rapid.IntRange(1, len(entries)-1).Draw(check, "fail-after")

	return func(_ context.Context, visit func(db.HistoryEntry) error) error {
		for index, entry := range entries {
			if phase == "stream" && index == failAt {
				return io.ErrUnexpectedEOF
			}

			if err := visit(entry); err != nil {
				return err
			}
		}

		return nil
	}
}

func TestImportRollbackProperty(t *testing.T) {
	rapid.Check(t, func(check *rapid.T) {
		database := openPropertyHistory(check, filepath.Join(t.TempDir(), "history.db"))
		original := rapid.SliceOfN(historyRecordGenerator(), 0, 8).Draw(check, "original")
		storePropertyHistory(check, database, original)
		before := readPropertyHistory(check, database)
		repo := db.NewHistoryRepo(database)

		beforeCandidates, err := repo.FetchCandidates(0, true, db.FailFilterInclude)
		if err != nil {
			check.Fatal(err)
		}

		entries := rapid.SliceOfN(historyRecordGenerator(), 2, 12).Draw(check, "source")
		for index := range entries {
			entries[index].Command = fmt.Sprintf("incoming-%d %s", index, entries[index].Command)
		}

		phase := rapid.SampledFrom([]string{"stream", "history", "cache"}).Draw(check, "failure-phase")
		stream := failingImportStream(check, database, entries, phase)

		_, err = importHistoryEntriesFromStreamer(database, importOptions{includeFailed: true, includeMissingPaths: true}, stream)
		if err == nil || (phase == "stream" && !errors.Is(err, io.ErrUnexpectedEOF)) {
			check.Fatalf("failed import error = %v", err)
		}

		after := readPropertyHistory(check, database)
		if !reflect.DeepEqual(after, before) {
			check.Fatalf("failed import changed history: %#v, want %#v", after, before)
		}

		afterCandidates, err := repo.FetchCandidates(0, true, db.FailFilterInclude)
		if err != nil || !reflect.DeepEqual(afterCandidates, beforeCandidates) {
			check.Fatalf("failed import changed candidates: %#v, want %#v, error: %v", afterCandidates, beforeCandidates, err)
		}
	})
}

func TestRunImportReadableSourceImportsEntries(t *testing.T) {
	setCLITestHomes(t)

	sourcePath := filepath.Join(t.TempDir(), "source.db")

	sourceDB, err := db.Open(sourcePath)
	if err != nil {
		t.Fatalf("Open(source) error: %v", err)
	}

	sourceRepo := db.NewHistoryRepo(sourceDB)
	if _, err = sourceRepo.Insert(db.HistoryEntry{TimestampMS: 1000, Command: "echo imported"}); err != nil {
		_ = sourceDB.Close()

		t.Fatalf("Insert(source) error: %v", err)
	}

	if err = sourceDB.Close(); err != nil {
		t.Fatalf("Close(source) error: %v", err)
	}

	cmd := &cobra.Command{}
	cmd.Flags().Bool("include-failed", false, "")
	cmd.Flags().Bool("include-missing-paths", false, "")

	var stdout bytes.Buffer
	cmd.SetErr(&stdout)

	if err = runImport(cmd, []string{sourcePath}); err != nil {
		t.Fatalf("runImport() error: %v", err)
	}

	targetPath, err := resolveTargetImportPath(&cobra.Command{})
	if err != nil {
		t.Fatalf("resolveTargetImportPath() error: %v", err)
	}

	targetDB, err := db.OpenReadOnly(targetPath)
	if err != nil {
		t.Fatalf("OpenReadOnly(target) error: %v", err)
	}
	defer func() { _ = targetDB.Close() }()

	targetRepo := db.NewHistoryRepo(targetDB)

	entries, err := targetRepo.Recent(10)
	if err != nil {
		t.Fatalf("Recent() error: %v", err)
	}

	if len(entries) != 1 || entries[0].Command != "echo imported" {
		t.Fatalf("imported entries = %+v, want 1 entry with 'echo imported'", entries)
	}

	if !strings.Contains(stdout.String(), "imported=1") {
		t.Fatalf("runImport output = %q, want imported=1", stdout.String())
	}
}

func TestRunImportDoesNotCreateTargetForInvalidSource(t *testing.T) {
	setCLITestHomes(t)

	sourcePath := filepath.Join(t.TempDir(), "source.db")
	if err := os.WriteFile(sourcePath, []byte("not sqlite"), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) error: %v", sourcePath, err)
	}

	targetPath, err := resolveTargetImportPath(&cobra.Command{})
	if err != nil {
		t.Fatalf("resolveTargetImportPath() error: %v", err)
	}

	cmd := &cobra.Command{}
	cmd.Flags().Bool("include-failed", false, "")
	cmd.Flags().Bool("include-missing-paths", false, "")

	var stdout bytes.Buffer
	cmd.SetErr(&stdout)

	err = runImport(cmd, []string{sourcePath})
	if err == nil {
		t.Fatal("runImport() error = nil, want invalid source error")
	}

	if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
		t.Fatalf("target database should not be created, stat err = %v", err)
	}
}
