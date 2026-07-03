package config

type KeyConfig struct {
	ModeNext       string `toml:"mode_next"`
	ModeFuzzy      string `toml:"mode_fuzzy"`
	ModeRegex      string `toml:"mode_regex"`
	ModeGlob       string `toml:"mode_glob"`
	ToggleCWD      string `toml:"toggle_cwd"`
	ToggleDedupe   string `toml:"toggle_dedupe"`
	ToggleFails    string `toml:"toggle_fails"`
	SortHistory    string `toml:"sort_history"`
	Accept         string `toml:"accept"`
	Cancel         string `toml:"cancel"`
	Up             string `toml:"up"`
	Down           string `toml:"down"`
	PageUp         string `toml:"page_up"`
	PageDown       string `toml:"page_down"`
	Top            string `toml:"top"`
	Bottom         string `toml:"bottom"`
	Select1        string `toml:"select_1"`
	Select2        string `toml:"select_2"`
	Select3        string `toml:"select_3"`
	Select4        string `toml:"select_4"`
	Select5        string `toml:"select_5"`
	Select6        string `toml:"select_6"`
	Select7        string `toml:"select_7"`
	Select8        string `toml:"select_8"`
	Select9        string `toml:"select_9"`
	Select0        string `toml:"select_0"`
	Help           string `toml:"help"`
	PreviewCommand string `toml:"preview_command"`
}

func DefaultKeys() KeyConfig {
	return KeyConfig{
		ModeNext:       "ctrl+s",
		ModeFuzzy:      "alt+f",
		ModeRegex:      "alt+r",
		ModeGlob:       "alt+g",
		ToggleCWD:      "ctrl+d",
		ToggleDedupe:   "ctrl+g",
		ToggleFails:    "ctrl+f",
		SortHistory:    "alt+t",
		Accept:         "enter",
		Cancel:         "esc",
		Up:             "up",
		Down:           "down",
		PageUp:         "pgup",
		PageDown:       "pgdown",
		Top:            "home",
		Bottom:         "end",
		Select1:        "ctrl+1",
		Select2:        "ctrl+2",
		Select3:        "ctrl+3",
		Select4:        "ctrl+4",
		Select5:        "ctrl+5",
		Select6:        "ctrl+6",
		Select7:        "ctrl+7",
		Select8:        "ctrl+8",
		Select9:        "ctrl+9",
		Select0:        "ctrl+0",
		Help:           "?",
		PreviewCommand: "alt+p",
	}
}
