package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/zigai/zgod/internal/shell"
)

func registerInstallCommand(root *cobra.Command) {
	installCmd := &cobra.Command{
		Use:          "install <shell>",
		GroupID:      "management",
		Args:         cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		Short:        "Install zgod shell integration",
		Long:         "Install zgod shell integration by adding the setup to your shell config file. Supported shells: bash, zsh, fish, powershell.",
		ValidArgs:    []string{"zsh", "bash", "fish", "powershell", "pwsh"},
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := shell.Parse(args[0])
			if err != nil {
				return fmt.Errorf("parsing shell %q: %w", args[0], err)
			}

			if err = shell.Install(s, flagString(cmd, "config")); err != nil {
				return fmt.Errorf("installing shell integration for %s: %w", s, err)
			}

			return nil
		},
	}
	root.AddCommand(installCmd)
}
