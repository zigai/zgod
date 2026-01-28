package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/zigai/zgod/internal/config"
	"github.com/zigai/zgod/internal/db"
	"github.com/zigai/zgod/internal/history"
	"github.com/zigai/zgod/internal/paths"
)

var recordCmd = &cobra.Command{
	Use:    "record",
	Short:  "Record a command to history",
	Hidden: true,
	RunE:   runRecord,
}

//nolint:gochecknoinits // cobra CLI pattern
func init() {
	recordCmd.Flags().Int64("ts", 0, "timestamp in milliseconds")
	recordCmd.Flags().Int64("duration", 0, "duration in milliseconds")
	recordCmd.Flags().Int("exit-code", 0, "exit code")
	recordCmd.Flags().String("command", "", "command string")
	recordCmd.Flags().String("directory", "", "working directory")
	recordCmd.Flags().String("session", "", "session ID")
	rootCmd.AddCommand(recordCmd)
}

func runRecord(cmd *cobra.Command, args []string) error {
	command, _ := cmd.Flags().GetString("command")
	if command == "" {
		return nil
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	exitCode, _ := cmd.Flags().GetInt("exit-code")
	directory, _ := cmd.Flags().GetString("directory")

	filter, err := history.NewFilter(cfg.Filters)
	if err != nil {
		return fmt.Errorf("building filter: %w", err)
	}
	if !filter.ShouldRecord(command, exitCode, directory) {
		return nil
	}

	if err = paths.EnsureDirs(); err != nil {
		return fmt.Errorf("ensuring directories: %w", err)
	}

	database, err := db.Open(cfg.DatabasePath())
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer func() { _ = database.Close() }()

	ts, _ := cmd.Flags().GetInt64("ts")
	duration, _ := cmd.Flags().GetInt64("duration")
	sessionID, _ := cmd.Flags().GetString("session")
	hostname := getHostname()

	repo := db.NewHistoryRepo(database)
	_, err = repo.Insert(db.HistoryEntry{
		TsMs:      ts,
		Duration:  duration,
		ExitCode:  exitCode,
		Command:   command,
		Directory: directory,
		SessionID: sessionID,
		Hostname:  hostname,
	})
	return err
}

func getHostname() string {
	h, _ := os.Hostname()
	return h
}
