package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/lib/pq"

	"gopkg.in/yaml.v3"

	"github.com/williamokano/go-torrent-trader/migration-tool/internal/testenv"
)

// The commands, end to end, against the legacy corpus. The internal packages
// are tested on their own; what is only provable here is that the flags, the
// output writers and the pipeline are wired to each other.
//
// Run with -short to skip when Docker is unavailable.

func TestMain(m *testing.M) {
	code := m.Run()
	testenv.Cleanup()
	os.Exit(code)
}

func legacyDSN(t *testing.T) string { return testenv.Legacy(t) }

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

	for _, want := range []string{"bonus_log", "seedbonus", "polls", "Result: usable",
		"latin1", "not Unicode", "mojibake"} {
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

// --ddl dumps the server's own CREATE TABLE for every table, which is the
// record of what the source schema was on the night of the run.
func TestDiscoverDumpsAllDDL(t *testing.T) {
	noDatabaseConfigured(t)

	out, err := execute(t, "discover", "--source", legacyDSN(t), "--ddl")
	if err != nil {
		t.Fatalf("discover --ddl: %v\n%s", err, out)
	}

	// Every table, not just one.
	for _, want := range []string{"CREATE TABLE `users`", "CREATE TABLE `torrents`",
		"CREATE TABLE `groups`", "CREATE TABLE `bonus_log`"} {
		if !strings.Contains(out, want) {
			t.Errorf("the dump is missing %q", want)
		}
	}
	if strings.Contains(out, "tt-secret") {
		t.Error("the dump contains the password")
	}
}

// --dry-run must not write the file. `migrate mapping --dry-run` writing one
// anyway is exactly the kind of thing an operator finds out about too late.
func TestMappingDryRunWritesNothing(t *testing.T) {
	noDatabaseConfigured(t)
	path := filepath.Join(t.TempDir(), "mapping.yaml")

	out, err := execute(t, "mapping", "--source", legacyDSN(t), "--out", path, "--dry-run")
	if err != nil {
		t.Fatalf("mapping --dry-run: %v\n%s", err, out)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("--dry-run wrote %s", path)
	}
	if !strings.Contains(out, "dry-run") {
		t.Errorf("the output does not say the file was withheld:\n%s", out)
	}
}

// The file records where it came from, and that provenance must not carry
// credentials — the README tells operators to commit it.
func TestMappingRecordsServerWithoutCredentials(t *testing.T) {
	noDatabaseConfigured(t)
	path := filepath.Join(t.TempDir(), "mapping.yaml")

	if _, err := execute(t, "mapping", "--source", legacyDSN(t), "--out", path); err != nil {
		t.Fatalf("mapping: %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the mapping: %v", err)
	}

	for _, forbidden := range []string{"tt-secret", "tt:***", "tt@"} {
		if strings.Contains(string(content), forbidden) {
			t.Errorf("the mapping records %q", forbidden)
		}
	}
	if !strings.Contains(string(content), "/torrenttrader") {
		t.Error("the mapping does not record which database it came from")
	}
}

// --target and --mapping turn `validate` into the pre-flight check: does the
// target have the schema this tool writes into, and does the mapping fit both
// databases. This is the run an operator makes before the cutover, and it has
// to fail in seconds rather than halfway through a million rows.

func TestValidateChecksTheTargetSchema(t *testing.T) {
	noDatabaseConfigured(t)

	out, err := execute(t, "validate", "--source", legacyDSN(t), "--target", testenv.Target(t))
	if err != nil {
		t.Fatalf("validate --target: %v\n%s", err, out)
	}
	for _, want := range []string{"Target:", "The schema is what this tool expects."} {
		if !strings.Contains(out, want) {
			t.Errorf("the output does not mention %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "tt-secret") {
		t.Error("the output printed the target password")
	}
}

func TestValidateRejectsATargetWithoutTheSchema(t *testing.T) {
	noDatabaseConfigured(t)

	// A database that exists but has never had the migrations run: the
	// ordinary way this goes wrong.
	empty := createEmptyDatabase(t)

	out, err := execute(t, "validate", "--source", legacyDSN(t), "--target", empty)
	if err == nil {
		t.Fatalf("validate against an unmigrated target passed:\n%s", out)
	}
	if !strings.Contains(err.Error(), "no tables") {
		t.Errorf("the error does not explain the target is empty: %v", err)
	}
}

func TestValidateChecksAMappingAgainstBothDatabases(t *testing.T) {
	noDatabaseConfigured(t)
	path := filepath.Join(t.TempDir(), "mapping.yaml")

	if _, err := execute(t, "mapping", "--source", legacyDSN(t), "--out", path); err != nil {
		t.Fatalf("generating the mapping: %v", err)
	}

	// As generated, the corpus's mod columns are undecided, so the mapping
	// must be refused rather than run.
	out, err := execute(t, "validate",
		"--source", legacyDSN(t), "--target", testenv.Target(t), "--mapping", path)
	if err == nil {
		t.Fatalf("a mapping with undecided columns was accepted:\n%s", out)
	}
	for _, want := range []string{"Mapping:", "seedbonus", "custom"} {
		if !strings.Contains(out, want) {
			t.Errorf("the report does not mention %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "stop the migration") {
		t.Errorf("the report does not say how many problems are fatal:\n%s", out)
	}
}

func TestValidateAcceptsADecidedMapping(t *testing.T) {
	noDatabaseConfigured(t)
	path := filepath.Join(t.TempDir(), "mapping.yaml")

	if _, err := execute(t, "mapping", "--source", legacyDSN(t), "--out", path); err != nil {
		t.Fatalf("generating the mapping: %v", err)
	}

	// Settle every decision the way an operator would: skip what the mods
	// added and skip the tables with no rules yet.
	decideEverything(t, path)

	out, err := execute(t, "validate",
		"--source", legacyDSN(t), "--target", testenv.Target(t), "--mapping", path)
	if err != nil {
		t.Fatalf("a fully decided mapping was refused: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Usable against both databases.") {
		t.Errorf("the report does not say the mapping is usable:\n%s", out)
	}
}

func TestValidateRejectsAnUnreadableMapping(t *testing.T) {
	noDatabaseConfigured(t)
	path := filepath.Join(t.TempDir(), "mapping.yaml")

	// A mapping from a future version of the tool.
	if err := os.WriteFile(path, []byte("version: 99\ntables:\n  users:\n    action: skip\n"), 0o600); err != nil {
		t.Fatalf("writing the mapping: %v", err)
	}

	_, err := execute(t, "validate", "--source", legacyDSN(t), "--mapping", path)
	if err == nil {
		t.Fatal("a mapping from an unknown version was accepted")
	}
	if !strings.Contains(err.Error(), "version 99") {
		t.Errorf("the error does not explain the version: %v", err)
	}
}

// createEmptyDatabase makes a database on the shared server with nothing in it,
// standing in for a target whose migrations were never run.
func createEmptyDatabase(t *testing.T) string {
	t.Helper()

	admin, err := sql.Open("postgres", testenv.Target(t))
	if err != nil {
		t.Fatalf("connecting to the target server: %v", err)
	}
	defer func() { _ = admin.Close() }()

	name := "unmigrated_" + strings.ToLower(t.Name())
	for _, stmt := range []string{
		`DROP DATABASE IF EXISTS ` + pgQuote(name),
		`CREATE DATABASE ` + pgQuote(name),
	} {
		if _, err := admin.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	t.Cleanup(func() {
		conn, err := sql.Open("postgres", testenv.Target(t))
		if err != nil {
			t.Logf("cleaning up %s: %v", name, err)
			return
		}
		defer func() { _ = conn.Close() }()
		if _, err := conn.Exec(`DROP DATABASE IF EXISTS ` + pgQuote(name)); err != nil {
			t.Logf("dropping %s: %v", name, err)
		}
	})

	// The shared DSN points at the migrated database; swap in the empty one.
	return strings.Replace(testenv.Target(t), "/torrenttrader?", "/"+name+"?", 1)
}

func pgQuote(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// decideEverything settles every open decision in a mapping the way an operator
// would if they wanted nothing but the stock data: skip the mods, skip the
// tables with no rules.
func decideEverything(t *testing.T, path string) {
	t.Helper()

	content, err := os.ReadFile(path) //nolint:gosec // a path this test just wrote
	if err != nil {
		t.Fatalf("reading the mapping: %v", err)
	}

	out := strings.ReplaceAll(string(content), "action: custom", "action: skip")
	out = strings.ReplaceAll(out, "action: review", "action: skip")
	if err := os.WriteFile(path, []byte(out), 0o600); err != nil {
		t.Fatalf("writing the mapping: %v", err)
	}
}

// The pre-flight's whole purpose is answering "will this run?" before the first
// write, and target drift was printed and then **exited 0** — so a cutover script
// written as `migrate validate && migrate run` sailed straight past a database
// whose migrations had never been run, and died partway through a table with a
// message about one column, having already written half the members.
//
// TestValidateRejectsATargetWithoutTheSchema did not catch it because a *totally
// empty* database fails earlier and elsewhere, in connect.go's "no tables" check.
// The gap was the partially-migrated target: real tables, one column short. That is
// also the likelier accident, since it is what an interrupted or outdated migration
// leaves behind.
func TestValidateRejectsAPartiallyMigratedTarget(t *testing.T) {
	noDatabaseConfigured(t)

	db, err := sql.Open("postgres", testenv.Target(t))
	if err != nil {
		t.Fatalf("connecting to the target: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Drop a column the migration writes to, exactly as a target predating
	// 067_add_torrent_moderation.sql would lack it. Nullable with no default, so
	// putting it back is faithful.
	if _, err := db.Exec(`ALTER TABLE torrents DROP COLUMN IF EXISTS approved_by`); err != nil {
		t.Fatalf("dropping the column: %v", err)
	}
	// Its own connection, because the deferred Close above runs *before*
	// t.Cleanup — the first version of this test used `db` here, failed to restore
	// the column, and left the shared target broken for every test after it.
	t.Cleanup(func() {
		conn, err := sql.Open("postgres", testenv.Target(t))
		if err != nil {
			t.Fatalf("reconnecting to restore approved_by: %v", err)
		}
		defer func() { _ = conn.Close() }()
		if _, err := conn.Exec(
			`ALTER TABLE torrents ADD COLUMN IF NOT EXISTS approved_by BIGINT REFERENCES users(id) ON DELETE SET NULL`,
		); err != nil {
			t.Fatalf("restoring approved_by: %v", err)
		}
	})

	out, err := execute(t, "validate", "--source", legacyDSN(t), "--target", testenv.Target(t))

	if err == nil {
		t.Fatalf("validate passed against a target missing a column the migration "+
			"writes to, so `validate && run` would proceed into a half-written table:\n%s", out)
	}
	// The report still has to name the column, or a non-zero exit tells the
	// operator only that something is wrong.
	if !strings.Contains(out, "approved_by") {
		t.Errorf("the output does not name the missing column:\n%s", out)
	}
	if !strings.Contains(out, "Run the backend's migrations") {
		t.Errorf("the output does not say how to fix it:\n%s", out)
	}
}

// And a target that is fully migrated still passes, so the check above is not
// simply failing for everyone.
func TestValidateAcceptsAFullyMigratedTarget(t *testing.T) {
	noDatabaseConfigured(t)

	out, err := execute(t, "validate", "--source", legacyDSN(t), "--target", testenv.Target(t))
	if err != nil {
		t.Fatalf("validate failed against a fully migrated target: %v\n%s", err, out)
	}
	if !strings.Contains(out, "The schema is what this tool expects.") {
		t.Errorf("the output does not confirm the target schema:\n%s", out)
	}
}
