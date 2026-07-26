package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

// rollbackCmd reverts a migration by truncating target tables.
//
// Not implemented. It fails rather than exiting zero: a cutover script that
// reads success from a command that did nothing is worse than one that stops.
var rollbackCmd = &cobra.Command{
	Use:   "rollback",
	Short: "Rollback migration (truncate target tables)",
	RunE: func(_ *cobra.Command, _ []string) error {
		return fmt.Errorf("`rollback` is not implemented yet — it needs the cutover playbook")
	},
}
