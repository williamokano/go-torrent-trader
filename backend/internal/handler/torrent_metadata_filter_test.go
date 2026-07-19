package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// filterCategorySchema is the effective schema used by the metadata-filter
// tests: one field of each type so coercion can be exercised.
var filterCategorySchema = json.RawMessage(`[
	{"key":"year","label":"Year","type":"number","integer":true},
	{"key":"codec","label":"Codec","type":"select","options":["x264","x265"]},
	{"key":"title","label":"Title","type":"text"},
	{"key":"hdr","label":"HDR","type":"boolean"},
	{"key":"genres","label":"Genres","type":"multiselect","options":["Action","Drama"]}
]`)

// withFilterCategory registers category 5 carrying filterCategorySchema.
func withFilterCategory(d *torrentDeps) {
	d.cats.byID[5] = &model.Category{ID: 5, Name: "Movies", MetadataSchema: filterCategorySchema}
}

// findFilter returns the (first) filter matching key+op, or false if absent.
func findFilter(fs []repository.MetadataFilter, key string, op repository.MetadataFilterOp) (repository.MetadataFilter, bool) {
	for _, f := range fs {
		if f.Key == key && f.Op == op {
			return f, true
		}
	}
	return repository.MetadataFilter{}, false
}

func TestMetadataFilters_CoercesEachTypeAndPassesThrough(t *testing.T) {
	d := newTorrentDeps()
	withFilterCategory(d)
	h := d.handler()

	url := "/api/v1/torrents?cat=5" +
		"&meta_year=2024" +
		"&meta_codec=x265" +
		"&meta_title=The%20Matrix" +
		"&meta_hdr=true" +
		"&meta_genres=Action"
	w := httptest.NewRecorder()
	h.HandleList(w, httptest.NewRequest(http.MethodGet, url, nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	got := d.torrents.lastOpts.MetadataFilters
	if len(got) != 5 {
		t.Fatalf("got %d filters, want 5: %+v", len(got), got)
	}

	cases := []struct {
		key  string
		want any
	}{
		{"year", float64(2024)}, // numbers coerce to float64, not string
		{"codec", "x265"},
		{"title", "The Matrix"},
		{"hdr", true}, // booleans coerce to bool, not "true"
		{"genres", []string{"Action"}},
	}
	for _, c := range cases {
		f, ok := findFilter(got, c.key, repository.MetaFilterEq)
		if !ok {
			t.Errorf("missing eq filter for %q", c.key)
			continue
		}
		if !reflect.DeepEqual(f.Value, c.want) {
			t.Errorf("%q value = %#v (%T), want %#v", c.key, f.Value, f.Value, c.want)
		}
	}
}

func TestMetadataFilters_NumericRange(t *testing.T) {
	d := newTorrentDeps()
	withFilterCategory(d)
	h := d.handler()

	url := "/api/v1/torrents?cat=5&meta_year__gte=2000&meta_year__lte=2010"
	w := httptest.NewRecorder()
	h.HandleList(w, httptest.NewRequest(http.MethodGet, url, nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	got := d.torrents.lastOpts.MetadataFilters
	gte, ok := findFilter(got, "year", repository.MetaFilterGte)
	if !ok || gte.Value != float64(2000) {
		t.Errorf("gte filter = %#v, ok=%v, want 2000", gte, ok)
	}
	lte, ok := findFilter(got, "year", repository.MetaFilterLte)
	if !ok || lte.Value != float64(2010) {
		t.Errorf("lte filter = %#v, ok=%v, want 2010", lte, ok)
	}
}

func TestMetadataFilters_RequireCategory(t *testing.T) {
	d := newTorrentDeps()
	withFilterCategory(d)
	h := d.handler()

	// meta_* param but no cat → 400.
	w := httptest.NewRecorder()
	h.HandleList(w, httptest.NewRequest(http.MethodGet, "/api/v1/torrents?meta_year=2024", nil))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", w.Code, w.Body.String())
	}
}

func TestMetadataFilters_UnknownFieldRejected(t *testing.T) {
	d := newTorrentDeps()
	withFilterCategory(d)
	h := d.handler()

	w := httptest.NewRecorder()
	h.HandleList(w, httptest.NewRequest(http.MethodGet, "/api/v1/torrents?cat=5&meta_nonsense=1", nil))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", w.Code, w.Body.String())
	}
}

func TestMetadataFilters_NonNumericValueRejected(t *testing.T) {
	d := newTorrentDeps()
	withFilterCategory(d)
	h := d.handler()

	w := httptest.NewRecorder()
	h.HandleList(w, httptest.NewRequest(http.MethodGet, "/api/v1/torrents?cat=5&meta_year=abc", nil))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", w.Code, w.Body.String())
	}
}

// A range suffix on a non-numeric field isn't registered, so it reads as an
// unknown filter → 400 rather than silently ignored.
func TestMetadataFilters_RangeOnNonNumberRejected(t *testing.T) {
	d := newTorrentDeps()
	withFilterCategory(d)
	h := d.handler()

	w := httptest.NewRecorder()
	h.HandleList(w, httptest.NewRequest(http.MethodGet, "/api/v1/torrents?cat=5&meta_title__gte=x", nil))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", w.Code, w.Body.String())
	}
}

// Empty meta_* values are treated as "no filter", not an error.
func TestMetadataFilters_EmptyValuesIgnored(t *testing.T) {
	d := newTorrentDeps()
	withFilterCategory(d)
	h := d.handler()

	w := httptest.NewRecorder()
	h.HandleList(w, httptest.NewRequest(http.MethodGet, "/api/v1/torrents?cat=5&meta_year=&meta_codec=", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if len(d.torrents.lastOpts.MetadataFilters) != 0 {
		t.Errorf("filters = %+v, want none", d.torrents.lastOpts.MetadataFilters)
	}
}

// No meta_* params at all → no schema resolution, no filters, plain list.
func TestMetadataFilters_NoneLeavesOptsEmpty(t *testing.T) {
	d := newTorrentDeps()
	withFilterCategory(d)
	h := d.handler()

	w := httptest.NewRecorder()
	h.HandleList(w, httptest.NewRequest(http.MethodGet, "/api/v1/torrents?cat=5", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if d.torrents.lastOpts.MetadataFilters != nil {
		t.Errorf("filters = %+v, want nil", d.torrents.lastOpts.MetadataFilters)
	}
}
