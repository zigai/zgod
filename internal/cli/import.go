package cli

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

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
	skippedDuplicate   int
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

	targetDB, sourceDB, err := openImportDatabases(targetPath, sourcePath)
	if err != nil {
		return err
	}

	defer closeImportDatabases(targetDB, sourceDB)

	sourceEntries, err := listSourceEntries(sourceDB)
	if err != nil {
		return err
	}

	summary, err := importHistoryEntries(targetDB, sourceEntries, opts)
	if err != nil {
		return err
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
	if err := requireImportAuthentication(); err != nil {
		return nil, nil, fmt.Errorf("authenticating import: %w", err)
	}

	if err := paths.EnsureDirs(); err != nil {
		return nil, nil, fmt.Errorf("ensuring directories: %w", err)
	}

	targetDB, err := db.Open(targetPath)
	if err != nil {
		return nil, nil, fmt.Errorf("opening target database: %w", err)
	}

	sourceDB, err := db.OpenReadOnly(sourcePath)
	if err != nil {
		_ = targetDB.Close()
		return nil, nil, fmt.Errorf("opening source database: %w", err)
	}

	if err = db.ValidateHistorySchema(sourceDB); err != nil {
		_ = sourceDB.Close()
		_ = targetDB.Close()

		return nil, nil, fmt.Errorf("validating source database schema: %w", err)
	}

	return targetDB, sourceDB, nil
}

func closeImportDatabases(targetDB *sql.DB, sourceDB *sql.DB) {
	_ = sourceDB.Close()
	_ = targetDB.Close()
}

func listSourceEntries(sourceDB *sql.DB) ([]db.HistoryEntry, error) {
	sourceRepo := db.NewHistoryRepo(sourceDB)

	sourceEntries, err := sourceRepo.ListAll()
	if err != nil {
		return nil, fmt.Errorf("reading source history entries: %w", err)
	}

	return sourceEntries, nil
}

func printImportSummary(cmd *cobra.Command, summary importSummary) {
	cmd.Printf(
		"Import complete: total=%d imported=%d skipped_failed=%d skipped_missing_paths=%d skipped_duplicates=%d\n",
		summary.total,
		summary.imported,
		summary.skippedFailed,
		summary.skippedMissingPath,
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

		return "", fmt.Errorf("stating source database %q: %w", path, err)
	}

	return path, nil
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

func importHistoryEntries(
	targetDB *sql.DB,
	entries []db.HistoryEntry,
	opts importOptions,
) (importSummary, error) {
	tx, err := targetDB.BeginTx(context.Background(), nil)
	if err != nil {
		return importSummary{}, fmt.Errorf("starting import transaction: %w", err)
	}

	committed := false

	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	summary := newImportSummary()
	for _, entry := range entries {
		summary.total++

		if !opts.includeFailed && entry.ExitCode != 0 {
			summary.skippedFailed++
			continue
		}

		if !opts.includeMissingPaths {
			pathsExist, pathsErr := commandReferencesExistingPaths(entry.Command, entry.Directory)
			if pathsErr != nil || !pathsExist {
				summary.skippedMissingPath++
				continue
			}
		}

		inserted, insertErr := db.InsertIfNotExistsTx(tx, entry)
		if insertErr != nil {
			return importSummary{}, fmt.Errorf("importing history entry: %w", insertErr)
		}

		if inserted {
			summary.imported++
			continue
		}

		summary.skippedDuplicate++
	}

	if err = tx.Commit(); err != nil {
		return importSummary{}, fmt.Errorf("committing import transaction: %w", err)
	}

	committed = true

	return summary, nil
}

func newImportSummary() importSummary {
	return importSummary{
		total:              0,
		imported:           0,
		skippedFailed:      0,
		skippedMissingPath: 0,
		skippedDuplicate:   0,
	}
}
