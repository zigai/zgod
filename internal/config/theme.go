package config

type ThemeConfig struct {
	Prompt            string `json:"prompt"            toml:"prompt"`
	PromptColor       string `json:"promptColor"       toml:"prompt_color"`
	MatchColor        string `json:"matchColor"        toml:"match_color"`
	SelectedBg        string `json:"selectedBg"        toml:"selected_bg"`
	SelectedFg        string `json:"selectedFg"        toml:"selected_fg"`
	ModeColor         string `json:"modeColor"         toml:"mode_color"`
	BorderColor       string `json:"borderColor"       toml:"border_color"`
	MatchBold         *bool  `json:"matchBold"         toml:"match_bold"`
	MatchUnderline    *bool  `json:"matchUnderline"    toml:"match_underline"`
	MatchBg           string `json:"matchBg"           toml:"match_bg"`
	SelectionBarShow  *bool  `json:"selectionBarShow"  toml:"selection_bar_show"`
	SelectionBarChar  string `json:"selectionBarChar"  toml:"selection_bar_char"`
	SelectionBarColor string `json:"selectionBarColor" toml:"selection_bar_color"`
	SelectionFullLine *bool  `json:"selectionFullLine" toml:"selection_full_line"`
}

func DefaultTheme() ThemeConfig {
	matchBold := true
	matchUnderline := true
	selectionBarShow := true
	selectionFullLine := true

	return ThemeConfig{
		Prompt:      "> ",
		PromptColor: "cyan",
		MatchColor:  "yellow",
		SelectedBg:  "4",
		SelectedFg:  "",
		ModeColor:   "8",
		BorderColor: "",

		MatchBold:      &matchBold,
		MatchUnderline: &matchUnderline,
		MatchBg:        "",

		SelectionBarShow:  &selectionBarShow,
		SelectionBarChar:  "▌ ",
		SelectionBarColor: "14",
		SelectionFullLine: &selectionFullLine,
	}
}

func BoolDefault(b *bool, def bool) bool {
	if b == nil {
		return def
	}

	return *b
}
