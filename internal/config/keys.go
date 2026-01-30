package config

type KeyConfig struct {
	ModeNext       string `toml:"mode_next"`
	ModeFuzzy      string `toml:"mode_fuzzy"`
	ModeRegex      string `toml:"mode_regex"`
	ModeGlob       string `toml:"mode_glob"`
	ToggleCWD      string `toml:"toggle_cwd"`
	ToggleDedupe   string `toml:"toggle_dedupe"`
	ToggleFails    string `toml:"toggle_fails"`
	Accept         string `toml:"accept"`
	Cancel         string `toml:"cancel"`
	Up             string `toml:"up"`
	Down           string `toml:"down"`
	PageUp         string `toml:"page_up"`
	PageDown       string `toml:"page_down"`
	Top            string `toml:"top"`
	Bottom         string `toml:"bottom"`
	Help           string `toml:"help"`
	PreviewCommand string `toml:"preview_command"`
}

func DefaultKeys() KeyConfig {
	return KeyConfig{
		ModeNext:       "ctrl+s",
		ModeFuzzy:      "alt+f",
		ModeRegex:      "alt+r",
		ModeGlob:       "alt+g",
		ToggleCWD:      "ctrl+g",
		ToggleDedupe:   "ctrl+d",
		ToggleFails:    "ctrl+f",
		Accept:         "enter",
		Cancel:         "esc",
		Up:             "up",
		Down:           "down",
		PageUp:         "pgup",
		PageDown:       "pgdown",
		Top:            "home",
		Bottom:         "end",
		Help:           "?",
		PreviewCommand: "alt+p",
	}
}
