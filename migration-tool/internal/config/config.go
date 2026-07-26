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

// RequireSource returns the source DSN, or an error naming both ways of
// supplying it. Commands that read the legacy database call this instead of
// letting an empty DSN fail somewhere less obvious.
func (c *Config) RequireSource() (string, error) {
	if c.SourceDSN == "" {
		return "", fmt.Errorf("no source database: pass --source or set MIGRATION_SOURCE_DSN")
	}
	return c.SourceDSN, nil
}

// RequireTarget returns the target DSN, or an error naming both ways of
// supplying it.
func (c *Config) RequireTarget() (string, error) {
	if c.TargetDSN == "" {
		return "", fmt.Errorf("no target database: pass --target or set MIGRATION_TARGET_DSN")
	}
	return c.TargetDSN, nil
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
