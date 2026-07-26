package main

import (
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "migrate",
	Short: "TorrentTrader legacy database migration tool",
	Long:  "Migrates data from a legacy TorrentTrader 3.x MySQL database to the new PostgreSQL schema.",
}

func init() {
	rootCmd.PersistentFlags().String("source", "", "Source MySQL DSN (required)")
	rootCmd.PersistentFlags().String("target", "", "Target PostgreSQL DSN (required)")
	rootCmd.PersistentFlags().String("log-level", "info", "Log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().Bool("dry-run", false, "Preview changes without writing")

	rootCmd.AddCommand(discoverCmd, validateCmd, mappingCmd, runCmd, verifyCmd, rollbackCmd)

	// A failed command has already explained itself; repeating the usage text
	// after the error buries it.
	rootCmd.SilenceUsage = true
}

func run() int {
	if err := rootCmd.Execute(); err != nil {
		return 1
	}
	return 0
}

func main() {
	os.Exit(run())
}
