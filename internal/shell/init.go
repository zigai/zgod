package shell

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

var errAlreadyInstalled = errors.New("zgod is already installed")

type InitOptions struct {
	ConfigPath string
	BinPath    string
}

func InitScript(s Shell, opts InitOptions) (string, error) {
	name := fmt.Sprintf("templates/%s.tmpl", templateName(s))

	data, err := templateFS.ReadFile(name)
	if err != nil {
		return "", fmt.Errorf("reading template for %s: %w", s, err)
	}

	tmpl, err := template.New(s.String()).Funcs(template.FuncMap{
		"bashQuote":       bashQuote,
		"zshQuote":        bashQuote,
		"fishQuote":       fishQuote,
		"powerShellQuote": powerShellQuote,
	}).Parse(string(data))
	if err != nil {
		return "", fmt.Errorf("parsing template for %s: %w", s, err)
	}

	if opts.BinPath == "" {
		opts.BinPath = "zgod"
	}

	var buf bytes.Buffer
	if err = tmpl.Execute(&buf, opts); err != nil {
		return "", fmt.Errorf("executing template for %s: %w", s, err)
	}

	return buf.String(), nil
}

func templateName(s Shell) string {
	if s == Pwsh {
		return shellNamePowerShell
	}

	return s.String()
}

func getPowerShellProfilePath(s Shell) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home directory for PowerShell profile: %w", err)
	}

	return powerShellProfilePathForHome(home, s, runtime.GOOS), nil
}

func powerShellProfilePathForHome(home string, s Shell, goos string) string {
	if goos == "windows" {
		if s == PowerShell {
			return filepath.Join(home, "Documents", "WindowsPowerShell", "Microsoft.PowerShell_profile.ps1")
		}

		return filepath.Join(home, "Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1")
	}

	return filepath.Join(home, ".config", shellNamePowerShell, "Microsoft.PowerShell_profile.ps1")
}

func ConfigFilePath(s Shell) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home directory: %w", err)
	}

	return configFilePathForHome(home, s)
}

func configFilePathForHome(home string, s Shell) (string, error) {
	switch s {
	case Bash:
		return filepath.Join(home, ".bashrc"), nil
	case Zsh:
		return filepath.Join(home, ".zshrc"), nil
	case Fish:
		return filepath.Join(home, ".config", "fish", "conf.d", "zgod.fish"), nil
	case PowerShell, Pwsh:
		return getPowerShellProfilePath(s)
	default:
		return "", fmt.Errorf("%w: %s", errUnsupportedShell, s)
	}
}

// CurrentExecutablePath returns the executable path shell integrations should run.
func CurrentExecutablePath() string {
	const fallback = "zgod"

	if override := os.Getenv("ZGOD_BIN"); override != "" {
		return absolutePathOrOriginal(override)
	}

	arg0 := os.Args[0]
	if arg0 == "" {
		return fallback
	}

	if !strings.ContainsRune(arg0, os.PathSeparator) {
		path, err := exec.LookPath(arg0)
		if err != nil {
			return arg0
		}

		return absolutePathOrOriginal(path)
	}

	path := absolutePathOrOriginal(arg0)
	if isGoRunExecutable(path) {
		return fallback
	}

	return path
}

func absolutePathOrOriginal(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}

	return abs
}

func isGoRunExecutable(path string) bool {
	goBuildDir := string(os.PathSeparator) + "go-build"
	exeDir := string(os.PathSeparator) + "exe" + string(os.PathSeparator)

	return strings.Contains(path, goBuildDir) && strings.Contains(path, exeDir)
}

func setupLine(s Shell, customConfigPath string) string {
	shellName := s.String()
	switch s {
	case Bash, Zsh:
		if customConfigPath != "" {
			return fmt.Sprintf(`if command -v zgod >/dev/null 2>&1; then eval "$(zgod init %s --config %s)"; fi`, shellName, bashQuote(customConfigPath))
		}

		return fmt.Sprintf(`if command -v zgod >/dev/null 2>&1; then eval "$(zgod init %s)"; fi`, shellName)
	case Fish:
		if customConfigPath != "" {
			return fmt.Sprintf(`type -q zgod; and zgod init %s --config %s | source`, shellName, fishQuote(customConfigPath))
		}

		return fmt.Sprintf(`type -q zgod; and zgod init %s | source`, shellName)
	case PowerShell, Pwsh:
		if customConfigPath != "" {
			return fmt.Sprintf(`if (Get-Command zgod -ErrorAction SilentlyContinue) { Invoke-Expression (& zgod init %s --config %s) }`, shellName, powerShellQuote(customConfigPath))
		}

		return fmt.Sprintf(`if (Get-Command zgod -ErrorAction SilentlyContinue) { Invoke-Expression (& zgod init %s) }`, shellName)
	}

	return ""
}

func setupLineWithBin(s Shell, customConfigPath string, binPath string) string {
	shellName := s.String()
	switch s {
	case Bash, Zsh:
		bin := binPath

		if binPath != "zgod" {
			bin = bashQuote(binPath)
		}

		if customConfigPath != "" {
			return fmt.Sprintf(`if command -v %s >/dev/null 2>&1; then eval "$(%s init %s --config %s)"; fi`, bin, bin, shellName, bashQuote(customConfigPath))
		}

		return fmt.Sprintf(`if command -v %s >/dev/null 2>&1; then eval "$(%s init %s)"; fi`, bin, bin, shellName)
	case Fish:
		bin := binPath

		if binPath != "zgod" {
			bin = fishQuote(binPath)
		}

		if customConfigPath != "" {
			return fmt.Sprintf(`if test -x %s; or type -q %s; %s init %s --config %s | source; end`, bin, bin, bin, shellName, fishQuote(customConfigPath))
		}

		return fmt.Sprintf(`if test -x %s; or type -q %s; %s init %s | source; end`, bin, bin, bin, shellName)
	case PowerShell, Pwsh:
		bin := binPath

		if binPath != "zgod" {
			bin = powerShellQuote(binPath)
		}

		if customConfigPath != "" {
			return fmt.Sprintf(`if ((Test-Path -LiteralPath %s -PathType Leaf) -or (Get-Command %s -ErrorAction SilentlyContinue)) { Invoke-Expression (& %s init %s --config %s) }`, bin, bin, bin, shellName, powerShellQuote(customConfigPath))
		}

		return fmt.Sprintf(`if ((Test-Path -LiteralPath %s -PathType Leaf) -or (Get-Command %s -ErrorAction SilentlyContinue)) { Invoke-Expression (& %s init %s) }`, bin, bin, bin, shellName)
	}

	return ""
}

func bashQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func fishQuote(s string) string {
	escaped := strings.NewReplacer(
		`\`, `\\`,
		`"`, `\"`,
		`$`, `\$`,
	).Replace(s)

	return `"` + escaped + `"`
}

func powerShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func writeSetupLine(configPath string, content []byte, line string) error {
	// #nosec G304,G302 -- configPath is derived from known shell config locations, 0644 needed for shell configs
	f, err := os.OpenFile(configPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("opening config file: %w", err)
	}

	defer func() { _ = f.Close() }()

	if len(content) > 0 && !strings.HasSuffix(string(content), "\n") {
		if _, err = f.WriteString("\n"); err != nil {
			return fmt.Errorf("writing to config file: %w", err)
		}
	}

	if _, err = f.WriteString("# zgod shell integration\n" + line + "\n"); err != nil {
		return fmt.Errorf("writing to config file: %w", err)
	}

	return nil
}

func ensureNoLegacyFishInstall(s Shell, configPath, line string) error {
	if s != Fish {
		return nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("getting home directory: %w", err)
	}

	legacyPath := filepath.Join(home, ".config", "fish", "config.fish")
	if legacyPath == configPath {
		return nil
	}

	legacyContent, err := os.ReadFile(legacyPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return fmt.Errorf("reading config file: %w", err)
	}

	if strings.Contains(string(legacyContent), line) {
		return fmt.Errorf("%w in %s", errAlreadyInstalled, legacyPath)
	}

	return nil
}

func Install(s Shell, customConfigPath string) error {
	configPath, err := ConfigFilePath(s)
	if err != nil {
		return err
	}

	if err = os.MkdirAll(filepath.Dir(configPath), 0o750); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	line := setupLineWithBin(s, customConfigPath, CurrentExecutablePath())
	legacyLine := setupLine(s, customConfigPath)

	if err = ensureNoLegacyFishInstall(s, configPath, line); err != nil {
		return err
	}

	if legacyLine != line {
		if err = ensureNoLegacyFishInstall(s, configPath, legacyLine); err != nil {
			return err
		}
	}

	// #nosec G304 -- configPath is derived from known shell config locations
	content, err := os.ReadFile(configPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading config file: %w", err)
	}

	contentText := string(content)
	if strings.Contains(contentText, line) {
		return fmt.Errorf("%w in %s", errAlreadyInstalled, configPath)
	}

	updated, err := updateSetupLine(configPath, contentText, legacyLine, line)
	if err != nil {
		return err
	}

	if updated {
		fmt.Printf("Updated zgod in %s\n", configPath)
		printRestartHint(s, configPath)

		return nil
	}

	if err = writeSetupLine(configPath, content, line); err != nil {
		return err
	}

	fmt.Printf("Added zgod to %s\n", configPath)
	printRestartHint(s, configPath)

	return nil
}

func updateSetupLine(configPath string, contentText string, oldLine string, newLine string) (bool, error) {
	if oldLine == newLine || !strings.Contains(contentText, oldLine) {
		return false, nil
	}

	updated := strings.Replace(contentText, oldLine, newLine, 1)
	// #nosec G306,G703 -- configPath is derived from known shell config locations
	if err := os.WriteFile(configPath, []byte(updated), 0o644); err != nil {
		return false, fmt.Errorf("updating config file: %w", err)
	}

	return true, nil
}

func printRestartHint(s Shell, configPath string) {
	if s == PowerShell || s == Pwsh {
		fmt.Println("Restart PowerShell or run: . $PROFILE")
	} else {
		fmt.Println("Restart your shell or run: source " + configPath)
	}
}
