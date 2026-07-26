package source_test

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/williamokano/go-torrent-trader/migration-tool/internal/baseline"
	"github.com/williamokano/go-torrent-trader/migration-tool/internal/compare"
	"github.com/williamokano/go-torrent-trader/migration-tool/internal/mapping"
	"github.com/williamokano/go-torrent-trader/migration-tool/internal/schema"
	"github.com/williamokano/go-torrent-trader/migration-tool/internal/source"
	"github.com/williamokano/go-torrent-trader/migration-tool/internal/testenv"
)

// These tests run against a real MySQL server, because the thing most likely to
// be wrong is not the logic but the assumption about what the server reports:
// MySQL 8 spells several of the reference document's types differently, and a
// comparison that cannot see past that would call every stock install modded.
//
// Run with -short to skip when Docker is unavailable.

func TestMain(m *testing.M) {
	code := m.Run()
	testenv.Cleanup()
	os.Exit(code)
}

func legacyDSN(t *testing.T) string { return testenv.Legacy(t) }

func openLegacy(t *testing.T) *source.DB {
	t.Helper()
	db, err := source.Open(context.Background(), legacyDSN(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return db
}

func TestOpenRejectsABadDSN(t *testing.T) {
	if _, err := source.Open(context.Background(), ""); err == nil {
		t.Error("Open(\"\") returned no error")
	}
	if _, err := source.Open(context.Background(), "not-a-dsn"); err == nil {
		t.Error("Open(\"not-a-dsn\") returned no error")
	}
}

// A wrong password is the most likely connection failure, and its error message
// is the most likely place for a password to leak into a log.
func TestOpenErrorDoesNotLeakThePassword(t *testing.T) {
	dsn := legacyDSN(t)
	broken := strings.Replace(dsn, "tt-secret", "wrong-password", 1)
	if broken == dsn {
		t.Fatal("test setup: password not found in the DSN")
	}

	_, err := source.Open(context.Background(), broken)
	if err == nil {
		t.Fatal("connecting with a wrong password succeeded")
	}
	if strings.Contains(err.Error(), "wrong-password") {
		t.Errorf("connection error leaked the password: %v", err)
	}
}

func TestSchemaReadsTablesAndColumns(t *testing.T) {
	db := openLegacy(t)

	s, err := db.Schema(context.Background())
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}

	if s.Database != "torrenttrader" {
		t.Errorf("Database = %q, want torrenttrader", s.Database)
	}
	for _, want := range []string{"users", "groups", "torrents", "peers", "bonus_log"} {
		if _, ok := s.Table(want); !ok {
			t.Errorf("table %s missing; found %v", want, s.TableNames())
		}
	}

	users, _ := s.Table("users")
	if got := len(users.Columns); got != testenv.UserColumns {
		t.Errorf("users has %d columns, want %d (43 stock plus 2 mods)", got, testenv.UserColumns)
	}

	// Columns come back in ordinal order, which is what makes the generated
	// mapping readable next to the original DDL.
	if users.Columns[0].Name != "id" || users.Columns[1].Name != "username" {
		t.Errorf("columns are not in ordinal order: %v", users.ColumnNames()[:2])
	}

	id, ok := users.Column("id")
	if !ok {
		t.Fatal("users.id missing")
	}
	if id.Key != "PRI" {
		t.Errorf("users.id key = %q, want PRI", id.Key)
	}
	if id.Extra != "auto_increment" {
		t.Errorf("users.id extra = %q, want auto_increment", id.Extra)
	}
	if id.Nullable {
		t.Error("users.id reported nullable")
	}

	info, ok := users.Column("info")
	if !ok {
		t.Fatal("users.info missing")
	}
	if !info.Nullable {
		t.Error("users.info reported not nullable")
	}
}

// The whole point of the round trip: what the reference document says a column
// is, and what MySQL 8 says it is, must reconcile.
func TestSchemaTypesReconcileWithTheBaseline(t *testing.T) {
	db := openLegacy(t)

	s, err := db.Schema(context.Background())
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	users, _ := s.Table("users")

	tests := []struct{ column, baselineType string }{
		{"id", "int unsigned"},
		{"username", "varchar(40)"},
		// The document writes this as VARCHAR(20) BINARY; MySQL 8 reports the
		// type and the collation separately.
		{"secret", "varchar(20)"},
		{"class", "tinyint unsigned"},
		{"uploaded", "bigint unsigned"},
		{"status", "enum('pending','confirmed')"},
		{"warned", "char(3)"},
		{"invites", "smallint"},
		{"added", "datetime"},
		{"info", "text"},
	}
	for _, tc := range tests {
		t.Run(tc.column, func(t *testing.T) {
			c, ok := users.Column(tc.column)
			if !ok {
				t.Fatalf("users.%s missing", tc.column)
			}
			if !schema.TypesEqual(tc.baselineType, c.Type) {
				t.Errorf("server reports %q, baseline says %q; they normalize to %q and %q",
					c.Type, tc.baselineType,
					schema.NormalizeType(c.Type), schema.NormalizeType(tc.baselineType))
			}
		})
	}
}

func TestCountRows(t *testing.T) {
	db := openLegacy(t)
	ctx := context.Background()

	for table, want := range map[string]int64{
		"users":    testenv.CorpusUsers,
		"torrents": testenv.CorpusTorrents,
		"peers":    testenv.CorpusPeers,
	} {
		got, err := db.CountRows(ctx, table)
		if err != nil {
			t.Fatalf("CountRows(%s): %v", table, err)
		}
		if got != want {
			t.Errorf("CountRows(%s) = %d, want %d", table, got, want)
		}
	}
}

// A table name reaches this query as text because MySQL cannot parameterize an
// identifier. It must be quoted, not interpolated.
func TestCountRowsRefusesAHostileTableName(t *testing.T) {
	db := openLegacy(t)
	ctx := context.Background()

	hostile := []string{
		"users`; DROP TABLE users; -- ",
		"users WHERE 1=1; DROP TABLE users",
		"",
	}
	for _, name := range hostile {
		if _, err := db.CountRows(ctx, name); err == nil {
			t.Errorf("CountRows(%q) succeeded", name)
		}
	}

	// The point of the exercise: users is still there.
	if n, err := db.CountRows(ctx, "users"); err != nil || n != testenv.CorpusUsers {
		t.Fatalf("users did not survive: count=%d err=%v", n, err)
	}
}

func TestCreateTableReturnsDDL(t *testing.T) {
	db := openLegacy(t)

	ddl, err := db.CreateTable(context.Background(), "users")
	if err != nil {
		t.Fatalf("CreateTable: %v", err)
	}
	for _, want := range []string{"CREATE TABLE", "username", "passkey", "seedbonus"} {
		if !strings.Contains(ddl, want) {
			t.Errorf("DDL does not mention %q:\n%s", want, ddl)
		}
	}

	if _, err := db.CreateTable(context.Background(), "no_such_table"); err == nil {
		t.Error("CreateTable on a missing table returned no error")
	}
}

// A backticked table name has to survive both introspection and the identifier
// quoting, since `groups` is reserved in MySQL 8.
func TestReservedTableNameIsHandled(t *testing.T) {
	db := openLegacy(t)

	if _, err := db.CountRows(context.Background(), "groups"); err != nil {
		t.Errorf("CountRows(groups): %v", err)
	}
	if _, err := db.CreateTable(context.Background(), "groups"); err != nil {
		t.Errorf("CreateTable(groups): %v", err)
	}
}

// The end-to-end claim: a modded install is reported as usable, with the mods
// named and nothing invented.
func TestCompareAgainstARealModdedDatabase(t *testing.T) {
	db := openLegacy(t)

	s, err := db.Schema(context.Background())
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	r := compare.Compare(baseline.TorrentTrader30(), s, mapping.ReadColumns())

	if r.Blocking() {
		t.Errorf("a modded but complete install was reported unusable: missing required %v, tables %+v",
			r.MissingRequiredTables, r.Tables)
	}
	if r.Clean() {
		t.Error("a modded install was reported as stock")
	}

	if !slices.Contains(r.AddedTables, "bonus_log") {
		t.Errorf("AddedTables = %v, want it to contain bonus_log", r.AddedTables)
	}
	if !slices.Contains(r.MissingTables, "polls") {
		t.Errorf("MissingTables = %v, want it to contain polls", r.MissingTables)
	}

	var users compare.TableReport
	for _, tr := range r.Tables {
		if tr.Name == "users" {
			users = tr
		}
	}
	if len(users.MissingColumns) != 0 {
		t.Errorf("users reported missing columns %v; the fixture has the full stock set", users.MissingColumns)
	}
	if !slices.Contains(users.AddedColumns, "seedbonus") || !slices.Contains(users.AddedColumns, "karma") {
		t.Errorf("users.AddedColumns = %v, want seedbonus and karma", users.AddedColumns)
	}
	if len(users.TypeMismatches) != 0 {
		t.Errorf("stock columns reported type mismatches against a real server: %+v", users.TypeMismatches)
	}
}

func TestMappingGeneratedFromARealDatabase(t *testing.T) {
	db := openLegacy(t)

	s, err := db.Schema(context.Background())
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}

	doc := mapping.Build(s, db.String())
	out, err := mapping.Render(doc)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	// The mods are the entries an operator has to act on, and the only ones.
	decisions := doc.Decisions()
	for _, want := range []string{"users.seedbonus", "users.karma"} {
		if !slices.Contains(decisions, want) {
			t.Errorf("Decisions() = %v, want it to contain %s", decisions, want)
		}
	}
	for _, notWanted := range []string{"users.passkey", "torrents.info_hash"} {
		if slices.Contains(decisions, notWanted) {
			t.Errorf("%s was left for the operator to decide; it has a rule", notWanted)
		}
	}

	// The fixture is complete, so nothing the plan reads should be absent.
	if absent := doc.AbsentColumns(); len(absent) != 0 {
		t.Errorf("AbsentColumns() = %v, want none against a complete install", absent)
	}

	rendered := string(out)
	for _, want := range []string{"seedbonus", "bonus_log", "passkey", "info_hash"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("rendered mapping does not mention %q", want)
		}
	}

	if os.Getenv("MIGRATION_DUMP_MAPPING") != "" {
		t.Logf("generated mapping:\n%s", rendered)
	}
}

// A 2008-era TorrentTrader is latin1 throughout, and the target stores UTF-8.
// The tool has to be able to see that, because copying the bytes across
// unconverted is what turns accented usernames into mojibake.
func TestSchemaReadsTextEncoding(t *testing.T) {
	db := openLegacy(t)

	s, err := db.Schema(context.Background())
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}

	encodings := s.TextEncodings()
	if !slices.Contains(encodings, "latin1") {
		t.Errorf("TextEncodings() = %v, want it to contain latin1", encodings)
	}
	for _, e := range encodings {
		if schema.IsUTF8(e) {
			t.Errorf("%s reported as Unicode; this fixture is latin1 throughout", e)
		}
	}

	users, _ := s.Table("users")
	if !strings.Contains(strings.ToLower(users.Collation), "latin1") {
		t.Errorf("users collation = %q, want a latin1 one", users.Collation)
	}

	username, ok := users.Column("username")
	if !ok {
		t.Fatal("users.username missing")
	}
	if username.Charset != "latin1" {
		t.Errorf("users.username charset = %q, want latin1", username.Charset)
	}
	// A numeric column has no character set, and claiming one would send the
	// transformers converting integers.
	id, _ := users.Column("id")
	if id.Charset != "" {
		t.Errorf("users.id charset = %q, want empty", id.Charset)
	}
}

// The generated mapping has to carry the encoding, or a reviewer reading the
// file has no way to know the text needs converting.
func TestMappingRecordsNonUnicodeColumns(t *testing.T) {
	db := openLegacy(t)

	s, err := db.Schema(context.Background())
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	out, err := mapping.Render(mapping.Build(s, db.Server()))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	rendered := string(out)

	if !strings.Contains(rendered, "charset: latin1") {
		t.Error("the mapping does not record that the source is latin1")
	}
	// The source types travel with it, so the file can be reviewed without
	// reconnecting to the old database.
	for _, want := range []string{"type: varchar(40)", "type: bigint unsigned"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("the mapping does not record %q", want)
		}
	}
	// Credentials must not: this file is meant to be committed.
	for _, forbidden := range []string{"tt-secret", "tt:"} {
		if strings.Contains(rendered, forbidden) {
			t.Errorf("the mapping contains %q", forbidden)
		}
	}
}
