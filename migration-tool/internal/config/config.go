package config

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Config holds the migration tool configuration.
type Config struct {
	SourceDSN string
	TargetDSN string
	LogLevel  string
	DryRun    bool
}

// LoadFromFlags reads configuration from cobra command flags, falling back to
// environment variables for DSN values.
func LoadFromFlags(cmd *cobra.Command) (*Config, error) {
	sourceDSN, err := cmd.Flags().GetString("source")
	if err != nil {
		return nil, fmt.Errorf("reading source flag: %w", err)
	}
	if sourceDSN == "" {
		sourceDSN = os.Getenv("MIGRATION_SOURCE_DSN")
	}

	targetDSN, err := cmd.Flags().GetString("target")
	if err != nil {
		return nil, fmt.Errorf("reading target flag: %w", err)
	}
	if targetDSN == "" {
		targetDSN = os.Getenv("MIGRATION_TARGET_DSN")
	}

	logLevel, err := cmd.Flags().GetString("log-level")
	if err != nil {
		return nil, fmt.Errorf("reading log-level flag: %w", err)
	}

	dryRun, err := cmd.Flags().GetBool("dry-run")
	if err != nil {
		return nil, fmt.Errorf("reading dry-run flag: %w", err)
	}

	return &Config{
		SourceDSN: sourceDSN,
		TargetDSN: targetDSN,
		LogLevel:  logLevel,
		DryRun:    dryRun,
	}, nil
}
