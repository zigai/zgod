package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"slices"
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
		TimestampMS: 1000,
		DurationMS:  50,
		ExitCode:    0,
		Command:     "echo hello",
		Directory:   "/tmp",
		SessionID:   "test-session",
		Hostname:    "test-host",
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
			TimestampMS: int64(i * 1000),
			Command:     cmd,
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

	id, err := repo.Insert(HistoryEntry{TimestampMS: 1000, Command: "delete me"})
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
	if _, err = repo.Insert(HistoryEntry{TimestampMS: 1000, Command: "in dir", Directory: "/home"}); err != nil {
		t.Fatal(err)
	}

	if _, err = repo.Insert(HistoryEntry{TimestampMS: 2000, Command: "other dir", Directory: "/tmp"}); err != nil {
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
	if _, err = repo.Insert(HistoryEntry{TimestampMS: 1000, Command: "echo hello"}); err != nil {
		t.Fatal(err)
	}

	if _, err = repo.Insert(HistoryEntry{TimestampMS: 2000, Command: "echo hello"}); err != nil {
		t.Fatal(err)
	}

	if _, err = repo.Insert(HistoryEntry{TimestampMS: 3000, Command: "echo world"}); err != nil {
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
		{TimestampMS: 1000, ExitCode: 0, Command: "echo ok one"},
		{TimestampMS: 2000, ExitCode: 1, Command: "echo fail one"},
		{TimestampMS: 3000, ExitCode: 0, Command: "echo ok two"},
		{TimestampMS: 4000, ExitCode: 2, Command: "echo fail two"},
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
			got, err := repo.FetchCandidates(100, tc.dedupe, tc.mode)
			if err != nil {
				t.Fatalf("FetchCandidates() error: %v", err)
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
		{TimestampMS: 1000, ExitCode: 1, Command: "echo boom"},
		{TimestampMS: 2000, ExitCode: 2, Command: "echo boom"},
		{TimestampMS: 3000, ExitCode: 0, Command: "echo ok"},
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

func TestFetchCandidatesOrdersEqualTimestampsByNewestID(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	database, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}

	defer func() { _ = database.Close() }()

	repo := NewHistoryRepo(database)
	for _, command := range []string{"first", "second"} {
		if _, err = repo.Insert(HistoryEntry{TimestampMS: 1000, Command: command}); err != nil {
			t.Fatalf("Insert(%q) error: %v", command, err)
		}
	}

	got, err := repo.FetchCandidates(100, false, FailFilterInclude)
	if err != nil {
		t.Fatalf("FetchCandidates() error: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("len(FetchCandidates()) = %d, want 2", len(got))
	}

	if got[0].Command != "second" || got[1].Command != "first" {
		t.Fatalf("FetchCandidates() order = %q, %q; want second, first", got[0].Command, got[1].Command)
	}
}

func TestFetchCandidatesInDir(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	database, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}

	defer func() { _ = database.Close() }()

	repo := NewHistoryRepo(database)
	entries := []HistoryEntry{
		{TimestampMS: 1000, Command: "old cwd", Directory: "/repo"},
		{TimestampMS: 2000, Command: "other", Directory: "/elsewhere"},
		{TimestampMS: 3000, Command: "new cwd", Directory: "/repo"},
	}

	for _, entry := range entries {
		if _, err = repo.Insert(entry); err != nil {
			t.Fatalf("Insert(%q) error: %v", entry.Command, err)
		}
	}

	got, err := repo.FetchCandidatesInDir(100, false, FailFilterInclude, "/repo")
	if err != nil {
		t.Fatalf("FetchCandidatesInDir() error: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("len(FetchCandidatesInDir()) = %d, want 2", len(got))
	}

	if got[0].Command != "new cwd" || got[1].Command != "old cwd" {
		t.Fatalf("FetchCandidatesInDir() order = %q, %q; want new cwd, old cwd", got[0].Command, got[1].Command)
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
	if _, err = repo.Insert(HistoryEntry{TimestampMS: 1000, Command: "echo seeded"}); err != nil {
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

func TestSQLiteDSNsApplyConnectionPragmas(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "history #1.db")

	writableDSN, err := sqliteWritableDSN(dbPath)
	if err != nil {
		t.Fatalf("sqliteWritableDSN() error: %v", err)
	}

	writableURL, err := url.Parse(writableDSN)
	if err != nil {
		t.Fatalf("url.Parse(writableDSN) error: %v", err)
	}

	writableQuery := writableURL.Query()
	if writableQuery.Get("mode") != "" {
		t.Fatalf("writable DSN mode = %q, want empty", writableQuery.Get("mode"))
	}

	writablePragmas := writableQuery["_pragma"]
	for _, want := range []string{
		"busy_timeout(2000)",
		"foreign_keys(ON)",
		"synchronous(NORMAL)",
	} {
		if !slices.Contains(writablePragmas, want) {
			t.Fatalf("writable DSN pragmas = %v, missing %q", writablePragmas, want)
		}
	}

	readOnlyDSN, err := sqliteReadOnlyDSN(dbPath)
	if err != nil {
		t.Fatalf("sqliteReadOnlyDSN() error: %v", err)
	}

	readOnlyURL, err := url.Parse(readOnlyDSN)
	if err != nil {
		t.Fatalf("url.Parse(readOnlyDSN) error: %v", err)
	}

	readOnlyQuery := readOnlyURL.Query()
	if got, want := readOnlyQuery.Get("mode"), "ro"; got != want {
		t.Fatalf("read-only DSN mode = %q, want %q", got, want)
	}

	readOnlyPragmas := readOnlyQuery["_pragma"]
	for _, want := range []string{
		"busy_timeout(2000)",
		"foreign_keys(ON)",
		"query_only(ON)",
	} {
		if !slices.Contains(readOnlyPragmas, want) {
			t.Fatalf("read-only DSN pragmas = %v, missing %q", readOnlyPragmas, want)
		}
	}

	if slices.Contains(readOnlyPragmas, "synchronous(NORMAL)") {
		t.Fatalf("read-only DSN pragmas = %v, want no synchronous pragma", readOnlyPragmas)
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

func TestOpenRejectsUnsupportedSchemaWithoutDowngrading(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "future-schema.db")
	ctx := context.Background()
	futureVersion := currentSchemaVersion + 1

	seedDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open(seed) error: %v", err)
	}

	if _, err = seedDB.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", futureVersion)); err != nil {
		t.Fatalf("setting future user_version: %v", err)
	}

	if err = seedDB.Close(); err != nil {
		t.Fatalf("Close(seed) error: %v", err)
	}

	opened, err := Open(dbPath)
	if err == nil {
		_ = opened.Close()

		t.Fatal("Open() should reject unsupported future schema")
	}

	if !errors.Is(err, errUnsupportedSchema) {
		t.Fatalf("Open() error = %v, want errUnsupportedSchema", err)
	}

	verifyDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open(verify) error: %v", err)
	}

	defer func() { _ = verifyDB.Close() }()

	version, err := readSQLiteUserVersion(verifyDB)
	if err != nil {
		t.Fatalf("readSQLiteUserVersion() error: %v", err)
	}

	if version != futureVersion {
		t.Fatalf("user_version = %d, want %d", version, futureVersion)
	}

	var tableName string

	err = verifyDB.QueryRowContext(
		ctx,
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'history'`,
	).Scan(&tableName)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("history table lookup error = %v, want sql.ErrNoRows", err)
	}
}

func TestOpenCurrentSchemaDoesNotApplySchemaUnderWriteLock(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "schema-current.db")
	ctx := context.Background()

	database, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}

	if err = database.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}

	locker, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open(locker) error: %v", err)
	}

	defer func() { _ = locker.Close() }()

	if _, err = locker.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		t.Fatalf("starting write-reservation transaction: %v", err)
	}

	defer func() {
		_, _ = locker.ExecContext(ctx, "ROLLBACK")
	}()

	opened, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() with current schema under write lock error: %v", err)
	}

	defer func() { _ = opened.Close() }()
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
		TimestampMS: 2000,
		DurationMS:  50,
		ExitCode:    0,
		Command:     "echo first",
		Directory:   "/tmp",
		SessionID:   "session-1",
		Hostname:    "host-1",
	}
	second := HistoryEntry{
		TimestampMS: 1000,
		DurationMS:  10,
		ExitCode:    1,
		Command:     "echo second",
		Directory:   "/home",
		SessionID:   "session-2",
		Hostname:    "host-2",
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

	if entries[0].TimestampMS != 1000 || entries[1].TimestampMS != 2000 {
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
		TimestampMS: 2000,
		DurationMS:  50,
		ExitCode:    0,
		Command:     "echo first",
		Directory:   "/tmp",
		SessionID:   "session-1",
		Hostname:    "host-1",
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
		TimestampMS: 3000,
		DurationMS:  20,
		ExitCode:    0,
		Command:     "echo third",
		Directory:   "/var",
		SessionID:   "session-3",
		Hostname:    "host-3",
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

func TestLatestCommandCacheTracksNewestCommand(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	database, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}

	defer func() { _ = database.Close() }()

	repo := NewHistoryRepo(database)

	oldID, err := repo.Insert(HistoryEntry{TimestampMS: 1000, Command: "repeat", Directory: "/old"})
	if err != nil {
		t.Fatalf("Insert(old) error: %v", err)
	}

	newID, err := repo.Insert(HistoryEntry{TimestampMS: 2000, Command: "repeat", Directory: "/new"})
	if err != nil {
		t.Fatalf("Insert(new) error: %v", err)
	}

	var (
		historyID int64
		dir       string
	)

	row := database.QueryRowContext(context.Background(), `SELECT history_id, directory FROM latest_command WHERE command = ?`, "repeat")
	if err = row.Scan(&historyID, &dir); err != nil {
		t.Fatalf("reading latest_command: %v", err)
	}

	if historyID != newID || dir != "/new" {
		t.Fatalf("latest_command = (%d, %q), want (%d, /new)", historyID, dir, newID)
	}

	if err = repo.Delete(newID); err != nil {
		t.Fatalf("Delete(new) error: %v", err)
	}

	row = database.QueryRowContext(context.Background(), `SELECT history_id, directory FROM latest_command WHERE command = ?`, "repeat")
	if err = row.Scan(&historyID, &dir); err != nil {
		t.Fatalf("reading latest_command after delete: %v", err)
	}

	if historyID != oldID || dir != "/old" {
		t.Fatalf("latest_command after delete = (%d, %q), want (%d, /old)", historyID, dir, oldID)
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
