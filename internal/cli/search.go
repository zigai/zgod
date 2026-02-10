package cli

import (
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

var searchCmd = &cobra.Command{
	Use:   "search",
	Short: "Interactive history search",
	RunE:  runSearch,
}

//nolint:gochecknoinits // cobra CLI pattern
func init() {
	searchCmd.Flags().Bool("cwd", false, "filter by current directory")
	searchCmd.Flags().Int("height", 15, "visible result lines")
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
	cfg, err := config.Load()
	if err != nil {
		return 0, fmt.Errorf("loading config: %w", err)
	}

	if err = paths.EnsureDirs(); err != nil {
		return 0, fmt.Errorf("ensuring directories: %w", err)
	}

	dbPath, err := cfg.DatabasePath()
	if err != nil {
		return 0, fmt.Errorf("resolving database path: %w", err)
	}
	database, err := db.Open(dbPath)
	if err != nil {
		return 0, fmt.Errorf("opening database: %w", err)
	}
	defer func() { _ = database.Close() }()

	cwdFlag, _ := cmd.Flags().GetBool("cwd")
	height, _ := cmd.Flags().GetInt("height")
	query, _ := cmd.Flags().GetString("query")

	tty, err := openTTY()
	if err != nil {
		return 0, fmt.Errorf("opening TTY: %w", err)
	}
	defer func() { _ = tty.Close() }()

	profile := termenv.TrueColor
	output := termenv.NewOutput(tty, termenv.WithProfile(profile))
	termenv.SetDefaultOutput(output)
	renderer := lipgloss.NewRenderer(tty)
	renderer.SetColorProfile(profile)
	lipgloss.SetDefaultRenderer(renderer)

	cwd, err := os.Getwd()
	if err != nil {
		return 0, fmt.Errorf("getting current directory: %w", err)
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return 0, fmt.Errorf("getting home directory: %w", err)
	}
	repo := db.NewHistoryRepo(database)
	model := tui.NewModel(cfg, repo, cwd, homeDir, height, cwdFlag, query)

	p := tea.NewProgram(
		model,
		tea.WithInput(tty),
		tea.WithOutput(tty),
	)

	finalModel, err := p.Run()
	if err != nil {
		return 0, fmt.Errorf("running TUI: %w", err)
	}

	m, ok := finalModel.(tui.Model)
	if !ok {
		return 0, fmt.Errorf("unexpected model type: %T", finalModel)
	}
	if m.Canceled() {
		return 1, nil
	}
	if selected := m.Selected(); selected != "" {
		fmt.Print(selected)
		if cfg.Display.InstantExecute {
			return 2, nil
		}
	}
	return 0, nil
}
