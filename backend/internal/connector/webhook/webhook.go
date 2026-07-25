// Package webhook POSTs announcements as JSON to an admin-supplied URL.
//
// The URL is untrusted input, so every request goes through the shared
// httpguard client (see that package for why the check lives in the dialer).
package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/connector"
	"github.com/williamokano/go-torrent-trader/backend/internal/connector/httpguard"
)

// Header names. X-Announce-Delivery carries the delivery key so a receiver can
// deduplicate a retried delivery on its own side.
const (
	HeaderEvent     = "X-Announce-Event"
	HeaderDelivery  = "X-Announce-Delivery"
	HeaderTimestamp = "X-Announce-Timestamp"
	HeaderSignature = "X-Announce-Signature"
)

// maxDrainBytes is how much of a response body is read before closing. The body
// is never used, but draining a little lets the connection be reused; draining
// all of it would let a hostile endpoint stream forever.
const maxDrainBytes = 4 << 10

// Config is the webhook instance's admin-editable settings.
type Config struct {
	URL    string `json:"url"`
	Method string `json:"method"`
	// Headers are sent verbatim. They are NOT treated as secret fields in v1 —
	// an Authorization header put here is stored and returned like any other
	// config value, which is why the UI steers static credentials towards
	// hmac_secret instead. They are never logged either way.
	Headers    map[string]string `json:"headers"`
	HMACSecret string            `json:"hmac_secret"`
}

// Connector delivers announcements over HTTP.
type Connector struct {
	client *http.Client
	now    func() time.Time // swapped in tests to pin the timestamp header
}

// New creates the webhook connector around a guarded HTTP client.
func New(client *http.Client) *Connector {
	return &Connector{client: client, now: time.Now}
}

func (c *Connector) Kind() string           { return "webhook" }
func (c *Connector) Singleton() bool        { return false }
func (c *Connector) SecretFields() []string { return []string{"hmac_secret"} }

// Coalescable is false: a webhook feeds software, and a "+N more" summary names
// one torrent and a count, leaving a receiver no way to learn about the rest.
// Rate-limited announcements wait for the next window instead of being folded
// away.
func (c *Connector) Coalescable() bool { return false }

// ValidateConfig checks everything that can be checked without a network call.
func (c *Connector) ValidateConfig(cfg json.RawMessage) error {
	var parsed Config
	if err := connector.DecodeConfig(cfg, &parsed); err != nil {
		return err
	}
	if err := httpguard.ValidateURL(parsed.URL); err != nil {
		return err
	}
	if _, err := normalizeMethod(parsed.Method); err != nil {
		return err
	}
	for name, value := range parsed.Headers {
		if err := validateHeader(name, value); err != nil {
			return err
		}
	}
	return connector.ValidateRatePerMin(cfg)
}

// Deliver POSTs the canonical announcement JSON to the configured URL.
func (c *Connector) Deliver(ctx context.Context, inst connector.Instance, a connector.Announcement) error {
	var cfg Config
	if err := connector.DecodeConfig(inst.Config, &cfg); err != nil {
		return err
	}
	method, err := normalizeMethod(cfg.Method)
	if err != nil {
		return err
	}

	body, err := json.Marshal(a)
	if err != nil {
		return fmt.Errorf("marshal announcement: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, method, cfg.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build webhook request: %w", stripURL(err))
	}

	timestamp := strconv.FormatInt(c.now().Unix(), 10)
	for name, value := range cfg.Headers {
		req.Header.Set(name, value)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(HeaderEvent, a.Event)
	req.Header.Set(HeaderDelivery, a.DeliveryKey)
	req.Header.Set(HeaderTimestamp, timestamp)
	if cfg.HMACSecret != "" {
		req.Header.Set(HeaderSignature, Sign(cfg.HMACSecret, timestamp, body))
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook request failed: %w", stripURL(err))
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxDrainBytes))

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		// Status only — no body echo (it may quote the request, including
		// headers) and no URL echo.
		return fmt.Errorf("webhook returned %d", resp.StatusCode)
	}
	return nil
}

// stripURL unwraps *url.Error, which carries the full request URL.
//
// ValidateConfig rejects credentials in the userinfo, but plenty of receivers
// (Slack, Discord, Mattermost) put the token in the path instead — and this
// error is what lands in the delivery log for the whole retention window.
// Keeping only the cause loses nothing worth reading.
func stripURL(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) && urlErr.Err != nil {
		return fmt.Errorf("%s: %w", urlErr.Op, urlErr.Err)
	}
	return err
}

// Sign produces the X-Announce-Signature value. The timestamp is part of the
// signed material so a receiver can reject replays.
func Sign(secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// normalizeMethod defaults to POST and allows only the two verbs that make
// sense for an announcement.
func normalizeMethod(method string) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case "", http.MethodPost:
		return http.MethodPost, nil
	case http.MethodPut:
		return http.MethodPut, nil
	default:
		return "", fmt.Errorf("method must be POST or PUT, got %q", method)
	}
}

// validateHeader rejects names/values that would let an admin smuggle extra
// headers (or split the request) through CR/LF injection.
func validateHeader(name, value string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("header name cannot be empty")
	}
	if strings.ContainsAny(name, "\r\n") || strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("header %q must not contain line breaks", name)
	}
	for _, r := range name {
		if r <= ' ' || r >= 0x7f || strings.ContainsRune(":()<>@,;\\\"/[]?={}", r) {
			return fmt.Errorf("header name %q contains invalid characters", name)
		}
	}
	return nil
}
