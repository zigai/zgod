package config

type ThemeConfig struct {
	Prompt            string `toml:"prompt"`
	PromptColor       string `toml:"prompt_color"`
	MatchColor        string `toml:"match_color"`
	SelectedBg        string `toml:"selected_bg"`
	SelectedFg        string `toml:"selected_fg"`
	ModeColor         string `toml:"mode_color"`
	BorderColor       string `toml:"border_color"`
	MatchBold         *bool  `toml:"match_bold"`
	MatchUnderline    *bool  `toml:"match_underline"`
	MatchBg           string `toml:"match_bg"`
	SelectionBarShow  *bool  `toml:"selection_bar_show"`
	SelectionBarChar  string `toml:"selection_bar_char"`
	SelectionBarColor string `toml:"selection_bar_color"`
	SelectionFullLine *bool  `toml:"selection_full_line"`
}

func DefaultTheme() ThemeConfig {
	t := true
	return ThemeConfig{
		Prompt:      "> ",
		PromptColor: "cyan",
		MatchColor:  "yellow",
		SelectedBg:  "24",
		SelectedFg:  "",
		ModeColor:   "240",
		BorderColor: "",

		MatchBold:      &t,
		MatchUnderline: &t,
		MatchBg:        "",

		SelectionBarShow:  &t,
		SelectionBarChar:  "▌ ",
		SelectionBarColor: "14",
		SelectionFullLine: &t,
	}
}

func BoolDefault(b *bool, def bool) bool {
	if b == nil {
		return def
	}
	return *b
}
