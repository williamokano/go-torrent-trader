package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/pflag"
)

// resetFlags puts every flag back to its default.
//
// A real process parses one command and exits, so cobra keeps parsed values on
// the command tree. In a test binary that state outlives the test: without this,
// a command run with --source would leave that DSN behind for the next one, and
// a test asserting "fails without a source" would quietly pass by connecting to
// the previous test's database.
func resetFlags() {
	reset := func(fs *pflag.FlagSet) {
		fs.VisitAll(func(f *pflag.Flag) {
			_ = f.Value.Set(f.DefValue)
			f.Changed = false
		})
	}
	reset(rootCmd.PersistentFlags())
	reset(rootCmd.Flags())
	for _, c := range rootCmd.Commands() {
		reset(c.Flags())
	}
}

// execute runs the root command with args, returning its output and error.
// Output is captured rather than printed so a failing assertion can show it.
func execute(t *testing.T, args ...string) (string, error) {
	t.Helper()
	resetFlags()

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs(args)
	t.Cleanup(func() {
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
		rootCmd.SetArgs(nil)
		resetFlags()
	})

	err := rootCmd.Execute()
	return out.String(), err
}

// noDatabaseConfigured clears the environment fallbacks so a test cannot
// accidentally reach a real database on the machine running it.
func noDatabaseConfigured(t *testing.T) {
	t.Helper()
	t.Setenv("MIGRATION_SOURCE_DSN", "")
	t.Setenv("MIGRATION_TARGET_DSN", "")
}

func TestRootCommandWithNoArgs(t *testing.T) {
	noDatabaseConfigured(t)

	rootCmd.SetArgs([]string{})
	if code := run(); code != 0 {
		t.Errorf("run() = %d, want 0", code)
	}
}

func TestSubcommandsRegistered(t *testing.T) {
	expected := []string{"discover", "validate", "mapping", "run", "verify", "rollback"}

	registered := make(map[string]bool)
	for _, cmd := range rootCmd.Commands() {
		registered[cmd.Name()] = true
	}

	for _, name := range expected {
		if !registered[name] {
			t.Errorf("subcommand %q is not registered", name)
		}
	}
}

func TestPersistentFlagsExist(t *testing.T) {
	for _, name := range []string{"source", "target", "log-level", "dry-run"} {
		if rootCmd.PersistentFlags().Lookup(name) == nil {
			t.Errorf("persistent flag %q not found", name)
		}
	}
}

func TestPersistentFlagDefaults(t *testing.T) {
	if got := rootCmd.PersistentFlags().Lookup("log-level").DefValue; got != "info" {
		t.Errorf("log-level default = %q, want info", got)
	}
	if got := rootCmd.PersistentFlags().Lookup("dry-run").DefValue; got != "false" {
		t.Errorf("dry-run default = %q, want false", got)
	}
}

// Every command that reads the legacy database has to say how to point it at
// one. Failing with a bare connection error, or worse succeeding silently,
// would leave an operator guessing.
func TestSourceCommandsExplainAMissingDSN(t *testing.T) {
	for _, name := range []string{"discover", "validate", "mapping"} {
		t.Run(name, func(t *testing.T) {
			noDatabaseConfigured(t)

			_, err := execute(t, name)
			if err == nil {
				t.Fatalf("%s with no source DSN returned no error", name)
			}
			msg := err.Error()
			for _, want := range []string{"--source", "MIGRATION_SOURCE_DSN"} {
				if !strings.Contains(msg, want) {
					t.Errorf("error does not mention %s: %v", want, err)
				}
			}
		})
	}
}

func TestSourceCommandsReportAnUnusableDSN(t *testing.T) {
	for _, name := range []string{"discover", "validate", "mapping"} {
		t.Run(name, func(t *testing.T) {
			noDatabaseConfigured(t)

			_, err := execute(t, name, "--source", "mysql://user@host:3306/")
			if err == nil {
				t.Fatalf("%s with a database-less DSN returned no error", name)
			}
			if !strings.Contains(err.Error(), "database name") {
				t.Errorf("error does not explain what is wrong: %v", err)
			}
		})
	}
}

// A DSN arrives on a command line and the error goes to a log. The password
// must not make that trip.
func TestCommandErrorsDoNotLeakThePassword(t *testing.T) {
	noDatabaseConfigured(t)

	// Port 1 refuses connections quickly, so this fails at connect time with
	// the password in hand.
	_, err := execute(t, "discover", "--source", "mysql://root:hunter2@127.0.0.1:1/tt")
	if err == nil {
		t.Fatal("connecting to a closed port returned no error")
	}
	if strings.Contains(err.Error(), "hunter2") {
		t.Errorf("error leaked the password: %v", err)
	}
	if !strings.Contains(err.Error(), "root:***") {
		t.Errorf("error does not identify the server it tried: %v", err)
	}
}

// The commands that still have no implementation must fail, not exit zero. A
// cutover script that reads `migrate run` exiting 0 as "the migration ran" is
// the worst outcome this tool can produce.
func TestUnimplementedCommandsFail(t *testing.T) {
	for _, name := range []string{"run", "verify", "rollback"} {
		t.Run(name, func(t *testing.T) {
			noDatabaseConfigured(t)

			_, err := execute(t, name)
			if err == nil {
				t.Fatalf("%s exited successfully without doing anything", name)
			}
			if !strings.Contains(strings.ToLower(err.Error()), "not implemented") {
				t.Errorf("%s does not say it is unimplemented: %v", name, err)
			}
		})
	}
}

func TestHelpListsTheSourceCommands(t *testing.T) {
	noDatabaseConfigured(t)

	out, err := execute(t, "--help")
	if err != nil {
		t.Fatalf("--help returned %v", err)
	}
	for _, name := range []string{"discover", "validate", "mapping"} {
		if !strings.Contains(out, name) {
			t.Errorf("help does not list %q:\n%s", name, out)
		}
	}
}
