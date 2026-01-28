package cli

import (
	"os"

	"github.com/spf13/cobra"
)

const version = "0.1.0"

var rootCmd = &cobra.Command{
	Use:   "zgod",
	Short: "Local shell history search tool",
	CompletionOptions: cobra.CompletionOptions{
		HiddenDefaultCmd: true,
	},
	Run: func(cmd *cobra.Command, args []string) {
		if v, _ := cmd.Flags().GetBool("version"); v {
			cmd.Printf("zgod %s\n", version)
			return
		}
		_ = cmd.Help()
	},
}

func Execute() {
	rootCmd.Flags().BoolP("version", "v", false, "Print version")
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
