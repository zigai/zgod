package cli

import (
	"path/filepath"
	"testing"

	"github.com/zigai/zgod/internal/db"
)

func TestOpenSearchDatabaseCreatesMissingDatabase(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "missing.db")

	database, err := openSearchDatabase(dbPath)
	if err != nil {
		t.Fatalf("openSearchDatabase() error: %v", err)
	}

	defer func() { _ = database.Close() }()

	if err = db.ValidateHistorySchema(database); err != nil {
		t.Fatalf("ValidateHistorySchema() error: %v", err)
	}
}

func TestOpenSearchDatabaseReadsExistingDatabase(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "history.db")

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open() error: %v", err)
	}

	repo := db.NewHistoryRepo(database)
	if _, err = repo.Insert(db.HistoryEntry{TSMs: 1000, Command: "echo ok"}); err != nil {
		t.Fatalf("repo.Insert() error: %v", err)
	}

	if err = database.Close(); err != nil {
		t.Fatalf("database.Close() error: %v", err)
	}

	searchDB, err := openSearchDatabase(dbPath)
	if err != nil {
		t.Fatalf("openSearchDatabase() error: %v", err)
	}

	defer func() { _ = searchDB.Close() }()

	entries, err := db.NewHistoryRepo(searchDB).FetchCandidates(10, false, db.FailFilterInclude)
	if err != nil {
		t.Fatalf("FetchCandidates() error: %v", err)
	}

	if len(entries) != 1 || entries[0].Command != "echo ok" {
		t.Fatalf("FetchCandidates() = %+v, want echo ok", entries)
	}
}
