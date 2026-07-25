package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

func newConnector(t *testing.T, db *sql.DB, kind, name string, enabled bool) *model.NotificationConnector {
	t.Helper()
	c := &model.NotificationConnector{
		Kind:    kind,
		Name:    name,
		Enabled: enabled,
		Config:  json.RawMessage(`{"url":"https://example.test/hook","rate_per_min":5}`),
		Filters: json.RawMessage(`{"category_ids":[1,2],"min_size":1024}`),
	}
	if err := NewConnectorRepo(db).Create(context.Background(), c); err != nil {
		t.Fatalf("creating connector: %v", err)
	}
	return c
}

func TestConnectorRepoRoundTrip(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewConnectorRepo(db)

	created := newConnector(t, db, "webhook", "My Hook", true)
	if created.ID == 0 || created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() {
		t.Fatalf("Create did not populate the generated columns: %+v", created)
	}

	got, err := repo.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Kind != "webhook" || got.Name != "My Hook" || !got.Enabled {
		t.Fatalf("got = %+v", got)
	}

	// JSONB must survive the round trip semantically, not byte-for-byte:
	// Postgres normalises key order and whitespace.
	var config map[string]any
	if err := json.Unmarshal(got.Config, &config); err != nil {
		t.Fatalf("config did not round-trip: %v (%s)", err, got.Config)
	}
	if config["url"] != "https://example.test/hook" || config["rate_per_min"].(float64) != 5 {
		t.Fatalf("config = %v", config)
	}
	var filters map[string]any
	if err := json.Unmarshal(got.Filters, &filters); err != nil {
		t.Fatalf("filters did not round-trip: %v (%s)", err, got.Filters)
	}
	if len(filters["category_ids"].([]any)) != 2 {
		t.Fatalf("filters = %v", filters)
	}
}

func TestConnectorRepoUpdate(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewConnectorRepo(db)

	c := newConnector(t, db, "webhook", "My Hook", true)
	c.Name = "Renamed"
	c.Enabled = false
	c.Config = json.RawMessage(`{"url":"https://other.test/hook"}`)
	if err := repo.Update(ctx, c); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := repo.GetByID(ctx, c.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Name != "Renamed" || got.Enabled {
		t.Fatalf("got = %+v", got)
	}
	// The kind is not in the UPDATE statement at all — changing it would
	// reinterpret the stored config under a different connector.
	if got.Kind != "webhook" {
		t.Fatalf("kind = %q, want it unchanged by Update", got.Kind)
	}
}

func TestConnectorRepoUpdateMissingRow(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)

	err := NewConnectorRepo(db).Update(context.Background(), &model.NotificationConnector{ID: 9999, Name: "x"})
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("Update(missing) = %v, want sql.ErrNoRows", err)
	}
}

func TestConnectorRepoListAndListEnabled(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewConnectorRepo(db)

	newConnector(t, db, "webhook", "Enabled Hook", true)
	newConnector(t, db, "webhook", "Disabled Hook", false)

	all, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("List returned %d rows, want 2", len(all))
	}

	enabled, err := repo.ListEnabled(ctx)
	if err != nil {
		t.Fatalf("ListEnabled: %v", err)
	}
	if len(enabled) != 1 || enabled[0].Name != "Enabled Hook" {
		t.Fatalf("ListEnabled = %+v, want only the enabled row", enabled)
	}
}

func TestConnectorRepoCountByKind(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewConnectorRepo(db)

	newConnector(t, db, "webhook", "One", true)
	newConnector(t, db, "webhook", "Two", true)

	if n, err := repo.CountByKind(ctx, "webhook"); err != nil || n != 2 {
		t.Fatalf("CountByKind(webhook) = %d, %v; want 2, nil", n, err)
	}
	if n, err := repo.CountByKind(ctx, "chat"); err != nil || n != 0 {
		t.Fatalf("CountByKind(chat) = %d, %v; want 0, nil", n, err)
	}
}

// The service returns a 409 first, but the index is what holds when two admins
// (or two nodes) create the second instance at the same moment.
// Migration 074 narrowed the singleton index to sse alone, so this asserts the
// index really was replaced rather than that the code merely stopped asking.
func TestConnectorRepoAllowsSeveralChatRows(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewConnectorRepo(db)

	newConnector(t, db, "chat", "Shoutbox", true)

	if err := repo.Create(ctx, &model.NotificationConnector{
		Kind: "chat", Name: "Shoutbox — Anime", Config: json.RawMessage(`{}`), Filters: json.RawMessage(`{}`),
	}); err != nil {
		t.Fatalf("second chat instance must be allowed: %v", err)
	}

	// The partial index still exists for sse; other kinds may repeat freely.
	if err := repo.Create(ctx, &model.NotificationConnector{
		Kind: "webhook", Name: "Hook A", Config: json.RawMessage(`{}`), Filters: json.RawMessage(`{}`),
	}); err != nil {
		t.Fatalf("first webhook: %v", err)
	}
	if err := repo.Create(ctx, &model.NotificationConnector{
		Kind: "webhook", Name: "Hook B", Config: json.RawMessage(`{}`), Filters: json.RawMessage(`{}`),
	}); err != nil {
		t.Fatalf("second webhook must be allowed: %v", err)
	}
}

func TestConnectorRepoSingletonIndexCoversSSE(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewConnectorRepo(db)

	newConnector(t, db, "sse", "Live Feed", true)

	if err := repo.Create(ctx, &model.NotificationConnector{
		Kind: "sse", Name: "Live Feed 2", Config: json.RawMessage(`{}`), Filters: json.RawMessage(`{}`),
	}); err == nil {
		t.Fatal("expected the singleton index to reject a second sse instance")
	}
}

func TestConnectorRepoDelete(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewConnectorRepo(db)

	c := newConnector(t, db, "webhook", "My Hook", true)

	if err := repo.Delete(ctx, c.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := repo.GetByID(ctx, c.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetByID after delete = %v, want sql.ErrNoRows", err)
	}
	if err := repo.Delete(ctx, c.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("Delete(missing) = %v, want sql.ErrNoRows", err)
	}
}

// An empty config must land as '{}' rather than SQL NULL, which the NOT NULL
// column would reject.
func TestConnectorRepoDefaultsEmptyJSONColumns(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewConnectorRepo(db)

	c := &model.NotificationConnector{Kind: "webhook", Name: "Bare"}
	if err := repo.Create(ctx, c); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := repo.GetByID(ctx, c.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if string(got.Config) != "{}" || string(got.Filters) != "{}" {
		t.Fatalf("config=%s filters=%s, want {} for both", got.Config, got.Filters)
	}
}
