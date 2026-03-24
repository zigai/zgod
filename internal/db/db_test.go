package db

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

	entries, _ := repo.FetchCandidates(100, true, FailFilterInclude)
	if len(entries) != 2 {
		t.Errorf("FetchCandidates(dedupe=true) returned %d entries, want 2", len(entries))
	}
}

func TestFetchCandidatesFailFilterModes(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	database, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}

	defer func() { _ = database.Close() }()

	repo := NewHistoryRepo(database)
	entries := []HistoryEntry{
		{TsMs: 1000, ExitCode: 0, Command: "echo ok one"},
		{TsMs: 2000, ExitCode: 1, Command: "echo fail one"},
		{TsMs: 3000, ExitCode: 0, Command: "echo ok two"},
		{TsMs: 4000, ExitCode: 2, Command: "echo fail two"},
	}

	for _, entry := range entries {
		if _, err = repo.Insert(entry); err != nil {
			t.Fatalf("Insert(%q) error: %v", entry.Command, err)
		}
	}

	tests := []struct {
		name       string
		mode       FailFilterMode
		wantCmds   []string
		dedupe     bool
		wantLength int
	}{
		{
			name:       "include",
			mode:       FailFilterInclude,
			wantCmds:   []string{"echo fail two", "echo ok two", "echo fail one", "echo ok one"},
			wantLength: 4,
		},
		{
			name:       "exclude",
			mode:       FailFilterExclude,
			wantCmds:   []string{"echo ok two", "echo ok one"},
			wantLength: 2,
		},
		{
			name:       "only",
			mode:       FailFilterOnly,
			wantCmds:   []string{"echo fail two", "echo fail one"},
			wantLength: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, fetchErr := repo.FetchCandidates(100, tc.dedupe, tc.mode)
			if fetchErr != nil {
				t.Fatalf("FetchCandidates() error: %v", fetchErr)
			}

			if len(got) != tc.wantLength {
				t.Fatalf("len(FetchCandidates()) = %d, want %d", len(got), tc.wantLength)
			}

			for i, want := range tc.wantCmds {
				if got[i].Command != want {
					t.Fatalf("FetchCandidates()[%d].Command = %q, want %q", i, got[i].Command, want)
				}
			}
		})
	}
}

func TestFetchCandidatesAppliesDedupeAfterFailFilter(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	database, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}

	defer func() { _ = database.Close() }()

	repo := NewHistoryRepo(database)
	entries := []HistoryEntry{
		{TsMs: 1000, ExitCode: 1, Command: "echo boom"},
		{TsMs: 2000, ExitCode: 2, Command: "echo boom"},
		{TsMs: 3000, ExitCode: 0, Command: "echo ok"},
	}

	for _, entry := range entries {
		if _, err = repo.Insert(entry); err != nil {
			t.Fatalf("Insert(%q) error: %v", entry.Command, err)
		}
	}

	got, err := repo.FetchCandidates(100, true, FailFilterOnly)
	if err != nil {
		t.Fatalf("FetchCandidates() error: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("len(FetchCandidates()) = %d, want 1", len(got))
	}

	if got[0].Command != "echo boom" {
		t.Fatalf("FetchCandidates()[0].Command = %q, want %q", got[0].Command, "echo boom")
	}
}

func TestOpenReadOnlyMissingFile(t *testing.T) {
	missingPath := filepath.Join(t.TempDir(), "missing.db")

	_, err := OpenReadOnly(missingPath)
	if err == nil {
		t.Fatal("OpenReadOnly() should fail for missing file")
	}
}

func TestOpenCreatesMissingParentDirectories(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "nested", "db", "history.db")

	database, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}

	defer func() { _ = database.Close() }()

	if _, err = os.Stat(dbPath); err != nil {
		t.Fatalf("expected database file to be created: %v", err)
	}
}

func TestOpenDirectoryPathDoesNotChangeDirectoryPermissions(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "history-dir")
	if err := os.MkdirAll(dbPath, 0o755); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}

	if err := os.Chmod(dbPath, 0o755); err != nil {
		t.Fatalf("Chmod() setup error: %v", err)
	}

	infoBefore, err := os.Stat(dbPath)
	if err != nil {
		t.Fatalf("Stat() before error: %v", err)
	}

	_, err = Open(dbPath)
	if err == nil {
		t.Fatal("Open() should fail for directory path")
	}

	if !errors.Is(err, errDatabasePathIsDirectory) {
		t.Fatalf("expected errDatabasePathIsDirectory, got %v", err)
	}

	infoAfter, err := os.Stat(dbPath)
	if err != nil {
		t.Fatalf("Stat() after error: %v", err)
	}

	if runtime.GOOS != "windows" && infoAfter.Mode().Perm() != infoBefore.Mode().Perm() {
		t.Fatalf("directory permissions changed from %o to %o", infoBefore.Mode().Perm(), infoAfter.Mode().Perm())
	}
}

func TestOpenReadOnlyAllowsReadsAndRejectsWrites(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "readonly.db")

	writableDB, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}

	repo := NewHistoryRepo(writableDB)
	if _, err = repo.Insert(HistoryEntry{TsMs: 1000, Command: "echo seeded"}); err != nil {
		_ = writableDB.Close()

		t.Fatalf("Insert() error: %v", err)
	}

	if err = writableDB.Close(); err != nil {
		t.Fatalf("Close() writable DB error: %v", err)
	}

	readOnlyDB, err := OpenReadOnly(dbPath)
	if err != nil {
		t.Fatalf("OpenReadOnly() error: %v", err)
	}

	defer func() { _ = readOnlyDB.Close() }()

	readOnlyRepo := NewHistoryRepo(readOnlyDB)

	entries, err := readOnlyRepo.ListAll()
	if err != nil {
		t.Fatalf("ListAll() error: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("ListAll() returned %d entries, want 1", len(entries))
	}

	_, err = readOnlyDB.ExecContext(
		context.Background(),
		`INSERT INTO history (ts_ms, duration, exit_code, command, directory, session_id, hostname)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		2000, 0, 0, "echo should fail", "", "", "",
	)
	if err == nil {
		t.Fatal("ExecContext() should fail for read-only database")
	}

	if !strings.Contains(strings.ToLower(err.Error()), "readonly") {
		t.Fatalf("expected readonly error, got: %v", err)
	}
}

func TestOpenReadOnlySupportsURIUnsafePathCharacters(t *testing.T) {
	dbDir := filepath.Join(t.TempDir(), "dir with spaces")
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}

	dbPath := filepath.Join(dbDir, "history #1.db")

	writableDB, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}

	if err = writableDB.Close(); err != nil {
		t.Fatalf("Close() writable DB error: %v", err)
	}

	readOnlyDB, err := OpenReadOnly(dbPath)
	if err != nil {
		t.Fatalf("OpenReadOnly() error: %v", err)
	}

	defer func() { _ = readOnlyDB.Close() }()

	if err = ValidateHistorySchema(readOnlyDB); err != nil {
		t.Fatalf("ValidateHistorySchema() error: %v", err)
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

func TestIsBusyErrorRecognizesSQLiteBusy(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "busy.db")
	ctx := context.Background()

	locker, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open(locker) error: %v", err)
	}

	defer func() { _ = locker.Close() }()

	blocked, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open(blocked) error: %v", err)
	}

	defer func() { _ = blocked.Close() }()

	if _, err = locker.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS busy_test(id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatalf("creating table for busy test: %v", err)
	}

	if _, err = blocked.ExecContext(ctx, "PRAGMA busy_timeout=0"); err != nil {
		t.Fatalf("setting busy timeout for busy test: %v", err)
	}

	if _, err = locker.ExecContext(ctx, "BEGIN EXCLUSIVE"); err != nil {
		t.Fatalf("starting exclusive transaction for busy test: %v", err)
	}

	defer func() {
		_, _ = locker.ExecContext(ctx, "ROLLBACK")
	}()

	_, err = blocked.ExecContext(ctx, "INSERT INTO busy_test(id) VALUES (1)")
	if err == nil {
		t.Fatal("expected SQLITE_BUSY error, got nil")
	}

	if !IsBusyError(err) {
		t.Fatalf("IsBusyError() = false, want true for error: %v", err)
	}
}

func TestIsBusyErrorReturnsFalseForNonBusyError(t *testing.T) {
	if IsBusyError(errDatabasePathIsDirectory) {
		t.Fatal("IsBusyError() = true, want false for non-busy error")
	}
}
