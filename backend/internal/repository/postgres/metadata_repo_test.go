package postgres

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/metadata"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

// These tests run against a real Postgres (testcontainers) with the real goose
// migrations applied, so they also prove migration 065 applies to a clean DB.

func TestCategoryMetadataSchemaRoundTrip(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	repo := NewCategoryRepo(db)

	cat := &model.Category{
		Name:           uniq("cat"),
		Slug:           uniq("cat"),
		MetadataSchema: json.RawMessage(`[{"key":"year","label":"Year","type":"number","integer":true}]`),
	}
	if err := repo.Create(context.Background(), cat); err != nil {
		t.Fatalf("create category: %v", err)
	}

	got, err := repo.GetByID(context.Background(), cat.ID)
	if err != nil {
		t.Fatalf("get category: %v", err)
	}
	fields, err := metadata.Parse(got.MetadataSchema)
	if err != nil || len(fields) != 1 || fields[0].Key != "year" {
		t.Fatalf("schema not persisted: fields=%v err=%v", fields, err)
	}
}

func TestCategoryDefaultSchemaIsEmptyArray(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	repo := NewCategoryRepo(db)

	// No schema provided — repo coalesces nil to "[]" so the NOT NULL column holds.
	cat := &model.Category{Name: uniq("cat"), Slug: uniq("cat")}
	if err := repo.Create(context.Background(), cat); err != nil {
		t.Fatalf("create category: %v", err)
	}
	got, err := repo.GetByID(context.Background(), cat.ID)
	if err != nil {
		t.Fatalf("get category: %v", err)
	}
	fields, err := metadata.Parse(got.MetadataSchema)
	if err != nil || len(fields) != 0 {
		t.Fatalf("default schema = %s (%v), want empty", got.MetadataSchema, err)
	}
}

func TestTorrentMetadataRoundTrip(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	user := newUser(t, db)
	catID := newCategoryID(t, db)
	repo := NewTorrentRepo(db)

	tor := &model.Torrent{
		Name:       uniq("Release"),
		InfoHash:   infoHash(uniq("hash")),
		Size:       1024,
		CategoryID: catID,
		UploaderID: user.ID,
		Visible:    true,
		FileCount:  1,
		Metadata:   json.RawMessage(`{"year":2024}`),
	}
	if err := repo.Create(context.Background(), tor); err != nil {
		t.Fatalf("create torrent: %v", err)
	}

	got, err := repo.GetByID(context.Background(), tor.ID)
	if err != nil {
		t.Fatalf("get torrent: %v", err)
	}
	var values map[string]any
	if err := json.Unmarshal(got.Metadata, &values); err != nil {
		t.Fatalf("unmarshal metadata: %v (%s)", err, got.Metadata)
	}
	if values["year"].(float64) != 2024 {
		t.Fatalf("metadata year = %v, want 2024", values["year"])
	}
}

func TestTorrentDefaultMetadataIsEmptyObject(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	user := newUser(t, db)
	tor := newTorrent(t, db, user.ID) // no metadata set

	got, err := NewTorrentRepo(db).GetByID(context.Background(), tor.ID)
	if err != nil {
		t.Fatalf("get torrent: %v", err)
	}
	if string(got.Metadata) != "{}" {
		t.Fatalf("default metadata = %s, want {}", got.Metadata)
	}
}
