package shell

import (
	"bytes"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

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
		return "", err
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
		return "", fmt.Errorf("unknown shell: %s", s)
	}
}

func Install(s Shell, customConfigPath string) error {
	configPath, err := ConfigFilePath(s)
	if err != nil {
		return err
	}

	if err = os.MkdirAll(filepath.Dir(configPath), 0750); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	var setupLine string
	switch s {
	case Bash, Zsh:
		if customConfigPath != "" {
			setupLine = fmt.Sprintf(`eval "$(zgod init %s --config '%s')"`, s.String(), customConfigPath)
		} else {
			setupLine = fmt.Sprintf(`eval "$(zgod init %s)"`, s.String())
		}
	case Fish:
		if customConfigPath != "" {
			setupLine = fmt.Sprintf(`zgod init %s --config '%s' | source`, s.String(), customConfigPath)
		} else {
			setupLine = fmt.Sprintf(`zgod init %s | source`, s.String())
		}
	case PowerShell:
		if customConfigPath != "" {
			setupLine = fmt.Sprintf(`. (zgod init powershell --config '%s')`, customConfigPath)
		} else {
			setupLine = `. (zgod init powershell)`
		}
	}

	// #nosec G304 -- configPath is derived from known shell config locations
	content, err := os.ReadFile(configPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading config file: %w", err)
	}

	if strings.Contains(string(content), setupLine) {
		return fmt.Errorf("zgod is already installed in %s", configPath)
	}

	// #nosec G304,G302 -- configPath is derived from known shell config locations, 0644 needed for shell configs
	f, err := os.OpenFile(configPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("opening config file: %w", err)
	}
	defer func() { _ = f.Close() }()

	if len(content) > 0 && !strings.HasSuffix(string(content), "\n") {
		if _, err = f.WriteString("\n"); err != nil {
			return fmt.Errorf("writing to config file: %w", err)
		}
	}

	if _, err = f.WriteString("# zgod shell integration\n" + setupLine + "\n"); err != nil {
		return fmt.Errorf("writing to config file: %w", err)
	}

	fmt.Printf("Added zgod to %s\n", configPath)
	if s == PowerShell {
		fmt.Println("Restart PowerShell or run: . $PROFILE")
	} else {
		fmt.Println("Restart your shell or run: source " + configPath)
	}
	return nil
}
