package database

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// knownSkippedCreates lists migrations whose CREATE TABLE IF NOT EXISTS of an
// existing table is already accounted for. Merged migrations are immutable, so
// these are recorded rather than fixed in place.
var knownSkippedCreates = map[string]bool{
	// forums, forum_topics and forum_posts already existed from 007; the
	// constraints its CREATE bodies described were applied by 097.
	"039_create_forums.sql": true,
}

// A CREATE TABLE IF NOT EXISTS for a table an earlier migration already made
// is skipped silently, along with every column, default and constraint in its
// body. That is how forum topics became impossible to delete: 039's cascade
// lived in such a body, and nothing noticed for 58 migrations. Alter the
// existing table instead.
func TestMigrationsNeverRecreateAnExistingTable(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "..", "migrations", "*.sql"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no migrations found")
	}
	sort.Strings(files)

	create := regexp.MustCompile(`(?i)CREATE\s+TABLE\s+(IF\s+NOT\s+EXISTS\s+)?(\w+)`)
	drop := regexp.MustCompile(`(?i)DROP\s+TABLE\s+(IF\s+EXISTS\s+)?(\w+)`)
	existing := map[string]string{} // table -> migration that created it

	for _, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		up := string(raw)
		if i := strings.Index(up, "-- +goose Down"); i >= 0 {
			up = up[:i]
		}
		name := filepath.Base(path)

		// Statements run in order, so walk drops and creates by position.
		type stmt struct {
			at     int
			table  string
			create bool
			ifNot  bool
		}
		var stmts []stmt
		for _, m := range drop.FindAllStringSubmatchIndex(up, -1) {
			stmts = append(stmts, stmt{at: m[0], table: strings.ToLower(up[m[4]:m[5]])})
		}
		for _, m := range create.FindAllStringSubmatchIndex(up, -1) {
			stmts = append(stmts, stmt{at: m[0], table: strings.ToLower(up[m[4]:m[5]]), create: true, ifNot: m[2] >= 0})
		}
		sort.Slice(stmts, func(i, j int) bool { return stmts[i].at < stmts[j].at })

		for _, s := range stmts {
			if !s.create {
				delete(existing, s.table)
				continue
			}
			if first, ok := existing[s.table]; ok && s.ifNot && !knownSkippedCreates[name] {
				t.Errorf("%s: CREATE TABLE IF NOT EXISTS %s is a no-op — %s already created it; ALTER it instead", name, s.table, first)
			}
			if _, ok := existing[s.table]; !ok {
				existing[s.table] = name
			}
		}
	}
}
