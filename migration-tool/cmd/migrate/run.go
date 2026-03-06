package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

// runCmd executes the migration from source to target database.
var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the migration from source to target database",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Running migration...")
		// TODO: implement migration
		return nil
	},
}
