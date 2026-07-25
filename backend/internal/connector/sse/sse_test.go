package sse

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/connector"
)

type capture struct {
	feed    string
	id      string
	payload []byte
	calls   int
}

func newTestConnector() (*Connector, *capture) {
	got := &capture{}
	c := New(func(feed, id string, payload []byte) {
		got.feed = feed
		got.id = id
		got.payload = payload
		got.calls++
	})
	return c, got
}

// instance is a feed row with the given slug.
func instance(slug string) connector.Instance {
	return connector.Instance{ID: 1, Config: json.RawMessage(`{"slug":"` + slug + `"}`)}
}

func announcement() connector.Announcement {
	return connector.Announcement{
		Event:       connector.EventTorrentPublished,
		Title:       "Some.Release-GROUP",
		TorrentID:   7,
		Name:        "Some.Release-GROUP",
		CategoryID:  3,
		Category:    "Movies",
		Size:        2 * 1024 * 1024 * 1024,
		Uploader:    "alice",
		URL:         "https://tracker.test/torrent/7",
		DeliveryKey: "torrent.published:7",
	}
}

func TestKindAndShape(t *testing.T) {
	c, _ := newTestConnector()

	if c.Kind() != "sse" {
		t.Fatalf("Kind() = %q, want sse", c.Kind())
	}
	if c.Singleton() {
		// Feeds are told apart by slug and a watcher subscribes to exactly one,
		// so a second instance adds a feed rather than a duplicate.
		t.Fatal("several live feeds must be allowed")
	}
	if len(c.SecretFields()) != 0 {
		t.Fatalf("SecretFields() = %v, want none", c.SecretFields())
	}
	if !c.Coalescable() {
		t.Fatal("the page renders a coalesced event as a summary row, so it may coalesce")
	}
}

func TestDeliverBroadcastsTheCanonicalAnnouncement(t *testing.T) {
	c, got := newTestConnector()

	if err := c.Deliver(context.Background(), instance("default"), announcement()); err != nil {
		t.Fatalf("Deliver: %v", err)
	}

	if got.calls != 1 {
		t.Fatalf("broadcast %d times, want 1", got.calls)
	}
	if got.id != "torrent.published:7" {
		t.Fatalf("event id = %q, want the delivery key", got.id)
	}

	var round connector.Announcement
	if err := json.Unmarshal(got.payload, &round); err != nil {
		t.Fatalf("payload is not an Announcement: %v (%s)", err, got.payload)
	}
	if round.TorrentID != 7 || round.Category != "Movies" {
		t.Fatalf("payload round-tripped to %+v", round)
	}
}

// The live feed is one more place the real uploader must never reach.
func TestDeliverNeverNamesAnAnonymousUploader(t *testing.T) {
	c, got := newTestConnector()

	a := announcement()
	a.Anonymous = true
	a.Uploader = connector.AnonymousUploader

	if err := c.Deliver(context.Background(), instance("default"), a); err != nil {
		t.Fatalf("Deliver: %v", err)
	}

	body := string(got.payload)
	if strings.Contains(body, "alice") {
		t.Fatalf("payload leaked the real uploader: %s", body)
	}
	if !strings.Contains(body, `"uploader":"Anonymous"`) {
		t.Fatalf("payload = %s, want uploader Anonymous", body)
	}
}

func TestDeliverCarriesTheCoalescedCount(t *testing.T) {
	c, got := newTestConnector()

	a := announcement()
	a.Coalesced = 6
	if err := c.Deliver(context.Background(), instance("default"), a); err != nil {
		t.Fatalf("Deliver: %v", err)
	}

	var round connector.Announcement
	if err := json.Unmarshal(got.payload, &round); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if round.Coalesced != 6 {
		t.Fatalf("Coalesced = %d, want 6 so the page can render a summary row", round.Coalesced)
	}
}

// Without a hub there is nobody to deliver to, and reporting that as a failure
// would burn the retry budget on a deployment that simply has no HTTP side.
func TestDeliverWithoutAHubIsNotReady(t *testing.T) {
	c := New(nil)

	err := c.Deliver(context.Background(), instance("default"), announcement())
	if !errors.Is(err, connector.ErrNotReady) {
		t.Fatalf("err = %v, want connector.ErrNotReady", err)
	}
}

func TestDeliverBroadcastsToItsOwnFeedOnly(t *testing.T) {
	// The reason feeds exist: an announcement that passed this feed's filters
	// says nothing about what the others are configured to carry.
	c, got := newTestConnector()

	if err := c.Deliver(context.Background(), instance("no-adult"), announcement()); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if got.feed != "no-adult" {
		t.Fatalf("broadcast to feed %q, want the instance's own slug", got.feed)
	}
}

func TestDeliverFallsBackToTheDefaultFeed(t *testing.T) {
	// A row written before slugs existed still resolves, so an upgrade does not
	// silently strand its watchers on a feed nothing publishes to.
	c, got := newTestConnector()

	if err := c.Deliver(context.Background(), connector.Instance{ID: 1}, announcement()); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if got.feed != DefaultSlug {
		t.Fatalf("broadcast to feed %q, want %q", got.feed, DefaultSlug)
	}
}

func TestValidateConfig(t *testing.T) {
	c, _ := newTestConnector()

	for _, cfg := range []string{`{"slug":"default"}`, `{"slug":"no-adult","rate_per_min":5}`, `{"slug":"a1"}`} {
		if err := c.ValidateConfig(json.RawMessage(cfg)); err != nil {
			t.Errorf("ValidateConfig(%s) = %v, want nil", cfg, err)
		}
	}

	invalid := map[string]string{
		// Who sees the feed is decided by authentication and what appears in it
		// by the shared filters, so anything else is a misunderstanding worth
		// surfacing at save time.
		"unknown key": `{"slug":"default","template":"{{.Name}}"}`,
		"bad rate":    `{"slug":"default","rate_per_min":0}`,
		"malformed":   `nope`,
		// The slug is a URL path segment, so anything needing escaping there
		// would make the feed's address ambiguous.
		"no slug":    `{}`,
		"empty slug": `{"slug":"  "}`,
		// Padding is rejected rather than trimmed: nothing rewrites the stored
		// config, so a trimmed-on-read slug would disagree with what the unique
		// index sees.
		"leading space":  `{"slug":" news"}`,
		"trailing space": `{"slug":"news "}`,
		"uppercase":      `{"slug":"NoAdult"}`,
		"spaces":         `{"slug":"no adult"}`,
		"slash":          `{"slug":"a/b"}`,
		"leading dash":   `{"slug":"-lead"}`,
		"trailing dash":  `{"slug":"trail-"}`,
		"double dash":    `{"slug":"a--b"}`,
		"path traversal": `{"slug":".."}`,
		"too long":       `{"slug":"` + strings.Repeat("a", 41) + `"}`,
	}
	for name, cfg := range invalid {
		t.Run(name, func(t *testing.T) {
			if err := c.ValidateConfig(json.RawMessage(cfg)); err == nil {
				t.Fatalf("ValidateConfig(%s) = nil, want an error", cfg)
			}
		})
	}
}
