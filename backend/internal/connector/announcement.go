package connector

import (
	"fmt"
	"io"
	"strings"
	"text/template"
	"time"
)

// Event kinds carried on Announcement.Event. This stays a plain string rather
// than a typed enum so later event kinds (per-user relay, forum posts) can widen
// it without a redesign of the stored payloads.
const (
	EventTorrentPublished = "torrent.published"
	EventTest             = "test"
)

// DefaultTemplate is the one-line summary used to fill Announcement.Body and as
// the fallback for connectors whose instance sets no template of its own.
const DefaultTemplate = "New torrent: {{.Name}} [{{.Category}}, {{.SizeHuman}}] {{.URL}}"

// AnonymousUploader is the name every renderer shows for an anonymous upload.
// The real username never reaches an Announcement in the first place — this is
// what fills the gap.
const AnonymousUploader = "Anonymous"

// Announcement is the canonical, connector-agnostic shape of one thing worth
// announcing. It is what gets marshaled into connector_deliveries.payload, sent
// as the webhook body, and streamed over SSE — so it must never contain a
// secret, and never the real name behind an anonymous upload.
type Announcement struct {
	Event       string    `json:"event"`
	Title       string    `json:"title"`
	Body        string    `json:"body"`
	TorrentID   int64     `json:"torrent_id"`
	Name        string    `json:"name"`
	InfoHashHex string    `json:"info_hash"`
	CategoryID  int64     `json:"category_id"`
	Category    string    `json:"category"`
	Size        int64     `json:"size"`
	FileCount   int       `json:"file_count"`
	Uploader    string    `json:"uploader"`
	Anonymous   bool      `json:"anonymous"`
	Freeleech   bool      `json:"freeleech"`
	Silver      bool      `json:"silver"`
	URL         string    `json:"url"`
	PublishedAt time.Time `json:"published_at"`
	// Coalesced is >0 on the single summary announcement that stands in for a
	// batch the per-instance rate limit could not send individually.
	Coalesced int `json:"coalesced,omitempty"`
	// DeliveryKey is the connector_deliveries.event_key for this attempt, filled
	// in by the pipeline just before Deliver so a connector can hand the receiver
	// an idempotency token. It is not serialized: the delivery row already stores
	// it, and the payload should stay a pure description of the event.
	DeliveryKey string `json:"-"`
}

// EventKey is the dedupe key for a real announcement: one delivery per instance
// per published torrent, enforced by a unique index. Test-sends do not use this
// — they get a "test:<uuid>" key so they can never collide with a real event.
func EventKey(a Announcement) string {
	return fmt.Sprintf("%s:%d", a.Event, a.TorrentID)
}

// RenderContext is the fixed set of fields a per-instance template may
// reference. It is deliberately not the Announcement itself: the template
// surface is an admin-facing contract that should only change on purpose, and
// hiding the raw struct keeps fields like Anonymous from being interpolated by
// accident.
type RenderContext struct {
	Name      string
	Category  string
	Size      int64
	SizeHuman string
	URL       string
	Uploader  string
	Freeleech bool
	FileCount int
	Coalesced int
}

// NewRenderContext projects an Announcement onto the template surface.
func NewRenderContext(a Announcement) RenderContext {
	return RenderContext{
		Name:      a.Name,
		Category:  a.Category,
		Size:      a.Size,
		SizeHuman: HumanSize(a.Size),
		URL:       a.URL,
		Uploader:  a.Uploader,
		Freeleech: a.Freeleech,
		FileCount: a.FileCount,
		Coalesced: a.Coalesced,
	}
}

// ValidateTemplate checks an admin-supplied template, called from every
// connector's ValidateConfig so a template that cannot render is rejected at
// save time rather than silently failing every delivery afterwards.
//
// It renders as well as parses. Parsing alone would accept {{.Titel}} — the
// template package only rejects an unknown *field* at execution time, and
// missingkey=error does not help because the render context is a struct, not a
// map — so a single typo would save cleanly and then dead-letter every
// announcement for that instance until someone read the log.
func ValidateTemplate(tmpl string) error {
	if strings.TrimSpace(tmpl) == "" {
		return nil
	}
	t, err := newTemplate(tmpl)
	if err != nil {
		return err
	}
	if err := t.Execute(io.Discard, NewRenderContext(Sample())); err != nil {
		return fmt.Errorf("render template: %w", err)
	}
	return nil
}

// RenderTemplate renders tmpl against the announcement. An empty template
// renders the shared default line.
func RenderTemplate(tmpl string, a Announcement) (string, error) {
	if strings.TrimSpace(tmpl) == "" {
		tmpl = DefaultTemplate
	}
	t, err := newTemplate(tmpl)
	if err != nil {
		return "", err
	}
	var buf strings.Builder
	if err := t.Execute(&buf, NewRenderContext(a)); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}
	return buf.String(), nil
}

// newTemplate parses the template. missingkey=error only affects map lookups,
// which the struct render context never does — the real typo guard is the trial
// render in ValidateTemplate.
func newTemplate(tmpl string) (*template.Template, error) {
	t, err := template.New("announcement").Option("missingkey=error").Parse(tmpl)
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}
	return t, nil
}

// HumanSize formats a byte count with binary units, matching how sizes read
// everywhere else on the site.
func HumanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

// Sample is the canned announcement behind the admin test-send, so a
// destination can be verified without waiting for a real upload.
func Sample() Announcement {
	a := Announcement{
		Event:       EventTest,
		TorrentID:   0,
		Name:        "Test.Announcement.2024.1080p.BluRay-GROUP",
		InfoHashHex: "0000000000000000000000000000000000000000",
		CategoryID:  0,
		Category:    "Movies",
		Size:        4 * 1024 * 1024 * 1024,
		FileCount:   3,
		Uploader:    "TestUploader",
		Freeleech:   true,
		URL:         "https://example.invalid/torrent/0",
		PublishedAt: time.Now().UTC(),
	}
	a.Title = a.Name
	a.Body, _ = RenderTemplate(DefaultTemplate, a)
	return a
}
