package mapping

import (
	"strings"
	"testing"

	"github.com/williamokano/go-torrent-trader/migration-tool/internal/schema"
	"github.com/williamokano/go-torrent-trader/migration-tool/internal/target"
)

// findProblem returns the first problem about a table and column, so a test can
// assert on the one it cares about instead of the whole list.
func findProblem(ps Problems, table, column string) (Problem, bool) {
	for _, p := range ps {
		if p.Table == table && p.Column == column {
			return p, true
		}
	}
	return Problem{}, false
}

func messages(ps Problems) string {
	var b strings.Builder
	for _, p := range ps {
		b.WriteString("\n  " + p.Severity.String() + " " + p.String())
	}
	return b.String()
}

// A mapping straight out of the generator, against the database it was
// generated from, is usable except for the decisions it asks a human to make.
func TestValidateAcceptsAGeneratedMappingOnceDecided(t *testing.T) {
	source := corpusSchema()
	doc := Build(source, "db:3306/torrenttrader")

	// As generated, the mod column is undecided and that must stop a run.
	ps := Validate(doc, source, target.Schema{})
	if !ps.Fatal() {
		t.Error("a mapping with an undecided column was accepted")
	}
	if p, ok := findProblem(ps, "users", "seedbonus"); !ok {
		t.Errorf("users.seedbonus was not flagged:%s", messages(ps))
	} else if !strings.Contains(p.Message, "custom") {
		t.Errorf("the message does not say what to do: %q", p.Message)
	}

	// Decide it, and the mapping is usable.
	decide(&doc, "users", "seedbonus", Rule{Action: ActionSkip})
	if ps := Validate(doc, source, target.Schema{}); ps.Fatal() {
		t.Errorf("a decided mapping was still refused:%s", messages(ps))
	}
}

func decide(doc *Document, table, column string, rule Rule) {
	for i := range doc.Tables {
		if doc.Tables[i].Name != table {
			continue
		}
		for j := range doc.Tables[i].Columns {
			if doc.Tables[i].Columns[j].Name == column {
				doc.Tables[i].Columns[j].Rule = rule
			}
		}
	}
}

// A mapping generated before somebody added a mod describes a database that no
// longer exists. Running it would read a column that is not there.
func TestValidateCatchesAStaleMapping(t *testing.T) {
	source := corpusSchema()
	doc := Build(source, "server")
	decide(&doc, "users", "seedbonus", Rule{Action: ActionSkip})

	t.Run("a column the source has lost", func(t *testing.T) {
		reduced := corpusSchema()
		reduced.Tables[0].Columns = reduced.Tables[0].Columns[:2] // drops passkey
		ps := Validate(doc, reduced, target.Schema{})

		p, ok := findProblem(ps, "users", "passkey")
		if !ok {
			t.Fatalf("the missing column was not reported:%s", messages(ps))
		}
		if p.Severity != Fatal {
			t.Errorf("severity = %s, want error", p.Severity)
		}
	})

	t.Run("a column the source has gained", func(t *testing.T) {
		grown := corpusSchema()
		grown.Tables[0].Columns = append(grown.Tables[0].Columns,
			schema.Column{Name: "karma", Type: "int"})
		ps := Validate(doc, grown, target.Schema{})

		p, ok := findProblem(ps, "users", "karma")
		if !ok {
			t.Fatalf("the new column was not reported:%s", messages(ps))
		}
		if p.Severity != Fatal {
			t.Errorf("severity = %s, want error", p.Severity)
		}
		if !strings.Contains(p.Message, "regenerate") {
			t.Errorf("the message does not say what to do: %q", p.Message)
		}
	})

	t.Run("a table the source has gained", func(t *testing.T) {
		grown := corpusSchema()
		grown.Tables = append(grown.Tables, schema.Table{
			Name:    "bonus_log",
			Columns: []schema.Column{{Name: "id", Type: "int"}},
		})
		ps := Validate(doc, grown, target.Schema{})
		if _, ok := findProblem(ps, "bonus_log", ""); !ok {
			t.Errorf("the new table was not reported:%s", messages(ps))
		}
	})
}

// The half that could not be checked until the tool knew what the target looks
// like. Three rules in the first plan named columns that did not exist.
func TestValidateChecksTheTarget(t *testing.T) {
	source := corpusSchema()
	live := target.PostgreSQL()

	doc := Build(source, "server")
	decide(&doc, "users", "seedbonus", Rule{Action: ActionSkip})

	if ps := Validate(doc, source, live); ps.Fatal() {
		t.Errorf("the generated mapping does not fit the target:%s", messages(ps))
	}

	t.Run("a target column that does not exist", func(t *testing.T) {
		broken := Build(source, "server")
		decide(&broken, "users", "seedbonus", Rule{Action: ActionSkip})
		decide(&broken, "users", "passkey", Rule{Action: ActionMap, Target: "no_such_column"})

		ps := Validate(broken, source, live)
		p, ok := findProblem(ps, "users", "passkey")
		if !ok {
			t.Fatalf("the bad target column was not reported:%s", messages(ps))
		}
		if p.Severity != Fatal {
			t.Errorf("severity = %s, want error", p.Severity)
		}
		if !strings.Contains(p.Message, "no_such_column") {
			t.Errorf("the message does not name the column: %q", p.Message)
		}
	})

	t.Run("a target table that does not exist", func(t *testing.T) {
		broken := Build(source, "server")
		decide(&broken, "users", "seedbonus", Rule{Action: ActionSkip})
		for i := range broken.Tables {
			if broken.Tables[i].Name == "users" {
				broken.Tables[i].Target = "no_such_table"
			}
		}

		ps := Validate(broken, source, live)
		if _, ok := findProblem(ps, "users", ""); !ok {
			t.Errorf("the bad target table was not reported:%s", messages(ps))
		}
	})

	// Without a target, the target-side checks are simply not run — they are
	// not silently reported as passing.
	t.Run("no target given", func(t *testing.T) {
		broken := Build(source, "server")
		decide(&broken, "users", "seedbonus", Rule{Action: ActionSkip})
		decide(&broken, "users", "passkey", Rule{Action: ActionMap, Target: "no_such_column"})

		if ps := Validate(broken, source, target.Schema{}); ps.Fatal() {
			t.Errorf("target checks ran without a target:%s", messages(ps))
		}
	})
}

func TestValidateRejectsAnUnknownTransform(t *testing.T) {
	source := corpusSchema()
	doc := Build(source, "server")
	decide(&doc, "users", "seedbonus", Rule{Action: ActionSkip})
	decide(&doc, "users", "passkey", Rule{
		Action:    ActionMap,
		Target:    "passkey",
		Transform: "reticulate_splines",
	})

	ps := Validate(doc, source, target.Schema{})
	p, ok := findProblem(ps, "users", "passkey")
	if !ok {
		t.Fatalf("the unknown transform was not reported:%s", messages(ps))
	}
	if p.Severity != Fatal {
		t.Errorf("severity = %s, want error — a transform nobody wrote cannot run", p.Severity)
	}
}

// A table left as review is a decision nobody made.
func TestValidateRejectsAnUndecidedTable(t *testing.T) {
	source := corpusSchema()
	source.Tables = append(source.Tables, schema.Table{
		Name:    "bonus_log",
		Columns: []schema.Column{{Name: "id", Type: "int"}},
	})

	doc := Build(source, "server")
	decide(&doc, "users", "seedbonus", Rule{Action: ActionSkip})

	ps := Validate(doc, source, target.Schema{})
	p, ok := findProblem(ps, "bonus_log", "")
	if !ok {
		t.Fatalf("the undecided table was not reported:%s", messages(ps))
	}
	if p.Severity != Fatal {
		t.Errorf("severity = %s, want error", p.Severity)
	}

	// Deciding it — either way — clears the problem.
	for i := range doc.Tables {
		if doc.Tables[i].Name == "bonus_log" {
			doc.Tables[i].Action = TableSkip
		}
	}
	if ps := Validate(doc, source, target.Schema{}); ps.Fatal() {
		t.Errorf("skipping the table did not settle it:%s", messages(ps))
	}
}

// Everything wrong is reported at once. Fixing a mapping one error per run,
// each run needing a connection to two databases, is not a reasonable thing to
// ask of somebody mid-cutover.
func TestValidateReportsEverythingAtOnce(t *testing.T) {
	source := corpusSchema()
	doc := Build(source, "server")
	decide(&doc, "users", "passkey", Rule{Action: ActionMap, Target: "no_such_column"})
	decide(&doc, "users", "username", Rule{
		Action: ActionMap, Target: "username", Transform: "no_such_transform",
	})

	ps := Validate(doc, source, target.PostgreSQL())
	if got := ps.Count(Fatal); got < 3 {
		t.Errorf("%d errors reported, want at least 3 (undecided column, bad target, bad transform):%s",
			got, messages(ps))
	}
}

func TestProblemsSortErrorsFirst(t *testing.T) {
	source := corpusSchema()
	doc := Build(source, "server")

	ps := Validate(doc, source, target.Schema{})
	if len(ps) == 0 {
		t.Skip("nothing to sort")
	}
	seenWarning := false
	for _, p := range ps {
		if p.Severity == Warning {
			seenWarning = true
			continue
		}
		if seenWarning {
			t.Errorf("an error is listed after a warning: %s", p)
		}
	}
}
