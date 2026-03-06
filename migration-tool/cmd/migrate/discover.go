package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

// discoverCmd connects to source DB and lists available tables/row counts.
var discoverCmd = &cobra.Command{
	Use:   "discover",
	Short: "Discover tables and data in the source database",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Discovering source database schema...")
		// TODO: implement source DB discovery
		return nil
	},
}
