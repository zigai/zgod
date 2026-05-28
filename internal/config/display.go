package config

const (
	defaultCWDBoost     = 50
	defaultStartupLimit = 10000
)

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
	DefaultFailFilter string `toml:"default_fail_filter"`
	StartupLimit      int    `toml:"startup_limit"`
	HideMultiline     bool   `toml:"hide_multiline"`
	MultilinePreview  string `toml:"multiline_preview"`
	MultilineCollapse string `toml:"multiline_collapse"`
}

func DefaultDisplay() DisplayConfig {
	return DisplayConfig{
		TimeFormat:        "relative",
		DurationFormat:    "auto",
		ShowHints:         true,
		ShowDirectory:     false,
		InstantExecute:    false,
		EnableFuzzy:       true,
		EnableRegex:       true,
		EnableGlob:        true,
		CWDBoost:          defaultCWDBoost,
		DefaultScope:      "normal",
		DefaultMode:       "fuzzy",
		DefaultFailFilter: "include",
		StartupLimit:      defaultStartupLimit,
		HideMultiline:     false,
		MultilinePreview:  "popup",
		MultilineCollapse: " ",
	}
}
