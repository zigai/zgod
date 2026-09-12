package config

const (
	defaultCWDBoost     = 50
	defaultStartupLimit = 10000
)

type DisplayConfig struct {
	TimeFormat        string `json:"timeFormat"        toml:"time_format"`
	DurationFormat    string `json:"durationFormat"    toml:"duration_format"`
	ShowHints         bool   `json:"showHints"         toml:"show_hints"`
	ShowDirectory     bool   `json:"showDirectory"     toml:"show_directory"`
	InstantExecute    bool   `json:"instantExecute"    toml:"instant_execute"`
	EnableFuzzy       bool   `json:"enableFuzzy"       toml:"enable_fuzzy"`
	EnableRegex       bool   `json:"enableRegex"       toml:"enable_regex"`
	EnableGlob        bool   `json:"enableGlob"        toml:"enable_glob"`
	CWDBoost          int    `json:"cwdBoost"          toml:"cwd_boost"`
	DefaultScope      string `json:"defaultScope"      toml:"default_scope"`
	DefaultMode       string `json:"defaultMode"       toml:"default_mode"`
	DefaultFailFilter string `json:"defaultFailFilter" toml:"default_fail_filter"`
	StartupLimit      int    `json:"startupLimit"      toml:"startup_limit"`
	HideMultiline     bool   `json:"hideMultiline"     toml:"hide_multiline"`
	MultilinePreview  string `json:"multilinePreview"  toml:"multiline_preview"`
	MultilineCollapse string `json:"multilineCollapse" toml:"multiline_collapse"`
	NerdFont          bool   `json:"nerdFont"          toml:"nerd_font"`
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
		NerdFont:          true,
	}
}
