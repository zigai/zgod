package db

import (
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
