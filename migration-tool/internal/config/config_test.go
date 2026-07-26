package config

import (
	"strings"
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

// setFlag sets a flag or fails the test, so a typo in a flag name shows up as a
// failure rather than as a silently unset value.
func setFlag(t *testing.T, cmd *cobra.Command, name, value string) {
	t.Helper()
	if err := cmd.Flags().Set(name, value); err != nil {
		t.Fatalf("setting --%s: %v", name, err)
	}
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
	setFlag(t, cmd, "source", "mysql://root@localhost/legacy")
	setFlag(t, cmd, "target", "postgres://localhost/new")
	setFlag(t, cmd, "log-level", "debug")
	setFlag(t, cmd, "dry-run", "true")

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

func TestRequireSource(t *testing.T) {
	if _, err := (&Config{}).RequireSource(); err == nil {
		t.Error("RequireSource with no DSN returned no error")
	} else {
		for _, want := range []string{"--source", "MIGRATION_SOURCE_DSN"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error does not mention %s: %v", want, err)
			}
		}
	}

	cfg := &Config{SourceDSN: "mysql://root@localhost/legacy"}
	got, err := cfg.RequireSource()
	if err != nil {
		t.Fatalf("RequireSource returned %v", err)
	}
	if got != cfg.SourceDSN {
		t.Errorf("RequireSource() = %q, want %q", got, cfg.SourceDSN)
	}
}

func TestRequireTarget(t *testing.T) {
	if _, err := (&Config{}).RequireTarget(); err == nil {
		t.Error("RequireTarget with no DSN returned no error")
	} else {
		for _, want := range []string{"--target", "MIGRATION_TARGET_DSN"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error does not mention %s: %v", want, err)
			}
		}
	}

	cfg := &Config{TargetDSN: "postgres://localhost/new"}
	got, err := cfg.RequireTarget()
	if err != nil {
		t.Fatalf("RequireTarget returned %v", err)
	}
	if got != cfg.TargetDSN {
		t.Errorf("RequireTarget() = %q, want %q", got, cfg.TargetDSN)
	}
}
