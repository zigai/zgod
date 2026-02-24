package shell

import (
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		input string
		want  Shell
		err   bool
	}{
		{"zsh", Zsh, false},
		{"bash", Bash, false},
		{"fish", Fish, false},
		{"powershell", PowerShell, false},
		{"pwsh", PowerShell, false},
		{"nushell", 0, true},
	}
	for _, tt := range tests {
		got, err := Parse(tt.input)
		if (err != nil) != tt.err {
			t.Errorf("Parse(%q) error = %v, wantErr %v", tt.input, err, tt.err)
			continue
		}

		if !tt.err && got != tt.want {
			t.Errorf("Parse(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestInitScript(t *testing.T) {
	for _, s := range []Shell{Zsh, Bash, Fish, PowerShell} {
		script, err := InitScript(s, InitOptions{})
		if err != nil {
			t.Errorf("InitScript(%v) error: %v", s, err)
			continue
		}

		if !strings.Contains(script, "zgod") {
			t.Errorf("InitScript(%v) output doesn't contain 'zgod'", s)
		}
	}
}

func TestInitScriptWithConfig(t *testing.T) {
	opts := InitOptions{ConfigPath: "/custom/config.toml"}
	for _, s := range []Shell{Zsh, Bash, Fish, PowerShell} {
		script, err := InitScript(s, opts)
		if err != nil {
			t.Errorf("InitScript(%v) error: %v", s, err)
			continue
		}

		if !strings.Contains(script, "ZGOD_CONFIG") {
			t.Errorf("InitScript(%v) output doesn't contain 'ZGOD_CONFIG'", s)
		}

		if !strings.Contains(script, "/custom/config.toml") {
			t.Errorf("InitScript(%v) output doesn't contain config path", s)
		}
	}
}
