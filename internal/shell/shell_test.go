package shell

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

var (
	errRecordedCommandTimeout = errors.New("timed out waiting for recorded commands")
	errPowerShellNotAvailable = errors.New("powershell not available")
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
		{shellNamePowerShell, PowerShell, false},
		{"pwsh", Pwsh, false},
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
			want:       `type -q zgod; and zgod init fish --config "/custom/config.toml" | source`,
		},
		{
			name:       "powershell without config",
			shell:      PowerShell,
			configPath: "",
			want:       `if (Get-Command zgod -ErrorAction SilentlyContinue) { Invoke-Expression (& zgod init powershell) }`,
		},
		{
			name:       "powershell with config",
			shell:      PowerShell,
			configPath: "/custom/config.toml",
			want:       `if (Get-Command zgod -ErrorAction SilentlyContinue) { Invoke-Expression (& zgod init powershell --config '/custom/config.toml') }`,
		},
		{
			name:       "pwsh without config",
			shell:      Pwsh,
			configPath: "",
			want:       `if (Get-Command zgod -ErrorAction SilentlyContinue) { Invoke-Expression (& zgod init pwsh) }`,
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

func TestSetupLineEscapesConfigPath(t *testing.T) {
	tests := []struct {
		name       string
		shell      Shell
		configPath string
		want       string
	}{
		{
			name:       "bash",
			shell:      Bash,
			configPath: `/tmp/o'hare$cfg`,
			want:       `if command -v zgod >/dev/null 2>&1; then eval "$(zgod init bash --config '/tmp/o'\''hare$cfg')"; fi`,
		},
		{
			name:       "zsh",
			shell:      Zsh,
			configPath: `/tmp/o'hare$cfg`,
			want:       `if command -v zgod >/dev/null 2>&1; then eval "$(zgod init zsh --config '/tmp/o'\''hare$cfg')"; fi`,
		},
		{
			name:       "fish",
			shell:      Fish,
			configPath: `/tmp/$HOME"cfg`,
			want:       `type -q zgod; and zgod init fish --config "/tmp/\$HOME\"cfg" | source`,
		},
		{
			name:       shellNamePowerShell,
			shell:      PowerShell,
			configPath: `C:\tmp\o'hare`,
			want:       `if (Get-Command zgod -ErrorAction SilentlyContinue) { Invoke-Expression (& zgod init powershell --config 'C:\tmp\o''hare') }`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := setupLine(tt.shell, tt.configPath)
			if got != tt.want {
				t.Fatalf("setupLine(%v, %q) = %q, want %q", tt.shell, tt.configPath, got, tt.want)
			}
		})
	}
}

func TestInitScript(t *testing.T) {
	for _, s := range []Shell{Zsh, Bash, Fish, PowerShell, Pwsh} {
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

func findPowerShell() (string, error) {
	if p, err := exec.LookPath("pwsh"); err == nil {
		return p, nil
	}

	if p, err := exec.LookPath("powershell"); err == nil {
		return p, nil
	}

	return "", errPowerShellNotAvailable
}

func runPowerShellScript(t *testing.T, scriptContent string) string {
	t.Helper()

	psBin, err := findPowerShell()
	if err != nil {
		t.Skip("powershell not available")
	}

	tempDir := t.TempDir()

	psScriptPath := filepath.Join(tempDir, "test.ps1")
	if err := os.WriteFile(psScriptPath, []byte(scriptContent), 0o644); err != nil {
		t.Fatalf("WriteFile(test.ps1) error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, psBin, "-NoProfile", "-NonInteractive", "-File", psScriptPath)
	cmd.Dir = tempDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("running powershell failed: %v\n%s", err, output)
	}

	return string(output)
}

func installFakeZgod(t *testing.T, targetDir string) {
	t.Helper()

	binName := "zgod"
	if runtime.GOOS == "windows" {
		binName = "zgod.exe"
	}

	binPath := filepath.Join(targetDir, binName)
	srcPath := filepath.Join(t.TempDir(), "fake_zgod.go")

	src := `package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "search" {
		fmt.Println("Write-Output instant_ps_ran")
		os.Exit(2)
	}

	if len(os.Args) > 1 && os.Args[1] == "record" {
		captureFile := os.Getenv("ZGOD_CAPTURE_FILE")
		if captureFile != "" {
			f, err := os.OpenFile(captureFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
			if err == nil {
				defer f.Close()
				fmt.Fprintln(f, strings.Join(os.Args[1:], " "))
			}
		}
	}
}
`
	if err := os.WriteFile(srcPath, []byte(src), 0o644); err != nil {
		t.Fatalf("WriteFile(fake_zgod.go) error: %v", err)
	}

	cmd := exec.CommandContext(t.Context(), "go", "build", "-o", binPath, srcPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building fake zgod binary: %v\n%s", err, out)
	}
}

func TestInitScriptCommandGuardsPreventLaunchWhenExecutableMissing(t *testing.T) {
	t.Run("bash", testInitScriptGuardsBash)
	t.Run("zsh", testInitScriptGuardsZsh)
	t.Run("fish", testInitScriptGuardsFish)
	t.Run(shellNamePowerShell, testInitScriptGuardsPowerShell)
}

func testInitScriptGuardsBash(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}

	tempDir := t.TempDir()
	capturePath := filepath.Join(tempDir, "capture.log")
	rcPath := filepath.Join(tempDir, "bashrc")

	initScript, err := InitScript(Bash, InitOptions{BinPath: "zgod"})
	if err != nil {
		t.Fatalf("InitScript(Bash) error: %v", err)
	}

	if err := os.WriteFile(rcPath, []byte("PS1=''\n"+initScript), 0o644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "--noprofile", "--rcfile", rcPath, "-i")
	cmd.Dir = tempDir
	cmd.Env = []string{
		"HOME=" + tempDir,
		"PATH=" + tempDir,
		"TERM=dumb",
		"ZGOD_CAPTURE_FILE=" + capturePath,
	}
	cmd.Stdin = strings.NewReader("echo hi\nexit 0\n")

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bash failed without zgod executable: %v\n%s", err, output)
	}

	if _, err := os.Stat(capturePath); !os.IsNotExist(err) {
		t.Fatal("capture file should not be created when zgod is missing")
	}
}

func testInitScriptGuardsZsh(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not available")
	}

	tempDir := t.TempDir()
	capturePath := filepath.Join(tempDir, "capture.log")
	rcPath := filepath.Join(tempDir, ".zshrc")

	initScript, err := InitScript(Zsh, InitOptions{BinPath: "zgod"})
	if err != nil {
		t.Fatalf("InitScript(Zsh) error: %v", err)
	}

	if err := os.WriteFile(rcPath, []byte(initScript), 0o644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "zsh", "-c", "source $ZDOTDIR/.zshrc; preexec 'echo hi'; precmd")
	cmd.Dir = tempDir
	cmd.Env = []string{
		"HOME=" + tempDir,
		"ZDOTDIR=" + tempDir,
		"PATH=" + tempDir,
		"TERM=dumb",
		"ZGOD_CAPTURE_FILE=" + capturePath,
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("zsh failed without zgod executable: %v\n%s", err, output)
	}

	if _, err := os.Stat(capturePath); !os.IsNotExist(err) {
		t.Fatal("capture file should not be created when zgod is missing")
	}
}

func testInitScriptGuardsFish(t *testing.T) {
	if _, err := exec.LookPath("fish"); err != nil {
		t.Skip("fish not available")
	}

	tempDir := t.TempDir()
	capturePath := filepath.Join(tempDir, "capture.log")
	initPath := filepath.Join(tempDir, "init.fish")

	initScript, err := InitScript(Fish, InitOptions{BinPath: "zgod"})
	if err != nil {
		t.Fatalf("InitScript(Fish) error: %v", err)
	}

	if err := os.WriteFile(initPath, []byte(initScript), 0o644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "fish", "-c", "source "+initPath+"; __zgod_preexec 'echo hi'; __zgod_postexec")
	cmd.Dir = tempDir
	cmd.Env = []string{
		"HOME=" + tempDir,
		"PATH=" + tempDir,
		"TERM=dumb",
		"ZGOD_CAPTURE_FILE=" + capturePath,
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("fish failed without zgod executable: %v\n%s", err, output)
	}

	if _, err := os.Stat(capturePath); !os.IsNotExist(err) {
		t.Fatal("capture file should not be created when zgod is missing")
	}
}

func testInitScriptGuardsPowerShell(t *testing.T) {
	psBin, err := findPowerShell()
	if err != nil {
		t.Skip("powershell not available")
	}

	tempDir := t.TempDir()

	initScript, err := InitScript(PowerShell, InitOptions{BinPath: "zgod"})
	if err != nil {
		t.Fatalf("InitScript(PowerShell) error: %v", err)
	}

	scriptContent := fmt.Sprintf(`
$env:PATH = %q
%s
__zgod_preexec "echo hi"
__zgod_postexec
`, tempDir, initScript)

	psScriptPath := filepath.Join(tempDir, "test.ps1")
	if err := os.WriteFile(psScriptPath, []byte(scriptContent), 0o644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, psBin, "-NoProfile", "-NonInteractive", "-File", psScriptPath)

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("powershell failed without zgod executable: %v\n%s", err, output)
	}
}

func TestZshInitScriptPreservesExistingHooksAndRecordsCommands(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not available")
	}

	tempDir := t.TempDir()
	capturePath := filepath.Join(tempDir, "capture.log")
	userLogPath := filepath.Join(tempDir, "user.log")
	rcPath := filepath.Join(tempDir, ".zshrc")
	fakeZgodPath := filepath.Join(tempDir, "zgod")

	fakeZgod := `#!/bin/sh
if [ "$1" = "record" ]; then
    printf '%s\n' "$*" >> "$ZGOD_CAPTURE_FILE"
    exit 0
fi
`
	if err := os.WriteFile(fakeZgodPath, []byte(fakeZgod), 0o755); err != nil {
		t.Fatalf("WriteFile(zgod) error: %v", err)
	}

	initScript, err := InitScript(Zsh, InitOptions{BinPath: "zgod"})
	if err != nil {
		t.Fatalf("InitScript(Zsh) error: %v", err)
	}

	if err := os.WriteFile(rcPath, []byte(initScript), 0o644); err != nil {
		t.Fatalf("WriteFile(.zshrc) error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmdScript := fmt.Sprintf(`
preexec() { echo "user_preexec:$1" >> %q; }
precmd() { echo "user_precmd" >> %q; }
source "$ZDOTDIR/.zshrc"
preexec "git commit -m test"
precmd
`, userLogPath, userLogPath)

	cmd := exec.CommandContext(ctx, "zsh", "-c", cmdScript)
	cmd.Dir = tempDir
	cmd.Env = append(
		os.Environ(),
		"HOME="+tempDir,
		"ZDOTDIR="+tempDir,
		"PATH="+tempDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"TERM=dumb",
		"ZGOD_CAPTURE_FILE="+capturePath,
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("zsh execution failed: %v\n%s", err, out)
	}

	userData, err := os.ReadFile(userLogPath)
	if err != nil {
		t.Fatalf("ReadFile(user.log) error: %v", err)
	}

	if !strings.Contains(string(userData), "user_preexec:git commit -m test") || !strings.Contains(string(userData), "user_precmd") {
		t.Fatalf("user hooks not called as expected: %q", string(userData))
	}

	captureData, err := os.ReadFile(capturePath)
	if err != nil {
		t.Fatalf("ReadFile(capture.log) error: %v", err)
	}

	capStr := string(captureData)
	if !strings.Contains(capStr, "--command git commit -m test") || !strings.Contains(capStr, "--exit-code 0") {
		t.Fatalf("recorded output = %q, want command and exit-code", capStr)
	}
}

func TestFishInitScriptRecordsCommands(t *testing.T) {
	if _, err := exec.LookPath("fish"); err != nil {
		t.Skip("fish not available")
	}

	tempDir := t.TempDir()
	capturePath := filepath.Join(tempDir, "capture.log")
	initPath := filepath.Join(tempDir, "init.fish")
	fakeZgodPath := filepath.Join(tempDir, "zgod")

	fakeZgod := `#!/bin/sh
if [ "$1" = "record" ]; then
    printf '%s\n' "$*" >> "$ZGOD_CAPTURE_FILE"
    exit 0
fi
`
	if err := os.WriteFile(fakeZgodPath, []byte(fakeZgod), 0o755); err != nil {
		t.Fatalf("WriteFile(zgod) error: %v", err)
	}

	initScript, err := InitScript(Fish, InitOptions{BinPath: "zgod"})
	if err != nil {
		t.Fatalf("InitScript(Fish) error: %v", err)
	}

	if err := os.WriteFile(initPath, []byte(initScript), 0o644); err != nil {
		t.Fatalf("WriteFile(init.fish) error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "fish", "-c", "source "+initPath+"; __zgod_preexec 'echo fish_test'; __zgod_postexec")
	cmd.Dir = tempDir
	cmd.Env = append(
		os.Environ(),
		"HOME="+tempDir,
		"PATH="+tempDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"TERM=dumb",
		"ZGOD_CAPTURE_FILE="+capturePath,
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("fish execution failed: %v\n%s", err, out)
	}

	captureData, err := os.ReadFile(capturePath)
	if err != nil {
		t.Fatalf("ReadFile(capture.log) error: %v", err)
	}

	capStr := string(captureData)
	if !strings.Contains(capStr, "--command echo fish_test") || !strings.Contains(capStr, "--duration") {
		t.Fatalf("recorded output = %q, want command and duration", capStr)
	}
}

func TestInitScriptWithConfig(t *testing.T) {
	opts := InitOptions{ConfigPath: "/custom/config.toml"}
	for _, s := range []Shell{Zsh, Bash, Fish, PowerShell, Pwsh} {
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

func TestInitScriptEscapesConfigPath(t *testing.T) {
	tests := []struct {
		name       string
		shell      Shell
		configPath string
		want       string
	}{
		{
			name:       "bash",
			shell:      Bash,
			configPath: `/tmp/o'hare$cfg`,
			want:       `export ZGOD_CONFIG='/tmp/o'\''hare$cfg'`,
		},
		{
			name:       "zsh",
			shell:      Zsh,
			configPath: `/tmp/o'hare$cfg`,
			want:       `export ZGOD_CONFIG='/tmp/o'\''hare$cfg'`,
		},
		{
			name:       "fish",
			shell:      Fish,
			configPath: `/tmp/$HOME"cfg`,
			want:       `set -gx ZGOD_CONFIG "/tmp/\$HOME\"cfg"`,
		},
		{
			name:       shellNamePowerShell,
			shell:      PowerShell,
			configPath: `C:\tmp\o'hare`,
			want:       `$env:ZGOD_CONFIG = 'C:\tmp\o''hare'`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			script, err := InitScript(tt.shell, InitOptions{ConfigPath: tt.configPath})
			if err != nil {
				t.Fatalf("InitScript(%v) error: %v", tt.shell, err)
			}

			if !strings.Contains(script, tt.want) {
				t.Fatalf("InitScript(%v) output doesn't contain %q", tt.shell, tt.want)
			}
		})
	}
}

func TestPowerShellInitScriptTracksPowerShellFailures(t *testing.T) {
	psBin, err := findPowerShell()
	if err != nil {
		t.Skip("powershell not available")
	}

	tempDir := t.TempDir()
	capturePath := filepath.Join(tempDir, "capture.log")
	installFakeZgod(t, tempDir)

	initScript, err := InitScript(PowerShell, InitOptions{BinPath: "zgod"})
	if err != nil {
		t.Fatalf("InitScript(PowerShell) error: %v", err)
	}

	scriptContent := fmt.Sprintf(`
$env:PATH = %q + [System.IO.Path]::PathSeparator + $env:PATH
$env:ZGOD_CAPTURE_FILE = %q
%s
__zgod_preexec "failing_cmd"
& %q -NoProfile -NonInteractive -Command "exit 42"
__zgod_postexec
`, tempDir, capturePath, initScript, psBin)

	output := runPowerShellScript(t, scriptContent)

	recorded, err := waitForRecordedCommands(capturePath)
	if err != nil {
		t.Fatalf("waiting for recorded commands: %v\n%s", err, output)
	}

	if len(recorded) == 0 || !strings.Contains(recorded[0], "--exit-code 42") {
		t.Fatalf("recorded output = %v, want --exit-code 42", recorded)
	}
}

func TestInstantExecutePathsRecordSelectedCommand(t *testing.T) {
	t.Run("bash", testInstantExecutePathsBash)
	t.Run(shellNamePowerShell, testInstantExecutePathsPowerShell)
}

func testInstantExecutePathsBash(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}

	tempDir := t.TempDir()
	capturePath := filepath.Join(tempDir, "capture.log")
	captureDirPath := filepath.Join(tempDir, "capture-dir.log")
	fakeZgodPath := filepath.Join(tempDir, "zgod")
	rcPath := filepath.Join(tempDir, "bashrc")

	fakeZgod := `#!/usr/bin/env bash
set -eu

if [ "${1:-}" = "search" ]; then
	printf 'echo instant_ran\n'
	exit 2
fi

if [ "${1:-}" = "record" ]; then
	shift
	record_command=""
	record_directory=""
	record_exit=0
	while [ "$#" -gt 0 ]; do
		case "$1" in
			--command)
				record_command=$2
				shift 2
				;;
			--directory)
				record_directory=$2
				shift 2
				;;
			--exit-code)
				record_exit=$2
				shift 2
				;;
			*)
				shift
				;;
		esac
	done
	printf '%s\n' "$record_command" >> "$ZGOD_CAPTURE_FILE"
	printf '%s\n' "$record_directory" >> "$ZGOD_CAPTURE_DIRECTORY_FILE"
	exit 0
fi
`
	if err := os.WriteFile(fakeZgodPath, []byte(fakeZgod), 0o755); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	initScript, err := InitScript(Bash, InitOptions{BinPath: "zgod"})
	if err != nil {
		t.Fatalf("InitScript error: %v", err)
	}

	rcContent := "PS1=''\n" + strings.ReplaceAll(initScript, "</dev/tty", "</dev/null")
	if err := os.WriteFile(rcPath, []byte(rcContent), 0o644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "--noprofile", "--rcfile", rcPath, "-i")
	cmd.Dir = tempDir
	cmd.Env = append(
		os.Environ(),
		"HOME="+tempDir,
		"PATH="+tempDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"TERM=dumb",
		"ZGOD_CAPTURE_FILE="+capturePath,
		"ZGOD_CAPTURE_DIRECTORY_FILE="+captureDirPath,
	)
	cmd.Stdin = strings.NewReader("__zgod_search\nexit 0\n")

	output, runErr := cmd.CombinedOutput()
	if runErr != nil {
		t.Fatalf("running bash failed: %v\n%s", runErr, output)
	}

	if !strings.Contains(string(output), "instant_ran") {
		t.Fatalf("output %q does not contain instant_ran", string(output))
	}

	recorded, err := waitForRecordedCommands(capturePath)
	if err != nil {
		t.Fatalf("waiting for recorded commands: %v\n%s", err, output)
	}

	if len(recorded) == 0 || recorded[0] != "echo instant_ran" {
		t.Fatalf("recorded = %v, want ['echo instant_ran']", recorded)
	}
}

func testInstantExecutePathsPowerShell(t *testing.T) {
	if _, err := findPowerShell(); err != nil {
		t.Skip("powershell not available")
	}

	tempDir := t.TempDir()
	capturePath := filepath.Join(tempDir, "capture.log")
	installFakeZgod(t, tempDir)

	initScript, err := InitScript(PowerShell, InitOptions{BinPath: "zgod"})
	if err != nil {
		t.Fatalf("InitScript error: %v", err)
	}

	scriptContent := fmt.Sprintf(`
Add-Type @"
namespace Microsoft.PowerShell {
    public class PSConsoleReadLine {
        public static void GetBufferState(out string input, out int cursor) {
            input = "";
            cursor = 0;
        }
        public static void RevertLine() {}
        public static void Insert(string s) {}
    }
}
"@
function Get-Module {
    param($Name)
    if ($Name -eq "PSReadLine") { return [PSCustomObject]@{ Name = "PSReadLine" } }
    return $null
}
function Set-PSReadLineKeyHandler { param($Chord, $ScriptBlock) }
function Set-PSReadLineOption { param($AddToHistoryHandler) }
$env:PATH = %q + [System.IO.Path]::PathSeparator + $env:PATH
$env:ZGOD_CAPTURE_FILE = %q
%s
__zgod_search
`, tempDir, capturePath, initScript)

	output := runPowerShellScript(t, scriptContent)
	if !strings.Contains(output, "instant_ps_ran") {
		t.Fatalf("output %q does not contain instant_ps_ran", output)
	}

	recorded, err := waitForRecordedCommands(capturePath)
	if err != nil {
		t.Fatalf("waiting for recorded commands: %v\n%s", err, output)
	}

	if len(recorded) == 0 || !strings.Contains(recorded[0], "Write-Output instant_ps_ran") {
		t.Fatalf("recorded output = %v, want Write-Output instant_ps_ran", recorded)
	}
}

func TestPowerShellInitScriptDoesNotCreateJobsPerCommand(t *testing.T) {
	script, err := InitScript(PowerShell, InitOptions{})
	if err != nil {
		t.Fatalf("InitScript(PowerShell) error: %v", err)
	}

	if strings.Contains(script, "Start-Job") {
		t.Fatal("InitScript(PowerShell) should not use Start-Job")
	}

	mustContain := []string{
		"function __zgod_record_async",
		"[System.Diagnostics.ProcessStartInfo]::new()",
		"[void][System.Diagnostics.Process]::Start($psi)",
	}

	for _, needle := range mustContain {
		if !strings.Contains(script, needle) {
			t.Fatalf("InitScript(PowerShell) output doesn't contain %q", needle)
		}
	}
}

func TestPowerShellInitScriptChecksPSReadLineBeforeHandlers(t *testing.T) {
	if _, err := findPowerShell(); err != nil {
		t.Skip("powershell not available")
	}

	initScript, err := InitScript(PowerShell, InitOptions{})
	if err != nil {
		t.Fatalf("InitScript(PowerShell) error: %v", err)
	}

	scriptContent := fmt.Sprintf(`
function Get-Module { param($Name) return $null }
function Set-PSReadLineKeyHandler { throw "Set-PSReadLineKeyHandler should not be called when PSReadLine is absent" }
function Set-PSReadLineOption { throw "Set-PSReadLineOption should not be called when PSReadLine is absent" }
%s
Write-Output "sourced successfully"
`, initScript)

	output := runPowerShellScript(t, scriptContent)
	if !strings.Contains(output, "sourced successfully") {
		t.Fatalf("output %q does not contain 'sourced successfully'", output)
	}
}

func TestPowerShellInitScriptPreservesPostCommandLookupAction(t *testing.T) {
	script, err := InitScript(PowerShell, InitOptions{})
	if err != nil {
		t.Fatalf("InitScript(PowerShell) error: %v", err)
	}

	if strings.Contains(script, "PostCommandLookupAction") {
		t.Fatal("InitScript(PowerShell) should not overwrite PostCommandLookupAction")
	}

	if _, err := findPowerShell(); err != nil {
		t.Skip("powershell not available")
	}

	scriptContent := fmt.Sprintf(`
$called = $false
$ExecutionContext.InvokeCommand.PostCommandLookupAction = { param($cmd, $args) $global:called = $true }
%s
$null = Get-Command Get-Date -ErrorAction SilentlyContinue
if ($called) {
    Write-Output "preserved"
}
`, script)

	output := runPowerShellScript(t, scriptContent)
	if !strings.Contains(output, "preserved") {
		t.Fatalf("output %q does not contain 'preserved'", output)
	}
}

func TestConfigFilePathFishUsesConfD(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	got, err := ConfigFilePath(Fish)
	if err != nil {
		t.Fatalf("ConfigFilePath(Fish) error: %v", err)
	}

	want := filepath.Join(home, ".config", "fish", "conf.d", "zgod.fish")
	if got != want {
		t.Fatalf("ConfigFilePath(Fish) = %q, want %q", got, want)
	}
}

func TestPowerShellProfilePathDistinguishesWindowsShells(t *testing.T) {
	home := filepath.Join("C:", "Users", "me")

	classic := powerShellProfilePathForHome(home, PowerShell, "windows")
	if !strings.Contains(classic, filepath.Join("Documents", "WindowsPowerShell", "Microsoft.PowerShell_profile.ps1")) {
		t.Fatalf("classic PowerShell profile path = %q, want WindowsPowerShell profile", classic)
	}

	modern := powerShellProfilePathForHome(home, Pwsh, "windows")
	if !strings.Contains(modern, filepath.Join("Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1")) {
		t.Fatalf("pwsh profile path = %q, want PowerShell profile", modern)
	}
}

func TestInstallFishWritesToConfD(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	binPath := filepath.Join(home, "bin", "zgod")
	t.Setenv("ZGOD_BIN", binPath)

	if err := Install(Fish, ""); err != nil {
		t.Fatalf("Install(Fish) error: %v", err)
	}

	configPath := filepath.Join(home, ".config", "fish", "conf.d", "zgod.fish")

	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error: %v", configPath, err)
	}

	want := "# zgod shell integration\n" + setupLineWithBin(Fish, "", binPath) + "\n"
	if string(content) != want {
		t.Fatalf("installed config = %q, want %q", string(content), want)
	}

	legacyPath := filepath.Join(home, ".config", "fish", "config.fish")
	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Fatalf("legacy fish config should not be created, stat err = %v", err)
	}
}

func TestInstallFishDetectsLegacyConfigFishInstall(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	legacyPath := filepath.Join(home, ".config", "fish", "config.fish")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o750); err != nil {
		t.Fatalf("MkdirAll(%q) error: %v", filepath.Dir(legacyPath), err)
	}

	legacyContent := "# zgod shell integration\n" + setupLine(Fish, "") + "\n"
	if err := os.WriteFile(legacyPath, []byte(legacyContent), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error: %v", legacyPath, err)
	}

	err := Install(Fish, "")
	if !errors.Is(err, errAlreadyInstalled) {
		t.Fatalf("Install(Fish) error = %v, want errAlreadyInstalled", err)
	}

	if !strings.Contains(err.Error(), legacyPath) {
		t.Fatalf("Install(Fish) error = %q, want path %q", err.Error(), legacyPath)
	}

	configPath := filepath.Join(home, ".config", "fish", "conf.d", "zgod.fish")
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatalf("new fish config should not be created, stat err = %v", err)
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

func TestBashInitScriptPreservesLeadingSpaceCommand(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}

	got := runBashInitScriptCommandCapture(t, bashCaptureOptions{
		command: " echo private",
	})
	if got != " echo private" {
		t.Fatalf("recorded command = %q, want %q", got, " echo private")
	}
}

func TestBashInitScriptDoesNotRecordStaleHistoryLine(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}

	got := runBashInitScriptCommandCaptures(t, bashCaptureOptions{
		prelude: "HISTCONTROL=ignorespace",
		command: "echo before\n echo skipped\necho after",
	})

	want := []string{"echo before", "echo after"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("recorded commands = %q, want %q", got, want)
	}
}

func TestBashInitScriptRecordsStartingDirectory(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}

	result := runBashInitScript(t, bashCaptureOptions{
		prelude: "mkdir subdir",
		command: "cd subdir",
	})

	if len(result.recordedDirs) == 0 {
		t.Fatal("no directories were recorded")
	}

	want := canonicalTestPath(bashWorkingDir(t, result.tempDir))
	if got := canonicalTestPath(result.recordedDirs[len(result.recordedDirs)-1]); got != want {
		t.Fatalf("recorded directory = %q, want %q", got, want)
	}
}

func TestBashInitScriptPreservesOriginalPromptCommand(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}

	result := runBashInitScript(t, bashCaptureOptions{
		promptCommand: `printf 'seen\n' >> prompt-seen.log`,
		command:       "echo hi",
	})

	logPath := filepath.Join(result.tempDir, "prompt-seen.log")

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error: %v\n%s", logPath, err, result.output)
	}

	if !strings.Contains(string(data), "seen\n") {
		t.Fatalf("prompt log = %q, want to contain %q", string(data), "seen\n")
	}
}

func TestBashInitScriptPreservesArrayPromptCommand(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}

	result := runBashInitScript(t, bashCaptureOptions{
		prelude: `PROMPT_COMMAND=(
  'printf "array-a\n" >> prompt-array.log'
  'printf "%s\n" "$?" >> prompt-array-exit.log'
)`,
		command: "false",
	})

	for _, tc := range []struct {
		name string
		want string
	}{
		{name: "prompt-array.log", want: "array-a\n"},
		{name: "prompt-array-exit.log", want: "1\n"},
	} {
		path := filepath.Join(result.tempDir, tc.name)

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%q) error: %v\n%s", path, err, result.output)
		}

		if !strings.Contains(string(data), tc.want) {
			t.Fatalf("%s = %q, want to contain %q", tc.name, string(data), tc.want)
		}
	}
}

func TestBashInitScriptPreservesOriginalDebugTrap(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}

	result := runBashInitScript(t, bashCaptureOptions{
		prelude: `trap 'if [[ "$BASH_COMMAND" == *"echo keep-me"* ]]; then printf "seen\n" >> debug.log; fi' DEBUG`,
		command: "echo keep-me",
	})

	logPath := filepath.Join(result.tempDir, "debug.log")

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error: %v\n%s", logPath, err, result.output)
	}

	if !strings.Contains(string(data), "seen\n") {
		t.Fatalf("debug log = %q, want to contain %q", string(data), "seen\n")
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

type bashRunResult struct {
	recorded     []string
	recordedDirs []string
	tempDir      string
	output       string
}

func runBashInitScriptCommandCapture(t *testing.T, opts bashCaptureOptions) string {
	t.Helper()

	result := runBashInitScript(t, opts)
	if len(result.recorded) == 0 {
		t.Fatalf("no command was recorded")
	}

	return result.recorded[len(result.recorded)-1]
}

func runBashInitScriptCommandCaptures(t *testing.T, opts bashCaptureOptions) []string {
	t.Helper()

	return runBashInitScript(t, opts).recorded
}

func runBashInitScript(t *testing.T, opts bashCaptureOptions) bashRunResult {
	t.Helper()

	initScript, err := InitScript(Bash, InitOptions{})
	if err != nil {
		t.Fatalf("InitScript(Bash) error: %v", err)
	}

	tempDir := t.TempDir()
	capturePath := filepath.Join(tempDir, "capture.log")
	captureDirPath := filepath.Join(tempDir, "capture-dir.log")
	fakeZgodPath := filepath.Join(tempDir, "zgod")
	rcPath := filepath.Join(tempDir, "bashrc")

	fakeZgod := `#!/usr/bin/env bash
set -eu

if [ "${1:-}" = "record" ]; then
	shift
	record_command=""
	record_directory=""
	while [ "$#" -gt 0 ]; do
		case "$1" in
			--command)
				record_command=$2
				shift 2
				;;
			--directory)
				record_directory=$2
				shift 2
				;;
			*)
				shift
				;;
		esac
	done
	printf '%s\n' "$record_command" >> "$ZGOD_CAPTURE_FILE"
	if [ -n "${ZGOD_CAPTURE_DIRECTORY_FILE:-}" ]; then
		printf '%s\n' "$record_directory" >> "$ZGOD_CAPTURE_DIRECTORY_FILE"
	fi
	exit 0
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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "--noprofile", "--rcfile", rcPath, "-i")
	cmd.Dir = tempDir
	cmd.Env = append(
		os.Environ(),
		"HOME="+tempDir,
		"PATH="+tempDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"TERM=dumb",
		"ZGOD_CAPTURE_FILE="+capturePath,
		"ZGOD_CAPTURE_DIRECTORY_FILE="+captureDirPath,
	)
	cmd.Stdin = strings.NewReader(opts.command + "\nexit 0\n")

	output, runErr := cmd.CombinedOutput()
	if runErr != nil {
		t.Fatalf("running bash failed: %v\n%s", runErr, output)
	}

	recorded, err := waitForRecordedCommands(capturePath)
	if err != nil {
		t.Fatalf("waiting for recorded command failed: %v\n%s", err, output)
	}

	recordedDirs, err := waitForRecordedCommands(captureDirPath)
	if err != nil {
		t.Fatalf("waiting for recorded directories failed: %v\n%s", err, output)
	}

	return bashRunResult{
		recorded:     recorded,
		recordedDirs: recordedDirs,
		tempDir:      tempDir,
		output:       string(output),
	}
}

func bashWorkingDir(t *testing.T, dir string) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "--noprofile", "--norc", "-c", "pwd")
	cmd.Dir = dir

	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("running bash pwd failed: %v", err)
	}

	return strings.TrimSpace(string(output))
}

func canonicalTestPath(path string) string {
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return path
	}

	return canonical
}

func setTestHome(t *testing.T, home string) {
	t.Helper()

	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOMEDRIVE", "")
	t.Setenv("HOMEPATH", "")
}

func waitForRecordedCommands(path string) ([]string, error) {
	deadline := time.Now().Add(2 * time.Second)

	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil {
			lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")

			recorded := make([]string, 0, len(lines))
			for i := range lines {
				if lines[i] != "" {
					recorded = append(recorded, lines[i])
				}
			}

			if len(recorded) > 0 {
				return recorded, nil
			}
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("reading recorded commands from %q: %w", path, err)
		}

		time.Sleep(10 * time.Millisecond)
	}

	return nil, fmt.Errorf("%w: %s", errRecordedCommandTimeout, path)
}
