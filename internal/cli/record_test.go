package cli

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/zigai/zgod/internal/db"
)

func TestInsertRecordWithRetryWaitsForBusyDatabase(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "history.db")

	locker, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open(locker) error: %v", err)
	}

	defer func() { _ = locker.Close() }()

	tx, err := locker.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("BeginTx() error: %v", err)
	}

	_, err = tx.ExecContext(
		context.Background(),
		`INSERT INTO history (ts_ms, duration, exit_code, command, directory, session_id, hostname)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		1, 0, 0, "held write lock", "", "", "",
	)
	if err != nil {
		t.Fatalf("seeding write transaction: %v", err)
	}

	committed := make(chan struct{})

	go func() {
		time.Sleep(2500 * time.Millisecond)

		_ = tx.Commit()

		close(committed)
	}()

	err = insertRecordWithRetry(dbPath, db.HistoryEntry{
		TSMs:    2,
		Command: "recorded after lock",
	})
	if err != nil {
		t.Fatalf("insertRecordWithRetry() error: %v", err)
	}

	<-committed

	repo := db.NewHistoryRepo(locker)

	entries, err := repo.ListAll()
	if err != nil {
		t.Fatalf("ListAll() error: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("ListAll() returned %d entries, want 2", len(entries))
	}

	if got, want := entries[1].Command, "recorded after lock"; got != want {
		t.Fatalf("entries[1].Command = %q, want %q", got, want)
	}
}
