package cli

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
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

func TestOpenSearchDatabaseRejectsExistingEmptyDatabase(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "empty.db")

	if err := os.WriteFile(dbPath, nil, 0o600); err != nil {
		t.Fatalf("creating empty database file: %v", err)
	}

	database, err := openSearchDatabase(dbPath)
	if err == nil {
		_ = database.Close()

		t.Fatal("openSearchDatabase() should reject an existing empty database")
	}

	if !strings.Contains(err.Error(), "validating history schema") {
		t.Fatalf("openSearchDatabase() error = %v, want history schema validation error", err)
	}
}

func TestOpenSearchDatabaseRejectsExistingWrongSchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "wrong-schema.db")

	database, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open() error: %v", err)
	}

	if _, err = database.ExecContext(
		context.Background(),
		`CREATE TABLE history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			command TEXT NOT NULL
		)`,
	); err != nil {
		t.Fatalf("creating wrong history schema: %v", err)
	}

	if err = database.Close(); err != nil {
		t.Fatalf("database.Close() error: %v", err)
	}

	searchDB, err := openSearchDatabase(dbPath)
	if err == nil {
		_ = searchDB.Close()

		t.Fatal("openSearchDatabase() should reject an existing database with the wrong schema")
	}

	if !strings.Contains(err.Error(), "validating history schema") {
		t.Fatalf("openSearchDatabase() error = %v, want history schema validation error", err)
	}
}
