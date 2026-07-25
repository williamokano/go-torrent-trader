// Package discord posts announcements to a Discord channel webhook.
//
// It is a thin specialisation of the generic webhook path — same guarded HTTP
// client, same pipeline — differing only in the body shape and in one important
// respect: the webhook URL *is* the credential, so it is a secret field rather
// than ordinary config.
package discord

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/connector"
	"github.com/williamokano/go-torrent-trader/backend/internal/connector/httpguard"
)

// DefaultTemplate is the embed description used when an instance sets none.
const DefaultTemplate = "{{.Name}}"

// maxExcerpt bounds how much of an error body is quoted back to the admin.
const maxExcerpt = 200

// defaultAllowedHosts are the only hosts a Discord webhook may point at.
//
// The SSRF guard already stops the dangerous destinations; this is a
// typo-and-mistake check on top, so a URL pasted from the wrong place fails at
// save time instead of quietly shipping every announcement somewhere else.
var defaultAllowedHosts = []string{"discord.com", "discordapp.com"}

// Config is the Discord instance's admin-editable settings.
type Config struct {
	// WebhookURL is a secret: anyone holding it can post to the channel.
	WebhookURL string `json:"webhook_url"`
	Username   string `json:"username"`
	AvatarURL  string `json:"avatar_url"`
	Template   string `json:"template"`
}

// Connector posts announcements as Discord embeds.
type Connector struct {
	client *http.Client
	// allowedHosts and requireHTTPS are the destination policy. They are fields
	// rather than constants so a test can point at a local server — the same
	// seam the Telegram connector needs for its API base — while production
	// always gets the strict defaults.
	allowedHosts []string
	requireHTTPS bool
}

// New creates the Discord connector around the shared guarded HTTP client.
func New(client *http.Client) *Connector {
	return &Connector{
		client:       client,
		allowedHosts: defaultAllowedHosts,
		requireHTTPS: true,
	}
}

func (c *Connector) Kind() string    { return "discord" }
func (c *Connector) Singleton() bool { return false }

// SecretFields covers the whole URL, which is why redaction removes entire
// config keys rather than substrings: there is no part of this value that is
// safe to show.
func (c *Connector) SecretFields() []string { return []string{"webhook_url"} }

// Coalescable: a Discord channel is read by people, and twenty embeds in a row
// from a bulk import is what the "+N more" summary exists to prevent.
func (c *Connector) Coalescable() bool { return true }

// ValidateConfig checks everything that can be checked without posting.
func (c *Connector) ValidateConfig(cfg json.RawMessage) error {
	var parsed Config
	if err := connector.DecodeConfig(cfg, &parsed); err != nil {
		return err
	}
	if err := c.validateWebhookURL(parsed.WebhookURL); err != nil {
		return err
	}
	if parsed.AvatarURL != "" {
		if err := httpguard.ValidateURL(parsed.AvatarURL); err != nil {
			return fmt.Errorf("avatar_url: %w", err)
		}
	}
	if err := connector.ValidateTemplate(parsed.Template); err != nil {
		return err
	}
	return connector.ValidateRatePerMin(cfg)
}

// Deliver posts one embed, or a plain message for a coalesced summary.
func (c *Connector) Deliver(ctx context.Context, inst connector.Instance, a connector.Announcement) error {
	var cfg Config
	if err := connector.DecodeConfig(inst.Config, &cfg); err != nil {
		return err
	}

	// Re-validated on the delivery path, not only at save time: a row restored
	// from a backup or written directly could carry an http:// URL, and this
	// credential must never go over the wire in the clear.
	if err := c.validateWebhookURL(cfg.WebhookURL); err != nil {
		return fmt.Errorf("%w: %s", connector.ErrPermanent, err)
	}

	body, err := c.buildBody(cfg, a)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.WebhookURL, bytes.NewReader(body))
	if err != nil {
		// The URL is the credential, so it must not travel with the error even
		// when the failure is our own request construction.
		return fmt.Errorf("build discord request: %w", connector.StripURL(err))
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("discord request failed: %w", connector.StripURL(err))
	}
	defer func() { _ = resp.Body.Close() }()

	// Read a bounded prefix once: it is the diagnosis on an error, and draining
	// it keeps the connection reusable on success.
	preview, _ := io.ReadAll(io.LimitReader(resp.Body, connector.MaxDrainBytes))

	if resp.StatusCode == http.StatusTooManyRequests {
		// Discord's limit is roughly five requests per two seconds per webhook,
		// and a drain can send a whole minute's budget back to back — so being
		// rate limited is ordinary operation, not a fault of this delivery.
		// ErrNotReady reschedules it without spending one of its five attempts,
		// which is what stops a bulk import dead-lettering half its
		// announcements.
		if retryAfter := strings.TrimSpace(resp.Header.Get("Retry-After")); retryAfter != "" {
			return fmt.Errorf("%w: discord rate limited, retry after %ss",
				connector.ErrNotReady, retryAfter)
		}
		return fmt.Errorf("%w: discord rate limited", connector.ErrNotReady)
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		// Status plus a bounded excerpt of Discord's own message, which names
		// the offending field. Neither carries the URL, and redaction still runs
		// over it before it is stored.
		if excerpt := connector.ErrorExcerpt(preview, maxExcerpt); excerpt != "" {
			return fmt.Errorf("discord webhook returned %d: %s", resp.StatusCode, excerpt)
		}
		return fmt.Errorf("discord webhook returned %d", resp.StatusCode)
	}
	return nil
}

// embed is Discord's rich-message shape.
type embed struct {
	Title       string       `json:"title,omitempty"`
	URL         string       `json:"url,omitempty"`
	Description string       `json:"description,omitempty"`
	Timestamp   string       `json:"timestamp,omitempty"`
	Fields      []embedField `json:"fields,omitempty"`
}

type embedField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline"`
}

type payload struct {
	Username  string  `json:"username,omitempty"`
	AvatarURL string  `json:"avatar_url,omitempty"`
	Content   string  `json:"content,omitempty"`
	Embeds    []embed `json:"embeds,omitempty"`
}

func (c *Connector) buildBody(cfg Config, a connector.Announcement) ([]byte, error) {
	out := payload{Username: cfg.Username, AvatarURL: cfg.AvatarURL}

	if a.Coalesced > 0 {
		// A summary stands for several torrents, so an embed titled after one of
		// them would misrepresent it. Plain content instead.
		out.Content = fmt.Sprintf("%d new torrents published", a.Coalesced)
		if a.URL != "" {
			out.Content += " — " + a.URL
		}
		return json.Marshal(out)
	}

	// The shared default line repeats the title, URL, category and size that the
	// embed already shows in its own fields, so this kind has its own.
	tmpl := cfg.Template
	if strings.TrimSpace(tmpl) == "" {
		tmpl = DefaultTemplate
	}
	description, err := connector.RenderTemplate(tmpl, a)
	if err != nil {
		return nil, err
	}

	out.Embeds = []embed{{
		Title:       a.Name,
		URL:         a.URL,
		Description: description,
		Timestamp:   a.PublishedAt.UTC().Format(time.RFC3339),
		Fields: []embedField{
			{Name: "Category", Value: fallback(a.Category, "—"), Inline: true},
			{Name: "Size", Value: connector.HumanSize(a.Size), Inline: true},
			{Name: "Uploader", Value: fallback(a.Uploader, connector.AnonymousUploader), Inline: true},
		},
	}}
	if a.Freeleech {
		out.Embeds[0].Fields = append(out.Embeds[0].Fields,
			embedField{Name: "Freeleech", Value: "yes", Inline: true})
	}
	return json.Marshal(out)
}

// validateWebhookURL requires https and a Discord host.
func (c *Connector) validateWebhookURL(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return fmt.Errorf("webhook_url is required")
	}
	// The wrapped error would quote the URL, which is the credential — and this
	// message reaches the admin API.
	if err := httpguard.ValidateURL(raw); err != nil {
		return fmt.Errorf("webhook_url is not a valid URL")
	}
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("webhook_url is not a valid URL")
	}
	if c.requireHTTPS && parsed.Scheme != "https" {
		// The URL is a bearer credential; sending it in the clear would hand it
		// to anyone on the path.
		return fmt.Errorf("webhook_url must use https")
	}
	host := strings.ToLower(parsed.Hostname())
	for _, allowed := range c.allowedHosts {
		if host == allowed || strings.HasSuffix(host, "."+allowed) {
			return nil
		}
	}
	return fmt.Errorf("webhook_url host %q is not a Discord host", host)
}

func fallback(value, whenEmpty string) string {
	if strings.TrimSpace(value) == "" {
		return whenEmpty
	}
	return value
}
