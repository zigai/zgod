package cli

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

const (
	documentationURL = "https://github.com/zigai/zgod#usage"
	exitInterrupted  = 130
)

var (
	version  = "dev"
	commit   = "none"
	date     = "unknown"
	errUsage = errors.New("invalid usage")
)

type exitError struct {
	code int
}

func (e exitError) Error() string {
	return fmt.Sprintf("exit status %d", e.code)
}

func Execute() {
	os.Exit(execute(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func execute(args []string, input io.Reader, output, diagnostics io.Writer) int {
	root := newRootCommand()
	root.SetArgs(args)
	root.SetIn(input)
	root.SetOut(output)
	root.SetErr(diagnostics)
	routeFlagDiagnostics(root, diagnostics)

	// Find reports unknown commands before any action can run.
	if _, _, err := root.Find(args); err != nil {
		return reportError(root, fmt.Errorf("%w: %w", errUsage, err))
	}

	cmd, err := root.ExecuteC()
	if err == nil {
		return 0
	}

	if cmd == nil {
		cmd = root
	}

	return reportError(cmd, err)
}

func routeFlagDiagnostics(cmd *cobra.Command, output io.Writer) {
	cmd.Flags().SetOutput(output)
	cmd.PersistentFlags().SetOutput(output)

	for _, child := range cmd.Commands() {
		routeFlagDiagnostics(child, output)
	}
}

func reportError(cmd *cobra.Command, err error) int {
	if result, ok := errors.AsType[exitError](err); ok {
		return result.code
	}

	code := 1
	if errors.Is(err, errUsage) {
		code = 2
	}

	if _, writeErr := fmt.Fprintf(cmd.ErrOrStderr(), "Error: %s\nRun '%s --help' for valid arguments and examples.\nDocs: %s\n", err, cmd.CommandPath(), documentationURL); writeErr != nil {
		return 1
	}

	return code
}

func newRootCommand() *cobra.Command {
	cobra.EnableCommandSorting = false
	root := &cobra.Command{
		Use:           "zgod",
		Short:         "Local shell history search tool",
		Long:          "Search and record local shell history.\n\nConfiguration: flags > environment > ./.zgod.toml > user config > system config > defaults.\nUse --no-config to skip configuration files, or --set section.key=value to override a TOML setting.",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if flagBool(cmd, "version") || flagBool(cmd, "legacy-version") {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "zgod %s (commit: %s, built: %s)\n", version, commit, date); err != nil {
					return fmt.Errorf("writing version: %w", err)
				}

				return nil
			}

			return cmd.Help()
		},
	}
	root.AddGroup(
		&cobra.Group{ID: "core", Title: "Core Commands:"},
		&cobra.Group{ID: "integration", Title: "Shell Integration:"},
		&cobra.Group{ID: "management", Title: "Management Commands:"},
	)
	root.SetHelpCommandGroupID("management")
	root.SetCompletionCommandGroupID("integration")
	root.Flags().BoolP("version", "V", false, "Print version and build metadata")
	root.Flags().BoolP("legacy-version", "v", false, "Deprecated version alias")
	_ = root.Flags().MarkDeprecated("legacy-version", "use -V or --version; -v will be released in the next major version")
	root.PersistentFlags().String("config", "", "Read this TOML file instead of discovered files (env: ZGOD_CONFIG)")
	root.PersistentFlags().Bool("no-config", false, "Skip all disk configuration and default-file creation")
	root.PersistentFlags().StringArray("set", nil, "Override section.key with a TOML value; repeatable; arrays replace lists")
	root.PersistentFlags().Bool("no-color", false, "Disable colors and styling")
	root.PersistentFlags().Bool("color", false, "Enable base ANSI colors on interactive terminals")
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return fmt.Errorf("%w: %w", errUsage, err)
	})
	registerSearchCommand(root)
	registerConfigCommand(root)
	registerInitCommand(root)
	registerRecordCommand(root)
	registerImportCommand(root)
	registerInstallCommand(root)
	root.InitDefaultHelpCmd()
	root.InitDefaultCompletionCmd()
	prepareCommand(root)

	return root
}

func prepareCommand(cmd *cobra.Command) {
	// Preserve old scripts during the public deprecation window; canonical help
	// exposes --help only. Remove this shorthand in the next major release.
	cmd.Flags().BoolP("help", "h", false, "Show help")
	_ = cmd.Flags().MarkShorthandDeprecated("help", "use --help; -h will be released in the next major version")

	validate := cmd.Args
	if validate == nil {
		validate = cobra.NoArgs
		if cmd.Name() == "help" {
			validate = cobra.ArbitraryArgs
		}
	}

	cmd.Args = func(command *cobra.Command, args []string) error {
		if err := validate(command, args); err != nil {
			return fmt.Errorf("%w: %w", errUsage, err)
		}

		return validateCommand(command)
	}
	for _, child := range cmd.Commands() {
		prepareCommand(child)
	}
}

func flagBool(cmd *cobra.Command, name string) bool {
	value, _ := cmd.Flags().GetBool(name)
	return value
}

func flagString(cmd *cobra.Command, name string) string {
	value, _ := cmd.Flags().GetString(name)
	return value
}
