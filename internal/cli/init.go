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
	Long:      "Print shell integration script. Supported shells: bash, zsh, fish.",
	ValidArgs: []string{"zsh", "bash", "fish"},
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			_ = cmd.Help()
			return nil
		}
		s, err := shell.Parse(args[0])
		if err != nil {
			return err
		}
		opts := shell.InitOptions{}
		if initConfigPath != "" {
			opts.ConfigPath = initConfigPath
		}
		script, err := shell.InitScript(s, opts)
		if err != nil {
			return err
		}
		fmt.Print(script)
		return nil
	},
}

//nolint:gochecknoinits // cobra CLI pattern
func init() {
	initCmd.Flags().StringVar(&initConfigPath, "config", "", "Path to config file")
	rootCmd.AddCommand(initCmd)
}
