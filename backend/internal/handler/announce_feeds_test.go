package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/connector"
	"github.com/williamokano/go-torrent-trader/backend/internal/connector/sse"
	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// feedListRepo is the slice of the connector repository the feed list reads.
type feedListRepo struct {
	rows []model.NotificationConnector
	err  error
}

func (r *feedListRepo) ListEnabled(context.Context) ([]model.NotificationConnector, error) {
	return r.rows, r.err
}

func (r *feedListRepo) List(context.Context) ([]model.NotificationConnector, error) {
	return r.rows, r.err
}
func (r *feedListRepo) GetByID(context.Context, int64) (*model.NotificationConnector, error) {
	return nil, nil
}
func (r *feedListRepo) Create(context.Context, *model.NotificationConnector) error { return nil }
func (r *feedListRepo) Update(context.Context, *model.NotificationConnector) error { return nil }
func (r *feedListRepo) Delete(context.Context, int64) error                        { return nil }
func (r *feedListRepo) CountByKind(context.Context, string) (int64, error)         { return 0, nil }
func (r *feedListRepo) SetEnabled(context.Context, int64, bool) error              { return nil }

func newFeedsHandler(t *testing.T, repo *feedListRepo) *AnnounceFeedsHandler {
	t.Helper()

	registry := connector.NewRegistry()
	registry.Register(sse.New(func(string, string, []byte) {}))
	svc := service.NewConnectorService(repo, nil, registry, event.NewInMemoryBus(), "https://tracker.test")
	return NewAnnounceFeedsHandler(svc)
}

func TestAnnounceFeedsListsEnabledFeeds(t *testing.T) {
	h := newFeedsHandler(t, &feedListRepo{rows: []model.NotificationConnector{
		{ID: 1, Kind: "sse", Name: "Everything", Enabled: true,
			Config: json.RawMessage(`{"slug":"default"}`)},
		{ID: 2, Kind: "sse", Name: "Safe for work", Enabled: true,
			Config: json.RawMessage(`{"slug":"no-adult"}`)},
		// Another kind must not appear in a list of live feeds.
		{ID: 3, Kind: "webhook", Name: "Hook", Enabled: true, Config: json.RawMessage(`{}`)},
	}})

	rec := httptest.NewRecorder()
	h.HandleList(rec, httptest.NewRequest(http.MethodGet, "/api/v1/announce-feeds", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	// The envelope is the contract the live page reads as data.feeds.
	var body struct {
		Feeds []struct {
			Slug string `json:"slug"`
			Name string `json:"name"`
		} `json:"feeds"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not the expected envelope: %v (%s)", err, rec.Body)
	}
	if len(body.Feeds) != 2 {
		t.Fatalf("got %d feeds, want the two sse instances: %s", len(body.Feeds), rec.Body)
	}
	if body.Feeds[0].Slug != "default" || body.Feeds[1].Name != "Safe for work" {
		t.Fatalf("feeds = %+v", body.Feeds)
	}
}

// A member gets a slug and a name. Filters and config decide what a feed carries
// and are the admin list's business — that one is behind RequireAdmin.
func TestAnnounceFeedsExposeNoConfiguration(t *testing.T) {
	h := newFeedsHandler(t, &feedListRepo{rows: []model.NotificationConnector{
		{ID: 1, Kind: "sse", Name: "Everything", Enabled: true,
			Config:  json.RawMessage(`{"slug":"default","rate_per_min":20}`),
			Filters: json.RawMessage(`{"category_ids":[7],"category_mode":"exclude"}`)},
	}})

	rec := httptest.NewRecorder()
	h.HandleList(rec, httptest.NewRequest(http.MethodGet, "/api/v1/announce-feeds", nil))

	for _, leak := range []string{"filters", "config", "rate_per_min", "category_ids", "exclude"} {
		if strings.Contains(rec.Body.String(), leak) {
			t.Errorf("response leaked %q: %s", leak, rec.Body)
		}
	}
}

func TestAnnounceFeedsReportsARepositoryFailure(t *testing.T) {
	h := newFeedsHandler(t, &feedListRepo{err: context.DeadlineExceeded})

	rec := httptest.NewRecorder()
	h.HandleList(rec, httptest.NewRequest(http.MethodGet, "/api/v1/announce-feeds", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}
