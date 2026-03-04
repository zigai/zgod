package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"

	_ "modernc.org/sqlite"
)

var (
	errDatabaseFileDoesNotExist = errors.New("database file does not exist")
	errDatabasePathIsDirectory  = errors.New("database path is a directory")
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
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA busy_timeout=2000",
		"PRAGMA foreign_keys=ON",
	}); err != nil {
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
		"PRAGMA busy_timeout=2000",
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
