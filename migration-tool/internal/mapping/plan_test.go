package mapping

import (
	"slices"
	"strings"
	"testing"

	"github.com/williamokano/go-torrent-trader/migration-tool/internal/baseline"
)

// knownTransforms is every transform a rule is allowed to name. A rule naming
// anything else is a typo that would reach an operator as a mapping nobody can
// execute.
func knownTransforms() []string {
	return []string{
		TransformYesNoToBool,
		TransformYesNoToBoolInverted,
		TransformZeroOneToBool,
		TransformPositiveToBool,
		TransformHexToBytea,
		TransformTextToBytea,
		TransformTextToInet,
		TransformLegacyHash,
		TransformBBCodeToMarkdown,
		TransformClassToGroupID,
		TransformSlugify,
	}
}

// Every documented baseline column needs a rule. Without this, adding a column
// to the baseline silently produces a mapping that tells the operator to review
// something the tool already knows the answer to.
func TestPlanCoversEveryDocumentedColumn(t *testing.T) {
	plan := Plan()

	for _, bt := range baseline.TorrentTrader30().Tables {
		if !bt.ColumnsDocumented {
			continue
		}
		tp, ok := plan[bt.Name]
		if !ok {
			t.Errorf("no plan for documented table %s", bt.Name)
			continue
		}
		for _, bc := range bt.Columns {
			if _, ok := tp.Columns[bc.Name]; !ok {
				t.Errorf("no rule for %s.%s", bt.Name, bc.Name)
			}
		}
	}
}

// The mirror of the test above: a rule for a column that does not exist is a
// typo, and a typo here means a column the operator thinks is handled is not.
func TestPlanRulesNameRealColumns(t *testing.T) {
	base := baseline.TorrentTrader30()

	for table, tp := range Plan() {
		bt, ok := base.Table(table)
		if !ok || !bt.ColumnsDocumented {
			continue
		}
		for column := range tp.Columns {
			if _, ok := bt.Column(column); !ok {
				t.Errorf("rule for %s.%s, which is not in the baseline", table, column)
			}
		}
	}
}

func TestEveryRuleIsWellFormed(t *testing.T) {
	transforms := knownTransforms()

	for table, tp := range Plan() {
		for column, rule := range tp.Columns {
			where := table + "." + column

			switch rule.Action {
			case ActionMap, ActionDerive:
				if rule.Target == "" {
					t.Errorf("%s: action %q with no target", where, rule.Action)
				}
			case ActionSkip:
				if rule.Target != "" {
					t.Errorf("%s: skipped but names a target %q", where, rule.Target)
				}
				// A skip is a decision to drop data. It has to say why.
				if rule.Comment == "" {
					t.Errorf("%s: skipped with no reason given", where)
				}
			default:
				t.Errorf("%s: action %q must not appear in the plan; it is assigned at build time", where, rule.Action)
			}

			if rule.Transform != "" {
				if rule.Action != ActionMap {
					t.Errorf("%s: transform %q on a %q rule, which never runs it", where, rule.Transform, rule.Action)
				}
				if !slices.Contains(transforms, rule.Transform) {
					t.Errorf("%s: unknown transform %q", where, rule.Transform)
				}
			}
		}
	}
}

func TestEveryTablePlanIsWellFormed(t *testing.T) {
	for table, tp := range Plan() {
		switch tp.Action {
		case TableMigrate:
			if tp.Target == "" {
				t.Errorf("%s: migrated but names no target table", table)
			}
		case TableSkip:
			if tp.Target != "" {
				t.Errorf("%s: skipped but names a target %q", table, tp.Target)
			}
			if tp.Comment == "" {
				t.Errorf("%s: the whole table is skipped with no reason given", table)
			}
			if len(tp.Columns) > 0 {
				t.Errorf("%s: skipped tables must not carry column rules", table)
			}
		case TableReview:
			if tp.Comment == "" {
				t.Errorf("%s: left for review with no explanation", table)
			}
		default:
			t.Errorf("%s: unknown table action %q", table, tp.Action)
		}
	}
}

// The tables carrying the data a tracker cannot lose. If one of these ever
// stops being migrated it should take a deliberate edit and a failing test.
func TestCoreTablesAreMigrated(t *testing.T) {
	want := map[string]string{
		"users":        "users",
		"groups":       "groups",
		"torrents":     "torrents",
		"peers":        "peers",
		"completed":    "transfer_history",
		"categories":   "categories",
		"messages":     "messages",
		"forum_forums": "forums",
		"forum_topics": "forum_topics",
		"forum_posts":  "forum_posts",
	}

	plan := Plan()
	for table, target := range want {
		tp, ok := plan[table]
		if !ok {
			t.Errorf("no plan for %s", table)
			continue
		}
		if tp.Action != TableMigrate {
			t.Errorf("%s has action %q, want migrate", table, tp.Action)
		}
		if tp.Target != target {
			t.Errorf("%s targets %q, want %q", table, tp.Target, target)
		}
	}
}

// The columns that make the difference between a cutover nobody notices and one
// that breaks every client. Asserted individually because each is a specific
// promise made in the epic.
func TestCutoverCriticalColumns(t *testing.T) {
	plan := Plan()

	tests := []struct {
		table, column string
		wantTarget    string
		wantTransform string
		why           string
	}{
		{"users", "passkey", "passkey", "", "already-downloaded .torrent files announce with this key"},
		{"users", "id", "id", "", "foreign keys across the dump resolve against it"},
		{"users", "password", "password_hash", TransformLegacyHash, "nobody should be asked to reset a password"},
		{"torrents", "info_hash", "info_hash", TransformHexToBytea, "40 hex characters must become the 20 bytes the tracker compares"},
		{"torrents", "id", "id", "", "peers and completions refer to it"},
	}

	for _, tc := range tests {
		t.Run(tc.table+"."+tc.column, func(t *testing.T) {
			rule, ok := plan[tc.table].Columns[tc.column]
			if !ok {
				t.Fatalf("no rule for %s.%s — %s", tc.table, tc.column, tc.why)
			}
			if rule.Action != ActionMap {
				t.Errorf("action = %q, want map — %s", rule.Action, tc.why)
			}
			if rule.Target != tc.wantTarget {
				t.Errorf("target = %q, want %q", rule.Target, tc.wantTarget)
			}
			if rule.Transform != tc.wantTransform {
				t.Errorf("transform = %q, want %q", rule.Transform, tc.wantTransform)
			}
		})
	}
}

// Two columns whose sense is reversed in the target. Getting either backwards
// inverts a permission or a read flag for the whole member table, which is the
// kind of thing that looks fine until someone complains.
func TestInvertedColumnsAreMarkedInverted(t *testing.T) {
	plan := Plan()

	inverted := []struct{ table, column, target string }{
		{"users", "forumbanned", "can_forum"},
		{"messages", "unread", "is_read"},
	}

	for _, tc := range inverted {
		rule := plan[tc.table].Columns[tc.column]
		if rule.Target != tc.target {
			t.Errorf("%s.%s targets %q, want %q", tc.table, tc.column, rule.Target, tc.target)
		}
		if rule.Transform != TransformYesNoToBoolInverted {
			t.Errorf("%s.%s uses transform %q, want %q — its sense is reversed in the target",
				tc.table, tc.column, rule.Transform, TransformYesNoToBoolInverted)
		}
	}
}

func TestDerivedNotesExplainThemselves(t *testing.T) {
	for table, tp := range Plan() {
		for column, note := range tp.Derived {
			if strings.TrimSpace(note) == "" {
				t.Errorf("%s: derived column %s has no explanation", table, column)
			}
		}
	}
}
