package tui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/zigai/zgod/internal/config"
)

type Styles struct {
	Prompt          lipgloss.Style
	Match           lipgloss.Style
	Selected        lipgloss.Style
	Normal          lipgloss.Style
	Mode            lipgloss.Style
	Cursor          lipgloss.Style
	HeaderBar       lipgloss.Style
	Header          lipgloss.Style
	Input           lipgloss.Style
	Footer          lipgloss.Style
	Border          lipgloss.Style
	Title           lipgloss.Style
	HelpKey         lipgloss.Style
	HelpDesc        lipgloss.Style
	SelectedItem    lipgloss.Style
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
	base := lipgloss.NewStyle()
	borderColor := parseColor(theme.BorderColor)
	if theme.BorderColor == "" {
		borderColor = parseColor(theme.ModeColor)
		if theme.ModeColor == "" {
			borderColor = lipgloss.Color("240")
		}
	}

	return Styles{
		Prompt: lipgloss.NewStyle().
			Foreground(parseColor(theme.PromptColor)).
			Bold(true).
			PaddingRight(1),

		Match: lipgloss.NewStyle().
			Foreground(parseColor(theme.MatchColor)).
			Bold(true).
			Underline(true),

		Selected: lipgloss.NewStyle().
			Background(parseColor(theme.SelectedBg)).
			Foreground(parseColor(theme.SelectedFg)),

		Normal: base,

		Mode: lipgloss.NewStyle().
			Foreground(parseColor(theme.ModeColor)).
			Bold(true),

		Cursor: lipgloss.NewStyle().
			Foreground(parseColor(theme.PromptColor)).
			Bold(true),

		HeaderBar: lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")).
			Padding(0, 1),

		Header: lipgloss.NewStyle().
			Bold(true),

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

		SelectedItem: lipgloss.NewStyle().
			Background(lipgloss.Color("33")).
			Bold(true),

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
			Foreground(lipgloss.Color("244")).
			Padding(0, 1),
		SelectionBar: lipgloss.NewStyle().
			Foreground(lipgloss.Color("14")).
			Bold(true),
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
