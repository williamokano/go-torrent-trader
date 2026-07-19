package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

// The torrent detail response should carry both the stored metadata values and
// the effective (inherited) schema, so the detail page renders labels without a
// second round trip.
func TestTorrentDetailIncludesMetadataAndSchema(t *testing.T) {
	d := newTorrentDeps()
	d.cats.byID[1] = &model.Category{
		ID:             1,
		Name:           "Movies",
		MetadataSchema: json.RawMessage(`[{"key":"year","label":"Year","type":"number"}]`),
	}
	d.torrents.torrents[3] = &model.Torrent{
		ID:         3,
		Name:       "A movie",
		CategoryID: 1,
		InfoHash:   []byte("0123456789abcdef0123"),
		Metadata:   json.RawMessage(`{"year":2024}`),
	}
	h := d.handler()

	req := withURLParam(httptest.NewRequest(http.MethodGet, "/api/v1/torrents/3", nil), "id", "3")
	w := httptest.NewRecorder()
	h.HandleGetByID(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	torrent := decodeBody(t, w)["torrent"].(map[string]interface{})

	meta, ok := torrent["metadata"].(map[string]interface{})
	if !ok || meta["year"].(float64) != 2024 {
		t.Fatalf("metadata = %v, want {year:2024}", torrent["metadata"])
	}

	schema, ok := torrent["metadata_schema"].([]interface{})
	if !ok || len(schema) != 1 {
		t.Fatalf("metadata_schema = %v, want one field", torrent["metadata_schema"])
	}
	if schema[0].(map[string]interface{})["key"] != "year" {
		t.Fatalf("schema field key = %v, want year", schema[0])
	}
}

// A torrent with no metadata still reports an empty object rather than null.
func TestTorrentDetailEmptyMetadataIsObject(t *testing.T) {
	d := newTorrentDeps()
	d.cats.byID[1] = &model.Category{ID: 1, Name: "Movies"}
	d.torrents.torrents[3] = &model.Torrent{ID: 3, Name: "A movie", CategoryID: 1, InfoHash: []byte("0123456789abcdef0123")}
	h := d.handler()

	req := withURLParam(httptest.NewRequest(http.MethodGet, "/api/v1/torrents/3", nil), "id", "3")
	w := httptest.NewRecorder()
	h.HandleGetByID(w, req)

	torrent := decodeBody(t, w)["torrent"].(map[string]interface{})
	meta, ok := torrent["metadata"].(map[string]interface{})
	if !ok || len(meta) != 0 {
		t.Fatalf("metadata = %v, want {}", torrent["metadata"])
	}
}
