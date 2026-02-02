package cli

import (
	"github.com/spf13/cobra"

	"github.com/zigai/zgod/internal/shell"
)

var installConfigPath string

var installCmd = &cobra.Command{
	Use:          "install <shell>",
	Short:        "Install zgod shell integration",
	Long:         "Install zgod shell integration by adding the setup to your shell config file. Supported shells: bash, zsh, fish, powershell.",
	ValidArgs:    []string{"zsh", "bash", "fish", "powershell", "pwsh"},
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			_ = cmd.Help()
			return nil
		}
		s, err := shell.Parse(args[0])
		if err != nil {
			return err
		}
		return shell.Install(s, installConfigPath)
	},
}

//nolint:gochecknoinits // cobra CLI pattern
func init() {
	installCmd.Flags().StringVar(&installConfigPath, "config", "", "Path to config file")
	rootCmd.AddCommand(installCmd)
}
