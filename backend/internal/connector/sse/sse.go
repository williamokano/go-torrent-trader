// Package sse pushes announcements to browsers watching the live releases page.
//
// It is a connector like any other, which is the point: the live feed inherits
// filters, the kill-switch, the delivery log and test-send for free, and an
// admin can narrow or disable it from the same panel as everything else.
//
// Several feeds can exist at once. Each instance owns a slug, which is its
// stream URL, and its filters decide what appears in it — "everything",
// "everything except 18+", "just anime". Subscribers pick one, so two instances
// never double-deliver to the same watcher the way two chat instances can.
package sse

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/williamokano/go-torrent-trader/backend/internal/connector"
)

// Broadcaster is the hub the connector pushes into. It is a func rather than an
// interface so the connector never depends on the HTTP layer.
//
// feed is the instance's slug: the hub holds a separate client set per feed, and
// a frame must only reach the watchers of the feed that produced it.
type Broadcaster func(feed, id string, payload []byte)

// Config is the feed's admin-editable settings.
type Config struct {
	// Slug is the feed's URL: /api/v1/announce-stream/<slug>.
	Slug string `json:"slug"`
}

// DefaultSlug is the feed the unslugged legacy route resolves to.
const DefaultSlug = "default"

// slugPattern is deliberately narrow: the slug is a URL path segment, and
// anything needing escaping there would make the feed's address ambiguous.
var slugPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// maxSlugLen keeps the URL and the admin list readable.
const maxSlugLen = 40

// ValidateSlug reports whether a feed slug is usable as a URL path segment.
// Exported because the service checks uniqueness across instances and wants to
// reject a malformed one with the same message.
func ValidateSlug(slug string) error {
	if strings.TrimSpace(slug) == "" {
		return fmt.Errorf("slug is required: it is the feed's URL")
	}
	if len(slug) > maxSlugLen {
		return fmt.Errorf("slug must be at most %d characters", maxSlugLen)
	}
	// Checked as submitted rather than trimmed. Nothing rewrites the stored
	// config, so accepting " news" would store the padding, and the unique index
	// would then read " news" while the application reads "news" — two feeds on
	// one URL, each believing it owns it.
	if !slugPattern.MatchString(slug) {
		return fmt.Errorf("slug must be lowercase letters, digits and single dashes, with no spaces (e.g. %q)", "no-adult")
	}
	return nil
}

// SlugOf reads the slug out of a stored config, falling back to the default so a
// row written before slugs existed still resolves. It trims, because a row
// written directly to the database did not go through ValidateSlug.
func SlugOf(cfg json.RawMessage) string {
	var parsed Config
	if err := json.Unmarshal(cfg, &parsed); err != nil {
		return DefaultSlug
	}
	if slug := strings.TrimSpace(parsed.Slug); slug != "" {
		return slug
	}
	return DefaultSlug
}

// Connector streams announcements to connected browsers.
type Connector struct {
	broadcast Broadcaster
}

// New creates the SSE connector.
func New(broadcast Broadcaster) *Connector {
	return &Connector{broadcast: broadcast}
}

func (c *Connector) Kind() string { return "sse" }

// Singleton is false: feeds are told apart by slug, and a watcher subscribes to
// exactly one, so a second instance adds a feed rather than a duplicate.
func (c *Connector) Singleton() bool { return false }

func (c *Connector) SecretFields() []string { return nil }

// Coalescable: the page is read by a person, and it renders a coalesced event
// as a single "+N more" row rather than dropping it silently — so a bulk import
// shows up as one summary line instead of scrolling the feed away.
func (c *Connector) Coalescable() bool { return true }

// ValidateConfig checks the slug. Everything else about a feed is decided
// elsewhere: who may watch it by authentication, and what appears in it by the
// shared instance filters.
func (c *Connector) ValidateConfig(cfg json.RawMessage) error {
	var parsed map[string]json.RawMessage
	trimmed := strings.TrimSpace(string(cfg))
	if trimmed != "" && trimmed != "null" {
		if err := json.Unmarshal(cfg, &parsed); err != nil {
			return fmt.Errorf("parse config: %w", err)
		}
	}
	for key := range parsed {
		// The rate limit is parsed generically by the pipeline.
		if key != connector.RatePerMinKey && key != "slug" {
			return fmt.Errorf("the live feed takes no configuration beyond its slug (unexpected key %q)", key)
		}
	}

	var typed Config
	if len(cfg) > 0 && trimmed != "null" {
		if err := json.Unmarshal(cfg, &typed); err != nil {
			return fmt.Errorf("parse config: %w", err)
		}
	}
	if err := ValidateSlug(typed.Slug); err != nil {
		return err
	}
	return connector.ValidateRatePerMin(cfg)
}

// Deliver pushes the announcement to every connected browser.
//
// The payload is the canonical Announcement JSON — the same shape the webhook
// body uses — so it is already anonymous-safe and free of secrets.
func (c *Connector) Deliver(_ context.Context, inst connector.Instance, a connector.Announcement) error {
	if c.broadcast == nil {
		return fmt.Errorf("%w: no live feed hub is wired", connector.ErrNotReady)
	}

	payload, err := json.Marshal(a)
	if err != nil {
		return fmt.Errorf("marshal announcement: %w", err)
	}

	// Scoped to this instance's feed: an announcement that passed this feed's
	// filters says nothing about what the other feeds are configured to carry.
	c.broadcast(SlugOf(inst.Config), a.DeliveryKey, payload)
	return nil
}
