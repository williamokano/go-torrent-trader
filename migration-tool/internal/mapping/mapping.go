// Package mapping turns a discovered legacy schema into a YAML mapping file the
// operator reviews before any data moves.
//
// The file is generated from the database in front of it, not from the
// baseline: every column the operator actually has gets an entry, so a mod
// nobody anticipated shows up as something to decide rather than vanishing.
package mapping

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/williamokano/go-torrent-trader/migration-tool/internal/baseline"
	"github.com/williamokano/go-torrent-trader/migration-tool/internal/schema"
)

// Version is the mapping file format version. A reader that does not recognise
// the version must refuse the file rather than guess at it.
const Version = 1

// Action is what happens to one legacy column.
type Action string

const (
	// ActionMap copies the column to a target column, through a transform
	// where the types differ.
	ActionMap Action = "map"
	// ActionDerive marks a column whose value reaches the target by some
	// route other than a straight copy — folded into another column, spread
	// across a JSONB document, or turned into rows of another table.
	ActionDerive Action = "derive"
	// ActionSkip marks a column deliberately not migrated. The comment says
	// why; a skip without a reason is a bug in this package.
	ActionSkip Action = "skip"
	// ActionCustom marks a column the baseline does not have — a mod. Nobody
	// but the operator knows what it is for, so it needs a decision.
	ActionCustom Action = "custom"
	// ActionReview marks a column that belongs to a stock table this tool has
	// no rule for yet. Also a decision, but not a mod.
	ActionReview Action = "review"
)

// NeedsDecision reports whether the action leaves work for the operator.
func (a Action) NeedsDecision() bool {
	return a == ActionCustom || a == ActionReview
}

// TableAction is what happens to one legacy table.
type TableAction string

const (
	// TableMigrate marks a table with a target.
	TableMigrate TableAction = "migrate"
	// TableSkip marks a table deliberately not migrated.
	TableSkip TableAction = "skip"
	// TableReview marks a table with no decision yet — unknown to the
	// baseline, or known but with nowhere to go.
	TableReview TableAction = "review"
)

// Rule is the plan for one legacy column.
type Rule struct {
	Action    Action
	Target    string
	Transform string
	Comment   string
}

// TablePlan is the plan for one legacy table.
type TablePlan struct {
	Action  TableAction
	Target  string
	Comment string
	Columns map[string]Rule
	// Derived names target columns filled from something other than a single
	// legacy column, and says where each value comes from. It documents the
	// target side, which the column rules alone leave implicit.
	Derived map[string]string
}

// Column is a rendered column entry.
type Column struct {
	Name string
	Rule Rule
}

// Table is a rendered table entry: a plan resolved against the columns the
// source database actually has.
type Table struct {
	Name    string
	Action  TableAction
	Target  string
	Comment string
	Columns []Column
	Derived map[string]string
	// Absent lists columns the plan expects that this database does not have.
	Absent []string
}

// Document is a whole mapping file.
type Document struct {
	Version  int
	Database string
	Server   string
	Tables   []Table
}

// Decisions returns the columns still needing an operator's decision, as
// "table.column" strings in sorted order.
func (d Document) Decisions() []string {
	var out []string
	for _, t := range d.Tables {
		for _, c := range t.Columns {
			if c.Rule.Action.NeedsDecision() {
				out = append(out, t.Name+"."+c.Name)
			}
		}
	}
	sort.Strings(out)
	return out
}

// AbsentColumns returns the planned columns missing from this database, as
// "table.column" strings in sorted order. Each one is a transform with nothing
// to read.
func (d Document) AbsentColumns() []string {
	var out []string
	for _, t := range d.Tables {
		for _, c := range t.Absent {
			out = append(out, t.Name+"."+c)
		}
	}
	sort.Strings(out)
	return out
}

// Build resolves the plan against a discovered schema. Every table and every
// column found in the source appears in the result; nothing is dropped quietly.
func Build(found schema.Schema, server string) Document {
	base := baseline.TorrentTrader30()
	plan := Plan()

	doc := Document{
		Version:  Version,
		Database: found.Database,
		Server:   server,
		Tables:   make([]Table, 0, len(found.Tables)),
	}

	for _, ft := range found.Tables {
		tp, planned := lookupPlan(plan, ft.Name)
		_, known := base.Table(ft.Name)

		t := Table{Name: ft.Name}
		switch {
		case planned:
			t.Action = tp.Action
			t.Target = tp.Target
			t.Comment = tp.Comment
			t.Derived = tp.Derived
		case known:
			t.Action = TableReview
			t.Comment = "A stock TorrentTrader table with no mapping rule yet. Decide where it goes, or set action to skip."
		default:
			t.Action = TableReview
			t.Comment = "Not part of TorrentTrader 3.0 — a mod, or another application sharing this database. Decide where it goes, or set action to skip."
		}

		for _, fc := range ft.Columns {
			t.Columns = append(t.Columns, Column{
				Name: fc.Name,
				Rule: resolveRule(tp, planned, base, ft.Name, fc.Name),
			})
		}
		t.Absent = absentColumns(tp, planned, ft)
		t.Comment = noteUnmapped(t)

		doc.Tables = append(doc.Tables, t)
	}

	sort.Slice(doc.Tables, func(i, j int) bool { return doc.Tables[i].Name < doc.Tables[j].Name })
	return doc
}

// lookupPlan finds a plan for a table, matching case-insensitively because
// MySQL table-name case sensitivity depends on the host filesystem.
func lookupPlan(plan map[string]TablePlan, table string) (TablePlan, bool) {
	if tp, ok := plan[table]; ok {
		return tp, true
	}
	for name, tp := range plan {
		if strings.EqualFold(name, table) {
			return tp, true
		}
	}
	return TablePlan{}, false
}

func lookupRule(tp TablePlan, column string) (Rule, bool) {
	if r, ok := tp.Columns[column]; ok {
		return r, true
	}
	for name, r := range tp.Columns {
		if strings.EqualFold(name, column) {
			return r, true
		}
	}
	return Rule{}, false
}

func resolveRule(tp TablePlan, planned bool, base baseline.Schema, table, column string) Rule {
	if planned {
		if r, ok := lookupRule(tp, column); ok {
			return r
		}
		if tp.Action == TableSkip {
			return Rule{
				Action:  ActionSkip,
				Comment: "The whole table is skipped.",
			}
		}
	}

	// No rule. Whether that is a mod or a gap in this tool's knowledge changes
	// what the operator should do about it, so the two get different actions.
	// Neither carries a comment: the header explains both, and repeating the
	// same sentence above every entry buries the comments that say something.
	if bt, ok := base.Table(table); ok {
		if _, documented := bt.Column(column); documented || !bt.ColumnsDocumented {
			return Rule{Action: ActionReview}
		}
	}
	return Rule{Action: ActionCustom}
}

// noteUnmapped adds a line to a table's comment when the table has a target but
// none of its columns do. Without it the reader sees a wall of bare `review`
// entries with nothing saying they are all still to be decided.
func noteUnmapped(t Table) string {
	if t.Action != TableMigrate || len(t.Columns) == 0 {
		return t.Comment
	}
	for _, c := range t.Columns {
		if !c.Rule.Action.NeedsDecision() {
			return t.Comment
		}
	}

	note := "No column rules yet — give every column below a target, or skip it."
	if t.Comment == "" {
		return note
	}
	return t.Comment + "\n" + note
}

func absentColumns(tp TablePlan, planned bool, ft schema.Table) []string {
	if !planned {
		return nil
	}
	var absent []string
	for name, r := range tp.Columns {
		if r.Action == ActionSkip {
			continue
		}
		if _, ok := ft.Column(name); !ok {
			absent = append(absent, name)
		}
	}
	sort.Strings(absent)
	return absent
}

// Render writes the document as YAML with the reasoning kept as comments, so
// the file explains itself to whoever reviews it.
func Render(doc Document) ([]byte, error) {
	root := newMapping()
	addField(root, "version", intScalar(doc.Version), "")

	source := newMapping()
	addString(source, "database", doc.Database, "")
	addString(source, "server", doc.Server, "")
	addField(root, "source", source, "Where this file was generated from.")

	tables := newMapping()
	for _, t := range doc.Tables {
		tables.Content = append(tables.Content, tableNodes(t)...)
	}
	addField(root, "tables", tables, "One entry per table found in the source database.")

	root.HeadComment = header(doc)

	out, err := yaml.Marshal(root)
	if err != nil {
		return nil, fmt.Errorf("rendering mapping: %w", err)
	}
	return out, nil
}

func header(doc Document) string {
	return strings.Join([]string{
		"TorrentTrader -> go-torrent-trader column mapping.",
		"",
		"Generated by `migrate mapping` from " + doc.Server + ".",
		"This is a proposal. Read it, edit it, keep it in version control, and pass",
		"it back with --mapping when you run the migration.",
		"",
		"action:",
		"  map     copy this column to the target column, through `transform` if set",
		"  derive  the value reaches the target another way; `target` says where",
		"  skip    deliberately not migrated; the comment says why",
		"  custom  a mod added this column and only you know what it is for",
		"  review  a stock column this tool has no rule for yet",
		"",
		"Every `custom` and `review` entry needs a decision before the run.",
	}, "\n")
}

func tableNodes(t Table) []*yaml.Node {
	body := newMapping()
	addString(body, "action", string(t.Action), "")
	if t.Target != "" {
		addString(body, "target", t.Target, "")
	}

	if len(t.Columns) > 0 {
		columns := newMapping()
		for _, c := range t.Columns {
			columns.Content = append(columns.Content, columnNodes(c)...)
		}
		addField(body, "columns", columns, "")
	}

	if len(t.Derived) > 0 {
		derived := newMapping()
		for _, name := range sortedKeys(t.Derived) {
			addString(derived, name, t.Derived[name], "")
		}
		addField(body, "derived", derived,
			"Target columns filled from something other than a single legacy column.")
	}

	if len(t.Absent) > 0 {
		absent := &yaml.Node{Kind: yaml.SequenceNode, Style: yaml.FlowStyle}
		for _, name := range t.Absent {
			absent.Content = append(absent.Content, scalar(name))
		}
		addField(body, "absent_columns", absent,
			"Columns this mapping expects that your database does not have. Each one is a rule with nothing to read.")
	}

	key := scalar(t.Name)
	key.HeadComment = t.Comment
	return []*yaml.Node{key, body}
}

func columnNodes(c Column) []*yaml.Node {
	body := newMapping()
	addString(body, "action", string(c.Rule.Action), "")
	if c.Rule.Target != "" {
		addString(body, "target", c.Rule.Target, "")
	}
	if c.Rule.Transform != "" {
		addString(body, "transform", c.Rule.Transform, "")
	}

	key := scalar(c.Name)
	key.HeadComment = c.Rule.Comment
	return []*yaml.Node{key, body}
}

func newMapping() *yaml.Node {
	return &yaml.Node{Kind: yaml.MappingNode}
}

// scalar builds a scalar node, letting the encoder choose a quoting style that
// keeps the value readable and unambiguous.
func scalar(v string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Value: v}
}

// intScalar builds a scalar the encoder will write unquoted as a number.
func intScalar(n int) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: fmt.Sprintf("%d", n)}
}

func addString(m *yaml.Node, key, value, headComment string) {
	addField(m, key, scalar(value), headComment)
}

// addField appends a key/value pair to a mapping node, hanging any comment off
// the key so it lands above the entry.
func addField(m *yaml.Node, key string, value *yaml.Node, headComment string) {
	k := scalar(key)
	k.HeadComment = headComment
	m.Content = append(m.Content, k, value)
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
