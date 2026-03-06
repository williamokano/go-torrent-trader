package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

// rollbackCmd reverts a migration by truncating target tables.
var rollbackCmd = &cobra.Command{
	Use:   "rollback",
	Short: "Rollback migration (truncate target tables)",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Rolling back migration...")
		// TODO: implement rollback
		return nil
	},
}
