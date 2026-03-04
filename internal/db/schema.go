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
`

var (
	errHistoryTableMissing    = errors.New("history table is missing")
	errHistoryColumnsMissing  = errors.New("history table is missing required columns")
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
