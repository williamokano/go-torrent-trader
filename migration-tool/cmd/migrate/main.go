package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "migrate",
	Short: "TorrentTrader legacy database migration tool",
	Long:  "Migrates data from a legacy TorrentTrader 3.x MySQL database to the new PostgreSQL schema.",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("torrenttrader migration tool")
		fmt.Println("use --help to see available commands")
		return nil
	},
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
