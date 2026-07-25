package chat

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/connector"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

type fakePoster struct {
	posted []string
	err    error
	nextID int64
}

func (f *fakePoster) SendSystemMessage(_ context.Context, msg string) (*model.ChatMessage, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.posted = append(f.posted, msg)
	f.nextID++
	return &model.ChatMessage{
		ID:        f.nextID,
		Username:  model.SystemChatUsername,
		Message:   msg,
		System:    true,
		CreatedAt: time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
	}, nil
}

func announcement() connector.Announcement {
	return connector.Announcement{
		Event:      connector.EventTorrentPublished,
		TorrentID:  7,
		Name:       "Some.Release-GROUP",
		CategoryID: 3,
		Category:   "Movies",
		Size:       2 * 1024 * 1024 * 1024,
		Uploader:   "alice",
		URL:        "https://tracker.test/torrent/7",
	}
}

func newTestConnector() (*Connector, *fakePoster, *[][]byte) {
	poster := &fakePoster{}
	var broadcasts [][]byte
	c := New(poster, func(payload []byte) {
		broadcasts = append(broadcasts, payload)
	})
	return c, poster, &broadcasts
}

func TestKindAndShape(t *testing.T) {
	c, _, _ := newTestConnector()
	if c.Kind() != "chat" {
		t.Fatalf("Kind() = %q, want chat", c.Kind())
	}
	if c.Singleton() {
		// Several instances post to the one shoutbox on purpose: each carries
		// its own template and filters.
		t.Fatal("chat must allow several instances")
	}
	if len(c.SecretFields()) != 0 {
		t.Fatalf("SecretFields() = %v, want none", c.SecretFields())
	}
}

func TestDeliverPostsAndBroadcasts(t *testing.T) {
	c, poster, broadcasts := newTestConnector()

	inst := connector.Instance{ID: 1, Name: "shoutbox", Config: json.RawMessage(`{}`)}
	if err := c.Deliver(context.Background(), inst, announcement()); err != nil {
		t.Fatalf("Deliver: %v", err)
	}

	if len(poster.posted) != 1 {
		t.Fatalf("posted %d messages, want 1", len(poster.posted))
	}
	line := poster.posted[0]
	for _, want := range []string{"Some.Release-GROUP", "Movies", "2.00 GiB", "https://tracker.test/torrent/7"} {
		if !strings.Contains(line, want) {
			t.Fatalf("posted %q, missing %q", line, want)
		}
	}

	if len(*broadcasts) != 1 {
		t.Fatalf("broadcast %d payloads, want 1", len(*broadcasts))
	}
	var payload map[string]any
	if err := json.Unmarshal((*broadcasts)[0], &payload); err != nil {
		t.Fatalf("unmarshal broadcast: %v", err)
	}
	if payload["type"] != "message" {
		t.Fatalf("type = %v, want message", payload["type"])
	}
	if payload["system"] != true {
		t.Fatalf("system = %v, want true", payload["system"])
	}
	if payload["username"] != model.SystemChatUsername {
		t.Fatalf("username = %v, want %q", payload["username"], model.SystemChatUsername)
	}
	if payload["user_id"].(float64) != 0 {
		t.Fatalf("user_id = %v, want 0 (system messages have no author)", payload["user_id"])
	}
}

func TestDeliverUsesCustomTemplate(t *testing.T) {
	c, poster, _ := newTestConnector()

	inst := connector.Instance{ID: 1, Config: json.RawMessage(`{"template":"NEW: {{.Name}}"}`)}
	if err := c.Deliver(context.Background(), inst, announcement()); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if got, want := poster.posted[0], "NEW: Some.Release-GROUP"; got != want {
		t.Fatalf("posted %q, want %q", got, want)
	}
}

// The coalesced summary stands in for several torrents, so it must not name one
// of them — even if the instance configured a per-torrent template.
func TestDeliverCoalescedIgnoresPerTorrentTemplate(t *testing.T) {
	c, poster, _ := newTestConnector()

	a := announcement()
	a.Coalesced = 4

	inst := connector.Instance{ID: 1, Config: json.RawMessage(`{"template":"NEW: {{.Name}}"}`)}
	if err := c.Deliver(context.Background(), inst, a); err != nil {
		t.Fatalf("Deliver: %v", err)
	}

	line := poster.posted[0]
	if !strings.Contains(line, "4 new torrents") {
		t.Fatalf("posted %q, want a +N summary", line)
	}
	if strings.Contains(line, "Some.Release-GROUP") {
		t.Fatalf("posted %q, a summary must not name a single torrent", line)
	}
}

func TestDeliverRendersAnonymousUploader(t *testing.T) {
	c, poster, _ := newTestConnector()

	a := announcement()
	a.Anonymous = true
	a.Uploader = connector.AnonymousUploader

	inst := connector.Instance{ID: 1, Config: json.RawMessage(`{"template":"{{.Name}} by {{.Uploader}}"}`)}
	if err := c.Deliver(context.Background(), inst, a); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if !strings.Contains(poster.posted[0], connector.AnonymousUploader) {
		t.Fatalf("posted %q, want it to say Anonymous", poster.posted[0])
	}
}

func TestDeliverPropagatesPosterFailure(t *testing.T) {
	poster := &fakePoster{err: errors.New("database down")}
	c := New(poster, nil)

	err := c.Deliver(context.Background(), connector.Instance{ID: 1}, announcement())
	if err == nil {
		t.Fatal("expected a poster failure to surface so the delivery retries")
	}
}

func TestDeliverWithoutBroadcastStillPosts(t *testing.T) {
	poster := &fakePoster{}
	c := New(poster, nil)

	if err := c.Deliver(context.Background(), connector.Instance{ID: 1}, announcement()); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if len(poster.posted) != 1 {
		t.Fatal("message must still be persisted when no WS bridge is wired")
	}
}

// A bad template is an admin mistake, and it has to surface at save time rather
// than as a delivery failure on every future announcement.
func TestValidateConfigRejectsBadTemplate(t *testing.T) {
	c, _, _ := newTestConnector()

	if err := c.ValidateConfig(json.RawMessage(`{"template":"{{.Name"}`)); err == nil {
		t.Fatal("expected an unparseable template to be rejected")
	}
	if err := c.ValidateConfig(json.RawMessage(`{"template":"{{.Nope}}"}`)); err != nil {
		// A syntactically valid template referencing an unknown field only fails
		// at execution, which ValidateTemplate does not run — documented as the
		// boundary of save-time checking.
		t.Logf("unknown-field template accepted at save time: %v", err)
	}
	if err := c.ValidateConfig(json.RawMessage(`{}`)); err != nil {
		t.Fatalf("empty config rejected: %v", err)
	}
}

func TestValidateConfigRejectsBadRateLimit(t *testing.T) {
	c, _, _ := newTestConnector()
	if err := c.ValidateConfig(json.RawMessage(`{"rate_per_min":0}`)); err == nil {
		t.Fatal("expected rate_per_min 0 to be rejected")
	}
}
