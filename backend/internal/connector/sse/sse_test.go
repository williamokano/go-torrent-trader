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
	id      string
	payload []byte
	calls   int
}

func newTestConnector() (*Connector, *capture) {
	got := &capture{}
	c := New(func(id string, payload []byte) {
		got.id = id
		got.payload = payload
		got.calls++
	})
	return c, got
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
	if !c.Singleton() {
		t.Fatal("there is one live feed, so a second instance would double-deliver to every watcher")
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

	if err := c.Deliver(context.Background(), connector.Instance{ID: 1}, announcement()); err != nil {
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

	if err := c.Deliver(context.Background(), connector.Instance{ID: 1}, a); err != nil {
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
	if err := c.Deliver(context.Background(), connector.Instance{ID: 1}, a); err != nil {
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

	err := c.Deliver(context.Background(), connector.Instance{ID: 1}, announcement())
	if !errors.Is(err, connector.ErrNotReady) {
		t.Fatalf("err = %v, want connector.ErrNotReady", err)
	}
}

func TestValidateConfig(t *testing.T) {
	c, _ := newTestConnector()

	for _, cfg := range []string{``, `{}`, `null`, `{"rate_per_min":5}`} {
		if err := c.ValidateConfig(json.RawMessage(cfg)); err != nil {
			t.Errorf("ValidateConfig(%s) = %v, want nil", cfg, err)
		}
	}

	invalid := map[string]string{
		// Who sees the feed is decided by authentication and what appears in it
		// by the shared filters, so anything else is a misunderstanding worth
		// surfacing at save time.
		"unknown key": `{"template":"{{.Name}}"}`,
		"bad rate":    `{"rate_per_min":0}`,
		"malformed":   `nope`,
	}
	for name, cfg := range invalid {
		t.Run(name, func(t *testing.T) {
			if err := c.ValidateConfig(json.RawMessage(cfg)); err == nil {
				t.Fatalf("ValidateConfig(%s) = nil, want an error", cfg)
			}
		})
	}
}
