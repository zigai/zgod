package cli

import (
	"context"
	"os"
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

func TestInsertRecordWithRetryQueuesAndFlushesWhenWriteLockUnavailable(t *testing.T) {
	originalTimeout := recordWriteLockTimeout
	recordWriteLockTimeout = 50 * time.Millisecond

	t.Cleanup(func() {
		recordWriteLockTimeout = originalTimeout
	})

	dbPath := filepath.Join(t.TempDir(), "history.db")
	releaseLock := make(chan struct{})
	lockAcquired := make(chan struct{})
	lockDone := make(chan error, 1)

	go func() {
		err := db.WithDatabaseWriteLock(context.Background(), dbPath, func() error {
			close(lockAcquired)
			<-releaseLock

			return nil
		})
		lockDone <- err
	}()

	<-lockAcquired

	err := insertRecordWithRetry(dbPath, db.HistoryEntry{
		TSMs:    10,
		Command: "queued while lock held",
	})
	if err != nil {
		t.Fatalf("insertRecordWithRetry() while lock held error: %v", err)
	}

	assertPendingRecordCount(t, dbPath, 1)

	close(releaseLock)

	if err = <-lockDone; err != nil {
		t.Fatalf("releasing test database write lock: %v", err)
	}

	err = insertRecordWithRetry(dbPath, db.HistoryEntry{
		TSMs:    20,
		Command: "recorded after queue",
	})
	if err != nil {
		t.Fatalf("insertRecordWithRetry() after lock release error: %v", err)
	}

	assertPendingRecordCount(t, dbPath, 0)

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open() error: %v", err)
	}

	defer func() { _ = database.Close() }()

	entries, err := db.NewHistoryRepo(database).ListAll()
	if err != nil {
		t.Fatalf("ListAll() error: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("ListAll() returned %d entries, want 2", len(entries))
	}

	if got, want := entries[0].Command, "queued while lock held"; got != want {
		t.Fatalf("entries[0].Command = %q, want %q", got, want)
	}

	if got, want := entries[1].Command, "recorded after queue"; got != want {
		t.Fatalf("entries[1].Command = %q, want %q", got, want)
	}
}

func assertPendingRecordCount(t *testing.T, dbPath string, want int) {
	t.Helper()

	entries, err := os.ReadDir(pendingRecordsDir(dbPath))
	if os.IsNotExist(err) && want == 0 {
		return
	}

	if err != nil {
		t.Fatalf("reading pending records: %v", err)
	}

	got := 0

	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == recordPendingFileExtension {
			got++
		}
	}

	if got != want {
		t.Fatalf("pending record count = %d, want %d", got, want)
	}
}
