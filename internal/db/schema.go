package db

import (
	"database/sql"
	"fmt"
)

const schema = `
CREATE TABLE IF NOT EXISTS history (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    ts_ms         INTEGER NOT NULL,
    duration      INTEGER NOT NULL DEFAULT 0,
    exit_code     INTEGER NOT NULL DEFAULT 0,
    command       TEXT    NOT NULL,
    directory     TEXT    NOT NULL DEFAULT '',
    session_id    TEXT    NOT NULL DEFAULT '',
    hostname      TEXT    NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_history_ts_ms         ON history(ts_ms);
CREATE INDEX IF NOT EXISTS idx_history_directory      ON history(directory);
CREATE INDEX IF NOT EXISTS idx_history_session_id     ON history(session_id);
CREATE INDEX IF NOT EXISTS idx_history_command        ON history(command);
`

func ensureSchema(db *sql.DB) error {
	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("applying schema: %w", err)
	}
	return nil
}
