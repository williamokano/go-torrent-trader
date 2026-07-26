package postgres

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/pressly/goose/v3"
)

// oldChatDefaultTemplate is the line every shoutbox instance created before
// migration 078 stored verbatim. It is written out rather than referenced from
// the connector package because that constant is allowed to change again — this
// test is about the rows already in the database.
const oldChatDefaultTemplate = "New torrent: {{.Name}} [{{.Category}}, {{.SizeHuman}}] {{.URL}}"

const linkedChatDefaultTemplate = "New torrent: {{.Link}} — {{.Category}}, {{.SizeHuman}}"

// Migration 077 has to seed the row, not just define the key: the Site Settings
// admin page renders the rows the API returns, so an unseeded setting is
// invisible and therefore uneditable.
//
// Asserted across a rollback rather than against the already-migrated container,
// which would pass if the row arrived from anywhere at all — including a later
// migration, or a fixture — and would keep passing if 077's INSERT were deleted.
func TestMigration077SeedsTheSystemDisplayName(t *testing.T) {
	db := requireDB(t)

	value := func() (string, error) {
		var v string
		err := db.QueryRow(
			`SELECT value FROM site_settings WHERE key = 'chat_system_display_name'`).Scan(&v)
		return v, err
	}

	if _, err := value(); err != nil {
		t.Fatalf("chat_system_display_name must be seeded so it appears in Site Settings: %v", err)
	}

	migrateDownTo(t, db, 76)
	if _, err := value(); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("row still present at version 76 (err = %v) — 077 is not what seeds it", err)
	}

	if err := goose.Up(db, "."); err != nil {
		t.Fatalf("re-applying migrations: %v", err)
	}
	got, err := value()
	if err != nil {
		t.Fatalf("077 did not seed the row: %v", err)
	}
	if got != "System" {
		t.Fatalf("seeded value = %q, want %q — the seed must not change existing behaviour", got, "System")
	}
}

// migrateDownTo rolls the shared container back and registers the roll-forward,
// so a failed assertion cannot leave every later test short of a migration.
//
// Note this rolls back *everything* above version: when a migration numbered
// higher than the ones under test is added, it is exercised by this cycle too.
func migrateDownTo(t *testing.T, db *sql.DB, version int64) {
	t.Helper()

	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatalf("goose dialect: %v", err)
	}
	if err := goose.DownTo(db, ".", version); err != nil {
		t.Fatalf("rolling back to %d: %v", version, err)
	}
	t.Cleanup(func() {
		if err := goose.Up(db, "."); err != nil {
			t.Fatalf("re-applying migrations: %v", err)
		}
	})
}

// Migration 078 relinks the shoutbox line for instances created before the
// linked default existed. The container is fully migrated before any test runs,
// so the only honest way to exercise the migration is to roll it back over
// seeded rows and forward again.
func TestMigration078RewritesOnlyTheOldDefaultTemplate(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	migrateDownTo(t, db, 77)

	insert := func(name, template string) int64 {
		t.Helper()
		var id int64
		err := db.QueryRowContext(ctx,
			`INSERT INTO notification_connectors (kind, name, config)
			 VALUES ('chat', $1, jsonb_build_object('template', $2::text)) RETURNING id`,
			name, template).Scan(&id)
		if err != nil {
			t.Fatalf("insert %s: %v", name, err)
		}
		return id
	}

	stock := insert("Shoutbox", oldChatDefaultTemplate)
	custom := insert("Shoutbox — Anime", "NEW ANIME: {{.Name}}")

	// A row that never had a template at all: it already tracks whatever the
	// code default is, so pinning one here would take that away.
	var untemplated int64
	if err := db.QueryRowContext(ctx,
		`INSERT INTO notification_connectors (kind, name, config)
		 VALUES ('chat', 'Shoutbox — Bare', '{}'::jsonb) RETURNING id`).Scan(&untemplated); err != nil {
		t.Fatalf("insert bare instance: %v", err)
	}

	// A different kind holding the same string: Markdown is noise on IRC, so the
	// migration must not touch it.
	var irc int64
	if err := db.QueryRowContext(ctx,
		`INSERT INTO notification_connectors (kind, name, config)
		 VALUES ('irc', 'Announce', jsonb_build_object('template', $1::text)) RETURNING id`,
		oldChatDefaultTemplate).Scan(&irc); err != nil {
		t.Fatalf("insert irc instance: %v", err)
	}

	if err := goose.Up(db, "."); err != nil {
		t.Fatalf("re-applying 078: %v", err)
	}

	template := func(id int64) string {
		t.Helper()
		var tmpl sql.NullString
		if err := db.QueryRowContext(ctx,
			`SELECT config->>'template' FROM notification_connectors WHERE id = $1`, id).Scan(&tmpl); err != nil {
			t.Fatalf("read template %d: %v", id, err)
		}
		return tmpl.String
	}

	if got := template(stock); got != linkedChatDefaultTemplate {
		t.Fatalf("stock instance template = %q, want %q", got, linkedChatDefaultTemplate)
	}
	if got := template(custom); got != "NEW ANIME: {{.Name}}" {
		t.Fatalf("custom instance template = %q, want it untouched — that is the admin's wording", got)
	}
	if got := template(untemplated); got != "" {
		t.Fatalf("bare instance template = %q, want it left absent so it keeps tracking the code default", got)
	}
	if got := template(irc); got != oldChatDefaultTemplate {
		t.Fatalf("irc template = %q, want it untouched — Markdown does not render on IRC", got)
	}
}
