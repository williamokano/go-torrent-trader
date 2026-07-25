// Package telegram posts announcements to Telegram chats via the Bot API.
//
// Two things make it different from the generic webhook: the bot token lives in
// the request *path*, so no error may ever carry the URL; and messages are
// HTML-formatted, so every interpolated field has to be escaped or a torrent
// name containing "&" or "<" breaks the message.
package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"strings"

	"github.com/williamokano/go-torrent-trader/backend/internal/connector"
)

// DefaultTemplate is the message sent when an instance sets none. It is HTML
// because parse_mode is HTML; the values interpolated into it are escaped first.
const DefaultTemplate = `<b>{{.Name}}</b>` + "\n" +
	`{{.Category}} · {{.SizeHuman}}{{if .Freeleech}} · freeleech{{end}}` + "\n" +
	`<a href="{{.URL}}">view torrent</a>`

// CoalescedTemplate stands in for a batch the rate limit could not send one by
// one. It names no single torrent, because it speaks for several.
const CoalescedTemplate = `<b>{{.Coalesced}} new torrents published</b>`

// defaultAPIBase is Telegram's Bot API. It is a field on the connector so tests
// can point it at a local server — the token is in the path, so there is no way
// to intercept the request without replacing the base.
const defaultAPIBase = "https://api.telegram.org"

// maxExcerpt bounds how much of an error body is quoted back to the admin.
const maxExcerpt = 200

// Config is the Telegram instance's admin-editable settings.
type Config struct {
	// BotToken is a secret and appears in every request URL.
	BotToken string `json:"bot_token"`
	// ChatIDs are the chats to post to. Group ids are negative and can exceed
	// int32, so they are strings — which is also what the API accepts for
	// @channelusername.
	ChatIDs  []string `json:"chat_ids"`
	Template string   `json:"template"`
}

// Connector posts announcements to Telegram chats.
type Connector struct {
	client  *http.Client
	apiBase string
}

// New creates the Telegram connector around the shared guarded HTTP client.
func New(client *http.Client) *Connector {
	return &Connector{client: client, apiBase: defaultAPIBase}
}

func (c *Connector) Kind() string           { return "telegram" }
func (c *Connector) Singleton() bool        { return false }
func (c *Connector) SecretFields() []string { return []string{"bot_token"} }

// Coalescable: a Telegram chat is read by people, so a bulk import should
// arrive as one summary rather than twenty notifications.
func (c *Connector) Coalescable() bool { return true }

// ValidateConfig checks everything that can be checked without calling the API.
func (c *Connector) ValidateConfig(cfg json.RawMessage) error {
	var parsed Config
	if err := connector.DecodeConfig(cfg, &parsed); err != nil {
		return err
	}
	if err := validateToken(parsed.BotToken); err != nil {
		return err
	}
	if len(parsed.ChatIDs) == 0 {
		return fmt.Errorf("at least one chat_id is required")
	}
	seen := make(map[string]bool, len(parsed.ChatIDs))
	for _, chatID := range parsed.ChatIDs {
		trimmed := strings.TrimSpace(chatID)
		if trimmed == "" {
			return fmt.Errorf("chat_id cannot be empty")
		}
		if seen[trimmed] {
			return fmt.Errorf("chat_id %q is listed twice", trimmed)
		}
		seen[trimmed] = true
	}
	if err := connector.ValidateTemplate(parsed.Template); err != nil {
		return err
	}
	return connector.ValidateRatePerMin(cfg)
}

// Deliver sends one message per configured chat.
//
// A partial failure fails the whole delivery, so the pipeline retries it and
// the chats that already received the message get it again. sendMessage has no
// idempotency key, and a duplicate announcement in a chat is a much smaller
// problem than an announcement that never arrives.
func (c *Connector) Deliver(ctx context.Context, inst connector.Instance, a connector.Announcement) error {
	var cfg Config
	if err := connector.DecodeConfig(inst.Config, &cfg); err != nil {
		return err
	}

	// Re-validated on the delivery path, not only at save time: a row restored
	// from a backup or written directly could carry an unusable token or no
	// chats at all, and an empty chat list would otherwise "succeed" while
	// sending nothing.
	if err := validateToken(cfg.BotToken); err != nil {
		return fmt.Errorf("%w: %s", connector.ErrPermanent, err)
	}
	if len(cfg.ChatIDs) == 0 {
		return fmt.Errorf("%w: no chat_ids configured", connector.ErrPermanent)
	}

	tmpl := cfg.Template
	if a.Coalesced > 0 {
		tmpl = CoalescedTemplate
	} else if tmpl == "" {
		tmpl = DefaultTemplate
	}

	text, err := connector.RenderTemplate(tmpl, escapeForHTML(a))
	if err != nil {
		return err
	}

	var (
		failures     []string
		anyTransient bool
		anyNotReady  bool
	)
	for _, chatID := range cfg.ChatIDs {
		err := c.send(ctx, strings.TrimSpace(cfg.BotToken), strings.TrimSpace(chatID), text)
		if err == nil {
			continue
		}
		failures = append(failures, err.Error())
		switch {
		case errors.Is(err, connector.ErrPermanent):
		case errors.Is(err, connector.ErrNotReady):
			anyNotReady = true
		default:
			anyTransient = true
		}
	}
	if len(failures) == 0 {
		return nil
	}

	// The aggregate has to carry the right classification, not just the text:
	// the pipeline reads the sentinel, and joining with %s would strip it and
	// silently turn a rate limit back into a burned retry attempt.
	joined := strings.Join(failures, "; ")
	switch {
	case anyTransient:
		// Something might recover on the ordinary backoff ladder.
		return fmt.Errorf("telegram: %s", joined)
	case anyNotReady:
		// Only rate limits: retry soon without spending an attempt.
		return fmt.Errorf("%w: telegram: %s", connector.ErrNotReady, joined)
	default:
		// Every failure was permanent — a chat that no longer exists, a revoked
		// token. Retrying would not fix it and would re-send to the chats that
		// did work, five times, for every torrent. Dead-letter it instead so the
		// admin log shows what to fix.
		return fmt.Errorf("%w: telegram: %s", connector.ErrPermanent, joined)
	}
}

func (c *Connector) send(ctx context.Context, token, chatID, text string) error {
	body, err := json.Marshal(map[string]any{
		"chat_id":                  chatID,
		"text":                     text,
		"parse_mode":               "HTML",
		"disable_web_page_preview": true,
	})
	if err != nil {
		return fmt.Errorf("marshal message for chat %s: %w", chatID, err)
	}

	endpoint := fmt.Sprintf("%s/bot%s/sendMessage", strings.TrimRight(c.apiBase, "/"), token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		// StripURL matters more here than anywhere else: the token is in the
		// path, so an error carrying the URL carries the credential.
		return fmt.Errorf("build request for chat %s: %w", chatID, connector.StripURL(err))
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("chat %s: %w", chatID, connector.StripURL(err))
	}
	defer func() { _ = resp.Body.Close() }()
	// Read a bounded prefix once: it is the diagnosis on an error, and draining
	// it keeps the connection reusable on success.
	preview, _ := io.ReadAll(io.LimitReader(resp.Body, connector.MaxDrainBytes))

	if resp.StatusCode >= 200 && resp.StatusCode <= 299 {
		return nil
	}

	// Telegram's description names the actual problem ("chat not found",
	// "Can't parse entities: Unclosed start tag"), and a bare status code does
	// not. It carries neither the URL nor the token, and redaction still runs
	// over it before it is stored.
	detail := fmt.Sprintf("chat %s: sendMessage returned %d", chatID, resp.StatusCode)
	if excerpt := connector.ErrorExcerpt(preview, maxExcerpt); excerpt != "" {
		detail += ": " + excerpt
	}

	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		// Telegram's own rate limit: ordinary operation, not a fault of this
		// delivery, so it must not spend one of the five attempts.
		return fmt.Errorf("%w: %s", connector.ErrNotReady, detail)
	case resp.StatusCode == http.StatusRequestTimeout,
		resp.StatusCode >= 500:
		// Transient: worth retrying on the normal backoff.
		return errors.New(detail)
	case resp.StatusCode >= 400:
		// A 4xx from the Bot API means this request will never be accepted —
		// the chat is gone, the token is revoked, the markup is malformed.
		return fmt.Errorf("%w: %s", connector.ErrPermanent, detail)
	default:
		return errors.New(detail)
	}
}

// escapeForHTML escapes every field a template can interpolate.
//
// parse_mode is HTML, and a torrent name containing "&" or "<" is completely
// ordinary — unescaped it would make Telegram reject the whole message. Escaping
// the inputs rather than the rendered output is what keeps the template's own
// markup working while making any custom template safe.
// html.EscapeString covers <, >, &, " and ', the last two as the numeric
// references &#34; and &#39;. Telegram's HTML parse mode accepts numeric
// character references, and escaping the quote matters because the default
// template puts the URL inside href="…" — apostrophes in release names are
// routine, so both are exercised by the tests.
func escapeForHTML(a connector.Announcement) connector.Announcement {
	a.Name = html.EscapeString(a.Name)
	a.Category = html.EscapeString(fallback(a.Category, "uncategorised"))
	a.Uploader = html.EscapeString(fallback(a.Uploader, connector.AnonymousUploader))
	a.URL = html.EscapeString(a.URL)
	return a
}

func fallback(value, whenEmpty string) string {
	if strings.TrimSpace(value) == "" {
		return whenEmpty
	}
	return value
}

func validateToken(token string) error {
	trimmed := strings.TrimSpace(token)
	if trimmed == "" {
		return fmt.Errorf("bot_token is required")
	}
	// The token goes straight into a URL path, so anything that could alter the
	// request shape is refused rather than escaped.
	if strings.ContainsAny(trimmed, "/?#\r\n ") {
		return fmt.Errorf("bot_token contains invalid characters")
	}
	return nil
}
