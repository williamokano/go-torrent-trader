package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

// verifyCmd checks migrated data integrity and completeness.
//
// Not implemented. It fails rather than exiting zero: a cutover script that
// reads success from a command that did nothing is worse than one that stops.
var verifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "Verify migrated data integrity and completeness",
	RunE: func(_ *cobra.Command, _ []string) error {
		return fmt.Errorf("`verify` is not implemented yet — it needs the verification suite")
	},
}
