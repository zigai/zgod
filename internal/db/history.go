package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

type HistoryEntry struct {
	ID        int64
	TSMs      int64
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
	ctx := context.Background()

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("starting insert transaction: %w", err)
	}

	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(
		ctx,
		`INSERT INTO history (ts_ms, duration, exit_code, command, directory, session_id, hostname)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		entry.TSMs, entry.Duration, entry.ExitCode, entry.Command,
		entry.Directory, entry.SessionID, entry.Hostname,
	)
	if err != nil {
		return 0, fmt.Errorf("inserting history entry: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("reading inserted history ID: %w", err)
	}

	entry.ID = id
	if err = upsertLatestCommandTx(ctx, tx, entry); err != nil {
		return 0, err
	}

	if err = tx.Commit(); err != nil {
		return 0, fmt.Errorf("committing inserted history entry: %w", err)
	}

	return id, nil
}

func (r *HistoryRepo) Delete(id int64) error {
	ctx := context.Background()

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("starting delete transaction: %w", err)
	}

	defer func() { _ = tx.Rollback() }()

	var command string

	row := tx.QueryRowContext(ctx, `SELECT command FROM history WHERE id = ?`, id)
	if err = row.Scan(&command); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("reading history entry %d before delete: %w", id, err)
	}

	_, err = tx.ExecContext(ctx, `DELETE FROM history WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting history entry %d: %w", id, err)
	}

	if command != "" {
		if err = rebuildLatestCommandTx(ctx, tx, command); err != nil {
			return err
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("committing deleted history entry %d: %w", id, err)
	}

	return nil
}

func (r *HistoryRepo) Recent(limit int) ([]HistoryEntry, error) {
	rows, err := r.db.QueryContext(
		context.Background(),
		`SELECT id, ts_ms, duration, exit_code, command, directory, session_id, hostname
		 FROM history
		 ORDER BY ts_ms DESC, id DESC LIMIT ?`,
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
		 ORDER BY ts_ms DESC, id DESC LIMIT ?`,
		dir, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("querying recent history entries for %q: %w", dir, err)
	}

	defer func() { _ = rows.Close() }()

	return scanEntries(rows)
}

func (r *HistoryRepo) ListAll() ([]HistoryEntry, error) {
	rows, err := r.db.QueryContext(
		context.Background(),
		`SELECT id, ts_ms, duration, exit_code, command, directory, session_id, hostname
		 FROM history
		 ORDER BY ts_ms ASC, id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("querying full history: %w", err)
	}

	defer func() { _ = rows.Close() }()

	return scanEntries(rows)
}

func (r *HistoryRepo) FetchCandidates(limit int, dedupe bool, failFilter FailFilterMode) ([]HistoryEntry, error) {
	return r.FetchCandidatesInDir(limit, dedupe, failFilter, "")
}

func (r *HistoryRepo) FetchCandidatesInDir(limit int, dedupe bool, failFilter FailFilterMode, dir string) ([]HistoryEntry, error) {
	rows, err := r.queryCandidates(limit, dedupe, failFilter, dir)
	if err != nil {
		return nil, fmt.Errorf("querying history candidates: %w", err)
	}

	defer func() { _ = rows.Close() }()

	entries, err := scanCandidateEntries(rows)
	if err != nil {
		return nil, fmt.Errorf("scanning history candidates: %w", err)
	}

	if dedupe {
		entries = dedupeEntries(entries)
	}

	return entries, nil
}

func (r *HistoryRepo) queryCandidates(limit int, dedupe bool, failFilter FailFilterMode, dir string) (*sql.Rows, error) {
	specs := candidateQuerySpecs(dedupe, failFilter, dir)
	for _, spec := range specs {
		query, args := spec.withLimit(limit)

		rows, err := r.db.QueryContext(context.Background(), query, args...)
		if err == nil {
			return rows, nil
		}

		if !spec.fallback {
			return nil, fmt.Errorf("querying history candidates: %w", err)
		}
	}

	query, args := candidateQuery(failFilter, dir)
	query, args = appendCandidateLimit(query, args, limit)

	rows, err := r.db.QueryContext(context.Background(), query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying fallback history candidates: %w", err)
	}

	return rows, nil
}

type candidateQuerySpec struct {
	query    string
	args     []any
	fallback bool
}

func (s candidateQuerySpec) withLimit(limit int) (string, []any) {
	return appendCandidateLimit(s.query, s.args, limit)
}

func candidateQuerySpecs(dedupe bool, failFilter FailFilterMode, dir string) []candidateQuerySpec {
	query, args := candidateQuery(failFilter, dir)
	specs := []candidateQuerySpec{{query: query, args: args, fallback: false}}

	if !dedupe {
		return specs
	}

	query, args = dedupedRawCandidateQuery(failFilter, dir)
	specs[0] = candidateQuerySpec{query: query, args: args, fallback: true}

	if failFilter == FailFilterInclude && dir == "" {
		query, args = dedupedCandidateQuery(dir)
		specs = append([]candidateQuerySpec{{query: query, args: args, fallback: true}}, specs...)
	}

	return specs
}

func appendCandidateLimit(query string, args []any, limit int) (string, []any) {
	query += " ORDER BY ts_ms DESC, id DESC"
	if limit <= 0 {
		return query, args
	}

	args = append(args, limit)

	return query + " LIMIT ?", args
}

func candidateQuery(failFilter FailFilterMode, dir string) (string, []any) {
	baseQuery := `SELECT id, ts_ms, duration, exit_code, command, directory
		 FROM history`

	switch {
	case dir == "" && failFilter == FailFilterInclude:
		return baseQuery, nil
	case dir == "" && failFilter == FailFilterExclude:
		return baseQuery + " WHERE exit_code = 0", nil
	case dir == "" && failFilter == FailFilterOnly:
		return baseQuery + " WHERE exit_code != 0", nil
	case failFilter == FailFilterInclude:
		return baseQuery + " WHERE directory = ?", []any{dir}
	case failFilter == FailFilterExclude:
		return baseQuery + " WHERE exit_code = 0 AND directory = ?", []any{dir}
	case failFilter == FailFilterOnly:
		return baseQuery + " WHERE exit_code != 0 AND directory = ?", []any{dir}
	default:
		return baseQuery, nil
	}
}

func dedupedCandidateQuery(dir string) (string, []any) {
	baseQuery := `SELECT history_id AS id, ts_ms, duration, exit_code, command, directory
		 FROM latest_command`

	if dir == "" {
		return baseQuery, nil
	}

	return baseQuery + " WHERE directory = ?", []any{dir}
}

func dedupedRawCandidateQuery(failFilter FailFilterMode, dir string) (string, []any) {
	where, args := candidateWhere(failFilter, dir)
	query := `SELECT id, ts_ms, duration, exit_code, command, directory
		 FROM (
		   SELECT id, ts_ms, duration, exit_code, command, directory,
		          ROW_NUMBER() OVER (PARTITION BY command ORDER BY ts_ms DESC, id DESC) AS rn
		   FROM history` + where + `
		 )
		 WHERE rn = 1`

	return query, args
}

func candidateWhere(failFilter FailFilterMode, dir string) (string, []any) {
	switch {
	case dir == "" && failFilter == FailFilterInclude:
		return "", nil
	case dir == "" && failFilter == FailFilterExclude:
		return " WHERE exit_code = 0", nil
	case dir == "" && failFilter == FailFilterOnly:
		return " WHERE exit_code != 0", nil
	case failFilter == FailFilterInclude:
		return " WHERE directory = ?", []any{dir}
	case failFilter == FailFilterExclude:
		return " WHERE exit_code = 0 AND directory = ?", []any{dir}
	case failFilter == FailFilterOnly:
		return " WHERE exit_code != 0 AND directory = ?", []any{dir}
	default:
		return "", nil
	}
}

func InsertIfNotExistsTx(tx *sql.Tx, entry HistoryEntry) (bool, error) {
	res, err := tx.ExecContext(
		context.Background(),
		`INSERT INTO history (ts_ms, duration, exit_code, command, directory, session_id, hostname)
		 SELECT ?, ?, ?, ?, ?, ?, ?
		 WHERE NOT EXISTS (
		   SELECT 1 FROM history
		   WHERE ts_ms = ?
		     AND duration = ?
		     AND exit_code = ?
		     AND command = ?
		     AND directory = ?
		     AND session_id = ?
		     AND hostname = ?
		 )`,
		entry.TSMs,
		entry.Duration,
		entry.ExitCode,
		entry.Command,
		entry.Directory,
		entry.SessionID,
		entry.Hostname,
		entry.TSMs,
		entry.Duration,
		entry.ExitCode,
		entry.Command,
		entry.Directory,
		entry.SessionID,
		entry.Hostname,
	)
	if err != nil {
		return false, fmt.Errorf("inserting history entry if not exists: %w", err)
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("reading affected rows for conditional insert: %w", err)
	}

	if rowsAffected > 0 {
		id, err := res.LastInsertId()
		if err != nil {
			return false, fmt.Errorf("reading inserted history ID: %w", err)
		}

		entry.ID = id
		if err = upsertLatestCommandTx(context.Background(), tx, entry); err != nil {
			return false, err
		}
	}

	return rowsAffected > 0, nil
}

func upsertLatestCommandTx(ctx context.Context, tx *sql.Tx, entry HistoryEntry) error {
	_, err := tx.ExecContext(
		ctx,
		`INSERT INTO latest_command (command, history_id, ts_ms, duration, exit_code, directory)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(command) DO UPDATE SET
		   history_id = excluded.history_id,
		   ts_ms = excluded.ts_ms,
		   duration = excluded.duration,
		   exit_code = excluded.exit_code,
		   directory = excluded.directory
		 WHERE excluded.ts_ms > latest_command.ts_ms
		    OR (excluded.ts_ms = latest_command.ts_ms AND excluded.history_id > latest_command.history_id)`,
		entry.Command,
		entry.ID,
		entry.TSMs,
		entry.Duration,
		entry.ExitCode,
		entry.Directory,
	)
	if err != nil {
		return fmt.Errorf("updating latest command cache: %w", err)
	}

	return nil
}

func rebuildLatestCommandTx(ctx context.Context, tx *sql.Tx, command string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM latest_command WHERE command = ?`, command); err != nil {
		return fmt.Errorf("clearing latest command cache for %q: %w", command, err)
	}

	_, err := tx.ExecContext(
		ctx,
		`INSERT INTO latest_command (command, history_id, ts_ms, duration, exit_code, directory)
		 SELECT command, id, ts_ms, duration, exit_code, directory
		 FROM history
		 WHERE command = ?
		 ORDER BY ts_ms DESC, id DESC
		 LIMIT 1`,
		command,
	)
	if err != nil {
		return fmt.Errorf("rebuilding latest command cache for %q: %w", command, err)
	}

	return nil
}

func dedupeEntries(entries []HistoryEntry) []HistoryEntry {
	seen := make(map[string]struct{}, len(entries))

	result := make([]HistoryEntry, 0, len(entries))
	for _, e := range entries {
		if _, ok := seen[e.Command]; ok {
			continue
		}

		seen[e.Command] = struct{}{}
		result = append(result, e)
	}

	return result
}

func scanCandidateEntries(rows *sql.Rows) ([]HistoryEntry, error) {
	var entries []HistoryEntry

	for rows.Next() {
		var e HistoryEntry

		err := rows.Scan(&e.ID, &e.TSMs, &e.Duration, &e.ExitCode, &e.Command, &e.Directory)
		if err != nil {
			return nil, fmt.Errorf("scanning history candidate row: %w", err)
		}

		entries = append(entries, e)
	}

	err := rows.Err()
	if err != nil {
		return nil, fmt.Errorf("iterating history candidate rows: %w", err)
	}

	return entries, nil
}

func scanEntries(rows *sql.Rows) ([]HistoryEntry, error) {
	var entries []HistoryEntry

	for rows.Next() {
		var e HistoryEntry

		err := rows.Scan(&e.ID, &e.TSMs, &e.Duration, &e.ExitCode,
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
