package db

import "database/sql"

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
	res, err := r.db.Exec(
		`INSERT INTO history (ts_ms, duration, exit_code, command, directory, session_id, hostname)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		entry.TsMs, entry.Duration, entry.ExitCode, entry.Command,
		entry.Directory, entry.SessionID, entry.Hostname,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *HistoryRepo) Delete(id int64) error {
	_, err := r.db.Exec(`DELETE FROM history WHERE id = ?`, id)
	return err
}

func (r *HistoryRepo) Recent(limit int) ([]HistoryEntry, error) {
	rows, err := r.db.Query(
		`SELECT id, ts_ms, duration, exit_code, command, directory, session_id, hostname
		 FROM history
		 ORDER BY ts_ms DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanEntries(rows)
}

func (r *HistoryRepo) RecentInDir(dir string, limit int) ([]HistoryEntry, error) {
	rows, err := r.db.Query(
		`SELECT id, ts_ms, duration, exit_code, command, directory, session_id, hostname
		 FROM history WHERE directory = ?
		 ORDER BY ts_ms DESC LIMIT ?`,
		dir, limit,
	)
	if err != nil {
		return nil, err
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

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	entries, err := scanEntries(rows)
	if err != nil {
		return nil, err
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
		if err := rows.Scan(&e.ID, &e.TsMs, &e.Duration, &e.ExitCode,
			&e.Command, &e.Directory, &e.SessionID, &e.Hostname); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
