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

// forums.min_group_level and forums.min_post_level are compared directly against
// groups.level, and the two are mapped by different rules that never mention each
// other. That is a silent authorization change waiting to happen: the forum levels
// carry the legacy class numbers, groups.level defaults to the legacy group_id, and
// the groups comment offers "merge into the seeded row" as an equally-blessed
// option — where the seeded groups sit at levels 10-100. Take that option without
// renumbering the forums and a staff forum at minclassread=5 becomes readable by an
// ordinary member at level 20. Nothing about the migrated site looks wrong.
//
// Neither rule can be checked for correctness by a test, because the right answer
// depends on a choice the operator makes. What can be checked is that both rules
// tell them the choice exists and that the two must agree — so the coupling cannot
// be dropped from either side without this failing.
func TestTheForumLevelAndGroupLevelRulesWarnAboutEachOther(t *testing.T) {
	plans := Plan()

	forums, ok := plans["forum_forums"]
	if !ok {
		t.Fatal("no plan for forum_forums")
	}
	groups, ok := plans["groups"]
	if !ok {
		t.Fatal("no plan for groups")
	}

	// Each side has to name the other's column, so a reader of either comment can
	// find the coupling without already knowing about it.
	for _, want := range []string{"groups.level", "minclassread"} {
		if !strings.Contains(forums.Comment, want) {
			t.Errorf("the forums comment does not mention %q, so an operator merging "+
				"groups into the seeded rows has no way to learn that every forum's "+
				"level silently changed meaning", want)
		}
	}
	for _, want := range []string{"min_group_level", "level"} {
		if !strings.Contains(groups.Comment, want) {
			t.Errorf("the groups comment does not mention %q, so the option to merge "+
				"is offered without saying what it invalidates", want)
		}
	}
	// And both must actually say a private forum can open up, since that is the
	// consequence rather than a detail of the mechanism.
	if !strings.Contains(forums.Comment, "20") || !strings.Contains(groups.Comment, "20") {
		t.Error("neither comment gives the concrete outcome (a level-5 staff forum " +
			"readable at level 20); an abstract warning about scales does not land")
	}
}

// The mapping is generated from the operator's database, so a transform has to
// reflect the type the column actually has — not the type stock TorrentTrader has.
//
// info_hash is the case that matters. Several mods changed it to binary(20) to
// halve the index, and those columns already hold the 20 raw bytes. Emitting
// hex_to_bytea for them decodes bytes that are not hex: #158 either aborts or
// writes garbage into a BYTEA NOT NULL UNIQUE column, and every torrent on the
// site stops announcing with no recovery from the target side.
func TestABinaryInfoHashIsNotHexDecoded(t *testing.T) {
	for _, tc := range []struct {
		columnType string
		want       string
	}{
		// Stock: 40 hex characters, so decode them.
		{columnType: "varchar(40)", want: TransformHexToBytea},
		{columnType: "char(40)", want: TransformHexToBytea},
		// Modded: already raw bytes, so carry them across.
		{columnType: "binary(20)", want: TransformTextToBytea},
		{columnType: "BINARY(20)", want: TransformTextToBytea},
		{columnType: "varbinary(20)", want: TransformTextToBytea},
		{columnType: "blob", want: TransformTextToBytea},
		{columnType: "tinyblob", want: TransformTextToBytea},
	} {
		t.Run(tc.columnType, func(t *testing.T) {
			got := reconcileTransformWithType(
				Rule{Action: ActionMap, Target: "info_hash", Transform: TransformHexToBytea},
				tc.columnType)
			if got.Transform != tc.want {
				t.Errorf("a %s column got transform %q, want %q",
					tc.columnType, got.Transform, tc.want)
			}
			// A silent correction is nearly as bad as no correction: the operator
			// has to audit this file, so a departure from the stock plan must say so.
			if tc.want == TransformTextToBytea && !strings.Contains(got.Comment, tc.columnType) {
				t.Errorf("comment = %q, want it to name the actual column type", got.Comment)
			}
		})
	}
}

// Only hex_to_bytea is reconciled. A binary column that was never going to be
// hex-decoded must be left exactly as planned, or this would quietly rewrite
// transforms it knows nothing about.
func TestReconcilingATransformTouchesNothingElse(t *testing.T) {
	for _, transform := range []string{
		TransformTextToBytea, TransformTextToInet, TransformYesNoToBool, "",
	} {
		in := Rule{Action: ActionMap, Target: "x", Transform: transform, Comment: "original"}
		got := reconcileTransformWithType(in, "binary(20)")
		if got != in {
			t.Errorf("transform %q on a binary column changed to %+v, want it untouched",
				transform, got)
		}
	}
}

// The number of skipped baseline columns is cited in compare.go, the README and
// docs/ARCHITECTURE.md as the reason a dropped column does not block a migration.
// It was cited as 36 and is 35 — `grep -c 'skip('` counts the helper's own
// definition. A number in prose that nothing checks drifts, and lessons.md is
// explicit that a cited number becomes the basis of the next design, so it is
// derived from the plan here instead of being trusted.
func TestTheSkippedColumnCountMatchesWhatTheDocsClaim(t *testing.T) {
	const documented = 35 // compare.go, migration-tool/README.md, docs/ARCHITECTURE.md

	skipped := 0
	for _, tp := range Plan() {
		for _, r := range tp.Columns {
			if r.Action == ActionSkip {
				skipped++
			}
		}
	}

	if skipped != documented {
		t.Errorf("the plan skips %d baseline columns but compare.go, the README and "+
			"docs/ARCHITECTURE.md all say %d — update the three citations together",
			skipped, documented)
	}
}
