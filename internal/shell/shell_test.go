package shell

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestBashInitScriptRecordsFullCommandLine(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}

	tests := []struct {
		name    string
		command string
		want    string
	}{
		{
			name:    "pipeline",
			command: "echo one | cat",
			want:    "echo one | cat",
		},
		{
			name:    "compound if",
			command: "if true; then echo ok; fi",
			want:    "if true; then echo ok; fi",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := runBashInitScriptCommandCapture(t, bashCaptureOptions{
				command: tt.command,
			})
			if got != tt.want {
				t.Fatalf("recorded command = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBashInitScriptIgnoresExistingPromptCommand(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}

	got := runBashInitScriptCommandCaptures(t, bashCaptureOptions{
		prelude:       "set +o history",
		promptCommand: "echo oldpc >/dev/null",
		command:       "echo one\necho two",
	})
	want := []string{"echo one", "echo two"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("recorded commands = %q, want %q", got, want)
	}
}

func TestBashInitScriptDoesNotSkipUnderscoreCommands(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}

	got := runBashInitScriptCommandCapture(t, bashCaptureOptions{
		command: "_tool arg",
		executables: map[string]string{
			"_tool": "#!/usr/bin/env bash\nexit 0\n",
		},
	})
	if got != "_tool arg" {
		t.Fatalf("recorded command = %q, want %q", got, "_tool arg")
	}
}

type bashCaptureOptions struct {
	command       string
	prelude       string
	promptCommand string
	executables   map[string]string
}

func runBashInitScriptCommandCapture(t *testing.T, opts bashCaptureOptions) string {
	t.Helper()

	captures := runBashInitScriptCommandCaptures(t, opts)
	if len(captures) == 0 {
		t.Fatalf("no command was recorded")
	}

	return captures[len(captures)-1]
}

func runBashInitScriptCommandCaptures(t *testing.T, opts bashCaptureOptions) []string {
	t.Helper()

	initScript, err := InitScript(Bash, InitOptions{})
	if err != nil {
		t.Fatalf("InitScript(Bash) error: %v", err)
	}

	tempDir := t.TempDir()
	capturePath := filepath.Join(tempDir, "capture.log")
	fakeZgodPath := filepath.Join(tempDir, "zgod")
	rcPath := filepath.Join(tempDir, "bashrc")

	fakeZgod := `#!/usr/bin/env bash
set -eu

if [ "${1:-}" = "record" ]; then
	shift
	while [ "$#" -gt 0 ]; do
		if [ "$1" = "--command" ]; then
			printf '%s\n' "$2" >> "$ZGOD_CAPTURE_FILE"
			exit 0
		fi
		shift
	done
fi
`

	if err := os.WriteFile(fakeZgodPath, []byte(fakeZgod), 0o755); err != nil {
		t.Fatalf("WriteFile(%q) error: %v", fakeZgodPath, err)
	}

	for name, content := range opts.executables {
		path := filepath.Join(tempDir, name)
		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			t.Fatalf("WriteFile(%q) error: %v", path, err)
		}
	}

	rcContent := "PS1=''\n"
	if opts.prelude != "" {
		rcContent += opts.prelude + "\n"
	}
	if opts.promptCommand != "" {
		rcContent += fmt.Sprintf("PROMPT_COMMAND=%q\n", opts.promptCommand)
	}
	rcContent += initScript
	if err := os.WriteFile(rcPath, []byte(rcContent), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error: %v", rcPath, err)
	}

	cmd := exec.Command("bash", "--noprofile", "--rcfile", rcPath, "-i")
	cmd.Dir = tempDir
	cmd.Env = append(
		os.Environ(),
		"HOME="+tempDir,
		"PATH="+tempDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"TERM=dumb",
		"ZGOD_CAPTURE_FILE="+capturePath,
	)
	cmd.Stdin = strings.NewReader(opts.command + "\nexit\n")

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("running bash failed: %v\n%s", err, output)
	}

	recorded, err := waitForRecordedCommands(capturePath)
	if err != nil {
		t.Fatalf("waiting for recorded command failed: %v\n%s", err, output)
	}

	return recorded
}

func waitForRecordedCommands(path string) ([]string, error) {
	deadline := time.Now().Add(2 * time.Second)

	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil {
			lines := strings.Split(strings.TrimSpace(string(data)), "\n")
			recorded := make([]string, 0, len(lines))
			for i := 0; i < len(lines); i++ {
				if lines[i] != "" {
					recorded = append(recorded, lines[i])
				}
			}
			if len(recorded) > 0 {
				return recorded, nil
			}
		} else if !os.IsNotExist(err) {
			return nil, err
		}

		time.Sleep(10 * time.Millisecond)
	}

	return nil, fmt.Errorf("timed out waiting for %s", path)
}
