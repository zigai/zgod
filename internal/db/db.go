package db

import (
	"database/sql"
	"os"

	_ "modernc.org/sqlite"
)

func Open(dbPath string) (*sql.DB, error) {
	if err := ensureFilePermissions(dbPath, 0600); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA busy_timeout=2000",
		"PRAGMA foreign_keys=ON",
	}
	for _, p := range pragmas {
		if _, err = db.Exec(p); err != nil {
			_ = db.Close()
			return nil, err
		}
	}
	if err = ensureSchema(db); err != nil {
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
			return createErr
		}
		return f.Close()
	}
	if err != nil {
		return err
	}
	if info.Mode().Perm() != mode {
		return os.Chmod(path, mode)
	}
	return nil
}
