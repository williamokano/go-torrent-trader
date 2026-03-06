package config

import (
	"testing"

	"github.com/spf13/cobra"
)

func newTestCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("source", "", "Source DSN")
	cmd.Flags().String("target", "", "Target DSN")
	cmd.Flags().String("log-level", "info", "Log level")
	cmd.Flags().Bool("dry-run", false, "Dry run")
	return cmd
}

func TestLoadFromFlagsDefaults(t *testing.T) {
	cmd := newTestCmd()

	cfg, err := LoadFromFlags(cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.LogLevel != "info" {
		t.Errorf("expected LogLevel info, got %s", cfg.LogLevel)
	}
	if cfg.DryRun != false {
		t.Errorf("expected DryRun false, got %v", cfg.DryRun)
	}
}

func TestLoadFromFlagsWithValues(t *testing.T) {
	cmd := newTestCmd()
	cmd.Flags().Set("source", "mysql://root@localhost/legacy")
	cmd.Flags().Set("target", "postgres://localhost/new")
	cmd.Flags().Set("log-level", "debug")
	cmd.Flags().Set("dry-run", "true")

	cfg, err := LoadFromFlags(cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.SourceDSN != "mysql://root@localhost/legacy" {
		t.Errorf("expected SourceDSN from flag, got %s", cfg.SourceDSN)
	}
	if cfg.TargetDSN != "postgres://localhost/new" {
		t.Errorf("expected TargetDSN from flag, got %s", cfg.TargetDSN)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("expected LogLevel debug, got %s", cfg.LogLevel)
	}
	if cfg.DryRun != true {
		t.Errorf("expected DryRun true, got %v", cfg.DryRun)
	}
}

func TestLoadFromFlagsEnvFallback(t *testing.T) {
	cmd := newTestCmd()
	t.Setenv("MIGRATION_SOURCE_DSN", "mysql://env@localhost/src")
	t.Setenv("MIGRATION_TARGET_DSN", "postgres://env@localhost/tgt")

	cfg, err := LoadFromFlags(cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.SourceDSN != "mysql://env@localhost/src" {
		t.Errorf("expected SourceDSN from env, got %s", cfg.SourceDSN)
	}
	if cfg.TargetDSN != "postgres://env@localhost/tgt" {
		t.Errorf("expected TargetDSN from env, got %s", cfg.TargetDSN)
	}
}
