package cli

import (
	"os"

	"github.com/spf13/cobra"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

var rootCmd = &cobra.Command{
	Use:   "zgod",
	Short: "Local shell history search tool",
	CompletionOptions: cobra.CompletionOptions{
		HiddenDefaultCmd: true,
	},
	Run: func(cmd *cobra.Command, args []string) {
		if v, _ := cmd.Flags().GetBool("version"); v {
			cmd.Printf("zgod %s (commit: %s, built: %s)\n", version, commit, date)
			return
		}

		_ = cmd.Help()
	},
}

func Execute() {
	rootCmd.Flags().BoolP("version", "v", false, "Print version")

	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}
