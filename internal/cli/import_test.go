package cli

import (
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
	if writeErr := os.WriteFile(inputPath, []byte("a\n"), 0o600); writeErr != nil {
		t.Fatalf("WriteFile() error: %v", writeErr)
	}

	entry := db.HistoryEntry{
		TsMs:      1,
		Duration:  10,
		ExitCode:  0,
		Command:   `sed 's/a/b/' file.txt`,
		Directory: workingDirectory,
		SessionID: "session-1",
		Hostname:  "host-1",
	}

	summary, err := importHistoryEntries(database, []db.HistoryEntry{entry}, importOptions{})
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
		{TsMs: 1, Command: "touch new.txt", Directory: workingDirectory},
		{TsMs: 2, Command: "mkdir out", Directory: workingDirectory},
		{TsMs: 3, Command: "echo README.md", Directory: workingDirectory},
	}

	summary, err := importHistoryEntries(database, entries, importOptions{})
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
		{TsMs: 1, Command: "cd missing", Directory: workingDirectory},
		{TsMs: 2, Command: `sed 's/a/b/' missing.txt`, Directory: workingDirectory},
	}

	summary, err := importHistoryEntries(database, entries, importOptions{})
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

func TestOpenImportDatabasesReadableSourceDoesNotRequireAuth(t *testing.T) {
	setImportHomes(t)

	sourcePath := filepath.Join(t.TempDir(), "source.db")

	sourceDB, err := db.Open(sourcePath)
	if err != nil {
		t.Fatalf("Open(source) error: %v", err)
	}

	sourceRepo := db.NewHistoryRepo(sourceDB)
	if _, err = sourceRepo.Insert(db.HistoryEntry{TsMs: 1000, Command: "echo imported"}); err != nil {
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

	entries, err := listSourceEntries(readOnlySourceDB)
	if err != nil {
		t.Fatalf("listSourceEntries() error: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("len(listSourceEntries()) = %d, want 1", len(entries))
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
