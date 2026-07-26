package mapping

import (
	"sort"
	"strings"
	"testing"

	"github.com/williamokano/go-torrent-trader/migration-tool/internal/target"
)

// The source side of the plan is checked against internal/baseline. This file
// checks the other side, which the first version of this package got wrong
// three times: a rule pointing at a target column that does not exist writes
// nowhere, and a column the target has but the plan never mentions is a
// silently dropped feature.

// bareIdentifier reports whether s is a single unqualified column name. Rules
// that copy a value have to name one, or #158 cannot resolve them without
// parsing prose.
func bareIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r != '_' && (r < 'a' || r > 'z') && (r < '0' || r > '9') {
			return false
		}
	}
	return true
}

func TestMapRulesNameAColumnOfTheTargetTable(t *testing.T) {
	schema := target.PostgreSQL()

	for table, tp := range Plan() {
		if tp.Action != TableMigrate {
			continue
		}
		// A target of "torrents.files" means the rows fold into a column of
		// another table; those are checked below, not here.
		if strings.Contains(tp.Target, ".") {
			continue
		}
		if !schema.HasTable(tp.Target) {
			t.Errorf("%s targets table %q, which the target schema does not have", table, tp.Target)
			continue
		}

		for column, rule := range tp.Columns {
			if rule.Action != ActionMap {
				continue
			}
			where := table + "." + column
			if !bareIdentifier(rule.Target) {
				t.Errorf("%s: map target %q is not a plain column name", where, rule.Target)
				continue
			}
			if !schema.Has(tp.Target, rule.Target) {
				t.Errorf("%s maps to %s.%s, which does not exist", where, tp.Target, rule.Target)
			}
		}
	}
}

// Every column of a target table must be accounted for: filled by a rule, or
// named in Derived with where its value comes from. Anything else is a column
// nobody decided about, which is how forums.min_post_level came to be dropped
// under a comment claiming the target had no such thing.
func TestEveryTargetColumnIsAccountedFor(t *testing.T) {
	schema := target.PostgreSQL()

	for table, tp := range Plan() {
		if tp.Action != TableMigrate || strings.Contains(tp.Target, ".") {
			continue
		}
		// Only tables whose legacy columns are fully ruled can claim to
		// account for the target side.
		if len(tp.Columns) == 0 {
			continue
		}

		accounted := map[string]bool{}
		for _, rule := range tp.Columns {
			switch rule.Action {
			case ActionMap:
				accounted[rule.Target] = true
			case ActionDerive:
				// A derive may name a column of this table, a column of
				// another one, or several at once. Only the first form
				// accounts for anything here.
				for _, part := range strings.Split(rule.Target, ",") {
					accounted[strings.TrimSpace(part)] = true
				}
			}
		}
		for column := range tp.Derived {
			accounted[column] = true
		}

		var unaccounted []string
		for _, column := range schema.Columns(tp.Target) {
			if !accounted[column] {
				unaccounted = append(unaccounted, column)
			}
		}
		sort.Strings(unaccounted)
		if len(unaccounted) > 0 {
			t.Errorf("%s -> %s: no rule and no derived note for %v", table, tp.Target, unaccounted)
		}
	}
}

// A derived note naming a column of the target table must name a real one.
// Notes that describe a different table are prose and are skipped.
func TestDerivedNotesNameRealColumns(t *testing.T) {
	schema := target.PostgreSQL()

	for table, tp := range Plan() {
		if tp.Action != TableMigrate || strings.Contains(tp.Target, ".") {
			continue
		}
		for column := range tp.Derived {
			if !bareIdentifier(column) {
				t.Errorf("%s: derived key %q is not a plain column name", table, column)
				continue
			}
			if !schema.Has(tp.Target, column) {
				t.Errorf("%s: derived note for %s.%s, which does not exist", table, tp.Target, column)
			}
		}
	}
}

// Tables the target ships with rows in cannot take legacy ids without
// colliding. The plan has to say so where it applies, because "keep the legacy
// id" is the default everywhere else.
func TestSeededTargetsWarnAboutCollisions(t *testing.T) {
	plan := Plan()

	for legacy, tp := range plan {
		if tp.Action != TableMigrate {
			continue
		}
		note, seeded := target.Seeded[tp.Target]
		if !seeded {
			continue
		}
		if !strings.Contains(strings.ToLower(tp.Comment), "seed") {
			t.Errorf("%s targets %s, which ships seeded (%s), but its comment does not warn about it: %q",
				legacy, tp.Target, note, tp.Comment)
		}
	}
}
