package cli

import (
	"os"
	"path/filepath"
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
