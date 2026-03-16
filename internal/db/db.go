package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	sqlite "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

var (
	errDatabaseFileDoesNotExist = errors.New("database file does not exist")
	errDatabasePathIsDirectory  = errors.New("database path is a directory")
	errSQLitePragmaNoDetails    = errors.New("sqlite pragma failed without error details")
)

const (
	sqliteBusyTimeoutMs           = 2000
	sqliteJournalModeRetryCount   = 3
	sqliteJournalModeRetryBackoff = 100 * time.Millisecond
)

func Open(dbPath string) (*sql.DB, error) {
	if err := ensureFilePermissions(dbPath, 0o600); err != nil {
		return nil, fmt.Errorf("ensuring database file permissions for %q: %w", dbPath, err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite database %q: %w", dbPath, err)
	}

	if err = applySQLitePragmas(db, []string{
		fmt.Sprintf("PRAGMA busy_timeout=%d", sqliteBusyTimeoutMs),
		"PRAGMA synchronous=NORMAL",
		"PRAGMA foreign_keys=ON",
	}); err != nil {
		_ = db.Close()
		return nil, err
	}

	if err = ensureSQLiteJournalModeWAL(db); err != nil {
		_ = db.Close()
		return nil, err
	}

	if err = ensureSchema(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ensuring database schema: %w", err)
	}

	return db, nil
}

func OpenReadOnly(dbPath string) (*sql.DB, error) {
	info, err := os.Stat(dbPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: %q", errDatabaseFileDoesNotExist, dbPath)
		}

		return nil, fmt.Errorf("stating database file %q: %w", dbPath, err)
	}

	if info.IsDir() {
		return nil, fmt.Errorf("%w: %q", errDatabasePathIsDirectory, dbPath)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite database %q: %w", dbPath, err)
	}

	if err = applySQLitePragmas(db, []string{
		"PRAGMA query_only=ON",
		fmt.Sprintf("PRAGMA busy_timeout=%d", sqliteBusyTimeoutMs),
		"PRAGMA foreign_keys=ON",
	}); err != nil {
		_ = db.Close()
		return nil, err
	}

	return db, nil
}

func ensureFilePermissions(path string, mode os.FileMode) error {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		f, createErr := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, mode)
		if createErr != nil {
			return fmt.Errorf("creating database file %q: %w", path, createErr)
		}

		err = f.Close()
		if err != nil {
			return fmt.Errorf("closing newly created database file %q: %w", path, err)
		}

		return nil
	}

	if err != nil {
		return fmt.Errorf("stating database file %q: %w", path, err)
	}

	if info.Mode().Perm() != mode {
		err = os.Chmod(path, mode)
		if err != nil {
			return fmt.Errorf("setting database file permissions on %q: %w", path, err)
		}

		return nil
	}

	return nil
}

func applySQLitePragmas(db *sql.DB, pragmas []string) error {
	ctx := context.Background()
	for _, pragma := range pragmas {
		if _, err := db.ExecContext(ctx, pragma); err != nil {
			return fmt.Errorf("applying sqlite pragma %q: %w", pragma, err)
		}
	}

	return nil
}

func ensureSQLiteJournalModeWAL(db *sql.DB) error {
	const (
		readPragma  = "PRAGMA journal_mode"
		writePragma = "PRAGMA journal_mode=WAL"
	)

	mode, err := readSQLitePragmaString(db, readPragma)
	if err != nil {
		return fmt.Errorf("reading sqlite pragma %q: %w", readPragma, err)
	}

	if strings.EqualFold(mode, "wal") {
		return nil
	}

	if err = applySQLitePragmaWithRetry(db, writePragma, sqliteJournalModeRetryCount, sqliteJournalModeRetryBackoff); err != nil {
		return fmt.Errorf("applying sqlite pragma %q: %w", writePragma, err)
	}

	return nil
}

func readSQLitePragmaString(db *sql.DB, pragma string) (string, error) {
	ctx := context.Background()
	row := db.QueryRowContext(ctx, pragma)

	var value string
	if err := row.Scan(&value); err != nil {
		return "", fmt.Errorf("scanning sqlite pragma %q result: %w", pragma, err)
	}

	return value, nil
}

func applySQLitePragmaWithRetry(db *sql.DB, pragma string, retries int, backoff time.Duration) error {
	ctx := context.Background()

	var lastErr error

	for attempt := range retries {
		_, err := db.ExecContext(ctx, pragma)
		if err == nil {
			return nil
		}

		if !IsBusyError(err) {
			return fmt.Errorf("applying sqlite pragma %q: %w", pragma, err)
		}

		lastErr = err

		if attempt < retries-1 && backoff > 0 {
			time.Sleep(backoff)
		}
	}

	if lastErr == nil {
		return fmt.Errorf("%w: %q", errSQLitePragmaNoDetails, pragma)
	}

	return lastErr
}

func IsBusyError(err error) bool {
	if err == nil {
		return false
	}

	var sqliteErr *sqlite.Error
	if errors.As(err, &sqliteErr) {
		code := sqliteErr.Code()

		return code == sqlite3.SQLITE_BUSY ||
			code == sqlite3.SQLITE_BUSY_RECOVERY ||
			code == sqlite3.SQLITE_BUSY_SNAPSHOT ||
			code == sqlite3.SQLITE_BUSY_TIMEOUT
	}

	msg := strings.ToLower(err.Error())

	return strings.Contains(msg, "sqlite_busy") ||
		strings.Contains(msg, "database is locked") ||
		strings.Contains(msg, "database table is locked")
}
