package tui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/zigai/zgod/internal/config"
)

type Styles struct {
	Prompt          lipgloss.Style
	Match           lipgloss.Style
	HeaderBar       lipgloss.Style
	Input           lipgloss.Style
	Footer          lipgloss.Style
	Border          lipgloss.Style
	Title           lipgloss.Style
	HelpKey         lipgloss.Style
	HelpDesc        lipgloss.Style
	Dimmed          lipgloss.Style
	Meta            lipgloss.Style
	ExitOk          lipgloss.Style
	ExitFail        lipgloss.Style
	ColumnHeader    lipgloss.Style
	ColumnHeaderBar lipgloss.Style
	SelectionBar    lipgloss.Style
	SelectedCmd     lipgloss.Style
	Cmd             lipgloss.Style
}

func NewStyles(theme config.ThemeConfig) Styles {
	matchStyle := lipgloss.NewStyle().Foreground(parseColor(theme.MatchColor))
	if config.BoolDefault(theme.MatchBold, true) {
		matchStyle = matchStyle.Bold(true)
	}

	if config.BoolDefault(theme.MatchUnderline, true) {
		matchStyle = matchStyle.Underline(true)
	}

	if theme.MatchBg != "" {
		matchStyle = matchStyle.Background(parseColor(theme.MatchBg))
	}

	barColor := theme.SelectionBarColor
	if barColor == "" {
		barColor = "14"
	}

	selectionBarStyle := lipgloss.NewStyle().
		Foreground(parseColor(barColor)).
		Bold(true)

	return Styles{
		Prompt: lipgloss.NewStyle().
			Foreground(parseColor(theme.PromptColor)).
			Bold(true).
			PaddingRight(1),

		Match: matchStyle,

		HeaderBar: lipgloss.NewStyle().Padding(0, 1),

		Input: lipgloss.NewStyle(),

		Footer: lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")).
			Padding(0, 1),

		Border: lipgloss.NewStyle(),

		Title: lipgloss.NewStyle().
			Foreground(parseColor(theme.PromptColor)).
			Bold(true).
			Background(lipgloss.Color("0")).
			Padding(0, 2),

		HelpKey: lipgloss.NewStyle().
			Foreground(lipgloss.Color("12")).
			Bold(true),

		HelpDesc: lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")),

		Dimmed: lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")),
		Meta: lipgloss.NewStyle().Faint(true),
		ExitOk: lipgloss.NewStyle().
			Foreground(lipgloss.Color("10")).
			Bold(true),
		ExitFail: lipgloss.NewStyle().
			Foreground(lipgloss.Color("9")).
			Bold(true),
		ColumnHeader: lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")).
			Bold(true),
		ColumnHeaderBar: lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")),
		SelectionBar: selectionBarStyle,
		SelectedCmd: lipgloss.NewStyle().
			Foreground(lipgloss.Color("15")).
			Bold(true),
		Cmd: lipgloss.NewStyle(),
	}
}

func parseColor(s string) lipgloss.TerminalColor {
	if s == "" {
		return lipgloss.NoColor{}
	}

	names := map[string]string{"black": "0", "red": "1", "green": "2", "yellow": "3", "blue": "4", "magenta": "5", "cyan": "6", "white": "7", "gray": "8"}
	if color, ok := names[s]; ok {
		s = color
	}

	return lipgloss.Color(s)
}
