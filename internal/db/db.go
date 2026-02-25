package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	_ "modernc.org/sqlite"
)

func Open(dbPath string) (*sql.DB, error) {
	if err := ensureFilePermissions(dbPath, 0o600); err != nil {
		return nil, fmt.Errorf("ensuring database file permissions for %q: %w", dbPath, err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite database %q: %w", dbPath, err)
	}

	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA busy_timeout=2000",
		"PRAGMA foreign_keys=ON",
	}

	ctx := context.Background()
	for _, p := range pragmas {
		if _, err = db.ExecContext(ctx, p); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("applying sqlite pragma %q: %w", p, err)
		}
	}

	if err = ensureSchema(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ensuring database schema: %w", err)
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
