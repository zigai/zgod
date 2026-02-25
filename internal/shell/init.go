package shell

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"os"
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
}

func InitScript(s Shell, opts InitOptions) (string, error) {
	name := fmt.Sprintf("templates/%s.tmpl", s.String())

	data, err := templateFS.ReadFile(name)
	if err != nil {
		return "", fmt.Errorf("reading template for %s: %w", s, err)
	}

	tmpl, err := template.New(s.String()).Parse(string(data))
	if err != nil {
		return "", fmt.Errorf("parsing template for %s: %w", s, err)
	}

	var buf bytes.Buffer
	if err = tmpl.Execute(&buf, opts); err != nil {
		return "", fmt.Errorf("executing template for %s: %w", s, err)
	}

	return buf.String(), nil
}

func getPowerShellProfilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home directory for PowerShell profile: %w", err)
	}

	if runtime.GOOS == "windows" {
		return filepath.Join(home, "Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1"), nil
	}

	return filepath.Join(home, ".config", "powershell", "Microsoft.PowerShell_profile.ps1"), nil
}

func ConfigFilePath(s Shell) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home directory: %w", err)
	}

	switch s {
	case Bash:
		return filepath.Join(home, ".bashrc"), nil
	case Zsh:
		return filepath.Join(home, ".zshrc"), nil
	case Fish:
		return filepath.Join(home, ".config", "fish", "config.fish"), nil
	case PowerShell:
		return getPowerShellProfilePath()
	default:
		return "", fmt.Errorf("%w: %s", errUnsupportedShell, s)
	}
}

func setupLine(s Shell, customConfigPath string) string {
	shellName := s.String()
	switch s {
	case Bash, Zsh:
		if customConfigPath != "" {
			return fmt.Sprintf(`if command -v zgod >/dev/null 2>&1; then eval "$(zgod init %s --config '%s')"; fi`, shellName, customConfigPath)
		}

		return fmt.Sprintf(`if command -v zgod >/dev/null 2>&1; then eval "$(zgod init %s)"; fi`, shellName)
	case Fish:
		if customConfigPath != "" {
			return fmt.Sprintf(`type -q zgod; and zgod init %s --config '%s' | source`, shellName, customConfigPath)
		}

		return fmt.Sprintf(`type -q zgod; and zgod init %s | source`, shellName)
	case PowerShell:
		if customConfigPath != "" {
			return fmt.Sprintf(`if (Get-Command zgod -ErrorAction SilentlyContinue) { . (zgod init powershell --config '%s') }`, customConfigPath)
		}

		return `if (Get-Command zgod -ErrorAction SilentlyContinue) { . (zgod init powershell) }`
	}

	return ""
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

func Install(s Shell, customConfigPath string) error {
	configPath, err := ConfigFilePath(s)
	if err != nil {
		return err
	}

	if err = os.MkdirAll(filepath.Dir(configPath), 0o750); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	line := setupLine(s, customConfigPath)

	// #nosec G304 -- configPath is derived from known shell config locations
	content, err := os.ReadFile(configPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading config file: %w", err)
	}

	if strings.Contains(string(content), line) {
		return fmt.Errorf("%w in %s", errAlreadyInstalled, configPath)
	}

	if err = writeSetupLine(configPath, content, line); err != nil {
		return err
	}

	fmt.Printf("Added zgod to %s\n", configPath)

	if s == PowerShell {
		fmt.Println("Restart PowerShell or run: . $PROFILE")
	} else {
		fmt.Println("Restart your shell or run: source " + configPath)
	}

	return nil
}
