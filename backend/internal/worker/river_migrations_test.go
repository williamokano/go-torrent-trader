package worker

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
)

// River's schema is vendored into goose (migrations/085_river_v002.sql
// onwards), and River does not check the schema version when a client starts.
// So a River upgrade that adds a schema version, without its goose migration,
// fails nothing until a query hits a column that is not there. This compares
// the vendored files against the SQL the pinned River version ships, so the
// upgrade fails here instead.
func TestVendoredRiverMigrationsMatchRiver(t *testing.T) {
	migrator, err := rivermigrate.New(riverpgxv5.New(nil), nil)
	if err != nil {
		t.Fatalf("rivermigrate.New: %v", err)
	}

	files, err := filepath.Glob("../../migrations/*_river_v*.sql")
	if err != nil {
		t.Fatalf("listing vendored migrations: %v", err)
	}
	vendored := map[string]string{}
	for _, f := range files {
		vendored[strings.TrimSuffix(filepath.Base(f)[strings.Index(filepath.Base(f), "_river_v")+len("_river_v"):], ".sql")] = f
	}

	for _, m := range migrator.AllVersions() {
		if m.Version == 1 {
			// River's own tracking table; goose tracks migrations here.
			continue
		}
		key := fmt.Sprintf("%03d", m.Version)
		path, ok := vendored[key]
		if !ok {
			t.Errorf("River schema version %s is not vendored: generate it with "+
				"`river migrate-get --version %d --up/--down` into a new goose migration", key, m.Version)
			continue
		}
		delete(vendored, key)

		raw, err := os.ReadFile(path) //nolint:gosec // fixed test path
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		up, down := gooseSections(string(raw))
		if up != riverSQL(m.SQLUp) {
			t.Errorf("%s: Up differs from River's version %s", filepath.Base(path), key)
		}
		if down != riverSQL(m.SQLDown) {
			t.Errorf("%s: Down differs from River's version %s", filepath.Base(path), key)
		}
	}
	for key, path := range vendored {
		t.Errorf("%s vendors River version %s, which the pinned River does not have", filepath.Base(path), key)
	}
}

// gooseSections returns a vendored file's Up and Down SQL, without the goose
// annotations and the header line `river migrate-get` puts above each version.
func gooseSections(file string) (up, down string) {
	_, rest, _ := strings.Cut(file, "-- +goose Up")
	upPart, downPart, _ := strings.Cut(rest, "-- +goose Down")
	return statementBody(upPart), statementBody(downPart)
}

func statementBody(section string) string {
	_, body, _ := strings.Cut(section, "-- +goose StatementBegin")
	body, _, _ = strings.Cut(body, "-- +goose StatementEnd")
	lines := strings.Split(strings.TrimSpace(body), "\n")
	if len(lines) > 0 && strings.HasPrefix(lines[0], "-- River main migration") {
		lines = lines[1:]
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// riverSQL renders River's migration SQL the way `river migrate-get` does with
// no schema set: the schema placeholder becomes nothing, so tables live on the
// search path like every other table here.
func riverSQL(sql string) string {
	return strings.TrimSpace(strings.ReplaceAll(sql, "/* TEMPLATE: schema */", ""))
}
