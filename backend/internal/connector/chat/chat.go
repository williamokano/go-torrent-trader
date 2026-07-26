// Package chat announces new torrents in the site's own shoutbox.
//
// Several instances are allowed even though there is only one shoutbox: each
// carries its own template and filters, so a site can word the Anime line
// differently from the Movies line. Two instances whose filters overlap really
// do post twice — that is the admin's call, not something to prevent here.
package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/connector"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

// DefaultTemplate is the line posted when an instance sets no template.
//
// It differs from connector.DefaultTemplate — which still trails a bare URL —
// because the shoutbox renders Markdown: the torrent name itself is the link,
// so the line reads as a sentence instead of ending in a wall of URL. The
// shared default stays as it is for IRC and for Announcement.Body, where
// Markdown is noise.
const DefaultTemplate = "New torrent: {{.Link}} — {{.Category}}, {{.SizeHuman}}"

// CoalescedTemplate stands in for a batch the rate limit could not post
// individually. It intentionally references no per-torrent field: the summary
// covers several torrents, so naming one of them would be misleading.
const CoalescedTemplate = "{{.Coalesced}} new torrents published — see the browse page"

// Poster is the slice of ChatService the connector needs. Narrowing it to one
// method keeps the connector testable without a database.
type Poster interface {
	SendSystemMessage(ctx context.Context, msg string) (*model.ChatMessage, error)
}

// Config is the chat instance's admin-editable settings.
type Config struct {
	Template string `json:"template"`
}

// Connector posts announcements to the shoutbox as system messages.
type Connector struct {
	poster    Poster
	broadcast func([]byte)
}

// New creates the chat connector. broadcast pushes the same payload to connected
// WebSocket clients so the line appears live rather than on next reload; it may
// be nil, in which case the message is still persisted.
func New(poster Poster, broadcast func([]byte)) *Connector {
	return &Connector{poster: poster, broadcast: broadcast}
}

func (c *Connector) Kind() string           { return "chat" }
func (c *Connector) Singleton() bool        { return false }
func (c *Connector) SecretFields() []string { return nil }

// Coalescable: the shoutbox is read by people, and twenty lines in a row from a
// bulk import is exactly what the "+N more" summary exists to prevent.
func (c *Connector) Coalescable() bool { return true }

// ValidateConfig rejects a template that cannot parse, so a typo fails at save
// time instead of failing every delivery afterwards.
func (c *Connector) ValidateConfig(cfg json.RawMessage) error {
	var parsed Config
	if err := connector.DecodeConfig(cfg, &parsed); err != nil {
		return err
	}
	if err := connector.ValidateTemplate(parsed.Template); err != nil {
		return err
	}
	return connector.ValidateRatePerMin(cfg)
}

// Deliver renders the announcement and posts it as a system chat message.
func (c *Connector) Deliver(ctx context.Context, inst connector.Instance, a connector.Announcement) error {
	var cfg Config
	if err := connector.DecodeConfig(inst.Config, &cfg); err != nil {
		return err
	}

	// Substituted here rather than left to RenderTemplate, whose empty-template
	// fallback is the shared default meant for plain-text destinations.
	tmpl := cfg.Template
	if strings.TrimSpace(tmpl) == "" {
		tmpl = DefaultTemplate
	}
	if a.Coalesced > 0 {
		tmpl = CoalescedTemplate
	}

	line, err := connector.RenderTemplate(tmpl, a)
	if err != nil {
		return err
	}

	msg, err := c.poster.SendSystemMessage(ctx, line)
	if err != nil {
		return fmt.Errorf("post system chat message: %w", err)
	}

	if c.broadcast == nil {
		return nil
	}

	// Mirrors handler.chatMessagePayload plus the system flag. It is duplicated
	// rather than imported because handler depends on service, and a connector
	// reaching back into the HTTP layer would invert the dependency.
	//
	// The username comes from the message the service just returned rather than
	// from a constant here: the label is an operator setting, and a copy in this
	// payload would show one name live and a different one after a reload.
	payload, err := json.Marshal(map[string]any{
		"type":       "message",
		"id":         msg.ID,
		"user_id":    int64(0),
		"username":   msg.Username,
		"message":    msg.Message,
		"system":     true,
		"created_at": msg.CreatedAt.Format(time.RFC3339),
	})
	if err != nil {
		return fmt.Errorf("marshal chat broadcast: %w", err)
	}
	c.broadcast(payload)

	return nil
}
