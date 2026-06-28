package cli

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

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
		TSMs:      1,
		Duration:  10,
		ExitCode:  0,
		Command:   `sed 's/a/b/' file.txt`,
		Directory: workingDirectory,
		SessionID: "session-1",
		Hostname:  "host-1",
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
		{TSMs: 1, Command: "touch new.txt", Directory: workingDirectory},
		{TSMs: 2, Command: "mkdir out", Directory: workingDirectory},
		{TSMs: 3, Command: "echo README.md", Directory: workingDirectory},
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
		{TSMs: 1, Command: "cd missing", Directory: workingDirectory},
		{TSMs: 2, Command: `sed 's/a/b/' missing.txt`, Directory: workingDirectory},
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
		TSMs:      1000,
		Duration:  10,
		ExitCode:  0,
		Command:   "echo duplicate",
		Directory: "/tmp",
		SessionID: "session-1",
		Hostname:  "host-1",
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
		TSMs:      1000,
		Duration:  10,
		ExitCode:  0,
		Command:   "echo already imported",
		Directory: "/tmp",
		SessionID: "session-1",
		Hostname:  "host-1",
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
		{TSMs: 1000, Command: "repeat", Directory: "/old"},
		{TSMs: 2000, Command: "repeat", Directory: "/new"},
	}

	summary, err := importHistoryEntries(database, entries)
	if err != nil {
		t.Fatalf("importHistoryEntries() error: %v", err)
	}

	if summary.imported != len(entries) {
		t.Fatalf("summary.imported = %d, want %d", summary.imported, len(entries))
	}

	var (
		tsMs      int64
		directory string
	)

	row := database.QueryRowContext(context.Background(), `SELECT ts_ms, directory FROM latest_command WHERE command = ?`, "repeat")
	if err = row.Scan(&tsMs, &directory); err != nil {
		t.Fatalf("reading latest_command: %v", err)
	}

	if tsMs != 2000 || directory != "/new" {
		t.Fatalf("latest_command = (%d, %q), want (2000, /new)", tsMs, directory)
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
		{TSMs: 1, ExitCode: 1, Command: "echo failed", Directory: workingDirectory},
		{TSMs: 2, Command: "cat missing.txt", Directory: workingDirectory},
		{TSMs: 3, Command: "echo imported", Directory: workingDirectory},
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

func TestOpenImportDatabasesReadableSourceDoesNotRequireAuth(t *testing.T) {
	setImportHomes(t)

	sourcePath := filepath.Join(t.TempDir(), "source.db")

	sourceDB, err := db.Open(sourcePath)
	if err != nil {
		t.Fatalf("Open(source) error: %v", err)
	}

	sourceRepo := db.NewHistoryRepo(sourceDB)
	if _, err = sourceRepo.Insert(db.HistoryEntry{TSMs: 1000, Command: "echo imported"}); err != nil {
		_ = sourceDB.Close()

		t.Fatalf("Insert(source) error: %v", err)
	}

	if err = sourceDB.Close(); err != nil {
		t.Fatalf("Close(source) error: %v", err)
	}

	targetPath := filepath.Join(t.TempDir(), "target.db")

	targetDB, readOnlySourceDB, err := openImportDatabases(targetPath, sourcePath)
	if err != nil {
		t.Fatalf("openImportDatabases() error: %v", err)
	}

	defer closeImportDatabases(targetDB, readOnlySourceDB)

	summary, err := importSourceHistoryEntries(targetDB, readOnlySourceDB, importOptions{})
	if err != nil {
		t.Fatalf("importSourceHistoryEntries() error: %v", err)
	}

	if summary.imported != 1 {
		t.Fatalf("summary.imported = %d, want 1", summary.imported)
	}
}

func TestOpenImportDatabasesDoesNotCreateTargetForInvalidSource(t *testing.T) {
	setImportHomes(t)

	sourcePath := filepath.Join(t.TempDir(), "source.db")
	if err := os.WriteFile(sourcePath, []byte("not sqlite"), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) error: %v", sourcePath, err)
	}

	targetPath := filepath.Join(t.TempDir(), "target.db")

	targetDB, sourceDB, err := openImportDatabases(targetPath, sourcePath)
	if err == nil {
		closeImportDatabases(targetDB, sourceDB)
		t.Fatal("openImportDatabases() error = nil, want invalid source error")
	}

	if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
		t.Fatalf("target database should not be created, stat err = %v", err)
	}
}

func setImportHomes(t *testing.T) {
	t.Helper()

	baseDir := t.TempDir()

	if runtime.GOOS == "windows" {
		t.Setenv("APPDATA", filepath.Join(baseDir, "config"))
		t.Setenv("LOCALAPPDATA", filepath.Join(baseDir, "data"))

		return
	}

	t.Setenv("XDG_CONFIG_HOME", filepath.Join(baseDir, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(baseDir, "data"))
}
