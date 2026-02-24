package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/zigai/zgod/internal/shell"
)

var initConfigPath string

var initCmd = &cobra.Command{
	Use:       "init <shell>",
	Short:     "Print shell integration script",
	Long:      "Print shell integration script. Supported shells: bash, zsh, fish, powershell.",
	ValidArgs: []string{"zsh", "bash", "fish", "powershell", "pwsh"},
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			_ = cmd.Help()
			return nil
		}
		s, err := shell.Parse(args[0])
		if err != nil {
			return fmt.Errorf("parsing shell %q: %w", args[0], err)
		}
		opts := shell.InitOptions{ConfigPath: initConfigPath}
		script, err := shell.InitScript(s, opts)
		if err != nil {
			return fmt.Errorf("building init script for %s: %w", s, err)
		}
		fmt.Print(script)
		return nil
	},
}

func init() {
	initCmd.Flags().StringVar(&initConfigPath, "config", "", "Path to config file")
	rootCmd.AddCommand(initCmd)
}
