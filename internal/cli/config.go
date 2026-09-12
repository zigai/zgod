package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/spf13/cobra"
)

var (
	errNoEditorConfigured   = errors.New("no editor found: set $EDITOR or $VISUAL")
	errInvalidEditorCommand = errors.New("invalid editor command")
)

var runEditorProcess = func(name string, args []string) error {
	// #nosec G204 -- $EDITOR/$VISUAL is user-controlled by design for a local CLI
	c := exec.Command(name, args...)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr

	return c.Run()
}

func runConfigShow(cmd *cobra.Command, _ []string) error {
	raw, err := configShowRaw(cmd)
	if err != nil {
		return err
	}

	if raw {
		path, err := commandConfigPath(cmd)
		if err != nil {
			return err
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading config: %w", err)
		}

		if _, err = cmd.OutOrStdout().Write(data); err != nil {
			return fmt.Errorf("writing raw config: %w", err)
		}

		return nil
	}

	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}

	if flagBool(cmd, "json") {
		return writeJSON(cmd.OutOrStdout(), cfg)
	}

	if err = toml.NewEncoder(cmd.OutOrStdout()).Encode(cfg); err != nil {
		return fmt.Errorf("writing effective config: %w", err)
	}

	return nil
}

func runConfigEdit(cmd *cobra.Command, _ []string) error {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}

	if editor == "" {
		return errNoEditorConfigured
	}

	path, err := commandConfigPath(cmd)
	if err != nil {
		return err
	}

	if _, err = os.Stat(path); errors.Is(err, os.ErrNotExist) {
		if _, err = loadConfig(cmd); err != nil {
			return err
		}
	}

	return openEditor(editor, path)
}

func configShowRaw(cmd *cobra.Command) (bool, error) {
	flag := cmd.Flags().Lookup("raw")
	if flag == nil {
		return false, nil
	}

	raw, err := cmd.Flags().GetBool("raw")
	if err != nil {
		return false, fmt.Errorf("reading --raw flag: %w", err)
	}

	return raw, nil
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

	args, err := splitCommandTokens(command)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidEditorCommand, err)
	}

	if len(args) == 0 {
		return nil, fmt.Errorf("%w: empty command", errInvalidEditorCommand)
	}

	if args[0] == "" {
		return nil, fmt.Errorf("%w: empty executable", errInvalidEditorCommand)
	}

	return args, nil
}

func registerConfigCommand(root *cobra.Command) {
	cmd := &cobra.Command{Use: "config", Short: "Manage configuration", GroupID: "management", RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() }}
	show := &cobra.Command{Use: "show", Short: "Print effective configuration", RunE: runConfigShow}
	show.Flags().Bool("raw", false, "Print the chosen config file without validation")
	show.Flags().Bool("json", false, "Print effective configuration as JSON")

	edit := &cobra.Command{Use: "edit", Short: "Open the user configuration in an editor", RunE: runConfigEdit}
	cmd.AddCommand(show, edit)
	root.AddCommand(cmd)
}
