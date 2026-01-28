package db

import (
	"database/sql"

	_ "modernc.org/sqlite"
)

func Open(dbPath string) (*sql.DB, error) {
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
