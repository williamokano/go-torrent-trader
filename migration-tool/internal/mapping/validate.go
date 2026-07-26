package mapping

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/williamokano/go-torrent-trader/migration-tool/internal/schema"
	"github.com/williamokano/go-torrent-trader/migration-tool/internal/target"
)

// Severity says whether a problem stops a run.
type Severity int

const (
	// Fatal means the migration cannot run: something it would read or write
	// is not there, or a decision has not been made.
	Fatal Severity = iota
	// Warning means the run can proceed but the operator should know.
	Warning
)

func (s Severity) String() string {
	if s == Fatal {
		return "error"
	}
	return "warning"
}

// Problem is one thing wrong with a mapping, phrased for an operator who is
// about to run a migration and has not read this code.
type Problem struct {
	Severity Severity
	Table    string
	Column   string
	Message  string
}

func (p Problem) String() string {
	where := p.Table
	if p.Column != "" {
		where += "." + p.Column
	}
	if where == "" {
		return p.Message
	}
	return where + ": " + p.Message
}

// Problems is a set of findings.
type Problems []Problem

// Fatal reports whether anything here stops the migration.
func (ps Problems) Fatal() bool {
	for _, p := range ps {
		if p.Severity == Fatal {
			return true
		}
	}
	return false
}

// Count returns how many problems have the given severity.
func (ps Problems) Count(s Severity) int {
	n := 0
	for _, p := range ps {
		if p.Severity == s {
			n++
		}
	}
	return n
}

// Validate checks a mapping against the database it will read and the database
// it will write, and reports everything wrong with it at once.
//
// The point is to fail in seconds rather than halfway through a million rows. A
// migration that dies partway leaves an operator with a half-populated target
// and a decision to make at the worst possible moment, so every check that can
// be made before the first write belongs here.
//
// Passing a zero target.Schema skips the target-side checks, which is what
// `validate` does when it is given no --target.
func Validate(doc Document, source schema.Schema, tgt target.Schema) Problems {
	var ps Problems

	ps = append(ps, checkAgainstSource(doc, source)...)
	ps = append(ps, checkDecisions(doc)...)
	ps = append(ps, checkTransforms(doc)...)
	if len(tgt.Tables()) > 0 {
		ps = append(ps, checkAgainstTarget(doc, tgt)...)
	}

	sort.SliceStable(ps, func(i, j int) bool {
		if ps[i].Severity != ps[j].Severity {
			return ps[i].Severity < ps[j].Severity
		}
		if ps[i].Table != ps[j].Table {
			return ps[i].Table < ps[j].Table
		}
		return ps[i].Column < ps[j].Column
	})
	return ps
}

// checkAgainstSource catches a mapping that has drifted from the database it
// describes — generated before a mod was added, or against a different install
// altogether.
func checkAgainstSource(doc Document, source schema.Schema) Problems {
	var ps Problems
	if len(source.Tables) == 0 {
		return ps
	}

	for _, t := range doc.Tables {
		st, ok := source.Table(t.Name)
		if !ok {
			ps = append(ps, Problem{
				Severity: Warning,
				Table:    t.Name,
				Message:  "in the mapping but not in the source database; it will be ignored",
			})
			continue
		}
		if t.Action == TableSkip {
			continue
		}
		for _, c := range t.Columns {
			if c.Rule.Action == ActionSkip {
				continue
			}
			if _, ok := st.Column(c.Name); !ok {
				ps = append(ps, Problem{
					Severity: Fatal,
					Table:    t.Name,
					Column:   c.Name,
					Message:  "the mapping reads this column and the source database does not have it",
				})
			}
		}
	}

	// The other direction: something in the database that the mapping has
	// never heard of. Migrating it is not possible, but neither is deciding
	// silently that it does not matter.
	for _, st := range source.Tables {
		var dt *Table
		for i := range doc.Tables {
			if strings.EqualFold(doc.Tables[i].Name, st.Name) {
				dt = &doc.Tables[i]
				break
			}
		}
		if dt == nil {
			ps = append(ps, Problem{
				Severity: Fatal,
				Table:    st.Name,
				Message:  "in the source database but not in the mapping — regenerate it with `migrate mapping`",
			})
			continue
		}
		if dt.Action == TableSkip {
			continue
		}
		for _, sc := range st.Columns {
			if !hasColumn(dt.Columns, sc.Name) {
				ps = append(ps, Problem{
					Severity: Fatal,
					Table:    st.Name,
					Column:   sc.Name,
					Message:  "in the source database but not in the mapping — regenerate it with `migrate mapping`",
				})
			}
		}
	}
	return ps
}

// checkDecisions catches the entries the generated file asks a human to settle.
// Leaving one is not an oversight the migration can paper over: nobody but the
// operator knows what a mod's column is for.
func checkDecisions(doc Document) Problems {
	var ps Problems
	for _, t := range doc.Tables {
		if t.Action == TableSkip {
			continue
		}
		if t.Action == TableReview {
			ps = append(ps, Problem{
				Severity: Fatal,
				Table:    t.Name,
				Message:  "still marked review — give it a target and set action to migrate, or set action to skip",
			})
			continue
		}
		for _, c := range t.Columns {
			if !c.Rule.Action.NeedsDecision() {
				continue
			}
			ps = append(ps, Problem{
				Severity: Fatal,
				Table:    t.Name,
				Column:   c.Name,
				Message: fmt.Sprintf("still marked %s — give it a target, or set action to skip",
					c.Rule.Action),
			})
		}
	}
	return ps
}

func checkTransforms(doc Document) Problems {
	known := KnownTransforms()

	var ps Problems
	for _, t := range doc.Tables {
		if t.Action == TableSkip {
			continue
		}
		for _, c := range t.Columns {
			if c.Rule.Transform == "" {
				continue
			}
			if c.Rule.Action != ActionMap {
				ps = append(ps, Problem{
					Severity: Warning,
					Table:    t.Name,
					Column:   c.Name,
					Message: fmt.Sprintf("transform %q on a %s rule, which never runs it",
						c.Rule.Transform, c.Rule.Action),
				})
			}
			if !slices.Contains(known, c.Rule.Transform) {
				ps = append(ps, Problem{
					Severity: Fatal,
					Table:    t.Name,
					Column:   c.Name,
					Message:  fmt.Sprintf("unknown transform %q", c.Rule.Transform),
				})
			}
		}
	}
	return ps
}

// checkAgainstTarget is the half that could not be checked before this tool
// knew what the target actually looks like. Three rules in the first version of
// the plan named target columns that did not exist.
func checkAgainstTarget(doc Document, tgt target.Schema) Problems {
	var ps Problems
	for _, t := range doc.Tables {
		if t.Action != TableMigrate {
			continue
		}

		// A target of "torrents.files" means the rows fold into a column of
		// another table rather than becoming rows of their own.
		table, _, qualified := strings.Cut(t.Target, ".")
		if !tgt.HasTable(table) {
			ps = append(ps, Problem{
				Severity: Fatal,
				Table:    t.Name,
				Message:  fmt.Sprintf("targets table %q, which the target database does not have", t.Target),
			})
			continue
		}
		if qualified {
			continue
		}

		for _, c := range t.Columns {
			if c.Rule.Action != ActionMap {
				continue
			}
			if c.Rule.Target == "" {
				ps = append(ps, Problem{
					Severity: Fatal,
					Table:    t.Name,
					Column:   c.Name,
					Message:  "mapped, but names no target column",
				})
				continue
			}
			if !tgt.Has(table, c.Rule.Target) {
				ps = append(ps, Problem{
					Severity: Fatal,
					Table:    t.Name,
					Column:   c.Name,
					Message: fmt.Sprintf("maps to %s.%s, which the target database does not have",
						table, c.Rule.Target),
				})
			}
		}
	}
	return ps
}

func hasColumn(columns []Column, name string) bool {
	for _, c := range columns {
		if strings.EqualFold(c.Name, name) {
			return true
		}
	}
	return false
}
