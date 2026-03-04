package db

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

func TestOpenAndInsert(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	database, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}

	defer func() { _ = database.Close() }()

	repo := NewHistoryRepo(database)

	id, err := repo.Insert(HistoryEntry{
		TsMs:      1000,
		Duration:  50,
		ExitCode:  0,
		Command:   "echo hello",
		Directory: "/tmp",
		SessionID: "test-session",
		Hostname:  "test-host",
	})
	if err != nil {
		t.Fatalf("Insert() error: %v", err)
	}

	if id < 1 {
		t.Errorf("Insert() returned id=%d, want >= 1", id)
	}
}

func TestRecent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	database, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}

	defer func() { _ = database.Close() }()

	repo := NewHistoryRepo(database)

	for i, cmd := range []string{"first", "second", "third"} {
		if _, err = repo.Insert(HistoryEntry{
			TsMs:    int64(i * 1000),
			Command: cmd,
		}); err != nil {
			t.Fatal(err)
		}
	}

	entries, err := repo.Recent(10)
	if err != nil {
		t.Fatalf("Recent() error: %v", err)
	}

	if len(entries) != 3 {
		t.Fatalf("Recent() returned %d entries, want 3", len(entries))
	}

	if entries[0].Command != "third" {
		t.Errorf("Recent()[0].Command = %q, want 'third'", entries[0].Command)
	}
}

func TestDelete(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	database, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}

	defer func() { _ = database.Close() }()

	repo := NewHistoryRepo(database)

	id, err := repo.Insert(HistoryEntry{TsMs: 1000, Command: "delete me"})
	if err != nil {
		t.Fatal(err)
	}

	if err = repo.Delete(id); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}

	entries, _ := repo.Recent(10)
	for _, e := range entries {
		if e.ID == id {
			t.Error("deleted entry should not appear in Recent()")
		}
	}
}

func TestRecentInDir(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	database, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}

	defer func() { _ = database.Close() }()

	repo := NewHistoryRepo(database)
	if _, err = repo.Insert(HistoryEntry{TsMs: 1000, Command: "in dir", Directory: "/home"}); err != nil {
		t.Fatal(err)
	}

	if _, err = repo.Insert(HistoryEntry{TsMs: 2000, Command: "other dir", Directory: "/tmp"}); err != nil {
		t.Fatal(err)
	}

	entries, err := repo.RecentInDir("/home", 10)
	if err != nil {
		t.Fatalf("RecentInDir() error: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("RecentInDir() returned %d entries, want 1", len(entries))
	}

	if entries[0].Command != "in dir" {
		t.Errorf("got command %q, want 'in dir'", entries[0].Command)
	}
}

func TestFetchCandidatesDedupe(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	database, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}

	defer func() { _ = database.Close() }()

	repo := NewHistoryRepo(database)
	if _, err = repo.Insert(HistoryEntry{TsMs: 1000, Command: "echo hello"}); err != nil {
		t.Fatal(err)
	}

	if _, err = repo.Insert(HistoryEntry{TsMs: 2000, Command: "echo hello"}); err != nil {
		t.Fatal(err)
	}

	if _, err = repo.Insert(HistoryEntry{TsMs: 3000, Command: "echo world"}); err != nil {
		t.Fatal(err)
	}

	entries, _ := repo.FetchCandidates(100, true, false)
	if len(entries) != 2 {
		t.Errorf("FetchCandidates(dedupe=true) returned %d entries, want 2", len(entries))
	}
}

func TestOpenReadOnlyMissingFile(t *testing.T) {
	missingPath := filepath.Join(t.TempDir(), "missing.db")

	_, err := OpenReadOnly(missingPath)
	if err == nil {
		t.Fatal("OpenReadOnly() should fail for missing file")
	}
}

func TestValidateHistorySchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "schema.db")

	database, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}

	defer func() { _ = database.Close() }()

	if err = ValidateHistorySchema(database); err != nil {
		t.Fatalf("ValidateHistorySchema() error: %v", err)
	}
}

func TestValidateHistorySchemaMissingColumns(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "bad-schema.db")

	database, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open() error: %v", err)
	}

	defer func() { _ = database.Close() }()

	_, err = database.ExecContext(
		context.Background(),
		`CREATE TABLE IF NOT EXISTS history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			ts_ms INTEGER NOT NULL,
			command TEXT NOT NULL
		)`,
	)
	if err != nil {
		t.Fatalf("creating schema for test: %v", err)
	}

	err = ValidateHistorySchema(database)
	if err == nil {
		t.Fatal("ValidateHistorySchema() should fail when required columns are missing")
	}

	if !errors.Is(err, errHistoryColumnsMissing) {
		t.Fatalf("expected errHistoryColumnsMissing, got: %v", err)
	}
}

func TestValidateHistorySchemaMissingIDColumn(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "bad-schema-missing-id.db")

	database, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open() error: %v", err)
	}

	defer func() { _ = database.Close() }()

	_, err = database.ExecContext(
		context.Background(),
		`CREATE TABLE IF NOT EXISTS history (
			ts_ms INTEGER NOT NULL,
			duration INTEGER NOT NULL DEFAULT 0,
			exit_code INTEGER NOT NULL DEFAULT 0,
			command TEXT NOT NULL,
			directory TEXT NOT NULL DEFAULT '',
			session_id TEXT NOT NULL DEFAULT '',
			hostname TEXT NOT NULL DEFAULT ''
		)`,
	)
	if err != nil {
		t.Fatalf("creating schema for test: %v", err)
	}

	err = ValidateHistorySchema(database)
	if err == nil {
		t.Fatal("ValidateHistorySchema() should fail when id column is missing")
	}

	if !errors.Is(err, errHistoryColumnsMissing) {
		t.Fatalf("expected errHistoryColumnsMissing, got: %v", err)
	}
}

func TestListAll(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "history.db")

	database, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}

	defer func() { _ = database.Close() }()

	repo := NewHistoryRepo(database)

	first := HistoryEntry{
		TsMs:      2000,
		Duration:  50,
		ExitCode:  0,
		Command:   "echo first",
		Directory: "/tmp",
		SessionID: "session-1",
		Hostname:  "host-1",
	}
	second := HistoryEntry{
		TsMs:      1000,
		Duration:  10,
		ExitCode:  1,
		Command:   "echo second",
		Directory: "/home",
		SessionID: "session-2",
		Hostname:  "host-2",
	}

	if _, err = repo.Insert(first); err != nil {
		t.Fatalf("Insert(first) error: %v", err)
	}

	if _, err = repo.Insert(second); err != nil {
		t.Fatalf("Insert(second) error: %v", err)
	}

	entries, err := repo.ListAll()
	if err != nil {
		t.Fatalf("ListAll() error: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("ListAll() returned %d entries, want 2", len(entries))
	}

	if entries[0].TsMs != 1000 || entries[1].TsMs != 2000 {
		t.Fatalf("ListAll() returned unexpected order: %+v", entries)
	}
}

func TestInsertIfNotExistsTx(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "history.db")

	database, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}

	defer func() { _ = database.Close() }()

	repo := NewHistoryRepo(database)
	existing := HistoryEntry{
		TsMs:      2000,
		Duration:  50,
		ExitCode:  0,
		Command:   "echo first",
		Directory: "/tmp",
		SessionID: "session-1",
		Hostname:  "host-1",
	}

	if _, err = repo.Insert(existing); err != nil {
		t.Fatalf("Insert(existing) error: %v", err)
	}

	tx, err := database.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("BeginTx() error: %v", err)
	}

	inserted, err := InsertIfNotExistsTx(tx, existing)
	if err != nil {
		_ = tx.Rollback()

		t.Fatalf("InsertIfNotExistsTx(existing) error: %v", err)
	}

	if inserted {
		_ = tx.Rollback()

		t.Fatal("InsertIfNotExistsTx(existing) inserted duplicate row")
	}

	newEntry := HistoryEntry{
		TsMs:      3000,
		Duration:  20,
		ExitCode:  0,
		Command:   "echo third",
		Directory: "/var",
		SessionID: "session-3",
		Hostname:  "host-3",
	}

	inserted, err = InsertIfNotExistsTx(tx, newEntry)
	if err != nil {
		_ = tx.Rollback()

		t.Fatalf("InsertIfNotExistsTx(new) error: %v", err)
	}

	if !inserted {
		_ = tx.Rollback()

		t.Fatal("InsertIfNotExistsTx(new) did not insert new row")
	}

	if err = tx.Commit(); err != nil {
		t.Fatalf("Commit() error: %v", err)
	}

	entries, err := repo.ListAll()
	if err != nil {
		t.Fatalf("ListAll() after commit error: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("ListAll() returned %d entries after commit, want 2", len(entries))
	}
}
