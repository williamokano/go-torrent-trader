package mapping

import (
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/williamokano/go-torrent-trader/migration-tool/internal/schema"
)

func table(name string, columns ...string) schema.Table {
	t := schema.Table{Name: name}
	for _, c := range columns {
		t.Columns = append(t.Columns, schema.Column{Name: c, Type: "int"})
	}
	return t
}

func findTable(t *testing.T, doc Document, name string) Table {
	t.Helper()
	for _, tb := range doc.Tables {
		if tb.Name == name {
			return tb
		}
	}
	t.Fatalf("table %s missing from the document", name)
	return Table{}
}

func findColumn(t *testing.T, tb Table, name string) Rule {
	t.Helper()
	for _, c := range tb.Columns {
		if c.Name == name {
			return c.Rule
		}
	}
	t.Fatalf("column %s missing from table %s", name, tb.Name)
	return Rule{}
}

// The point of generating from the live database rather than the baseline: a
// column nobody anticipated has to appear as a decision, not vanish.
func TestBuildMarksModColumnsCustom(t *testing.T) {
	found := schema.Schema{Database: "tt", Tables: []schema.Table{
		table("users", "id", "username", "seedbonus"),
	}}

	doc := Build(found, "root@localhost:3306/tt")
	users := findTable(t, doc, "users")

	if rule := findColumn(t, users, "seedbonus"); rule.Action != ActionCustom {
		t.Errorf("seedbonus action = %q, want custom", rule.Action)
	}
	if rule := findColumn(t, users, "username"); rule.Action != ActionMap {
		t.Errorf("username action = %q, want map", rule.Action)
	}
}

// A stock column with no rule is a gap in this tool, not a mod. The distinction
// changes what the operator is being asked to do about it.
func TestBuildMarksUnplannedStockColumnsReview(t *testing.T) {
	found := schema.Schema{Database: "tt", Tables: []schema.Table{
		table("comments", "id", "torrent", "text"),
	}}

	doc := Build(found, "server")
	comments := findTable(t, doc, "comments")

	for _, name := range []string{"id", "torrent", "text"} {
		if rule := findColumn(t, comments, name); rule.Action != ActionReview {
			t.Errorf("comments.%s action = %q, want review", name, rule.Action)
		}
	}
	if comments.Target != "torrent_comments" {
		t.Errorf("comments targets %q, want torrent_comments", comments.Target)
	}
}

// A table with a target but no column rules is a wall of bare `review` entries.
// Something has to say they are all still to be decided.
func TestBuildFlagsTablesWithNoColumnRules(t *testing.T) {
	found := schema.Schema{Database: "tt", Tables: []schema.Table{
		table("comments", "id", "torrent", "text"),
		table("users", "id", "username"),
	}}

	doc := Build(found, "server")

	comments := findTable(t, doc, "comments")
	if !strings.Contains(comments.Comment, "No column rules yet") {
		t.Errorf("comments comment does not say its columns are unmapped: %q", comments.Comment)
	}
	// The original explanation is kept, not replaced.
	if !strings.Contains(comments.Comment, "torrent_comments") && !strings.Contains(comments.Comment, "News comments") {
		t.Errorf("the table's own comment was lost: %q", comments.Comment)
	}

	users := findTable(t, doc, "users")
	if strings.Contains(users.Comment, "No column rules yet") {
		t.Errorf("users has column rules but was flagged as unmapped: %q", users.Comment)
	}
}

func TestBuildMarksUnknownTablesForReview(t *testing.T) {
	found := schema.Schema{Database: "tt", Tables: []schema.Table{
		table("bonus_log", "id", "amount"),
	}}

	doc := Build(found, "server")
	tb := findTable(t, doc, "bonus_log")

	if tb.Action != TableReview {
		t.Errorf("action = %q, want review", tb.Action)
	}
	if !strings.Contains(tb.Comment, "mod") {
		t.Errorf("comment does not explain what the table is: %q", tb.Comment)
	}
	for _, c := range tb.Columns {
		if c.Rule.Action != ActionCustom {
			t.Errorf("%s action = %q, want custom", c.Name, c.Rule.Action)
		}
	}
}

// Columns of a table that is skipped wholesale do not each need a separate
// decision — that would drown the real ones.
func TestBuildSkipsColumnsOfSkippedTables(t *testing.T) {
	found := schema.Schema{Database: "tt", Tables: []schema.Table{
		table("sqlerr", "id", "query", "error"),
	}}

	doc := Build(found, "server")
	tb := findTable(t, doc, "sqlerr")

	if tb.Action != TableSkip {
		t.Errorf("action = %q, want skip", tb.Action)
	}
	for _, c := range tb.Columns {
		if c.Rule.Action != ActionSkip {
			t.Errorf("%s action = %q, want skip", c.Name, c.Rule.Action)
		}
	}
	if len(doc.Decisions()) != 0 {
		t.Errorf("a skipped table produced decisions: %v", doc.Decisions())
	}
}

// A rule with nothing to read is a transform that will fail at run time. It has
// to be visible before the run, not during it.
func TestBuildReportsPlannedColumnsTheDatabaseDoesNotHave(t *testing.T) {
	found := schema.Schema{Database: "tt", Tables: []schema.Table{
		table("users", "id", "username"),
	}}

	doc := Build(found, "server")

	absent := doc.AbsentColumns()
	if !slices.Contains(absent, "users.passkey") {
		t.Errorf("AbsentColumns() = %v, want it to contain users.passkey", absent)
	}
	// A column the plan skips is not missing anything.
	if slices.Contains(absent, "users.page") {
		t.Error("a skipped column was reported as absent")
	}
}

func TestBuildIncludesEveryTableAndColumnFound(t *testing.T) {
	found := schema.Schema{Database: "tt", Tables: []schema.Table{
		table("zeta", "a"),
		table("users", "id", "username"),
		table("alpha", "b", "c"),
	}}

	doc := Build(found, "server")

	if len(doc.Tables) != 3 {
		t.Fatalf("got %d tables, want 3", len(doc.Tables))
	}
	names := make([]string, 0, len(doc.Tables))
	total := 0
	for _, tb := range doc.Tables {
		names = append(names, tb.Name)
		total += len(tb.Columns)
	}
	if !slices.IsSorted(names) {
		t.Errorf("tables not sorted: %v", names)
	}
	if total != 5 {
		t.Errorf("got %d columns across the document, want 5", total)
	}
}

func TestBuildMatchesTableNamesCaseInsensitively(t *testing.T) {
	found := schema.Schema{Database: "tt", Tables: []schema.Table{
		table("Users", "ID", "PassKey"),
	}}

	doc := Build(found, "server")
	tb := findTable(t, doc, "Users")

	if tb.Target != "users" {
		t.Errorf("target = %q, want users — table name case must not defeat the plan", tb.Target)
	}
	if rule := findColumn(t, tb, "PassKey"); rule.Target != "passkey" {
		t.Errorf("PassKey target = %q, want passkey", rule.Target)
	}
}

func TestDecisionsListsWhatNeedsAnswering(t *testing.T) {
	found := schema.Schema{Database: "tt", Tables: []schema.Table{
		table("users", "id", "seedbonus"),
		table("comments", "id"),
	}}

	doc := Build(found, "server")
	got := doc.Decisions()
	want := []string{"comments.id", "users.seedbonus"}

	if !slices.Equal(got, want) {
		t.Errorf("Decisions() = %v, want %v", got, want)
	}
}

func TestRenderProducesParseableYAML(t *testing.T) {
	found := schema.Schema{Database: "torrenttrader", Tables: []schema.Table{
		table("users", "id", "username", "passkey", "seedbonus"),
		table("sqlerr", "id"),
	}}

	out, err := Render(Build(found, "root@localhost:3306/torrenttrader"))
	if err != nil {
		t.Fatalf("Render returned %v", err)
	}

	var doc struct {
		Version int `yaml:"version"`
		Source  struct {
			Database string `yaml:"database"`
			Server   string `yaml:"server"`
		} `yaml:"source"`
		Tables map[string]struct {
			Action  string `yaml:"action"`
			Target  string `yaml:"target"`
			Columns map[string]struct {
				Action    string `yaml:"action"`
				Target    string `yaml:"target"`
				Transform string `yaml:"transform"`
			} `yaml:"columns"`
			Derived       map[string]string `yaml:"derived"`
			AbsentColumns []string          `yaml:"absent_columns"`
		} `yaml:"tables"`
	}
	if err := yaml.Unmarshal(out, &doc); err != nil {
		t.Fatalf("generated YAML does not parse: %v\n%s", err, out)
	}

	if doc.Version != Version {
		t.Errorf("version = %d, want %d", doc.Version, Version)
	}
	if doc.Source.Database != "torrenttrader" {
		t.Errorf("source.database = %q, want torrenttrader", doc.Source.Database)
	}

	users, ok := doc.Tables["users"]
	if !ok {
		t.Fatal("users missing from the rendered file")
	}
	if users.Action != "migrate" || users.Target != "users" {
		t.Errorf("users = %q -> %q, want migrate -> users", users.Action, users.Target)
	}
	if got := users.Columns["passkey"].Target; got != "passkey" {
		t.Errorf("users.passkey target = %q, want passkey", got)
	}
	if got := users.Columns["seedbonus"].Action; got != "custom" {
		t.Errorf("users.seedbonus action = %q, want custom", got)
	}
	if users.Derived["password_scheme"] == "" {
		t.Error("users.derived.password_scheme missing; the target side is left unexplained")
	}
	if !slices.Contains(users.AbsentColumns, "email") {
		t.Errorf("absent_columns = %v, want it to contain email", users.AbsentColumns)
	}
	if doc.Tables["sqlerr"].Action != "skip" {
		t.Errorf("sqlerr action = %q, want skip", doc.Tables["sqlerr"].Action)
	}
}

// The reasoning is the deliverable. A mapping an operator cannot audit is one
// they have to take on trust on the one night it matters.
func TestRenderKeepsTheReasoningAsComments(t *testing.T) {
	found := schema.Schema{Database: "tt", Tables: []schema.Table{
		table("users", "id", "passkey", "page", "seedbonus"),
	}}

	out, err := Render(Build(found, "server"))
	if err != nil {
		t.Fatalf("Render returned %v", err)
	}
	got := string(out)

	wants := []struct{ what, substring string }{
		{"file header", "This is a proposal"},
		{"action legend", "map     copy this column"},
		{"why passkey is kept", "announce"},
		{"why page is dropped", "Who's-online"},
		{"what a custom entry means", "a mod added this column"},
	}
	for _, w := range wants {
		if !strings.Contains(got, w.substring) {
			t.Errorf("rendered file does not explain %s (looked for %q)\n%s", w.what, w.substring, got)
		}
	}

	// Comments must be comments, not values that happen to contain prose.
	for _, line := range strings.Split(got, "\n") {
		if strings.Contains(line, "This is a proposal") && !strings.Contains(line, "#") {
			t.Errorf("header text was rendered as a value, not a comment: %q", line)
		}
	}
}

func TestRenderIsStable(t *testing.T) {
	found := schema.Schema{Database: "tt", Tables: []schema.Table{
		table("users", "id", "username", "seedbonus"),
		table("torrents", "id", "info_hash"),
	}}

	first, err := Render(Build(found, "server"))
	if err != nil {
		t.Fatalf("Render returned %v", err)
	}
	for i := 0; i < 5; i++ {
		next, err := Render(Build(found, "server"))
		if err != nil {
			t.Fatalf("Render returned %v", err)
		}
		if string(next) != string(first) {
			t.Fatal("Render is not deterministic; the file would churn in version control")
		}
	}
}
