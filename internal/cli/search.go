package cli

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/spf13/cobra"

	"github.com/zigai/zgod/internal/config"
	"github.com/zigai/zgod/internal/db"
	"github.com/zigai/zgod/internal/tui"
)

const (
	searchDefaultHeight       = 15
	searchExitCodeCanceled    = 1
	searchExitCodeInstantExec = 2
	searchPendingDrainTimeout = 250 * time.Millisecond
)

var errUnexpectedModelType = errors.New("unexpected model type")

type searchContext struct {
	cfg     config.Config
	model   *tui.Model
	ttyIn   *os.File
	ttyOut  *os.File
	cleanup func()
}

func registerSearchCommand(root *cobra.Command) {
	searchCmd := &cobra.Command{
		Use: "search", Short: "Search shell history", GroupID: "core", RunE: runSearch,
		Long:    "Search interactively when stdin and stdout are terminals.\nWhen either stream is redirected, print matching commands as plain text.\nUse --json for structured records or --shell for interactive shell integration.",
		Example: "  zgod search\n  zgod search --cwd=false --query git\n  zgod search --query git | head\n  zgod search --json --mode regex --query '^git ' --limit 20",
	}
	searchCmd.Flags().Bool("cwd", false, "filter by current directory")
	searchCmd.Flags().Int("height", 0, "Visible result lines (1–1000; default 15)")
	searchCmd.Flags().String("query", "", "initial search query")
	searchCmd.Flags().Bool("json", false, "Print matching history records as JSON without opening the UI")
	searchCmd.Flags().String("mode", "", "Match mode: fuzzy (approximate), regex (regular expression), glob (wildcards)")
	searchCmd.Flags().Int("limit", 0, "Maximum matching records to print (1–100000; default 1000)")
	searchCmd.Flags().Bool("shell", false, "Return select/execute on the first line and the selected command below; success exits 0")
	root.AddCommand(searchCmd)
}

func runSearch(cmd *cobra.Command, _ []string) error {
	cfg, err := searchConfig(cmd)
	if err != nil {
		return err
	}

	if !searchIsInteractive(cmd) {
		return runSearchOutput(cmd, cfg)
	}

	exitCode, err := doSearch(cmd, cfg)
	if err != nil {
		if errors.Is(err, tea.ErrInterrupted) {
			return exitError{code: exitInterrupted}
		}

		return err
	}

	if exitCode != 0 {
		return exitError{code: exitCode}
	}

	return nil
}

func searchIsInteractive(cmd *cobra.Command) bool {
	if flagBool(cmd, "json") {
		return false
	}

	// Shell integration captures stdout but still needs the selection UI.
	if flagBool(cmd, "shell") {
		return true
	}

	return inputIsTerminal(cmd) && outputIsTerminal(cmd)
}

func searchConfig(cmd *cobra.Command) (config.Config, error) {
	opts := configOptions(cmd)
	opts.NoCreate = true

	cfg, err := config.LoadWithOptions(opts)
	if err != nil {
		return cfg, fmt.Errorf("loading configuration: %w", err)
	}

	if err = validateQuery(cfg.Display.DefaultMode, flagString(cmd, "query")); err != nil {
		return cfg, err
	}

	if err = config.EnsureDefault(opts); err != nil {
		return cfg, fmt.Errorf("creating default configuration: %w", err)
	}

	return cfg, nil
}

func doSearch(cmd *cobra.Command, cfg config.Config) (int, error) {
	if cfg.Keys.ToggleCWD == "ctrl+d" {
		if _, err := fmt.Fprintln(cmd.ErrOrStderr(), "Warning: keys.toggle_cwd=ctrl+d is deprecated; use alt+d. Ctrl-D on empty input now cancels search."); err != nil {
			return 0, fmt.Errorf("writing keybinding migration warning: %w", err)
		}
	}

	ctx, err := prepareSearchContext(cmd, cfg)
	if err != nil {
		return 0, err
	}
	defer ctx.cleanup()

	p := tea.NewProgram(
		ctx.model,
		tea.WithInput(ctx.ttyIn),
		tea.WithOutput(ctx.ttyOut),
		tea.WithMouseCellMotion(),
		tea.WithAltScreen(),
	)

	finalModel, err := p.Run()
	if err != nil {
		return 0, fmt.Errorf("running TUI: %w", err)
	}

	return resolveSearchResult(cmd, ctx.cfg, finalModel)
}

func prepareSearchContext(cmd *cobra.Command, cfg config.Config) (searchContext, error) {
	dbPath, err := cfg.DatabasePath()
	if err != nil {
		return searchContext{}, fmt.Errorf("resolving database path: %w", err)
	}

	if err = drainPendingRecordsForSearch(dbPath); err != nil {
		return searchContext{}, fmt.Errorf("draining pending history records: %w", err)
	}

	database, err := openSearchDatabase(dbPath)
	if err != nil {
		return searchContext{}, fmt.Errorf("opening database: %w", err)
	}

	cwdFlag, _ := cmd.Flags().GetBool("cwd")

	height, _ := cmd.Flags().GetInt("height")
	if !cmd.Flags().Changed("height") {
		height = searchDefaultHeight
	}

	query, _ := cmd.Flags().GetString("query")

	ttyIn, ttyOut, ttyCleanup, err := openTTY()
	if err != nil {
		_ = database.Close()
		return searchContext{}, fmt.Errorf("opening TTY: %w", err)
	}

	profile := termenv.ANSI
	if flagBool(cmd, "no-color") || (!flagBool(cmd, "color") && (os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb")) {
		profile = termenv.Ascii
	}

	output := termenv.NewOutput(ttyOut, termenv.WithProfile(profile))
	termenv.SetDefaultOutput(output)

	renderer := lipgloss.NewRenderer(ttyOut)
	renderer.SetColorProfile(profile)
	lipgloss.SetDefaultRenderer(renderer)

	cwd, err := os.Getwd()
	if err != nil {
		ttyCleanup()

		_ = database.Close()

		return searchContext{}, fmt.Errorf("getting current directory: %w", err)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		ttyCleanup()

		_ = database.Close()

		return searchContext{}, fmt.Errorf("getting home directory: %w", err)
	}

	repo := db.NewHistoryRepo(database)
	model := tui.NewModel(cfg, repo, cwd, homeDir, height, cwdFlag, query)
	cleanup := func() {
		ttyCleanup()

		_ = database.Close()
	}

	return searchContext{
		cfg:     cfg,
		model:   model,
		ttyIn:   ttyIn,
		ttyOut:  ttyOut,
		cleanup: cleanup,
	}, nil
}

func drainPendingRecordsForSearch(dbPath string) error {
	if _, err := os.Stat(pendingRecordsDir(dbPath)); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}

		return fmt.Errorf("stating pending history directory: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), searchPendingDrainTimeout)
	defer cancel()

	err := db.WithDatabaseWriteLock(ctx, dbPath, func() error {
		return drainPendingRecordsLocked(dbPath)
	})
	if errors.Is(err, db.ErrDatabaseWriteLockTimeout) || db.IsBusyError(err) {
		return nil
	}

	if err == nil {
		return nil
	}

	return fmt.Errorf("draining pending history records under database write lock: %w", err)
}

func openSearchDatabase(dbPath string) (*sql.DB, error) {
	if _, err := os.Stat(dbPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			database, err := db.Open(dbPath)
			if err != nil {
				return nil, fmt.Errorf("opening search database %q: %w", dbPath, err)
			}

			return database, nil
		}

		return nil, fmt.Errorf("stating database file %q: %w", dbPath, err)
	}

	database, err := db.OpenReadOnly(dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening search database read-only %q: %w", dbPath, err)
	}

	if err = db.ValidateSupportedSchemaVersion(database); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("validating history schema version: %w", err)
	}

	if err = db.ValidateHistorySchema(database); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("validating history schema: %w", err)
	}

	return database, nil
}

func resolveSearchResult(cmd *cobra.Command, cfg config.Config, finalModel tea.Model) (int, error) {
	m, ok := finalModel.(*tui.Model)
	if !ok {
		return 0, fmt.Errorf("%w: %T", errUnexpectedModelType, finalModel)
	}

	if m.Interrupted() {
		return exitInterrupted, nil
	}

	if m.Canceled() {
		return searchExitCodeCanceled, nil
	}

	selected := m.Selected()

	if flagBool(cmd, "shell") {
		action := "select"
		if cfg.Display.InstantExecute {
			action = "execute"
		}

		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\n%s", action, selected); err != nil {
			return 0, fmt.Errorf("writing shell result: %w", err)
		}

		return 0, nil
	}

	if _, err := fmt.Fprint(cmd.OutOrStdout(), selected); err != nil {
		return 0, fmt.Errorf("writing selection: %w", err)
	}

	if selected != "" && cfg.Display.InstantExecute {
		if _, err := fmt.Fprintln(cmd.ErrOrStderr(), "Warning: search exit code 2 is deprecated; regenerate shell integration with 'zgod init <shell>' or use 'zgod search --shell'."); err != nil {
			return 0, fmt.Errorf("writing compatibility warning: %w", err)
		}

		return searchExitCodeInstantExec, nil
	}

	return 0, nil
}
