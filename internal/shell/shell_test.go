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

func TestSetupLine(t *testing.T) {
	tests := []struct {
		name       string
		shell      Shell
		configPath string
		want       string
	}{
		{
			name:       "bash without config",
			shell:      Bash,
			configPath: "",
			want:       `if command -v zgod >/dev/null 2>&1; then eval "$(zgod init bash)"; fi`,
		},
		{
			name:       "bash with config",
			shell:      Bash,
			configPath: "/custom/config.toml",
			want:       `if command -v zgod >/dev/null 2>&1; then eval "$(zgod init bash --config '/custom/config.toml')"; fi`,
		},
		{
			name:       "zsh without config",
			shell:      Zsh,
			configPath: "",
			want:       `if command -v zgod >/dev/null 2>&1; then eval "$(zgod init zsh)"; fi`,
		},
		{
			name:       "zsh with config",
			shell:      Zsh,
			configPath: "/custom/config.toml",
			want:       `if command -v zgod >/dev/null 2>&1; then eval "$(zgod init zsh --config '/custom/config.toml')"; fi`,
		},
		{
			name:       "fish without config",
			shell:      Fish,
			configPath: "",
			want:       `type -q zgod; and zgod init fish | source`,
		},
		{
			name:       "fish with config",
			shell:      Fish,
			configPath: "/custom/config.toml",
			want:       `type -q zgod; and zgod init fish --config '/custom/config.toml' | source`,
		},
		{
			name:       "powershell without config",
			shell:      PowerShell,
			configPath: "",
			want:       `if (Get-Command zgod -ErrorAction SilentlyContinue) { . (zgod init powershell) }`,
		},
		{
			name:       "powershell with config",
			shell:      PowerShell,
			configPath: "/custom/config.toml",
			want:       `if (Get-Command zgod -ErrorAction SilentlyContinue) { . (zgod init powershell --config '/custom/config.toml') }`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := setupLine(tt.shell, tt.configPath)
			if got != tt.want {
				t.Errorf("setupLine(%v, %q) = %q, want %q", tt.shell, tt.configPath, got, tt.want)
			}
		})
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

func TestInitScriptContainsRuntimeCommandGuards(t *testing.T) {
	tests := []struct {
		name        string
		shell       Shell
		mustContain []string
	}{
		{
			name:  "bash",
			shell: Bash,
			mustContain: []string{
				"__zgod_has_command()",
				"if ! __zgod_has_command; then",
				"zgod record",
				"selected=$(zgod search",
			},
		},
		{
			name:  "zsh",
			shell: Zsh,
			mustContain: []string{
				"__zgod_has_command()",
				"if ! __zgod_has_command; then",
				"zgod record",
				"selected=$(zgod search",
			},
		},
		{
			name:  "fish",
			shell: Fish,
			mustContain: []string{
				"function __zgod_has_command",
				"if not __zgod_has_command",
				"zgod record",
				"set -l selected (zgod search",
			},
		},
		{
			name:  "powershell",
			shell: PowerShell,
			mustContain: []string{
				"function __zgod_has_command",
				"if (-not (__zgod_has_command)) {",
				"zgod record",
				"$selected = zgod search",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			script, err := InitScript(tt.shell, InitOptions{})
			if err != nil {
				t.Fatalf("InitScript(%v) error: %v", tt.shell, err)
			}

			for _, needle := range tt.mustContain {
				if !strings.Contains(script, needle) {
					t.Errorf("InitScript(%v) output doesn't contain %q", tt.shell, needle)
				}
			}
		})
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
