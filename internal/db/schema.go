package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
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
CREATE INDEX IF NOT EXISTS idx_history_ts_id          ON history(ts_ms DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_history_dir_ts_id      ON history(directory, ts_ms DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_history_exit_ts_id     ON history(exit_code, ts_ms DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_history_dir_exit_ts_id ON history(directory, exit_code, ts_ms DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_history_success_ts_id  ON history(ts_ms DESC, id DESC) WHERE exit_code = 0;
CREATE INDEX IF NOT EXISTS idx_history_failure_ts_id  ON history(ts_ms DESC, id DESC) WHERE exit_code != 0;

CREATE TABLE IF NOT EXISTS latest_command (
    command    TEXT    PRIMARY KEY,
    history_id INTEGER NOT NULL,
    ts_ms      INTEGER NOT NULL,
    duration   INTEGER NOT NULL,
    exit_code  INTEGER NOT NULL,
    directory  TEXT    NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_latest_command_ts_id     ON latest_command(ts_ms DESC, history_id DESC);
CREATE INDEX IF NOT EXISTS idx_latest_command_dir_ts_id ON latest_command(directory, ts_ms DESC, history_id DESC);

INSERT OR IGNORE INTO latest_command (command, history_id, ts_ms, duration, exit_code, directory)
SELECT h.command, h.id, h.ts_ms, h.duration, h.exit_code, h.directory
FROM history h
WHERE NOT EXISTS (
    SELECT 1
    FROM history newer
    WHERE newer.command = h.command
      AND (newer.ts_ms > h.ts_ms OR (newer.ts_ms = h.ts_ms AND newer.id > h.id))
);

PRAGMA user_version = 1;
`

const currentSchemaVersion = 1

var (
	errHistoryTableMissing    = errors.New("history table is missing")
	errHistoryColumnsMissing  = errors.New("history table is missing required columns")
	errUnsupportedSchema      = errors.New("unsupported schema version")
	requiredHistoryColumnsSet = map[string]bool{
		"id":         true,
		"ts_ms":      true,
		"duration":   true,
		"exit_code":  true,
		"command":    true,
		"directory":  true,
		"session_id": true,
		"hostname":   true,
	}
)

func ensureSchema(db *sql.DB) error {
	if _, err := db.ExecContext(context.Background(), schema); err != nil {
		return fmt.Errorf("applying schema: %w", err)
	}

	return nil
}

func ValidateHistorySchema(db *sql.DB) error {
	rows, err := db.QueryContext(context.Background(), `PRAGMA table_info(history)`)
	if err != nil {
		return fmt.Errorf("reading history table info: %w", err)
	}

	defer func() { _ = rows.Close() }()

	present := map[string]bool{}

	for rows.Next() {
		var (
			cid        int
			name       string
			columnType string
			notNull    int
			defaultVal sql.NullString
			pk         int
		)

		if err = rows.Scan(&cid, &name, &columnType, &notNull, &defaultVal, &pk); err != nil {
			return fmt.Errorf("scanning history table info row: %w", err)
		}

		present[name] = true
	}

	if err = rows.Err(); err != nil {
		return fmt.Errorf("iterating history table info rows: %w", err)
	}

	if len(present) == 0 {
		return errHistoryTableMissing
	}

	missing := make([]string, 0, len(requiredHistoryColumnsSet))
	for col := range requiredHistoryColumnsSet {
		if !present[col] {
			missing = append(missing, col)
		}
	}

	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("%w: %s", errHistoryColumnsMissing, strings.Join(missing, ", "))
	}

	return nil
}

func ValidateSupportedSchemaVersion(db *sql.DB) error {
	row := db.QueryRowContext(context.Background(), `PRAGMA user_version`)

	var version int
	if err := row.Scan(&version); err != nil {
		return fmt.Errorf("reading schema version: %w", err)
	}

	if version > currentSchemaVersion {
		return fmt.Errorf("%w %d: max supported is %d", errUnsupportedSchema, version, currentSchemaVersion)
	}

	return nil
}
