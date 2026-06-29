package cli

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zigai/zgod/internal/config"
	"github.com/zigai/zgod/internal/db"
	"github.com/zigai/zgod/internal/paths"
)

var (
	errImportSourceEqualsTarget = errors.New("source database must be different from target database")
	errImportSourceRequired     = errors.New("source database path is required")
	errImportSourceNotFound     = errors.New("source database does not exist")
)

const (
	createImportStageSQL = `
CREATE TEMP TABLE import_stage (
    stage_id   INTEGER PRIMARY KEY,
    ts_ms      INTEGER NOT NULL,
    duration   INTEGER NOT NULL,
    exit_code  INTEGER NOT NULL,
    command    TEXT    NOT NULL,
    directory  TEXT    NOT NULL,
    session_id TEXT    NOT NULL,
    hostname   TEXT    NOT NULL,
    UNIQUE (ts_ms, duration, exit_code, command, directory, session_id, hostname)
)`

	dropImportStageSQL = `DROP TABLE IF EXISTS temp.import_stage`
)

var importCmd = &cobra.Command{
	Use:          "import <source-db-path>",
	Short:        "Import history from another SQLite database",
	SilenceUsage: true,
	Args:         cobra.ExactArgs(1),
	RunE:         runImport,
}

type importOptions struct {
	includeFailed       bool
	includeMissingPaths bool
}

type importSummary struct {
	total              int
	imported           int
	skippedFailed      int
	skippedMissingPath int
	skippedPathError   int
	skippedDuplicate   int
}

type historyEntryStreamer func(context.Context, func(db.HistoryEntry) error) error

type importStageResult struct {
	summary     importSummary
	stagedCount int
}

type importStageBuilder struct {
	tx          *sql.Tx
	opts        importOptions
	pathChecker *importPathCheckCache
	result      importStageResult
}

type importPathCheckStatus int

const (
	importPathExists importPathCheckStatus = iota
	importPathMissing
	importPathError
)

type importPathCheckCache struct {
	commands map[importCommandPathKey]importPathCheckStatus
	paths    map[importResolvedPathKey]importPathCheckResult
}

type importCommandPathKey struct {
	command          string
	workingDirectory string
}

type importResolvedPathKey struct {
	path        string
	requirement pathRequirement
}

type importPathCheckResult struct {
	exists bool
	err    error
}

func registerImportCommand() {
	importCmd.Flags().Bool("include-failed", false, "Include commands with non-zero exit code")
	importCmd.Flags().Bool(
		"include-missing-paths",
		false,
		"Include commands that reference paths missing on this machine",
	)
	rootCmd.AddCommand(importCmd)
}

func runImport(cmd *cobra.Command, args []string) error {
	opts, err := readImportOptions(cmd)
	if err != nil {
		return err
	}

	sourcePath, targetPath, err := resolveImportPaths(args)
	if err != nil {
		return err
	}

	sourceDB, err := openImportSourceDatabase(sourcePath)
	if err != nil {
		return err
	}

	defer func() { _ = sourceDB.Close() }()

	var summary importSummary

	err = db.WithDatabaseWriteLock(context.Background(), targetPath, func() error {
		targetDB, err := openImportTargetDatabase(targetPath)
		if err != nil {
			return err
		}

		defer func() { _ = targetDB.Close() }()

		summary, err = importSourceHistoryEntries(targetDB, sourceDB, opts)

		return err
	})
	if err != nil {
		return fmt.Errorf("locking target database for import: %w", err)
	}

	printImportSummary(cmd, summary)

	return nil
}

func resolveImportPaths(args []string) (string, string, error) {
	sourcePath, err := resolveExistingPath(args)
	if err != nil {
		return "", "", err
	}

	targetPath, err := resolveTargetImportPath()
	if err != nil {
		return "", "", err
	}

	sameFile, err := pathsReferToSameFile(sourcePath, targetPath)
	if err != nil {
		return "", "", err
	}

	if sameFile {
		return "", "", errImportSourceEqualsTarget
	}

	return sourcePath, targetPath, nil
}

func resolveTargetImportPath() (string, error) {
	cfg, err := config.Load()
	if err != nil {
		return "", fmt.Errorf("loading config: %w", err)
	}

	targetPath, err := cfg.DatabasePath()
	if err != nil {
		return "", fmt.Errorf("resolving database path: %w", err)
	}

	targetPath, err = normalizePath(targetPath)
	if err != nil {
		return "", fmt.Errorf("normalizing target database path: %w", err)
	}

	return targetPath, nil
}

func openImportDatabases(targetPath string, sourcePath string) (*sql.DB, *sql.DB, error) {
	sourceDB, err := openImportSourceDatabase(sourcePath)
	if err != nil {
		return nil, nil, err
	}

	targetDB, err := openImportTargetDatabase(targetPath)
	if err != nil {
		_ = sourceDB.Close()

		return nil, nil, err
	}

	return targetDB, sourceDB, nil
}

func openImportSourceDatabase(sourcePath string) (*sql.DB, error) {
	sourceDB, err := db.OpenReadOnly(sourcePath)
	if err != nil {
		return nil, wrapImportSourceAccessError("opening", sourcePath, err)
	}

	if err = db.ValidateHistorySchema(sourceDB); err != nil {
		_ = sourceDB.Close()

		return nil, fmt.Errorf("validating source database schema: %w", err)
	}

	return sourceDB, nil
}

func openImportTargetDatabase(targetPath string) (*sql.DB, error) {
	if err := paths.EnsureDirs(); err != nil {
		return nil, fmt.Errorf("ensuring directories: %w", err)
	}

	targetDB, err := db.Open(targetPath)
	if err != nil {
		return nil, fmt.Errorf("opening target database: %w", err)
	}

	return targetDB, nil
}

func closeImportDatabases(targetDB *sql.DB, sourceDB *sql.DB) {
	_ = sourceDB.Close()
	_ = targetDB.Close()
}

func printImportSummary(cmd *cobra.Command, summary importSummary) {
	cmd.Printf(
		"Import complete: total=%d imported=%d skipped_failed=%d skipped_missing_paths=%d skipped_path_errors=%d skipped_duplicates=%d\n",
		summary.total,
		summary.imported,
		summary.skippedFailed,
		summary.skippedMissingPath,
		summary.skippedPathError,
		summary.skippedDuplicate,
	)
}

func readImportOptions(cmd *cobra.Command) (importOptions, error) {
	includeFailed, err := cmd.Flags().GetBool("include-failed")
	if err != nil {
		return importOptions{}, fmt.Errorf("reading --include-failed flag: %w", err)
	}

	includeMissingPaths, err := cmd.Flags().GetBool("include-missing-paths")
	if err != nil {
		return importOptions{}, fmt.Errorf("reading --include-missing-paths flag: %w", err)
	}

	return importOptions{
		includeFailed:       includeFailed,
		includeMissingPaths: includeMissingPaths,
	}, nil
}

func resolveExistingPath(args []string) (string, error) {
	if len(args) == 0 || args[0] == "" {
		return "", errImportSourceRequired
	}

	path, err := paths.ExpandTilde(args[0])
	if err != nil {
		return "", fmt.Errorf("expanding source database path %q: %w", args[0], err)
	}

	path, err = normalizePath(path)
	if err != nil {
		return "", fmt.Errorf("normalizing source database path: %w", err)
	}

	if _, err = os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("%w: %q", errImportSourceNotFound, path)
		}

		return "", wrapImportSourceAccessError("stating", path, err)
	}

	return path, nil
}

func wrapImportSourceAccessError(action string, sourcePath string, err error) error {
	if !isPermissionDeniedError(err) {
		return fmt.Errorf("%s source database %q: %w", action, sourcePath, err)
	}

	return fmt.Errorf(
		"%s source database %q: permission denied; %s: %w",
		action,
		sourcePath,
		importSourcePermissionHint(),
		err,
	)
}

func isPermissionDeniedError(err error) bool {
	if errors.Is(err, os.ErrPermission) {
		return true
	}

	message := strings.ToLower(err.Error())

	return strings.Contains(message, "permission denied") || strings.Contains(message, "access is denied")
}

func importSourcePermissionHint() string {
	if runtime.GOOS == "windows" {
		return "rerun from an elevated terminal or adjust file permissions"
	}

	return "rerun the command with sudo or adjust file permissions"
}

func normalizePath(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("building absolute path for %q: %w", path, err)
	}

	absPath = filepath.Clean(absPath)

	resolvedPath, err := filepath.EvalSymlinks(absPath)
	if err == nil {
		return filepath.Clean(resolvedPath), nil
	}

	if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("resolving symlinks for %q: %w", absPath, err)
	}

	return absPath, nil
}

func pathsReferToSameFile(sourcePath string, targetPath string) (bool, error) {
	sourceInfo, err := os.Stat(sourcePath)
	if err != nil {
		return false, fmt.Errorf("stating source database %q: %w", sourcePath, err)
	}

	targetInfo, err := os.Stat(targetPath)
	if err == nil {
		return os.SameFile(sourceInfo, targetInfo), nil
	}

	if !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("stating target database %q: %w", targetPath, err)
	}

	return sourcePath == targetPath, nil
}

func importSourceHistoryEntries(targetDB *sql.DB, sourceDB *sql.DB, opts importOptions) (importSummary, error) {
	sourceRepo := db.NewHistoryRepo(sourceDB)

	return importHistoryEntriesFromStreamer(targetDB, opts, sourceRepo.ForEach)
}

func importHistoryEntries(
	targetDB *sql.DB,
	entries []db.HistoryEntry,
) (importSummary, error) {
	return importHistoryEntriesFromStreamer(
		targetDB,
		importOptions{
			includeFailed:       false,
			includeMissingPaths: false,
		},
		func(ctx context.Context, visit func(db.HistoryEntry) error) error {
			for _, entry := range entries {
				if err := ctx.Err(); err != nil {
					return fmt.Errorf("checking import context: %w", err)
				}

				if err := visit(entry); err != nil {
					return err
				}
			}

			return nil
		},
	)
}

func importHistoryEntriesFromStreamer(
	targetDB *sql.DB,
	opts importOptions,
	stream historyEntryStreamer,
) (importSummary, error) {
	ctx := context.Background()

	tx, err := targetDB.BeginTx(ctx, nil)
	if err != nil {
		return importSummary{}, fmt.Errorf("starting import transaction: %w", err)
	}

	committed := false

	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if err = resetImportStageTx(ctx, tx); err != nil {
		return importSummary{}, err
	}

	stageResult, err := stageStreamedImportEntriesTx(ctx, tx, opts, stream)
	if err != nil {
		return importSummary{}, err
	}

	imported, err := insertStagedHistoryTx(ctx, tx)
	if err != nil {
		return importSummary{}, err
	}

	summary := stageResult.summary
	summary.imported = imported
	summary.skippedDuplicate += stageResult.stagedCount - imported

	if stageResult.stagedCount > 0 {
		if err = upsertLatestCommandsForImportStageTx(ctx, tx); err != nil {
			return importSummary{}, err
		}
	}

	if err = dropImportStageTx(ctx, tx); err != nil {
		return importSummary{}, err
	}

	if err = tx.Commit(); err != nil {
		return importSummary{}, fmt.Errorf("committing import transaction: %w", err)
	}

	committed = true

	return summary, nil
}

func stageStreamedImportEntriesTx(
	ctx context.Context,
	tx *sql.Tx,
	opts importOptions,
	stream historyEntryStreamer,
) (importStageResult, error) {
	builder := importStageBuilder{
		tx:          tx,
		opts:        opts,
		pathChecker: newImportPathCheckCache(),
		result: importStageResult{
			summary:     newImportSummary(),
			stagedCount: 0,
		},
	}

	err := stream(ctx, func(entry db.HistoryEntry) error {
		return builder.stage(ctx, entry)
	})
	if err != nil {
		return importStageResult{}, fmt.Errorf("streaming source history entries: %w", err)
	}

	return builder.result, nil
}

func (b *importStageBuilder) stage(ctx context.Context, entry db.HistoryEntry) error {
	b.result.summary.total++

	if b.skip(entry) {
		return nil
	}

	inserted, err := stageImportEntryTx(ctx, b.tx, entry)
	if err != nil {
		return fmt.Errorf("staging history entry: %w", err)
	}

	if inserted {
		b.result.stagedCount++
		return nil
	}

	b.result.summary.skippedDuplicate++

	return nil
}

func (b *importStageBuilder) skip(entry db.HistoryEntry) bool {
	if !b.opts.includeFailed && entry.ExitCode != 0 {
		b.result.summary.skippedFailed++
		return true
	}

	if b.opts.includeMissingPaths {
		return false
	}

	switch b.pathChecker.commandPathStatus(entry.Command, entry.Directory) {
	case importPathExists:
		return false
	case importPathMissing:
		b.result.summary.skippedMissingPath++
	case importPathError:
		b.result.summary.skippedPathError++
	}

	return true
}

func resetImportStageTx(ctx context.Context, tx *sql.Tx) error {
	if err := dropImportStageTx(ctx, tx); err != nil {
		return err
	}

	_, err := tx.ExecContext(ctx, createImportStageSQL)
	if err != nil {
		return fmt.Errorf("creating temporary import stage: %w", err)
	}

	return nil
}

func dropImportStageTx(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, dropImportStageSQL)
	if err != nil {
		return fmt.Errorf("dropping temporary import stage: %w", err)
	}

	return nil
}

func stageImportEntryTx(ctx context.Context, tx *sql.Tx, entry db.HistoryEntry) (bool, error) {
	res, err := tx.ExecContext(
		ctx,
		`INSERT OR IGNORE INTO import_stage (
		   ts_ms, duration, exit_code, command, directory, session_id, hostname
		 )
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		entry.TimestampMS,
		entry.DurationMS,
		entry.ExitCode,
		entry.Command,
		entry.Directory,
		entry.SessionID,
		entry.Hostname,
	)
	if err != nil {
		return false, fmt.Errorf("inserting temporary import stage row: %w", err)
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("reading affected rows for temporary import stage row: %w", err)
	}

	return rowsAffected > 0, nil
}

func insertStagedHistoryTx(ctx context.Context, tx *sql.Tx) (int, error) {
	res, err := tx.ExecContext(
		ctx,
		`INSERT INTO history (ts_ms, duration, exit_code, command, directory, session_id, hostname)
		 SELECT s.ts_ms, s.duration, s.exit_code, s.command, s.directory, s.session_id, s.hostname
		 FROM import_stage s
		 WHERE NOT EXISTS (
		   SELECT 1 FROM history h
		   WHERE h.ts_ms = s.ts_ms
		     AND h.duration = s.duration
		     AND h.exit_code = s.exit_code
		     AND h.command = s.command
		     AND h.directory = s.directory
		     AND h.session_id = s.session_id
		     AND h.hostname = s.hostname
		 )
		 ORDER BY s.stage_id`,
	)
	if err != nil {
		return 0, fmt.Errorf("inserting staged history rows: %w", err)
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("reading affected rows for staged history insert: %w", err)
	}

	return int(rowsAffected), nil
}

func upsertLatestCommandsForImportStageTx(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(
		ctx,
		`INSERT INTO latest_command (command, history_id, ts_ms, duration, exit_code, directory)
		 SELECT h.command, h.id, h.ts_ms, h.duration, h.exit_code, h.directory
		 FROM history h
		 JOIN import_stage s
		   ON h.ts_ms = s.ts_ms
		  AND h.duration = s.duration
		  AND h.exit_code = s.exit_code
		  AND h.command = s.command
		  AND h.directory = s.directory
		  AND h.session_id = s.session_id
		  AND h.hostname = s.hostname
		 WHERE 1 = 1
		 ON CONFLICT(command) DO UPDATE SET
		   history_id = excluded.history_id,
		   ts_ms = excluded.ts_ms,
		   duration = excluded.duration,
		   exit_code = excluded.exit_code,
		   directory = excluded.directory
		 WHERE excluded.ts_ms > latest_command.ts_ms
		    OR (excluded.ts_ms = latest_command.ts_ms AND excluded.history_id > latest_command.history_id)`,
	)
	if err != nil {
		return fmt.Errorf("updating latest command cache for import: %w", err)
	}

	return nil
}

func newImportPathCheckCache() *importPathCheckCache {
	return &importPathCheckCache{
		commands: map[importCommandPathKey]importPathCheckStatus{},
		paths:    map[importResolvedPathKey]importPathCheckResult{},
	}
}

func (c *importPathCheckCache) commandPathStatus(command string, workingDirectory string) importPathCheckStatus {
	key := importCommandPathKey{
		command:          command,
		workingDirectory: workingDirectory,
	}
	if status, ok := c.commands[key]; ok {
		return status
	}

	exists, err := commandReferencesExistingPathsWithMatcher(command, workingDirectory, c.pathMatchesRequirement)
	status := importPathExists

	switch {
	case err != nil:
		status = importPathError
	case !exists:
		status = importPathMissing
	}

	c.commands[key] = status

	return status
}

func (c *importPathCheckCache) pathMatchesRequirement(path string, requirement pathRequirement) (bool, error) {
	key := importResolvedPathKey{
		path:        path,
		requirement: requirement,
	}
	if result, ok := c.paths[key]; ok {
		return result.exists, result.err
	}

	exists, err := commandPathMatchesRequirement(path, requirement)
	c.paths[key] = importPathCheckResult{
		exists: exists,
		err:    err,
	}

	return exists, err
}

func newImportSummary() importSummary {
	return importSummary{
		total:              0,
		imported:           0,
		skippedFailed:      0,
		skippedMissingPath: 0,
		skippedPathError:   0,
		skippedDuplicate:   0,
	}
}
