package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"
	"gopkg.in/yaml.v3"
)

// The commands, end to end, against a real legacy database. The internal
// packages are tested on their own; what is only provable here is that the
// flags, the output writers and the pipeline are wired to each other.
//
// Run with -short to skip when Docker is unavailable.

const containerStartTimeout = 3 * time.Minute

var (
	sharedOnce sync.Once
	sharedDSN  string
	sharedErr  error
	sharedStop func()
)

func startLegacyContainer() {
	ctx, cancel := context.WithTimeout(context.Background(), containerStartTimeout)
	defer cancel()

	container, err := tcmysql.Run(ctx,
		"mysql:8.0",
		tcmysql.WithDatabase("torrenttrader"),
		tcmysql.WithUsername("tt"),
		tcmysql.WithPassword("tt-secret"),
		tcmysql.WithScripts(filepath.Join("..", "..", "internal", "source", "testdata", "legacy.sql")),
	)
	if err != nil {
		sharedErr = fmt.Errorf("starting MySQL: %w", err)
		return
	}
	sharedStop = func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			log.Printf("terminating MySQL container: %v", err)
		}
	}

	dsn, err := container.ConnectionString(ctx)
	if err != nil {
		sharedErr = fmt.Errorf("building connection string: %w", err)
		return
	}
	sharedDSN = dsn
}

func TestMain(m *testing.M) {
	code := m.Run()
	if sharedStop != nil {
		sharedStop()
	}
	os.Exit(code)
}

func legacyDSN(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping: needs Docker")
	}

	sharedOnce.Do(startLegacyContainer)
	if sharedErr != nil {
		t.Fatal(sharedErr)
	}
	return sharedDSN
}

func TestDiscoverListsTables(t *testing.T) {
	noDatabaseConfigured(t)

	out, err := execute(t, "discover", "--source", legacyDSN(t))
	if err != nil {
		t.Fatalf("discover: %v\n%s", err, out)
	}

	for _, want := range []string{"users", "torrents", "bonus_log", "TABLE", "ENGINE", "ROWS"} {
		if !strings.Contains(out, want) {
			t.Errorf("discover output does not mention %q:\n%s", want, out)
		}
	}
	// The password reaches this command on its own command line.
	if strings.Contains(out, "tt-secret") {
		t.Errorf("discover printed the password:\n%s", out)
	}
}

func TestDiscoverExactCountsARealTable(t *testing.T) {
	noDatabaseConfigured(t)

	out, err := execute(t, "discover", "--source", legacyDSN(t), "--exact")
	if err != nil {
		t.Fatalf("discover --exact: %v\n%s", err, out)
	}
	if !strings.Contains(out, "counted exactly") {
		t.Errorf("output does not say the counts are exact:\n%s", out)
	}
	// Three users are in the fixture; MyISAM estimates would also give 3, so
	// this asserts the plumbing rather than the arithmetic.
	if !strings.Contains(out, "users") {
		t.Errorf("users missing from the listing:\n%s", out)
	}
}

func TestDiscoverDescribesOneTable(t *testing.T) {
	noDatabaseConfigured(t)

	out, err := execute(t, "discover", "--source", legacyDSN(t), "--table", "users")
	if err != nil {
		t.Fatalf("discover --table: %v\n%s", err, out)
	}

	for _, want := range []string{"COLUMN", "passkey", "seedbonus", "CREATE TABLE"} {
		if !strings.Contains(out, want) {
			t.Errorf("output does not mention %q:\n%s", want, out)
		}
	}
}

func TestDiscoverRejectsAnUnknownTable(t *testing.T) {
	noDatabaseConfigured(t)

	_, err := execute(t, "discover", "--source", legacyDSN(t), "--table", "no_such_table")

	if err == nil {
		t.Fatal("discover --table no_such_table returned no error")
	}
	if !strings.Contains(err.Error(), "no_such_table") {
		t.Errorf("error does not name the table: %v", err)
	}
}

// The fixture is a modded but complete install: validate must report the mods
// and still call it usable.
func TestValidateReportsModsAndPasses(t *testing.T) {
	noDatabaseConfigured(t)

	out, err := execute(t, "validate", "--source", legacyDSN(t))
	if err != nil {
		t.Fatalf("validate: %v\n%s", err, out)
	}

	for _, want := range []string{"bonus_log", "seedbonus", "polls", "Result: usable"} {
		if !strings.Contains(out, want) {
			t.Errorf("validate output does not mention %q:\n%s", want, out)
		}
	}
}

// --strict turns the same schema into a failure, which is what a cutover script
// wants when it expects a stock install.
func TestValidateStrictFailsOnAModdedSchema(t *testing.T) {
	noDatabaseConfigured(t)

	out, err := execute(t, "validate", "--source", legacyDSN(t), "--strict")

	if err == nil {
		t.Fatalf("validate --strict passed a modded schema:\n%s", out)
	}
	if !strings.Contains(err.Error(), "strict") {
		t.Errorf("error does not explain why it failed: %v", err)
	}
}

func TestMappingWritesAReviewableFile(t *testing.T) {
	noDatabaseConfigured(t)
	path := filepath.Join(t.TempDir(), "mapping.yaml")

	out, err := execute(t, "mapping", "--source", legacyDSN(t), "--out", path)
	if err != nil {
		t.Fatalf("mapping: %v\n%s", err, out)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the generated mapping: %v", err)
	}

	var parsed struct {
		Version int `yaml:"version"`
		Tables  map[string]struct {
			Action  string `yaml:"action"`
			Target  string `yaml:"target"`
			Columns map[string]struct {
				Action string `yaml:"action"`
				Target string `yaml:"target"`
			} `yaml:"columns"`
		} `yaml:"tables"`
	}
	if err := yaml.Unmarshal(content, &parsed); err != nil {
		t.Fatalf("the generated mapping does not parse: %v\n%s", err, content)
	}

	if parsed.Version != 1 {
		t.Errorf("version = %d, want 1", parsed.Version)
	}
	if got := parsed.Tables["users"].Columns["passkey"].Target; got != "passkey" {
		t.Errorf("users.passkey target = %q, want passkey", got)
	}
	if got := parsed.Tables["users"].Columns["seedbonus"].Action; got != "custom" {
		t.Errorf("users.seedbonus action = %q, want custom", got)
	}
	if got := parsed.Tables["bonus_log"].Action; got != "review" {
		t.Errorf("bonus_log action = %q, want review", got)
	}
	if strings.Contains(string(content), "tt-secret") {
		t.Error("the generated mapping contains the database password")
	}
}

// A mapping is hand-edited. Replacing one without being asked would throw away
// an operator's review.
func TestMappingRefusesToOverwrite(t *testing.T) {
	noDatabaseConfigured(t)
	path := filepath.Join(t.TempDir(), "mapping.yaml")

	if _, err := execute(t, "mapping", "--source", legacyDSN(t), "--out", path); err != nil {
		t.Fatalf("first mapping: %v", err)
	}
	if err := os.WriteFile(path, []byte("# reviewed by hand\n"), 0o600); err != nil {
		t.Fatalf("editing the mapping: %v", err)
	}

	_, err := execute(t, "mapping", "--source", legacyDSN(t), "--out", path)
	if err == nil {
		t.Fatal("mapping overwrote an existing file without being asked")
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Errorf("error does not say how to proceed: %v", err)
	}

	content, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("reading the mapping: %v", readErr)
	}
	if !strings.Contains(string(content), "reviewed by hand") {
		t.Error("the hand-edited file was overwritten anyway")
	}

	// --force is the way through.
	if _, err := execute(t, "mapping", "--source", legacyDSN(t), "--out", path, "--force"); err != nil {
		t.Fatalf("mapping --force: %v", err)
	}

	content, readErr = os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("reading the mapping: %v", readErr)
	}
	if strings.Contains(string(content), "reviewed by hand") {
		t.Error("--force did not replace the file")
	}
}

// `--out -` has to be a clean YAML stream, so the summary goes to stderr.
func TestMappingToStdoutIsPipeable(t *testing.T) {
	noDatabaseConfigured(t)

	out, err := execute(t, "mapping", "--source", legacyDSN(t), "--out", "-")
	if err != nil {
		t.Fatalf("mapping --out -: %v", err)
	}

	// execute captures stdout and stderr together, so the stream is checked by
	// parsing the leading YAML document rather than the whole buffer.
	yamlEnd := strings.Index(out, "\n\n1")
	if yamlEnd < 0 {
		yamlEnd = len(out)
	}
	var parsed struct {
		Version int `yaml:"version"`
	}
	if err := yaml.Unmarshal([]byte(out[:yamlEnd]), &parsed); err != nil {
		t.Fatalf("stdout is not a YAML document: %v\n%s", err, out)
	}
	if parsed.Version != 1 {
		t.Errorf("version = %d, want 1", parsed.Version)
	}
	if strings.Contains(out, "Wrote ") {
		t.Error("the stdout path printed a file-written message")
	}
}
