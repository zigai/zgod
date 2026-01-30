package config

type DisplayConfig struct {
	TimeFormat        string `toml:"time_format"`
	DurationFormat    string `toml:"duration_format"`
	ShowHints         bool   `toml:"show_hints"`
	ShowDirectory     bool   `toml:"show_directory"`
	InstantExecute    bool   `toml:"instant_execute"`
	EnableFuzzy       bool   `toml:"enable_fuzzy"`
	EnableRegex       bool   `toml:"enable_regex"`
	EnableGlob        bool   `toml:"enable_glob"`
	CWDBoost          int    `toml:"cwd_boost"`
	DefaultScope      string `toml:"default_scope"`
	DefaultMode       string `toml:"default_mode"`
	HideMultiline     bool   `toml:"hide_multiline"`
	MultilinePreview  string `toml:"multiline_preview"`
	MultilineCollapse string `toml:"multiline_collapse"`
}

func DefaultDisplay() DisplayConfig {
	return DisplayConfig{
		TimeFormat:        "relative",
		DurationFormat:    "auto",
		ShowHints:         true,
		EnableFuzzy:       true,
		EnableRegex:       true,
		EnableGlob:        true,
		CWDBoost:          50,
		DefaultScope:      "normal",
		DefaultMode:       "fuzzy",
		HideMultiline:     false,
		MultilinePreview:  "popup",
		MultilineCollapse: " ",
	}
}
