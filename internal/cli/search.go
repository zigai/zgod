package cli

import (
	"errors"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/spf13/cobra"

	"github.com/zigai/zgod/internal/config"
	"github.com/zigai/zgod/internal/db"
	"github.com/zigai/zgod/internal/paths"
	"github.com/zigai/zgod/internal/tui"
)

var errUnexpectedModelType = errors.New("unexpected model type")

var searchCmd = &cobra.Command{
	Use:   "search",
	Short: "Interactive history search",
	RunE:  runSearch,
}

const (
	searchDefaultHeight       = 15
	searchExitCodeCanceled    = 1
	searchExitCodeInstantExec = 2
)

type searchContext struct {
	cfg     config.Config
	model   *tui.Model
	tty     *os.File
	cleanup func()
}

func init() {
	searchCmd.Flags().Bool("cwd", false, "filter by current directory")
	searchCmd.Flags().Int("height", searchDefaultHeight, "visible result lines")
	searchCmd.Flags().String("query", "", "initial search query")
	rootCmd.AddCommand(searchCmd)
}

func runSearch(cmd *cobra.Command, args []string) error {
	exitCode, err := doSearch(cmd)
	if err != nil {
		return err
	}
	if exitCode != 0 {
		os.Exit(exitCode)
	}
	return nil
}

func doSearch(cmd *cobra.Command) (int, error) {
	ctx, err := prepareSearchContext(cmd)
	if err != nil {
		return 0, err
	}
	defer ctx.cleanup()

	p := tea.NewProgram(
		ctx.model,
		tea.WithInput(ctx.tty),
		tea.WithOutput(ctx.tty),
	)

	finalModel, err := p.Run()
	if err != nil {
		return 0, fmt.Errorf("running TUI: %w", err)
	}

	return resolveSearchResult(ctx.cfg, finalModel)
}

func prepareSearchContext(cmd *cobra.Command) (searchContext, error) {
	cfg, err := config.Load()
	if err != nil {
		return searchContext{}, fmt.Errorf("loading config: %w", err)
	}

	if err = paths.EnsureDirs(); err != nil {
		return searchContext{}, fmt.Errorf("ensuring directories: %w", err)
	}

	dbPath, err := cfg.DatabasePath()
	if err != nil {
		return searchContext{}, fmt.Errorf("resolving database path: %w", err)
	}
	database, err := db.Open(dbPath)
	if err != nil {
		return searchContext{}, fmt.Errorf("opening database: %w", err)
	}

	cwdFlag, _ := cmd.Flags().GetBool("cwd")
	height, _ := cmd.Flags().GetInt("height")
	query, _ := cmd.Flags().GetString("query")

	tty, err := openTTY()
	if err != nil {
		_ = database.Close()
		return searchContext{}, fmt.Errorf("opening TTY: %w", err)
	}

	profile := termenv.TrueColor
	output := termenv.NewOutput(tty, termenv.WithProfile(profile))
	termenv.SetDefaultOutput(output)
	renderer := lipgloss.NewRenderer(tty)
	renderer.SetColorProfile(profile)
	lipgloss.SetDefaultRenderer(renderer)

	cwd, err := os.Getwd()
	if err != nil {
		_ = tty.Close()
		_ = database.Close()
		return searchContext{}, fmt.Errorf("getting current directory: %w", err)
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		_ = tty.Close()
		_ = database.Close()
		return searchContext{}, fmt.Errorf("getting home directory: %w", err)
	}
	repo := db.NewHistoryRepo(database)
	model := tui.NewModel(cfg, repo, cwd, homeDir, height, cwdFlag, query)
	cleanup := func() {
		_ = tty.Close()
		_ = database.Close()
	}
	return searchContext{
		cfg:     cfg,
		model:   model,
		tty:     tty,
		cleanup: cleanup,
	}, nil
}

func resolveSearchResult(cfg config.Config, finalModel tea.Model) (int, error) {
	m, ok := finalModel.(*tui.Model)
	if !ok {
		return 0, fmt.Errorf("%w: %T", errUnexpectedModelType, finalModel)
	}
	if m.Canceled() {
		return searchExitCodeCanceled, nil
	}
	if selected := m.Selected(); selected != "" {
		fmt.Print(selected)
		if cfg.Display.InstantExecute {
			return searchExitCodeInstantExec, nil
		}
	}
	return 0, nil
}
