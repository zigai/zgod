package cli

import (
	"context"
	"database/sql"
	"embed"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	"pgregory.net/rapid"

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
		TimestampMS: 2,
		Command:     "recorded after lock",
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
		TimestampMS: 10,
		Command:     "queued while lock held",
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
		TimestampMS: 20,
		Command:     "recorded after queue",
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

func TestPendingRecordErrorsDoNotBlockRecordingOrSearch(t *testing.T) {
	for _, kind := range []string{"malformed", "unreadable"} {
		t.Run(kind, func(t *testing.T) {
			dbPath := filepath.Join(t.TempDir(), "history.db")

			dir := pendingRecordsDir(dbPath)
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}

			badPath := filepath.Join(dir, "000-broken.json")
			if kind == "malformed" {
				if err := os.WriteFile(badPath, []byte("{unfinished"), 0o600); err != nil {
					t.Fatal(err)
				}
			} else if err := os.Symlink(filepath.Join(dir, "missing"), badPath); err != nil {
				t.Fatal(err)
			}

			queued := db.HistoryEntry{TimestampMS: 1, Command: "queued", DurationMS: 3, Directory: "/work", SessionID: "session", Hostname: "host"}
			current := db.HistoryEntry{TimestampMS: 2, Command: "current", ExitCode: 7}

			if err := queuePendingRecord(dbPath, queued); err != nil {
				t.Fatal(err)
			}

			if err := insertRecordWithRetry(dbPath, current); err != nil {
				t.Fatalf("recording behind a %s file: %v", kind, err)
			}

			if err := drainPendingRecordsForSearch(dbPath); err != nil {
				t.Fatalf("search behind a %s file: %v", kind, err)
			}

			entries := readPropertyHistory(t, openPropertyHistory(t, dbPath))
			for index := range entries {
				entries[index].ID = 0
			}

			if !reflect.DeepEqual(entries, []db.HistoryEntry{queued, current}) {
				t.Fatalf("history = %#v, want queued and current records", entries)
			}

			if _, err := os.Lstat(badPath); err != nil {
				t.Fatalf("unrecoverable pending file: %v", err)
			}
		})
	}
}

func TestPendingRecordRemovalFailureDoesNotReplay(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix directory permissions are required to deny removal")
	}

	dbPath := filepath.Join(t.TempDir(), "history.db")

	entry := db.HistoryEntry{TimestampMS: 1, Command: "pending", DurationMS: 5, ExitCode: 2, Directory: "/work", SessionID: "session", Hostname: "host"}
	if err := queuePendingRecord(dbPath, entry); err != nil {
		t.Fatal(err)
	}

	dir := pendingRecordsDir(dbPath)

	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	if err = os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	if err = flushPendingRecordLocked(dbPath, filepath.Join(dir, files[0].Name())); err == nil {
		t.Fatal("expected removal failure")
	}

	if err = os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}

	if err = drainPendingRecordsForSearch(dbPath); err != nil {
		t.Fatal(err)
	}

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = database.Close() }()

	entries, err := db.NewHistoryRepo(database).ListAll()
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) != 1 {
		t.Fatalf("history has %d records after retry, want 1", len(entries))
	}

	entries[0].ID = 0
	if entries[0] != entry {
		t.Fatalf("history = %#v, want %#v", entries[0], entry)
	}

	assertPendingRecordCount(t, dbPath, 0)
}

//go:embed testdata/characters.txt
var historyFixtureFS embed.FS

type historyTestReporter interface {
	Helper()
	Fatal(arguments ...any)
	Error(arguments ...any)
	Cleanup(cleanup func())
}

func openPropertyHistory(check historyTestReporter, path string) *sql.DB {
	check.Helper()

	database, err := db.Open(path)
	if err != nil {
		check.Fatal(err)
	}

	check.Cleanup(func() {
		if err := database.Close(); err != nil {
			check.Error(err)
		}
	})

	return database
}

func storePropertyHistory(check historyTestReporter, database *sql.DB, entries []db.HistoryEntry) []db.HistoryEntry {
	check.Helper()

	stored := append([]db.HistoryEntry(nil), entries...)
	for index, entry := range entries {
		id, err := db.NewHistoryRepo(database).Insert(entry)
		if err != nil {
			check.Fatal(err)
		}

		stored[index].ID = id
	}

	return stored
}

func readPropertyHistory(check historyTestReporter, database *sql.DB) []db.HistoryEntry {
	check.Helper()

	entries, err := db.NewHistoryRepo(database).ListAll()
	if err != nil {
		check.Fatal(err)
	}

	return entries
}

func execPropertySQL(check historyTestReporter, database *sql.DB, statement string) {
	check.Helper()

	if _, err := database.ExecContext(context.Background(), statement); err != nil {
		check.Fatal(err)
	}
}

func historyTextRunes(check historyTestReporter) []rune {
	check.Helper()

	data, err := historyFixtureFS.ReadFile("testdata/characters.txt")
	if err != nil {
		check.Fatal(err)
	}

	return []rune(string(data))
}

func historyRecordGenerator() *rapid.Generator[db.HistoryEntry] {
	return rapid.Custom(func(check *rapid.T) db.HistoryEntry {
		text := rapid.StringOfN(rapid.RuneFrom(historyTextRunes(check)), 0, 32, -1)

		return db.HistoryEntry{
			TimestampMS: rapid.Int64Range(0, 4_000_000_000_000).Draw(check, "timestamp"),
			DurationMS:  rapid.Int64Range(0, 1_000_000).Draw(check, "duration"),
			ExitCode:    rapid.IntRange(0, 255).Draw(check, "exit"),
			Command:     "command " + text.Draw(check, "command"),
			Directory:   text.Draw(check, "directory"),
			SessionID:   text.Draw(check, "session"),
			Hostname:    text.Draw(check, "hostname"),
		}
	})
}

func historyRecordCounts(entries []db.HistoryEntry) map[db.HistoryEntry]int {
	counts := make(map[db.HistoryEntry]int)

	for _, entry := range entries {
		entry.ID = 0
		counts[entry]++
	}

	return counts
}

func preparePendingRecords(check *rapid.T, dbPath string, entries []db.HistoryEntry) map[string][]byte {
	check.Helper()

	for _, entry := range entries {
		if err := queuePendingRecord(dbPath, entry); err != nil {
			check.Fatal(err)
		}
	}

	dir := pendingRecordsDir(dbPath)

	protected := map[string][]byte{
		"000-broken.json":      []byte("{unfinished"),
		".tmp-unfinished.json": rapid.SliceOfN(rapid.Byte(), 0, 64).Draw(check, "unfinished"),
		"notes":                rapid.SliceOfN(rapid.Byte(), 0, 64).Draw(check, "unrelated"),
	}
	for name, content := range protected {
		if err := os.WriteFile(filepath.Join(dir, name), content, 0o600); err != nil {
			check.Fatal(err)
		}
	}

	return protected
}

func assertProtectedPendingFiles(check *rapid.T, dir string, protected map[string][]byte) {
	check.Helper()

	for name, content := range protected {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || string(data) != string(content) {
			check.Fatalf("protected file %q changed: %q, error: %v", name, data, err)
		}
	}
}

func TestPendingRecordDurabilityProperty(t *testing.T) {
	rapid.Check(t, func(check *rapid.T) {
		dbPath := filepath.Join(t.TempDir(), "history.db")
		entries := rapid.SliceOfN(historyRecordGenerator(), 1, 8).Draw(check, "entries")
		entries = append(entries, entries[0])
		protected := preparePendingRecords(check, dbPath, entries)
		dir := pendingRecordsDir(dbPath)
		database := openPropertyHistory(check, dbPath)
		execPropertySQL(check, database, `CREATE TRIGGER reject_record BEFORE INSERT ON history BEGIN SELECT RAISE(ABORT, 'unavailable'); END`)

		if err := drainPendingRecordsForSearch(dbPath); err == nil {
			check.Fatal("persistence failure was not reported")
		}

		got := readPropertyHistory(check, database)
		if len(got) != 0 {
			check.Fatalf("failed drain stored records: %#v", got)
		}

		files, err := os.ReadDir(dir)
		if err != nil || len(files) != len(entries)+len(protected) {
			check.Fatalf("failed drain lost files: %v, error: %v", files, err)
		}

		execPropertySQL(check, database, `DROP TRIGGER reject_record`)

		for range rapid.IntRange(2, 4).Draw(check, "drains") {
			if err = drainPendingRecordsForSearch(dbPath); err != nil {
				check.Fatal(err)
			}
		}

		got = readPropertyHistory(check, database)
		if !reflect.DeepEqual(historyRecordCounts(got), historyRecordCounts(entries)) {
			check.Fatalf("stored records = %#v, want %#v", got, entries)
		}

		assertProtectedPendingFiles(check, dir, protected)

		var receipts int
		if err = database.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM pending_record`).Scan(&receipts); err != nil || receipts != 0 {
			check.Fatalf("finished queue retained %d receipts, error: %v", receipts, err)
		}
	})
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
