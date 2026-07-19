package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/metadata"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

// --- Category schema definition + inheritance ---

func TestCategoryService_CreateWithSchema(t *testing.T) {
	svc := NewCategoryService(newMockCategoryRepo())
	schema := json.RawMessage(`[{"key":"year","label":"Year","type":"number","min":1900,"max":2100,"integer":true}]`)

	cat, err := svc.Create(context.Background(), CreateCategoryRequest{Name: "Movies", MetadataSchema: schema})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	fields, err := metadata.Parse(cat.MetadataSchema)
	if err != nil || len(fields) != 1 || fields[0].Key != "year" {
		t.Fatalf("schema not persisted: fields=%v err=%v", fields, err)
	}
}

func TestCategoryService_CreateEmptySchemaBecomesArray(t *testing.T) {
	svc := NewCategoryService(newMockCategoryRepo())
	cat, err := svc.Create(context.Background(), CreateCategoryRequest{Name: "Movies"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if string(cat.MetadataSchema) != "[]" {
		t.Fatalf("empty schema = %q, want []", cat.MetadataSchema)
	}
}

func TestCategoryService_CreateInvalidSchema(t *testing.T) {
	svc := NewCategoryService(newMockCategoryRepo())
	bad := json.RawMessage(`[{"key":"Year","label":"Year","type":"number"}]`) // uppercase key
	_, err := svc.Create(context.Background(), CreateCategoryRequest{Name: "Movies", MetadataSchema: bad})
	if !errors.Is(err, ErrInvalidCategory) {
		t.Fatalf("expected ErrInvalidCategory, got %v", err)
	}
}

func TestCategoryService_UpdateWithSchema(t *testing.T) {
	svc := NewCategoryService(newMockCategoryRepo())
	cat, _ := svc.Create(context.Background(), CreateCategoryRequest{Name: "Movies"})

	schema := json.RawMessage(`[{"key":"codec","label":"Codec","type":"select","options":["x264","x265"]}]`)
	updated, err := svc.Update(context.Background(), cat.ID, UpdateCategoryRequest{Name: "Movies", MetadataSchema: schema})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	fields, _ := metadata.Parse(updated.MetadataSchema)
	if len(fields) != 1 || fields[0].Key != "codec" {
		t.Fatalf("schema not updated: %v", fields)
	}
}

func TestCategoryService_UpdateInvalidSchema(t *testing.T) {
	svc := NewCategoryService(newMockCategoryRepo())
	cat, _ := svc.Create(context.Background(), CreateCategoryRequest{Name: "Movies"})
	bad := json.RawMessage(`[{"key":"codec","label":"Codec","type":"select"}]`) // select without options
	_, err := svc.Update(context.Background(), cat.ID, UpdateCategoryRequest{Name: "Movies", MetadataSchema: bad})
	if !errors.Is(err, ErrInvalidCategory) {
		t.Fatalf("expected ErrInvalidCategory, got %v", err)
	}
}

func TestCategoryService_ResolveSchemaInheritance(t *testing.T) {
	svc := NewCategoryService(newMockCategoryRepo())
	parent, _ := svc.Create(context.Background(), CreateCategoryRequest{
		Name:           "Video",
		MetadataSchema: json.RawMessage(`[{"key":"codec","label":"Codec","type":"select","options":["x264","x265"]},{"key":"quality","label":"Quality","type":"select","options":["1080p","720p"]}]`),
	})
	child, _ := svc.Create(context.Background(), CreateCategoryRequest{
		Name:           "Movies",
		ParentID:       &parent.ID,
		MetadataSchema: json.RawMessage(`[{"key":"year","label":"Year","type":"number"}]`),
	})

	fields, err := svc.ResolveSchema(context.Background(), child.ID)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	wantKeys := []string{"codec", "quality", "year"}
	if len(fields) != len(wantKeys) {
		t.Fatalf("expected %d fields, got %d", len(wantKeys), len(fields))
	}
	for i, k := range wantKeys {
		if fields[i].Key != k {
			t.Errorf("fields[%d].Key = %q, want %q", i, fields[i].Key, k)
		}
	}
}

func TestCategoryService_ResolveSchemaChildOverride(t *testing.T) {
	svc := NewCategoryService(newMockCategoryRepo())
	parent, _ := svc.Create(context.Background(), CreateCategoryRequest{
		Name:           "Video",
		MetadataSchema: json.RawMessage(`[{"key":"codec","label":"Codec","type":"select","options":["x264"]}]`),
	})
	child, _ := svc.Create(context.Background(), CreateCategoryRequest{
		Name:           "Movies",
		ParentID:       &parent.ID,
		MetadataSchema: json.RawMessage(`[{"key":"codec","label":"Video Codec","type":"select","options":["x264","x265","AV1"]}]`),
	})

	fields, err := svc.ResolveSchema(context.Background(), child.ID)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(fields) != 1 || fields[0].Label != "Video Codec" || len(fields[0].Options) != 3 {
		t.Fatalf("child override failed: %+v", fields)
	}
}

func TestCategoryService_ResolveSchemaNotFound(t *testing.T) {
	svc := NewCategoryService(newMockCategoryRepo())
	_, err := svc.ResolveSchema(context.Background(), 999)
	if !errors.Is(err, ErrCategoryNotFound) {
		t.Fatalf("expected ErrCategoryNotFound, got %v", err)
	}
}

// --- Torrent upload/edit metadata validation ---

func newTorrentSvcWithCategory() (*TorrentService, *memTorrentRepo, *mockCategoryRepo) {
	repo := newMemTorrentRepo()
	svc := NewTorrentService(nil, repo, newMemUserRepo(), newMemStorage(),
		TorrentServiceConfig{AnnounceURL: "http://localhost/announce"}, event.NewInMemoryBus(), nil)
	catRepo := newMockCategoryRepo()
	svc.SetCategoryRepo(catRepo)
	return svc, repo, catRepo
}

func addCategory(catRepo *mockCategoryRepo, name string, parentID *int64, schema string) int64 {
	cat := &model.Category{Name: name, ParentID: parentID, MetadataSchema: json.RawMessage(schema)}
	_ = catRepo.Create(context.Background(), cat)
	return cat.ID
}

func TestTorrentUpload_WithValidMetadata(t *testing.T) {
	svc, _, catRepo := newTorrentSvcWithCategory()
	catID := addCategory(catRepo, "Movies", nil, `[{"key":"year","label":"Year","type":"number","integer":true}]`)

	torrent, err := svc.Upload(context.Background(), buildTorrentFile("meta-ok"),
		UploadTorrentRequest{CategoryID: catID, Metadata: map[string]any{"year": 2024}}, 1)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if string(torrent.Metadata) != `{"year":2024}` {
		t.Fatalf("stored metadata = %s, want {\"year\":2024}", torrent.Metadata)
	}
}

func TestTorrentUpload_NoMetadataStoresEmptyObject(t *testing.T) {
	svc, _, catRepo := newTorrentSvcWithCategory()
	catID := addCategory(catRepo, "Movies", nil, `[{"key":"year","label":"Year","type":"number"}]`)

	torrent, err := svc.Upload(context.Background(), buildTorrentFile("meta-empty"),
		UploadTorrentRequest{CategoryID: catID}, 1)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if string(torrent.Metadata) != "{}" {
		t.Fatalf("metadata = %s, want {}", torrent.Metadata)
	}
}

func TestTorrentUpload_RejectsInvalidMetadata(t *testing.T) {
	tests := []struct {
		name   string
		schema string
		values map[string]any
	}{
		{"unknown field", `[{"key":"year","label":"Year","type":"number"}]`, map[string]any{"bogus": 1}},
		{"required missing", `[{"key":"year","label":"Year","type":"number","required":true}]`, map[string]any{}},
		{"out of range", `[{"key":"year","label":"Year","type":"number","min":1900,"max":2100}]`, map[string]any{"year": 3000}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _, catRepo := newTorrentSvcWithCategory()
			catID := addCategory(catRepo, "Movies", nil, tt.schema)
			_, err := svc.Upload(context.Background(), buildTorrentFile("meta-bad-"+tt.name),
				UploadTorrentRequest{CategoryID: catID, Metadata: tt.values}, 1)
			if !errors.Is(err, metadata.ErrInvalidValues) {
				t.Fatalf("expected ErrInvalidValues, got %v", err)
			}
		})
	}
}

func TestTorrentEdit_UpdatesMetadata(t *testing.T) {
	svc, _, catRepo := newTorrentSvcWithCategory()
	catID := addCategory(catRepo, "Movies", nil, `[{"key":"year","label":"Year","type":"number","integer":true}]`)

	up, err := svc.Upload(context.Background(), buildTorrentFile("edit-meta"),
		UploadTorrentRequest{CategoryID: catID, Metadata: map[string]any{"year": 2024}}, 1)
	if err != nil {
		t.Fatalf("upload err: %v", err)
	}

	newMeta := json.RawMessage(`{"year":2025}`)
	edited, err := svc.EditTorrent(context.Background(), up.ID, 1, model.Permissions{}, EditTorrentRequest{Metadata: &newMeta})
	if err != nil {
		t.Fatalf("edit err: %v", err)
	}
	if string(edited.Metadata) != `{"year":2025}` {
		t.Fatalf("edited metadata = %s, want {\"year\":2025}", edited.Metadata)
	}
}

func TestTorrentEdit_RejectsInvalidMetadata(t *testing.T) {
	svc, _, catRepo := newTorrentSvcWithCategory()
	catID := addCategory(catRepo, "Movies", nil, `[{"key":"year","label":"Year","type":"number","max":2100}]`)
	up, _ := svc.Upload(context.Background(), buildTorrentFile("edit-bad"),
		UploadTorrentRequest{CategoryID: catID}, 1)

	bad := json.RawMessage(`{"year":9999}`)
	_, err := svc.EditTorrent(context.Background(), up.ID, 1, model.Permissions{}, EditTorrentRequest{Metadata: &bad})
	if !errors.Is(err, metadata.ErrInvalidValues) {
		t.Fatalf("expected ErrInvalidValues, got %v", err)
	}
}

func TestTorrentEdit_OmittedMetadataUntouched(t *testing.T) {
	svc, _, catRepo := newTorrentSvcWithCategory()
	catID := addCategory(catRepo, "Movies", nil, `[{"key":"year","label":"Year","type":"number"}]`)
	up, _ := svc.Upload(context.Background(), buildTorrentFile("edit-untouched"),
		UploadTorrentRequest{CategoryID: catID, Metadata: map[string]any{"year": 2024}}, 1)

	newName := "renamed"
	edited, err := svc.EditTorrent(context.Background(), up.ID, 1, model.Permissions{}, EditTorrentRequest{Name: &newName})
	if err != nil {
		t.Fatalf("edit err: %v", err)
	}
	if string(edited.Metadata) != `{"year":2024}` {
		t.Fatalf("metadata changed unexpectedly: %s", edited.Metadata)
	}
}

func TestTorrentEdit_CategoryChangeValidatesAgainstNewCategory(t *testing.T) {
	svc, _, catRepo := newTorrentSvcWithCategory()
	moviesID := addCategory(catRepo, "Movies", nil, `[{"key":"year","label":"Year","type":"number"}]`)
	musicID := addCategory(catRepo, "Music", nil, `[{"key":"artist","label":"Artist","type":"text"}]`)

	up, _ := svc.Upload(context.Background(), buildTorrentFile("cat-change"),
		UploadTorrentRequest{CategoryID: moviesID, Metadata: map[string]any{"year": 2024}}, 1)

	// Move to Music and set Music's field — validated against the new category.
	artistMeta := json.RawMessage(`{"artist":"Radiohead"}`)
	edited, err := svc.EditTorrent(context.Background(), up.ID, 1, model.Permissions{},
		EditTorrentRequest{CategoryID: &musicID, Metadata: &artistMeta})
	if err != nil {
		t.Fatalf("edit err: %v", err)
	}
	if string(edited.Metadata) != `{"artist":"Radiohead"}` {
		t.Fatalf("metadata = %s, want {\"artist\":\"Radiohead\"}", edited.Metadata)
	}

	// A Movies field is now unknown under Music.
	yearMeta := json.RawMessage(`{"year":2024}`)
	_, err = svc.EditTorrent(context.Background(), up.ID, 1, model.Permissions{},
		EditTorrentRequest{CategoryID: &musicID, Metadata: &yearMeta})
	if !errors.Is(err, metadata.ErrInvalidValues) {
		t.Fatalf("expected ErrInvalidValues for stale field, got %v", err)
	}
}
