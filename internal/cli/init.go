package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/zigai/zgod/internal/shell"
)

func registerInitCommand(root *cobra.Command) {
	initCmd := &cobra.Command{
		Use:       "init <shell>",
		GroupID:   "integration",
		Args:      cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		Short:     "Print shell integration script",
		Long:      "Print shell integration script. Supported shells: bash, zsh, fish, powershell.",
		ValidArgs: []string{"zsh", "bash", "fish", "powershell", "pwsh"},
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := shell.Parse(args[0])
			if err != nil {
				return fmt.Errorf("parsing shell %q: %w", args[0], err)
			}

			opts := shell.InitOptions{
				ConfigPath: flagString(cmd, "config"),
				BinPath:    shell.CurrentExecutablePath(),
			}

			script, err := shell.InitScript(s, opts)
			if err != nil {
				return fmt.Errorf("building init script for %s: %w", s, err)
			}

			if _, err = fmt.Fprint(cmd.OutOrStdout(), script); err != nil {
				return fmt.Errorf("writing shell integration: %w", err)
			}

			return nil
		},
	}
	root.AddCommand(initCmd)
}
