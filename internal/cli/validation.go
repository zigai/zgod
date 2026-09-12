package cli

import (
	"fmt"
	"math"
	"os"
	"regexp"
	"runtime"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/zigai/zgod/internal/config"
	"github.com/zigai/zgod/internal/match"
	"github.com/zigai/zgod/internal/paths"
)

const (
	maxSearchHeight = 1000
	maxSearchLimit  = 100000
	maxCommandBytes = 1 << 20
)

func validateCommand(cmd *cobra.Command) error {
	if flagBool(cmd, "no-config") && cmd.Flags().Changed("config") {
		return fmt.Errorf("%w: --config and --no-config cannot be combined", errUsage)
	}

	if flagBool(cmd, "no-color") && flagBool(cmd, "color") {
		return fmt.Errorf("%w: --color and --no-color cannot be combined", errUsage)
	}

	overrides, _ := cmd.Flags().GetStringArray("set")
	if err := config.ValidateOverrides(overrides); err != nil {
		return fmt.Errorf("%w: %w", errUsage, err)
	}

	switch cmd.Name() {
	case "search":
		return validateSearch(cmd)
	case "record":
		return validateRecord(cmd)
	case "show", "edit":
		return validateConfigCommand(cmd, overrides)
	}

	return nil
}

func validateConfigCommand(cmd *cobra.Command, overrides []string) error {
	switch cmd.Name() {
	case "show":
		if flagBool(cmd, "raw") && (flagBool(cmd, "json") || flagBool(cmd, "no-config") || len(overrides) != 0) {
			return fmt.Errorf("%w: --raw requires a config file and cannot be combined with --json or --set", errUsage)
		}
	case "edit":
		if flagBool(cmd, "no-config") || len(overrides) != 0 {
			return fmt.Errorf("%w: config edit requires a disk file; omit --no-config and --set", errUsage)
		}

		if !inputIsTerminal(cmd) {
			return fmt.Errorf("%w: config edit needs interactive stdin; use 'zgod config show' to inspect configuration", errUsage)
		}
	}

	return nil
}

func validateSearch(cmd *cobra.Command) error {
	if err := validateSearchOptions(cmd); err != nil {
		return err
	}

	if flagBool(cmd, "shell") && !inputIsTerminal(cmd) {
		return fmt.Errorf("%w: --shell needs terminal stdin; omit --shell to print matching commands", errUsage)
	}

	return validateQuery(flagString(cmd, "mode"), flagString(cmd, "query"))
}

func validateSearchOptions(cmd *cobra.Command) error {
	for _, bound := range []struct {
		name string
		max  int
	}{{"height", maxSearchHeight}, {"limit", maxSearchLimit}} {
		value, _ := cmd.Flags().GetInt(bound.name)
		if cmd.Flags().Changed(bound.name) && (value < 1 || value > bound.max) {
			return fmt.Errorf("%w: --%s must be between 1 and %d", errUsage, bound.name, bound.max)
		}
	}

	if flagBool(cmd, "json") && flagBool(cmd, "shell") {
		return fmt.Errorf("%w: --json and --shell cannot be combined", errUsage)
	}

	mode := flagString(cmd, "mode")
	if _, ok := match.ParseMode(mode); !ok {
		return fmt.Errorf("%w: --mode must be fuzzy, regex or glob", errUsage)
	}

	return nil
}

func validateQuery(mode, query string) error {
	if mode == "glob" && !doublestar.ValidatePattern(query) {
		return fmt.Errorf("%w: invalid glob query", errUsage)
	}

	if mode == "regex" {
		if _, err := regexp.Compile("(?i)" + query); err != nil {
			return fmt.Errorf("%w: invalid regex query: %w", errUsage, err)
		}
	}

	return nil
}

func validateRecord(cmd *cobra.Command) error {
	if flagBool(cmd, "command-stdin") && cmd.Flags().Changed("command") {
		return fmt.Errorf("%w: choose --command-stdin or --command", errUsage)
	}

	if len(flagString(cmd, "command")) > maxCommandBytes {
		return fmt.Errorf("%w: command exceeds 1 MiB", errUsage)
	}

	if _, _, err := parseRecordTimingMS(cmd, 0); err != nil {
		return fmt.Errorf("%w: %w", errUsage, err)
	}

	code, _ := cmd.Flags().GetInt("exit-code")

	minimumExitCode := 0
	if runtime.GOOS == "windows" {
		minimumExitCode = math.MinInt32
	}

	if code < minimumExitCode || int64(code) > math.MaxUint32 {
		return fmt.Errorf("%w: --exit-code must be between %d and 4294967295", errUsage, minimumExitCode)
	}

	return nil
}

func inputIsTerminal(cmd *cobra.Command) bool {
	file, ok := cmd.InOrStdin().(*os.File)
	return ok && term.IsTerminal(file.Fd())
}

func outputIsTerminal(cmd *cobra.Command) bool {
	file, ok := cmd.OutOrStdout().(*os.File)
	return ok && term.IsTerminal(file.Fd())
}

func configOptions(cmd *cobra.Command) config.LoadOptions {
	overrides, _ := cmd.Flags().GetStringArray("set")
	if cmd.Name() == "search" {
		if cmd.Flags().Changed("cwd") {
			scope := "normal"
			if flagBool(cmd, "cwd") {
				scope = "cwd"
			}

			overrides = append(overrides, fmt.Sprintf("display.default_scope = %q", scope))
		}

		if cmd.Flags().Changed("mode") {
			overrides = append(overrides, fmt.Sprintf("display.default_mode = %q", flagString(cmd, "mode")))
		}
	}

	return config.LoadOptions{
		Path:      flagString(cmd, "config"),
		NoConfig:  flagBool(cmd, "no-config"),
		NoCreate:  false,
		Overrides: overrides,
	}
}

func loadConfig(cmd *cobra.Command) (config.Config, error) {
	cfg, err := config.LoadWithOptions(configOptions(cmd))
	if err != nil {
		return cfg, fmt.Errorf("loading configuration: %w", err)
	}

	return cfg, nil
}

func commandConfigPath(cmd *cobra.Command) (string, error) {
	if path := flagString(cmd, "config"); path != "" {
		expanded, err := paths.ExpandTilde(path)
		if err != nil {
			return "", fmt.Errorf("expanding config path: %w", err)
		}

		return expanded, nil
	}

	path, err := paths.ConfigFile()
	if err != nil {
		return "", fmt.Errorf("resolving config path: %w", err)
	}

	return path, nil
}
