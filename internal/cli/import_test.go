package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

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
	cmd.SetOut(&stdout)

	if err = runImport(cmd, []string{sourcePath}); err != nil {
		t.Fatalf("runImport() error: %v", err)
	}

	targetPath, err := resolveTargetImportPath()
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

	targetPath, err := resolveTargetImportPath()
	if err != nil {
		t.Fatalf("resolveTargetImportPath() error: %v", err)
	}

	cmd := &cobra.Command{}
	cmd.Flags().Bool("include-failed", false, "")
	cmd.Flags().Bool("include-missing-paths", false, "")

	var stdout bytes.Buffer
	cmd.SetOut(&stdout)

	err = runImport(cmd, []string{sourcePath})
	if err == nil {
		t.Fatal("runImport() error = nil, want invalid source error")
	}

	if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
		t.Fatalf("target database should not be created, stat err = %v", err)
	}
}
