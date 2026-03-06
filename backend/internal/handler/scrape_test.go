package handler_test

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/handler"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
	"github.com/zeebo/bencode"
)

// fakeTorrentRepo is a minimal in-memory TorrentRepository for testing scrape.
type fakeTorrentRepo struct {
	torrents map[string]*model.Torrent
}

func newFakeTorrentRepo() *fakeTorrentRepo {
	return &fakeTorrentRepo{torrents: make(map[string]*model.Torrent)}
}

func (r *fakeTorrentRepo) GetByInfoHash(_ context.Context, infoHash []byte) (*model.Torrent, error) {
	t, ok := r.torrents[string(infoHash)]
	if !ok {
		return nil, sql.ErrNoRows
	}
	return t, nil
}

func (r *fakeTorrentRepo) GetByID(context.Context, int64) (*model.Torrent, error) {
	return nil, sql.ErrNoRows
}

func (r *fakeTorrentRepo) List(context.Context, repository.ListTorrentsOptions) ([]model.Torrent, int64, error) {
	return nil, 0, nil
}

func (r *fakeTorrentRepo) Create(context.Context, *model.Torrent) error { return nil }
func (r *fakeTorrentRepo) Update(context.Context, *model.Torrent) error { return nil }
func (r *fakeTorrentRepo) Delete(context.Context, int64) error          { return nil }
func (r *fakeTorrentRepo) IncrementSeeders(context.Context, int64, int) error {
	return nil
}
func (r *fakeTorrentRepo) IncrementLeechers(context.Context, int64, int) error {
	return nil
}
func (r *fakeTorrentRepo) IncrementTimesCompleted(context.Context, int64) error {
	return nil
}

func makeInfoHash(fill byte) []byte {
	h := make([]byte, 20)
	for i := range h {
		h[i] = fill
	}
	return h
}

func encodedInfoHash(ih []byte) string {
	return url.QueryEscape(string(ih))
}

func setupScrapeRouter(repo *fakeTorrentRepo) http.Handler {
	trackerSvc := service.NewTrackerService(nil, repo, nil)
	deps := &handler.Deps{
		TrackerService: trackerSvc,
	}
	return handler.NewRouter(deps)
}

type scrapeFiles struct {
	Files map[string]scrapeEntry `bencode:"files"`
}

type scrapeEntry struct {
	Complete   int `bencode:"complete"`
	Incomplete int `bencode:"incomplete"`
	Downloaded int `bencode:"downloaded"`
}

type scrapeError struct {
	FailureReason string `bencode:"failure reason"`
}

func TestScrape_SingleTorrent(t *testing.T) {
	repo := newFakeTorrentRepo()
	ih := makeInfoHash(0xAA)
	repo.torrents[string(ih)] = &model.Torrent{
		ID:             1,
		InfoHash:       ih,
		Seeders:        10,
		Leechers:       5,
		TimesCompleted: 42,
	}

	router := setupScrapeRouter(repo)

	target := fmt.Sprintf("/scrape?info_hash=%s", encodedInfoHash(ih))
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp scrapeFiles
	if err := bencode.DecodeBytes(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	entry, ok := resp.Files[string(ih)]
	if !ok {
		t.Fatal("expected info_hash in response files")
	}

	if entry.Complete != 10 {
		t.Errorf("expected complete=10, got %d", entry.Complete)
	}
	if entry.Incomplete != 5 {
		t.Errorf("expected incomplete=5, got %d", entry.Incomplete)
	}
	if entry.Downloaded != 42 {
		t.Errorf("expected downloaded=42, got %d", entry.Downloaded)
	}
}

func TestScrape_UnknownInfoHash(t *testing.T) {
	repo := newFakeTorrentRepo()
	router := setupScrapeRouter(repo)

	ih := makeInfoHash(0xBB)
	target := fmt.Sprintf("/scrape?info_hash=%s", encodedInfoHash(ih))
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp scrapeFiles
	if err := bencode.DecodeBytes(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp.Files) != 0 {
		t.Errorf("expected empty files map, got %d entries", len(resp.Files))
	}
}

func TestScrape_MultipleTorrents(t *testing.T) {
	repo := newFakeTorrentRepo()

	ih1 := makeInfoHash(0x01)
	ih2 := makeInfoHash(0x02)
	ih3 := makeInfoHash(0x03) // unknown

	repo.torrents[string(ih1)] = &model.Torrent{
		ID:             1,
		InfoHash:       ih1,
		Seeders:        3,
		Leechers:       1,
		TimesCompleted: 100,
	}
	repo.torrents[string(ih2)] = &model.Torrent{
		ID:             2,
		InfoHash:       ih2,
		Seeders:        7,
		Leechers:       2,
		TimesCompleted: 50,
	}

	router := setupScrapeRouter(repo)

	target := fmt.Sprintf("/scrape?info_hash=%s&info_hash=%s&info_hash=%s",
		encodedInfoHash(ih1), encodedInfoHash(ih2), encodedInfoHash(ih3))
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp scrapeFiles
	if err := bencode.DecodeBytes(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Should have 2 entries (ih3 is unknown, omitted)
	if len(resp.Files) != 2 {
		t.Fatalf("expected 2 file entries, got %d", len(resp.Files))
	}

	e1, ok := resp.Files[string(ih1)]
	if !ok {
		t.Fatal("expected ih1 in response")
	}
	if e1.Complete != 3 || e1.Incomplete != 1 || e1.Downloaded != 100 {
		t.Errorf("ih1: got complete=%d incomplete=%d downloaded=%d", e1.Complete, e1.Incomplete, e1.Downloaded)
	}

	e2, ok := resp.Files[string(ih2)]
	if !ok {
		t.Fatal("expected ih2 in response")
	}
	if e2.Complete != 7 || e2.Incomplete != 2 || e2.Downloaded != 50 {
		t.Errorf("ih2: got complete=%d incomplete=%d downloaded=%d", e2.Complete, e2.Incomplete, e2.Downloaded)
	}
}

func TestScrape_MissingInfoHash(t *testing.T) {
	repo := newFakeTorrentRepo()
	router := setupScrapeRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/scrape", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp scrapeError
	if err := bencode.DecodeBytes(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}

	if resp.FailureReason != "missing info_hash parameter" {
		t.Errorf("expected failure reason about missing info_hash, got %q", resp.FailureReason)
	}
}
