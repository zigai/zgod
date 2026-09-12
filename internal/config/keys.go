package config

type KeyConfig struct {
	ModeNext       string `json:"modeNext"       toml:"mode_next"`
	ModeFuzzy      string `json:"modeFuzzy"      toml:"mode_fuzzy"`
	ModeRegex      string `json:"modeRegex"      toml:"mode_regex"`
	ModeGlob       string `json:"modeGlob"       toml:"mode_glob"`
	ToggleCWD      string `json:"toggleCwd"      toml:"toggle_cwd"`
	ToggleDedupe   string `json:"toggleDedupe"   toml:"toggle_dedupe"`
	ToggleFails    string `json:"toggleFails"    toml:"toggle_fails"`
	SortHistory    string `json:"sortHistory"    toml:"sort_history"`
	Accept         string `json:"accept"         toml:"accept"`
	Cancel         string `json:"cancel"         toml:"cancel"`
	Up             string `json:"up"             toml:"up"`
	Down           string `json:"down"           toml:"down"`
	PageUp         string `json:"pageUp"         toml:"page_up"`
	PageDown       string `json:"pageDown"       toml:"page_down"`
	Top            string `json:"top"            toml:"top"`
	Bottom         string `json:"bottom"         toml:"bottom"`
	Select1        string `json:"select1"        toml:"select_1"`
	Select2        string `json:"select2"        toml:"select_2"`
	Select3        string `json:"select3"        toml:"select_3"`
	Select4        string `json:"select4"        toml:"select_4"`
	Select5        string `json:"select5"        toml:"select_5"`
	Select6        string `json:"select6"        toml:"select_6"`
	Select7        string `json:"select7"        toml:"select_7"`
	Select8        string `json:"select8"        toml:"select_8"`
	Select9        string `json:"select9"        toml:"select_9"`
	Select0        string `json:"select0"        toml:"select_0"`
	Help           string `json:"help"           toml:"help"`
	PreviewCommand string `json:"previewCommand" toml:"preview_command"`
}

func DefaultKeys() KeyConfig {
	return KeyConfig{
		ModeNext:       "ctrl+s",
		ModeFuzzy:      "alt+f",
		ModeRegex:      "alt+r",
		ModeGlob:       "alt+g",
		ToggleCWD:      "alt+d",
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
