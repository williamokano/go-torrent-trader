package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

// runCmd executes the migration from source to target database.
//
// Not implemented. It fails rather than exiting zero: a cutover script that
// reads success from a command that did nothing is worse than one that stops.
var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the migration from source to target database",
	RunE: func(_ *cobra.Command, _ []string) error {
		return fmt.Errorf("`run` is not implemented yet — it needs writing to PostgreSQL, the entity transformers, and the resumable run")
	},
}
