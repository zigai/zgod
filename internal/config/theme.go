package config

type ThemeConfig struct {
	Prompt      string `toml:"prompt"`
	PromptColor string `toml:"prompt_color"`
	MatchColor  string `toml:"match_color"`
	SelectedBg  string `toml:"selected_bg"`
	SelectedFg  string `toml:"selected_fg"`
	ModeColor   string `toml:"mode_color"`
	BorderColor string `toml:"border_color"`
}

func DefaultTheme() ThemeConfig {
	return ThemeConfig{
		Prompt:      "> ",
		PromptColor: "cyan",
		MatchColor:  "yellow",
		SelectedBg:  "236",
		SelectedFg:  "",
		ModeColor:   "240",
		BorderColor: "",
	}
}
