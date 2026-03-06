package main

import (
	"testing"
)

func TestRootCommand(t *testing.T) {
	// Reset args so cobra doesn't pick up test flags.
	rootCmd.SetArgs([]string{})

	code := run()
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
}

func TestSubcommandsRegistered(t *testing.T) {
	expected := []string{"discover", "validate", "run", "verify", "rollback"}

	commands := rootCmd.Commands()
	registered := make(map[string]bool)
	for _, cmd := range commands {
		registered[cmd.Name()] = true
	}

	for _, name := range expected {
		if !registered[name] {
			t.Errorf("expected subcommand %q to be registered, but it was not", name)
		}
	}
}

func TestPersistentFlagsExist(t *testing.T) {
	flags := []string{"source", "target", "log-level", "dry-run"}

	for _, name := range flags {
		f := rootCmd.PersistentFlags().Lookup(name)
		if f == nil {
			t.Errorf("expected persistent flag %q to exist, but it was not found", name)
		}
	}
}

func TestPersistentFlagDefaults(t *testing.T) {
	logLevel := rootCmd.PersistentFlags().Lookup("log-level")
	if logLevel.DefValue != "info" {
		t.Errorf("expected log-level default to be %q, got %q", "info", logLevel.DefValue)
	}

	dryRun := rootCmd.PersistentFlags().Lookup("dry-run")
	if dryRun.DefValue != "false" {
		t.Errorf("expected dry-run default to be %q, got %q", "false", dryRun.DefValue)
	}
}

func TestSubcommandExecution(t *testing.T) {
	subcommands := []string{"discover", "validate", "run", "verify", "rollback"}

	for _, sub := range subcommands {
		t.Run(sub, func(t *testing.T) {
			rootCmd.SetArgs([]string{sub})
			err := rootCmd.Execute()
			if err != nil {
				t.Errorf("subcommand %q returned unexpected error: %v", sub, err)
			}
		})
	}
}
