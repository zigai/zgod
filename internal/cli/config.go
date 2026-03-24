package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zigai/zgod/internal/config"
	"github.com/zigai/zgod/internal/paths"
)

var errNoEditorConfigured = errors.New("no editor found: set $EDITOR or $VISUAL")
var errInvalidEditorCommand = errors.New("invalid editor command")

var runEditorProcess = func(name string, args []string) error {
	// #nosec G204 -- $EDITOR/$VISUAL is user-controlled by design for a local CLI
	c := exec.Command(name, args...)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr

	return c.Run()
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage configuration",
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Print the current configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		configPath, err := ensureConfigFile()
		if err != nil {
			return err
		}

		data, err := os.ReadFile(configPath)
		if err != nil {
			return fmt.Errorf("reading config file: %w", err)
		}

		_, err = cmd.OutOrStdout().Write(data)
		if err != nil {
			return fmt.Errorf("writing config to stdout: %w", err)
		}

		return nil
	},
}

var configEditCmd = &cobra.Command{
	Use:   "edit",
	Short: "Open the configuration file in an editor",
	RunE: func(cmd *cobra.Command, args []string) error {
		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = os.Getenv("VISUAL")
		}

		if editor == "" {
			return errNoEditorConfigured
		}

		path, err := ensureConfigFile()
		if err != nil {
			return err
		}

		return openEditor(editor, path)
	},
}

func ensureConfigFile() (string, error) {
	configPath, err := paths.ConfigFile()
	if err != nil {
		return "", fmt.Errorf("resolving config file path: %w", err)
	}

	_, err = os.Stat(configPath)
	if err == nil {
		return configPath, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("checking config file: %w", err)
	}

	if err = os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		return "", fmt.Errorf("creating config directory: %w", err)
	}

	if err = config.Default().Save(); err != nil {
		return "", fmt.Errorf("creating default config: %w", err)
	}

	return configPath, nil
}

func openEditor(editor, path string) error {
	args, err := splitCommandLine(editor)
	if err != nil {
		return err
	}

	return runEditorProcess(args[0], append(args[1:], path))
}

func splitCommandLine(command string) ([]string, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return nil, fmt.Errorf("%w: empty command", errInvalidEditorCommand)
	}

	var (
		args    []string
		current strings.Builder
		quote   rune
		inToken bool
		escaped bool
	)

	flush := func() {
		args = append(args, current.String())
		current.Reset()
		inToken = false
	}

	for _, r := range command {
		if quote == '"' && escaped {
			current.WriteRune(r)
			inToken = true
			escaped = false
			continue
		}

		switch quote {
		case 0:
			switch {
			case r == '\'' || r == '"':
				quote = r
				inToken = true
			case r == ' ' || r == '\t' || r == '\n':
				if inToken {
					flush()
				}
			default:
				current.WriteRune(r)
				inToken = true
			}
		case '\'':
			if r == '\'' {
				quote = 0
				continue
			}

			current.WriteRune(r)
			inToken = true
		case '"':
			switch r {
			case '\\':
				escaped = true
			case '"':
				quote = 0
			default:
				current.WriteRune(r)
				inToken = true
			}
		}
	}

	if escaped || quote != 0 {
		return nil, fmt.Errorf("%w: unterminated quoted string", errInvalidEditorCommand)
	}
	if inToken {
		flush()
	}
	if len(args) == 0 {
		return nil, fmt.Errorf("%w: empty command", errInvalidEditorCommand)
	}

	return args, nil
}

func registerConfigCommand() {
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configEditCmd)
	rootCmd.AddCommand(configCmd)
}
