package target

import (
	"fmt"
	"slices"
	"sort"
)

// Problem is something wrong with the target database, stated so an operator
// can act on it without reading the source.
type Problem struct {
	Table   string
	Column  string
	Message string
}

func (p Problem) String() string {
	switch {
	case p.Column != "":
		return fmt.Sprintf("%s.%s: %s", p.Table, p.Column, p.Message)
	case p.Table != "":
		return fmt.Sprintf("%s: %s", p.Table, p.Message)
	default:
		return p.Message
	}
}

// Verify checks a live target database against the schema this tool expects to
// write into.
//
// The failure it exists to catch is the ordinary one: pointing the migration at
// a database whose migrations have not been run, or have been run to a
// different version than the tool was built against. Both produce a run that
// dies partway with a message about one column, having already written half the
// members.
//
// Extra tables and columns are not reported. The backend owns its schema and
// may have more than this tool writes to; only what the migration needs matters
// here.
func Verify(live Schema) []Problem {
	declared := PostgreSQL()

	var problems []Problem
	for _, table := range declared.Tables() {
		if !live.HasTable(table) {
			problems = append(problems, Problem{
				Table:   table,
				Message: "missing from the target database",
			})
			continue
		}

		var missing []string
		for _, column := range declared.Columns(table) {
			if !live.Has(table, column) {
				missing = append(missing, column)
			}
		}
		sort.Strings(missing)
		for _, column := range missing {
			problems = append(problems, Problem{
				Table:   table,
				Column:  column,
				Message: "missing from the target database",
			})
		}
	}

	sort.Slice(problems, func(i, j int) bool {
		if problems[i].Table != problems[j].Table {
			return problems[i].Table < problems[j].Table
		}
		return problems[i].Column < problems[j].Column
	})
	return problems
}

// NonEmpty returns the declared tables that already hold rows, given the counts
// read from the target.
//
// The migration writes into a schema it expects to be empty apart from the
// backend's own seed data. A table with rows in it that are not seed data means
// either a half-finished previous run or a live site, and both are worth
// stopping for: re-running over the first duplicates everything, and running
// over the second is unrecoverable.
func NonEmpty(counts map[string]int64) []string {
	var out []string
	for table, n := range counts {
		if n == 0 {
			continue
		}
		if _, seeded := Seeded[table]; seeded {
			continue
		}
		out = append(out, table)
	}
	sort.Strings(out)
	return out
}

// DeclaredTables returns the tables the migration writes to, in the order they
// have to be written: a table appears after everything it refers to.
//
// The order is declared rather than derived from foreign keys, because the
// answer has to be reviewable — a topological sort silently picks one of many
// valid orders, and the one thing that must not happen is torrents landing
// before the members that own them.
func DeclaredTables() []string {
	return []string{
		"groups",
		"users",
		"categories",
		"torrents",
		"peers",
		"transfer_history",
		"torrent_comments",
		"torrent_ratings",
		"messages",
		"saved_messages",
		"forum_categories",
		"forums",
		"forum_topics",
		"forum_posts",
		"chat_messages",
		"news",
		"reports",
		"warnings",
		"mod_notes",
		"banned_ips",
		"banned_emails",
		"invites",
		"countries",
		"languages",
		"notification_preferences",
	}
}

// WriteOrderCoversDeclaration reports any declared table missing from the write
// order, which would otherwise be a table nothing ever writes to.
func WriteOrderCoversDeclaration() []string {
	order := DeclaredTables()
	var missing []string
	for _, table := range PostgreSQL().Tables() {
		if !slices.Contains(order, table) {
			missing = append(missing, table)
		}
	}
	sort.Strings(missing)
	return missing
}
