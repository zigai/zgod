package db

import (
	"context"
	"database/sql"
	"fmt"
)

type HistoryEntry struct {
	ID        int64
	TsMs      int64 //nolint:staticcheck // TsMs is clearer than TSMs
	Duration  int64
	ExitCode  int
	Command   string
	Directory string
	SessionID string
	Hostname  string
}

type HistoryRepo struct {
	db *sql.DB
}

func NewHistoryRepo(db *sql.DB) *HistoryRepo {
	return &HistoryRepo{db: db}
}

func (r *HistoryRepo) Insert(entry HistoryEntry) (int64, error) {
	res, err := r.db.ExecContext(
		context.Background(),
		`INSERT INTO history (ts_ms, duration, exit_code, command, directory, session_id, hostname)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		entry.TsMs, entry.Duration, entry.ExitCode, entry.Command,
		entry.Directory, entry.SessionID, entry.Hostname,
	)
	if err != nil {
		return 0, fmt.Errorf("inserting history entry: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("reading inserted history ID: %w", err)
	}
	return id, nil
}

func (r *HistoryRepo) Delete(id int64) error {
	_, err := r.db.ExecContext(context.Background(), `DELETE FROM history WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting history entry %d: %w", id, err)
	}
	return nil
}

func (r *HistoryRepo) Recent(limit int) ([]HistoryEntry, error) {
	rows, err := r.db.QueryContext(
		context.Background(),
		`SELECT id, ts_ms, duration, exit_code, command, directory, session_id, hostname
		 FROM history
		 ORDER BY ts_ms DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("querying recent history entries: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanEntries(rows)
}

func (r *HistoryRepo) RecentInDir(dir string, limit int) ([]HistoryEntry, error) {
	rows, err := r.db.QueryContext(
		context.Background(),
		`SELECT id, ts_ms, duration, exit_code, command, directory, session_id, hostname
		 FROM history WHERE directory = ?
		 ORDER BY ts_ms DESC LIMIT ?`,
		dir, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("querying recent history entries for %q: %w", dir, err)
	}
	defer func() { _ = rows.Close() }()
	return scanEntries(rows)
}

func (r *HistoryRepo) FetchCandidates(limit int, dedupe bool, onlyFails bool) ([]HistoryEntry, error) {
	query := `SELECT id, ts_ms, duration, exit_code, command, directory, session_id, hostname
		 FROM history`
	args := []any{}

	if onlyFails {
		query += " WHERE exit_code != 0"
	}
	query += " ORDER BY ts_ms DESC"
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := r.db.QueryContext(context.Background(), query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying history candidates: %w", err)
	}
	defer func() { _ = rows.Close() }()

	entries, err := scanEntries(rows)
	if err != nil {
		return nil, fmt.Errorf("scanning history candidates: %w", err)
	}

	if dedupe {
		entries = dedupeEntries(entries)
	}
	return entries, nil
}

func dedupeEntries(entries []HistoryEntry) []HistoryEntry {
	seen := map[string]bool{}
	result := make([]HistoryEntry, 0, len(entries))
	for _, e := range entries {
		if seen[e.Command] {
			continue
		}
		seen[e.Command] = true
		result = append(result, e)
	}
	return result
}

func scanEntries(rows *sql.Rows) ([]HistoryEntry, error) {
	var entries []HistoryEntry
	for rows.Next() {
		var e HistoryEntry
		err := rows.Scan(&e.ID, &e.TsMs, &e.Duration, &e.ExitCode,
			&e.Command, &e.Directory, &e.SessionID, &e.Hostname)
		if err != nil {
			return nil, fmt.Errorf("scanning history row: %w", err)
		}
		entries = append(entries, e)
	}
	err := rows.Err()
	if err != nil {
		return nil, fmt.Errorf("iterating history rows: %w", err)
	}
	return entries, nil
}
