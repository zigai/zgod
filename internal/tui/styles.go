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
	borderColor := parseColor(theme.BorderColor)
	if theme.BorderColor == "" {
		borderColor = parseColor(theme.ModeColor)
		if theme.ModeColor == "" {
			borderColor = lipgloss.Color("240")
		}
	}

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

		HeaderBar: lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")).
			Padding(0, 1),

		Input: lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")),

		Footer: lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")).
			Padding(0, 1),

		Border: lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(borderColor),

		Title: lipgloss.NewStyle().
			Foreground(parseColor(theme.PromptColor)).
			Bold(true).
			Background(lipgloss.Color("236")).
			Padding(0, 2),

		HelpKey: lipgloss.NewStyle().
			Foreground(lipgloss.Color("12")).
			Bold(true),

		HelpDesc: lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")),

		Dimmed: lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")),
		Meta: lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")),
		ExitOk: lipgloss.NewStyle().
			Foreground(lipgloss.Color("10")).
			Bold(true),
		ExitFail: lipgloss.NewStyle().
			Foreground(lipgloss.Color("9")).
			Bold(true),
		ColumnHeader: lipgloss.NewStyle().
			Foreground(lipgloss.Color("244")).
			Bold(true),
		ColumnHeaderBar: lipgloss.NewStyle().
			Foreground(lipgloss.Color("244")),
		SelectionBar: selectionBarStyle,
		SelectedCmd: lipgloss.NewStyle().
			Foreground(lipgloss.Color("15")).
			Bold(true),
		Cmd: lipgloss.NewStyle().
			Foreground(lipgloss.Color("250")),
	}
}

func parseColor(s string) lipgloss.TerminalColor {
	if s == "" {
		return lipgloss.NoColor{}
	}

	return lipgloss.Color(s)
}
