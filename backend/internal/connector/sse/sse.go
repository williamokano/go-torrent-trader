// Package sse pushes announcements to browsers watching the live releases page.
//
// It is a connector like any other, which is the point: the live feed inherits
// filters, the kill-switch, the delivery log and test-send for free, and an
// admin can narrow or disable it from the same panel as everything else.
package sse

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/williamokano/go-torrent-trader/backend/internal/connector"
)

// Broadcaster is the hub the connector pushes into. It is a func rather than an
// interface so the connector never depends on the HTTP layer.
type Broadcaster func(id string, payload []byte)

// Connector streams announcements to connected browsers.
type Connector struct {
	broadcast Broadcaster
}

// New creates the SSE connector.
func New(broadcast Broadcaster) *Connector {
	return &Connector{broadcast: broadcast}
}

func (c *Connector) Kind() string { return "sse" }

// Singleton: there is one live feed, so a second instance would just deliver
// every announcement to every watcher twice.
func (c *Connector) Singleton() bool { return true }

func (c *Connector) SecretFields() []string { return nil }

// Coalescable: the page is read by a person, and it renders a coalesced event
// as a single "+N more" row rather than dropping it silently — so a bulk import
// shows up as one summary line instead of scrolling the feed away.
func (c *Connector) Coalescable() bool { return true }

// ValidateConfig accepts only an empty object. The feed has nothing to
// configure: who sees it is decided by authentication, and what appears in it by
// the shared instance filters.
func (c *Connector) ValidateConfig(cfg json.RawMessage) error {
	trimmed := strings.TrimSpace(string(cfg))
	if trimmed == "" || trimmed == "null" || trimmed == "{}" {
		return nil
	}

	// Tolerate the shared rate limit, which the pipeline parses generically.
	var parsed map[string]json.RawMessage
	if err := json.Unmarshal(cfg, &parsed); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}
	for key := range parsed {
		if key != connector.RatePerMinKey {
			return fmt.Errorf("the live feed takes no configuration (unexpected key %q)", key)
		}
	}
	return connector.ValidateRatePerMin(cfg)
}

// Deliver pushes the announcement to every connected browser.
//
// The payload is the canonical Announcement JSON — the same shape the webhook
// body uses — so it is already anonymous-safe and free of secrets.
func (c *Connector) Deliver(_ context.Context, _ connector.Instance, a connector.Announcement) error {
	if c.broadcast == nil {
		return fmt.Errorf("%w: no live feed hub is wired", connector.ErrNotReady)
	}

	payload, err := json.Marshal(a)
	if err != nil {
		return fmt.Errorf("marshal announcement: %w", err)
	}

	c.broadcast(a.DeliveryKey, payload)
	return nil
}
