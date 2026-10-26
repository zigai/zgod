package db

import (
	"cmp"
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"

	"pgregory.net/rapid"
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

	targetID, err := repo.Insert(HistoryEntry{TimestampMS: 1000, Command: "delete me"})
	if err != nil {
		t.Fatal(err)
	}

	survivorID, err := repo.Insert(HistoryEntry{TimestampMS: 2000, Command: "keep me"})
	if err != nil {
		t.Fatal(err)
	}

	if err = repo.Delete(targetID); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}

	entries, err := repo.Recent(10)
	if err != nil {
		t.Fatalf("Recent() error: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("Recent() returned %d entries, want 1", len(entries))
	}

	if entries[0].ID != survivorID {
		t.Fatalf("remaining entry ID = %d, want %d", entries[0].ID, survivorID)
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
		{TimestampMS: 2000, ExitCode: 0, Command: "echo boom"},
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

	if got[0].Command != "echo boom" || got[0].TimestampMS != 1000 || got[0].ExitCode != 1 {
		t.Fatalf("FetchCandidates()[0] = %#v, want the older failed record", got[0])
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

//go:embed testdata/characters.txt testdata/filename_characters.txt
var historyFixtureFS embed.FS

func historyFixtureRunes(check *rapid.T, path string) []rune {
	check.Helper()

	data, err := historyFixtureFS.ReadFile(path)
	if err != nil {
		check.Fatal(err)
	}

	return []rune(string(data))
}

func historyEntryGenerator() *rapid.Generator[HistoryEntry] {
	return rapid.Custom(func(check *rapid.T) HistoryEntry {
		text := rapid.StringOfN(rapid.RuneFrom(historyFixtureRunes(check, "testdata/characters.txt")), 0, 32, -1)

		return HistoryEntry{
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

func TestHistoryPersistenceRoundTripProperty(t *testing.T) {
	rapid.Check(t, func(check *rapid.T) {
		path := filepath.Join(t.TempDir(), "history.db")
		entries := rapid.SliceOfN(historyEntryGenerator(), 0, 24).Draw(check, "entries")

		database, err := Open(path)
		if err != nil {
			check.Fatal(err)
		}

		repo := NewHistoryRepo(database)
		for index := range entries {
			entries[index].ID, err = repo.Insert(entries[index])
			if err != nil {
				check.Fatal(err)
			}
		}

		if err = database.Close(); err != nil {
			check.Fatal(err)
		}

		database, err = Open(path)
		if err != nil {
			check.Fatal(err)
		}

		check.Cleanup(func() {
			if closeErr := database.Close(); closeErr != nil {
				check.Error(closeErr)
			}
		})

		repo = NewHistoryRepo(database)

		slices.SortFunc(entries, func(left, right HistoryEntry) int {
			if order := cmp.Compare(left.TimestampMS, right.TimestampMS); order != 0 {
				return order
			}

			return cmp.Compare(left.ID, right.ID)
		})

		got, err := repo.ListAll()
		if err != nil {
			check.Fatal(err)
		}

		if !slices.Equal(got, entries) {
			check.Fatalf("reopened history=%+v, want=%+v", got, entries)
		}

		var visited []HistoryEntry

		err = repo.ForEach(check.Context(), func(entry HistoryEntry) error {
			visited = append(visited, entry)
			return nil
		})
		if err != nil {
			check.Fatal(err)
		}

		if !slices.Equal(visited, entries) {
			check.Fatalf("traversal=%+v, want=%+v", visited, entries)
		}
	})
}

func TestHistoryCandidateLifecycleProperty(t *testing.T) {
	rapid.Check(t, func(check *rapid.T) {
		path := filepath.Join(t.TempDir(), "history.db")

		database, err := Open(path)
		if err != nil {
			check.Fatal(err)
		}

		check.Cleanup(func() {
			if closeErr := database.Close(); closeErr != nil {
				check.Error(closeErr)
			}
		})

		repo := NewHistoryRepo(database)

		var model []HistoryEntry

		check.Repeat(map[string]func(*rapid.T){
			"insert": func(step *rapid.T) {
				entry := historyEntryGenerator().Draw(step, "entry")
				entry.Command = rapid.SampledFrom([]string{"alpha", "beta", "gamma", "delta"}).Draw(step, "command")
				entry.Directory = rapid.SampledFrom([]string{"/one", "/two", ""}).Draw(step, "directory")
				entry.TimestampMS = rapid.Int64Range(0, 5).Draw(step, "timestamp")

				entry.ID, err = repo.Insert(entry)
				if err != nil {
					step.Fatal(err)
				}

				model = append(model, entry)
			},
			"delete": func(step *rapid.T) {
				id := rapid.Int64Range(0, int64(len(model)+3)).Draw(step, "id")
				if len(model) > 0 && rapid.Bool().Draw(step, "existing") {
					id = rapid.SampledFrom(model).Draw(step, "entry").ID
				}

				if deleteErr := repo.Delete(id); deleteErr != nil {
					step.Fatal(deleteErr)
				}

				model = slices.DeleteFunc(model, func(entry HistoryEntry) bool { return entry.ID == id })
			},
			"reopen": func(step *rapid.T) {
				if closeErr := database.Close(); closeErr != nil {
					step.Fatal(closeErr)
				}

				database, err = Open(path)
				if err != nil {
					step.Fatal(err)
				}

				repo = NewHistoryRepo(database)
			},
			"": func(step *rapid.T) {
				assertHistoryRecords(step, repo, model)

				limit := rapid.IntRange(0, 8).Draw(step, "limit")
				dedupe := rapid.Bool().Draw(step, "dedupe")
				mode := rapid.SampledFrom([]FailFilterMode{FailFilterInclude, FailFilterExclude, FailFilterOnly}).Draw(step, "fail-mode")
				directory := rapid.SampledFrom([]string{"", "/one", "/two"}).Draw(step, "scope")
				want := expectedHistoryCandidates(model, limit, dedupe, mode, directory)

				got, fetchErr := repo.FetchCandidatesInDir(limit, dedupe, mode, directory)
				if fetchErr != nil {
					step.Fatal(fetchErr)
				}

				if !slices.Equal(got, want) {
					step.Fatalf("candidates=%+v, want=%+v", got, want)
				}
			},
		})
	})
}

func assertHistoryRecords(check *rapid.T, repo *HistoryRepo, expected []HistoryEntry) {
	check.Helper()

	all, err := repo.ListAll()
	if err != nil {
		check.Fatal(err)
	}

	if len(all) != len(expected) {
		check.Fatalf("history count=%d, want=%d", len(all), len(expected))
	}

	for _, entry := range expected {
		if !slices.Contains(all, entry) {
			check.Fatalf("lost or changed record=%+v; history=%+v", entry, all)
		}
	}
}

func expectedHistoryCandidates(entries []HistoryEntry, limit int, dedupe bool, mode FailFilterMode, directory string) []HistoryEntry {
	eligible := make([]HistoryEntry, 0, len(entries))
	for _, entry := range entries {
		if directory != "" && entry.Directory != directory {
			continue
		}

		if mode == FailFilterExclude && entry.ExitCode != 0 {
			continue
		}

		if mode == FailFilterOnly && entry.ExitCode == 0 {
			continue
		}

		entry.SessionID, entry.Hostname = "", ""
		eligible = append(eligible, entry)
	}

	slices.SortFunc(eligible, func(left, right HistoryEntry) int {
		if order := cmp.Compare(right.TimestampMS, left.TimestampMS); order != 0 {
			return order
		}

		return cmp.Compare(right.ID, left.ID)
	})

	if dedupe {
		seen := make(map[string]bool)
		eligible = slices.DeleteFunc(eligible, func(entry HistoryEntry) bool {
			duplicate := seen[entry.Command]
			seen[entry.Command] = true

			return duplicate
		})
	}

	if limit > 0 && len(eligible) > limit {
		eligible = eligible[:limit]
	}

	return eligible
}

func TestSQLiteFilenameAndReadOnlyProperty(t *testing.T) {
	rapid.Check(t, func(check *rapid.T) {
		name := rapid.StringOfN(rapid.RuneFrom(historyFixtureRunes(check, "testdata/filename_characters.txt")), 0, 24, -1).Draw(check, "filename")
		path := filepath.Join(t.TempDir(), "history-"+name+".db")
		entry := historyEntryGenerator().Draw(check, "entry")

		database, err := Open(path)
		if err != nil {
			check.Fatal(err)
		}

		entry.ID, err = NewHistoryRepo(database).Insert(entry)
		if err != nil {
			check.Fatal(err)
		}

		if err = database.Close(); err != nil {
			check.Fatal(err)
		}

		readOnly, err := OpenReadOnly(path)
		if err != nil {
			check.Fatal(err)
		}

		check.Cleanup(func() {
			if closeErr := readOnly.Close(); closeErr != nil {
				check.Error(closeErr)
			}
		})
		readOnly.SetMaxIdleConns(0)

		for range rapid.IntRange(1, 4).Draw(check, "connections") {
			got, readErr := NewHistoryRepo(readOnly).ListAll()
			if readErr != nil {
				check.Fatal(readErr)
			}

			if !slices.Equal(got, []HistoryEntry{entry}) {
				check.Fatalf("opened different data=%+v, want=%+v", got, entry)
			}

			if _, writeErr := NewHistoryRepo(readOnly).Insert(entry); writeErr == nil {
				check.Fatal("read-only connection permitted insertion")
			}
		}
	})
}

func TestHistorySchemaValidationProperty(t *testing.T) {
	rapid.Check(t, func(check *rapid.T) {
		database, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "source.db"))
		if err != nil {
			check.Fatal(err)
		}

		check.Cleanup(func() {
			if closeErr := database.Close(); closeErr != nil {
				check.Error(closeErr)
			}
		})

		required := []string{"id INTEGER", "ts_ms INTEGER", "duration INTEGER", "exit_code INTEGER", "command TEXT", "directory TEXT", "session_id TEXT", "hostname TEXT"}

		var columns []string

		for _, column := range required {
			if rapid.Bool().Draw(check, column) {
				columns = append(columns, column)
			}
		}

		columns = append(columns, "extra TEXT")
		if _, err = database.ExecContext(check.Context(), "CREATE TABLE history ("+strings.Join(columns, ",")+")"); err != nil {
			check.Fatal(err)
		}

		err = ValidateHistorySchema(database)

		wantValid := len(columns) == len(required)+1
		if (err == nil) != wantValid {
			check.Fatalf("columns=%v validation=%v, want valid=%t", columns, err, wantValid)
		}
	})
}

func assertUpgradedHistory(check *rapid.T, database *sql.DB, entries, candidates []HistoryEntry) {
	check.Helper()

	got, err := NewHistoryRepo(database).ListAll()
	if err != nil {
		check.Fatal(err)
	}

	slices.SortFunc(entries, func(first, second HistoryEntry) int {
		if order := cmp.Compare(first.TimestampMS, second.TimestampMS); order != 0 {
			return order
		}

		return cmp.Compare(first.ID, second.ID)
	})

	if !slices.Equal(got, entries) {
		check.Fatalf("upgrade changed records: got=%#v, want=%#v", got, entries)
	}

	after, err := NewHistoryRepo(database).FetchCandidates(0, true, FailFilterInclude)
	if err != nil || !slices.Equal(after, candidates) {
		check.Fatalf("upgrade changed candidates: got=%#v, want=%#v, error=%v", after, candidates, err)
	}
}

func TestHistorySchemaUpgradePreservesRecordsProperty(t *testing.T) {
	rapid.Check(t, func(check *rapid.T) {
		path := filepath.Join(t.TempDir(), "history.db")

		database, err := Open(path)
		if err != nil {
			check.Fatal(err)
		}

		entries := rapid.SliceOfN(historyEntryGenerator(), 0, 12).Draw(check, "history")
		for index, entry := range entries {
			entries[index].ID, err = NewHistoryRepo(database).Insert(entry)
			if err != nil {
				_ = database.Close()

				check.Fatal(err)
			}
		}

		before, err := NewHistoryRepo(database).FetchCandidates(0, true, FailFilterInclude)
		if err != nil {
			_ = database.Close()

			check.Fatal(err)
		}

		if _, err = database.ExecContext(t.Context(), `DROP TABLE pending_record; PRAGMA user_version = 1`); err != nil {
			_ = database.Close()

			check.Fatal(err)
		}

		if err = database.Close(); err != nil {
			check.Fatal(err)
		}

		database, err = Open(path)
		if err != nil {
			check.Fatal(err)
		}
		defer func() { _ = database.Close() }()

		assertUpgradedHistory(check, database, entries, before)

		pending := historyEntryGenerator().Draw(check, "pending")
		for range rapid.IntRange(2, 4).Draw(check, "replays") {
			if err = NewHistoryRepo(database).InsertPending("pending.json", pending); err != nil {
				check.Fatal(err)
			}
		}

		got, err := NewHistoryRepo(database).ListAll()
		if err != nil || len(got) != len(entries)+1 {
			check.Fatalf("upgraded queue replay: records=%#v, error=%v", got, err)
		}
	})
}

func TestUnsupportedSchemaPreservesDataProperty(t *testing.T) {
	rapid.Check(t, func(check *rapid.T) {
		path := filepath.Join(t.TempDir(), "source.db")

		database, err := Open(path)
		if err != nil {
			check.Fatal(err)
		}

		entry := historyEntryGenerator().Draw(check, "entry")

		entry.ID, err = NewHistoryRepo(database).Insert(entry)
		if err != nil {
			check.Fatal(err)
		}

		version := rapid.IntRange(currentSchemaVersion+1, currentSchemaVersion+1000).Draw(check, "version")
		if _, err = database.ExecContext(t.Context(), fmt.Sprintf("PRAGMA user_version=%d", version)); err != nil {
			check.Fatal(err)
		}

		if err = database.Close(); err != nil {
			check.Fatal(err)
		}

		rejected, openErr := Open(path)
		if rejected != nil {
			_ = rejected.Close()
		}

		if openErr == nil {
			check.Fatal("future schema accepted")
		}

		database, err = sql.Open("sqlite", path)
		if err != nil {
			check.Fatal(err)
		}

		check.Cleanup(func() {
			if closeErr := database.Close(); closeErr != nil {
				check.Error(closeErr)
			}
		})

		assertUnsupportedSchemaPreserved(check, database, version, entry)
	})
}

func assertUnsupportedSchemaPreserved(check *rapid.T, database *sql.DB, version int, entry HistoryEntry) {
	check.Helper()

	var gotVersion int
	if err := database.QueryRowContext(check.Context(), "PRAGMA user_version").Scan(&gotVersion); err != nil {
		check.Fatal(err)
	}

	got, err := NewHistoryRepo(database).ListAll()
	if err != nil {
		check.Fatal(err)
	}

	if gotVersion != version || !reflect.DeepEqual(got, []HistoryEntry{entry}) {
		check.Fatalf("rejected open changed version/data: version=%d records=%+v", gotVersion, got)
	}
}

func TestFailFilterTextRoundTrip(t *testing.T) {
	for _, mode := range []FailFilterMode{FailFilterInclude, FailFilterExclude, FailFilterOnly} {
		got, ok := ParseFailFilterMode(mode.String())
		if !ok || got != mode {
			t.Fatalf("round trip=%v/%t, want=%v", got, ok, mode)
		}
	}
}
