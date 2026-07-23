package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// Detail-view coverage for the torrent handler: the category breadcrumb, the
// peer list, download gating and the reseed endpoints.

type stubPeerRepo struct {
	peers   []model.Peer
	listErr error
}

func (s *stubPeerRepo) GetByTorrentAndUser(_ context.Context, _, _ int64) (*model.Peer, error) {
	return nil, errors.New("no peer")
}
func (s *stubPeerRepo) GetByTorrentUserAndPeerID(_ context.Context, _, _ int64, _ []byte) (*model.Peer, error) {
	return nil, errors.New("no peer")
}
func (s *stubPeerRepo) ListByTorrent(_ context.Context, _ int64, _ int) ([]model.Peer, error) {
	return s.peers, s.listErr
}
func (s *stubPeerRepo) ListByUserSeeding(_ context.Context, _ int64, _, _ int) ([]repository.PeerWithTorrent, int64, error) {
	return nil, 0, nil
}
func (s *stubPeerRepo) ListByUserLeeching(_ context.Context, _ int64, _, _ int) ([]repository.PeerWithTorrent, int64, error) {
	return nil, 0, nil
}
func (s *stubPeerRepo) CountByUser(_ context.Context, _ int64) (int, int, error) { return 0, 0, nil }
func (s *stubPeerRepo) CountByTorrent(_ context.Context, _ int64) (int, error)   { return 0, nil }
func (s *stubPeerRepo) CountTotalByUser(_ context.Context, _ int64) (int, error) { return 0, nil }
func (s *stubPeerRepo) Upsert(_ context.Context, _ *model.Peer) error            { return nil }
func (s *stubPeerRepo) Delete(_ context.Context, _, _ int64, _ []byte) error     { return nil }
func (s *stubPeerRepo) DeleteStale(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}

type stubCategoryRepo struct {
	byID map[int64]*model.Category
}

func (s *stubCategoryRepo) GetByID(_ context.Context, id int64) (*model.Category, error) {
	c, ok := s.byID[id]
	if !ok {
		return nil, errors.New("category not found")
	}
	return c, nil
}
func (s *stubCategoryRepo) List(_ context.Context) ([]model.Category, error)  { return nil, nil }
func (s *stubCategoryRepo) Create(_ context.Context, _ *model.Category) error { return nil }
func (s *stubCategoryRepo) Update(_ context.Context, _ *model.Category) error { return nil }
func (s *stubCategoryRepo) Delete(_ context.Context, _ int64) error           { return nil }
func (s *stubCategoryRepo) CountTorrentsByCategory(_ context.Context, _ int64) (int64, error) {
	return 0, nil
}
func (s *stubCategoryRepo) Reorder(_ context.Context, _ []repository.CategoryPlacement) error {
	return nil
}

type stubReseedRepo struct {
	exists   bool
	count    int
	created  []*model.ReseedRequest
	countErr error
}

func (s *stubReseedRepo) Create(_ context.Context, req *model.ReseedRequest) error {
	s.created = append(s.created, req)
	return nil
}
func (s *stubReseedRepo) ExistsByTorrentAndUser(_ context.Context, _, _ int64) (bool, error) {
	return s.exists, nil
}
func (s *stubReseedRepo) CountByTorrent(_ context.Context, _ int64) (int, error) {
	return s.count, s.countErr
}

type torrentDeps struct {
	torrents *stubTorrentRepo
	users    *stubUserRepo
	store    *stubStorage
	peers    *stubPeerRepo
	cats     *stubCategoryRepo
	reseeds  *stubReseedRepo
}

func newTorrentDeps() *torrentDeps {
	return &torrentDeps{
		torrents: &stubTorrentRepo{torrents: map[int64]*model.Torrent{}},
		users:    newStubUserRepo(&model.User{ID: 7, Username: "member", CanDownload: true}),
		store:    &stubStorage{},
		peers:    &stubPeerRepo{},
		cats:     &stubCategoryRepo{byID: map[int64]*model.Category{}},
		reseeds:  &stubReseedRepo{},
	}
}

func (d *torrentDeps) handler() *TorrentHandler {
	svc := service.NewTorrentService(nil, d.torrents, d.users, d.store,
		service.TorrentServiceConfig{AnnounceURL: "http://localhost/announce"},
		event.NewInMemoryBus(), d.reseeds)
	return NewTorrentHandler(svc, d.peers, d.users, d.cats)
}

// --- list -------------------------------------------------------------------

func TestTorrentListPassesFiltersToTheStore(t *testing.T) {
	d := newTorrentDeps()
	d.torrents.list = []model.Torrent{{ID: 3, Name: "A movie", CategoryID: 5}}
	d.torrents.total = 1
	h := d.handler()

	url := "/api/v1/torrents?search=movie&cat=5&sort=seeders&order=desc&page=2&per_page=10" +
		"&created_after=2026-01-02T15:04:05Z&max_seeders=0&uploader_id=7"
	w := httptest.NewRecorder()
	h.HandleList(w, httptest.NewRequest(http.MethodGet, url, nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	opts := d.torrents.lastOpts
	if opts.Search != "movie" || opts.SortBy != "seeders" || opts.SortOrder != "desc" {
		t.Errorf("opts = %+v, want the search/sort passed through", opts)
	}
	if opts.CategoryID == nil || *opts.CategoryID != 5 {
		t.Errorf("cat = %v, want 5", opts.CategoryID)
	}
	if opts.UploaderID == nil || *opts.UploaderID != 7 {
		t.Errorf("uploader_id = %v, want 7", opts.UploaderID)
	}
	if opts.MaxSeeders == nil || *opts.MaxSeeders != 0 {
		t.Errorf("max_seeders = %v, want 0 (the dead-torrent filter)", opts.MaxSeeders)
	}
	if opts.CreatedAfter == nil || opts.CreatedAfter.Year() != 2026 {
		t.Errorf("created_after = %v, want the parsed timestamp", opts.CreatedAfter)
	}
	if opts.Page != 2 || opts.PerPage != 10 {
		t.Errorf("page/per_page = %d/%d, want 2/10", opts.Page, opts.PerPage)
	}
}

// Garbage filters must be ignored rather than turned into a bad query.
func TestTorrentListIgnoresUnparseableFilters(t *testing.T) {
	d := newTorrentDeps()
	h := d.handler()

	url := "/api/v1/torrents?cat=abc&created_after=yesterday&max_seeders=lots&uploader_id=-1"
	w := httptest.NewRecorder()
	h.HandleList(w, httptest.NewRequest(http.MethodGet, url, nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	opts := d.torrents.lastOpts
	if opts.CategoryID != nil || opts.CreatedAfter != nil || opts.MaxSeeders != nil || opts.UploaderID != nil {
		t.Errorf("opts = %+v, want every unparseable filter dropped", opts)
	}
}

func TestTorrentListSurfacesStoreFailure(t *testing.T) {
	d := newTorrentDeps()
	d.torrents.listErr = errStub
	h := d.handler()

	w := httptest.NewRecorder()
	h.HandleList(w, httptest.NewRequest(http.MethodGet, "/api/v1/torrents", nil))

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

// --- detail -----------------------------------------------------------------

func TestTorrentDetailIncludesCategoryBreadcrumbRootFirst(t *testing.T) {
	d := newTorrentDeps()
	root := int64(1)
	img := "windows.png"
	d.cats.byID[1] = &model.Category{ID: 1, Name: "Software"}
	d.cats.byID[5] = &model.Category{ID: 5, Name: "Windows", ParentID: &root, ImageURL: &img}
	d.torrents.torrents[3] = &model.Torrent{ID: 3, Name: "A tool", CategoryID: 5, InfoHash: []byte("0123456789abcdef0123")}
	h := d.handler()

	req := withURLParam(httptest.NewRequest(http.MethodGet, "/api/v1/torrents/3", nil), "id", "3")
	w := httptest.NewRecorder()
	h.HandleGetByID(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	torrent := decodeBody(t, w)["torrent"].(map[string]interface{})
	path, ok := torrent["category_path"].([]interface{})
	if !ok || len(path) != 2 {
		t.Fatalf("category_path = %v, want a 2-level breadcrumb", torrent["category_path"])
	}
	if path[0].(map[string]interface{})["name"] != "Software" {
		t.Errorf("first crumb = %v, want the root \"Software\"", path[0])
	}
	leaf := path[1].(map[string]interface{})
	if leaf["name"] != "Windows" || leaf["image_url"] != "windows.png" {
		t.Errorf("leaf crumb = %v, want Windows with its image", leaf)
	}
}

// A parent cycle in the category table must not hang the request.
func TestTorrentDetailBreadcrumbSurvivesACategoryCycle(t *testing.T) {
	d := newTorrentDeps()
	a, b := int64(1), int64(2)
	d.cats.byID[1] = &model.Category{ID: 1, Name: "A", ParentID: &b}
	d.cats.byID[2] = &model.Category{ID: 2, Name: "B", ParentID: &a}
	d.torrents.torrents[3] = &model.Torrent{ID: 3, Name: "x", CategoryID: 1}
	h := d.handler()

	req := withURLParam(httptest.NewRequest(http.MethodGet, "/api/v1/torrents/3", nil), "id", "3")
	w := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		h.HandleGetByID(w, req)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("HandleGetByID hung on a cyclic category chain")
	}

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	path := decodeBody(t, w)["torrent"].(map[string]interface{})["category_path"].([]interface{})
	if len(path) != 2 {
		t.Errorf("category_path = %v, want the cycle broken after 2 entries", path)
	}
}

func TestTorrentDetailIncludesPeers(t *testing.T) {
	d := newTorrentDeps()
	agent := "qBittorrent/4.6"
	d.torrents.torrents[3] = &model.Torrent{ID: 3, Name: "A movie", CategoryID: 1}
	d.peers.peers = []model.Peer{
		{UserID: 7, Uploaded: 100, Downloaded: 50, LeftBytes: 0, Port: 6881, Seeder: true, Agent: &agent},
	}
	h := d.handler()

	req := withURLParam(httptest.NewRequest(http.MethodGet, "/api/v1/torrents/3", nil), "id", "3")
	w := httptest.NewRecorder()
	h.HandleGetByID(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	peers, ok := decodeBody(t, w)["peers"].([]interface{})
	if !ok || len(peers) != 1 {
		t.Fatalf("peers = %v, want 1 entry", peers)
	}
	peer := peers[0].(map[string]interface{})
	for _, key := range []string{"user_id", "uploaded", "downloaded", "left_bytes", "port", "seeder", "agent", "last_announce"} {
		if _, present := peer[key]; !present {
			t.Errorf("peer response is missing key %q", key)
		}
	}
	if peer["seeder"] != true {
		t.Error("seeder = false, want true")
	}
}

// A failing peer query must degrade to "no peers", not a 500.
func TestTorrentDetailStillRendersWhenPeerQueryFails(t *testing.T) {
	d := newTorrentDeps()
	d.torrents.torrents[3] = &model.Torrent{ID: 3, Name: "A movie", CategoryID: 1}
	d.peers.listErr = errStub
	h := d.handler()

	req := withURLParam(httptest.NewRequest(http.MethodGet, "/api/v1/torrents/3", nil), "id", "3")
	w := httptest.NewRecorder()
	h.HandleGetByID(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if _, present := decodeBody(t, w)["peers"]; present {
		t.Error("peers key present even though the peer query failed")
	}
}

func TestTorrentDetailReturns404WhenMissing(t *testing.T) {
	d := newTorrentDeps()
	h := d.handler()

	req := withURLParam(httptest.NewRequest(http.MethodGet, "/api/v1/torrents/3", nil), "id", "3")
	w := httptest.NewRecorder()
	h.HandleGetByID(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

// --- download ---------------------------------------------------------------

func TestTorrentDownloadRequiresAuth(t *testing.T) {
	d := newTorrentDeps()
	h := d.handler()

	req := withURLParam(httptest.NewRequest(http.MethodGet, "/api/v1/torrents/3/download", nil), "id", "3")
	w := httptest.NewRecorder()
	h.HandleDownload(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

// A user whose download privileges were revoked must be refused before the file
// is ever fetched.
func TestTorrentDownloadForbiddenWhenPrivilegeRevoked(t *testing.T) {
	d := newTorrentDeps()
	d.users.users[7].CanDownload = false
	d.torrents.torrents[3] = &model.Torrent{ID: 3, Name: "A movie"}
	h := d.handler()

	req := authed(httptest.NewRequest(http.MethodGet, "/api/v1/torrents/3/download", nil), 7, 20)
	req = withURLParam(req, "id", "3")
	w := httptest.NewRecorder()
	h.HandleDownload(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", w.Code)
	}
}

func TestTorrentDownloadRejectsInvalidID(t *testing.T) {
	d := newTorrentDeps()
	h := d.handler()

	req := authed(httptest.NewRequest(http.MethodGet, "/api/v1/torrents/0/download", nil), 7, 20)
	req = withURLParam(req, "id", "0")
	w := httptest.NewRecorder()
	h.HandleDownload(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestTorrentDownloadReturns404WhenMissing(t *testing.T) {
	d := newTorrentDeps()
	h := d.handler()

	req := authed(httptest.NewRequest(http.MethodGet, "/api/v1/torrents/3/download", nil), 7, 20)
	req = withURLParam(req, "id", "3")
	w := httptest.NewRecorder()
	h.HandleDownload(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

// --- reseed -----------------------------------------------------------------

func TestRequestReseedRequiresAuth(t *testing.T) {
	d := newTorrentDeps()
	h := d.handler()

	req := withURLParam(httptest.NewRequest(http.MethodPost, "/api/v1/torrents/3/reseed", nil), "id", "3")
	w := httptest.NewRecorder()
	h.HandleRequestReseed(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestRequestReseedRejectsInvalidID(t *testing.T) {
	d := newTorrentDeps()
	h := d.handler()

	req := authed(httptest.NewRequest(http.MethodPost, "/api/v1/torrents/x/reseed", nil), 7, 20)
	req = withURLParam(req, "id", "x")
	w := httptest.NewRecorder()
	h.HandleRequestReseed(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestRequestReseedReturns404WhenTorrentMissing(t *testing.T) {
	d := newTorrentDeps()
	h := d.handler()

	req := authed(httptest.NewRequest(http.MethodPost, "/api/v1/torrents/3/reseed", nil), 7, 20)
	req = withURLParam(req, "id", "3")
	w := httptest.NewRecorder()
	h.HandleRequestReseed(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestRequestReseedCreatesRequest(t *testing.T) {
	d := newTorrentDeps()
	d.torrents.torrents[3] = &model.Torrent{ID: 3, Name: "A movie", Seeders: 0}
	h := d.handler()

	req := authed(httptest.NewRequest(http.MethodPost, "/api/v1/torrents/3/reseed", nil), 7, 20)
	req = withURLParam(req, "id", "3")
	w := httptest.NewRecorder()
	h.HandleRequestReseed(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
	}
	if len(d.reseeds.created) != 1 || d.reseeds.created[0].RequesterID != 7 || d.reseeds.created[0].TorrentID != 3 {
		t.Errorf("reseed requests = %+v, want one from the authenticated user for torrent 3", d.reseeds.created)
	}
}

func TestGetReseedCountRejectsInvalidID(t *testing.T) {
	d := newTorrentDeps()
	h := d.handler()

	req := withURLParam(httptest.NewRequest(http.MethodGet, "/api/v1/torrents/0/reseed", nil), "id", "0")
	w := httptest.NewRecorder()
	h.HandleGetReseedCount(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestGetReseedCountReturnsCount(t *testing.T) {
	d := newTorrentDeps()
	d.reseeds.count = 4
	h := d.handler()

	req := withURLParam(httptest.NewRequest(http.MethodGet, "/api/v1/torrents/3/reseed", nil), "id", "3")
	w := httptest.NewRecorder()
	h.HandleGetReseedCount(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if got := decodeBody(t, w)["count"]; got != float64(4) {
		t.Errorf("count = %v, want 4", got)
	}
}

func TestGetReseedCountSurfacesStoreFailure(t *testing.T) {
	d := newTorrentDeps()
	d.reseeds.countErr = errStub
	h := d.handler()

	req := withURLParam(httptest.NewRequest(http.MethodGet, "/api/v1/torrents/3/reseed", nil), "id", "3")
	w := httptest.NewRecorder()
	h.HandleGetReseedCount(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}
