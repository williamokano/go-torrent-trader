package discord

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/connector"
	"github.com/williamokano/go-torrent-trader/backend/internal/connector/httpguard"
)

type captured struct {
	method  string
	headers http.Header
	body    []byte
}

// newReceiver stands in for Discord. httptest binds loopback, so the guard is
// built with private networks permitted — the blocking behaviour is covered in
// the httpguard package's own tests.
func newReceiver(t *testing.T, status int, respHeaders map[string]string) (*httptest.Server, *captured) {
	t.Helper()
	got := &captured{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got.method = r.Method
		got.headers = r.Header.Clone()
		got.body = body
		for k, v := range respHeaders {
			w.Header().Set(k, v)
		}
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv, got
}

// newTestConnector relaxes the destination policy to the local test server. The
// strict default is exercised separately, on both the save and delivery paths.
func newTestConnector(server string) *Connector {
	c := New(httpguard.NewClient(func() bool { return true }, 5*time.Second))
	c.requireHTTPS = false
	if server != "" {
		if parsed, err := url.Parse(server); err == nil {
			c.allowedHosts = []string{parsed.Hostname()}
		}
	}
	return c
}

func instance(cfg string) connector.Instance {
	return connector.Instance{ID: 1, Name: "Announcements", Config: json.RawMessage(cfg)}
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
		PublishedAt: time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
		DeliveryKey: "torrent.published:7",
	}
}

func TestKindAndShape(t *testing.T) {
	c := newTestConnector("")

	if c.Kind() != "discord" {
		t.Fatalf("Kind() = %q, want discord", c.Kind())
	}
	if c.Singleton() {
		t.Fatal("several Discord channels are a normal configuration")
	}
	if !c.Coalescable() {
		t.Fatal("a Discord channel is read by people, so it should coalesce")
	}
	// The webhook URL *is* the credential, which is why redaction strips whole
	// config keys rather than substrings.
	if got := c.SecretFields(); len(got) != 1 || got[0] != "webhook_url" {
		t.Fatalf("SecretFields() = %v, want [webhook_url]", got)
	}
}

func TestDeliverPostsAnEmbed(t *testing.T) {
	srv, got := newReceiver(t, http.StatusNoContent, nil)
	c := newTestConnector(srv.URL)

	cfg := fmt.Sprintf(`{"webhook_url":%q,"username":"Tracker","avatar_url":"https://tracker.test/logo.png"}`, srv.URL)
	if err := c.Deliver(context.Background(), instance(cfg), announcement()); err != nil {
		t.Fatalf("Deliver: %v", err)
	}

	if got.method != http.MethodPost {
		t.Fatalf("method = %s, want POST", got.method)
	}
	if ct := got.headers.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}

	var body payload
	if err := json.Unmarshal(got.body, &body); err != nil {
		t.Fatalf("body is not a Discord payload: %v (%s)", err, got.body)
	}
	if body.Username != "Tracker" || body.AvatarURL != "https://tracker.test/logo.png" {
		t.Fatalf("identity = %+v", body)
	}
	if len(body.Embeds) != 1 {
		t.Fatalf("got %d embeds, want 1", len(body.Embeds))
	}
	e := body.Embeds[0]
	if e.Title != "Some.Release-GROUP" {
		t.Fatalf("title = %q", e.Title)
	}
	if e.URL != "https://tracker.test/torrent/7" {
		t.Fatalf("url = %q, want the torrent link so the title is clickable", e.URL)
	}
	if e.Timestamp != "2026-07-25T12:00:00Z" {
		t.Fatalf("timestamp = %q, want RFC3339", e.Timestamp)
	}

	fields := map[string]string{}
	for _, f := range e.Fields {
		fields[f.Name] = f.Value
	}
	if fields["Category"] != "Movies" || fields["Size"] != "2.00 GiB" || fields["Uploader"] != "alice" {
		t.Fatalf("fields = %v", fields)
	}
}

func TestDeliverMarksFreeleech(t *testing.T) {
	srv, got := newReceiver(t, http.StatusOK, nil)
	c := newTestConnector(srv.URL)

	a := announcement()
	a.Freeleech = true
	cfg := fmt.Sprintf(`{"webhook_url":%q}`, srv.URL)
	if err := c.Deliver(context.Background(), instance(cfg), a); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if !strings.Contains(string(got.body), "Freeleech") {
		t.Fatalf("body = %s, want a freeleech field", got.body)
	}
}

// The whole-request assertion: the real uploader must appear nowhere.
func TestDeliverNeverNamesAnAnonymousUploader(t *testing.T) {
	srv, got := newReceiver(t, http.StatusNoContent, nil)
	c := newTestConnector(srv.URL)

	a := announcement()
	a.Anonymous = true
	a.Uploader = connector.AnonymousUploader

	cfg := fmt.Sprintf(`{"webhook_url":%q}`, srv.URL)
	if err := c.Deliver(context.Background(), instance(cfg), a); err != nil {
		t.Fatalf("Deliver: %v", err)
	}

	body := string(got.body)
	if strings.Contains(body, "alice") {
		t.Fatalf("request body leaked the real uploader: %s", body)
	}
	if !strings.Contains(body, connector.AnonymousUploader) {
		t.Fatalf("body = %s, want Anonymous", body)
	}
}

// A summary speaks for several torrents, so an embed titled after one of them
// would misrepresent it.
func TestDeliverCoalescedSendsPlainContent(t *testing.T) {
	srv, got := newReceiver(t, http.StatusNoContent, nil)
	c := newTestConnector(srv.URL)

	a := announcement()
	a.Coalesced = 5

	cfg := fmt.Sprintf(`{"webhook_url":%q}`, srv.URL)
	if err := c.Deliver(context.Background(), instance(cfg), a); err != nil {
		t.Fatalf("Deliver: %v", err)
	}

	var body payload
	if err := json.Unmarshal(got.body, &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Embeds) != 0 {
		t.Fatalf("got %d embeds, want none for a summary", len(body.Embeds))
	}
	if !strings.Contains(body.Content, "5 new torrents") {
		t.Fatalf("content = %q, want a +N summary", body.Content)
	}
	if strings.Contains(body.Content, "Some.Release-GROUP") {
		t.Fatalf("a summary must not name one torrent: %q", body.Content)
	}
}

func TestDeliverUsesACustomTemplateForTheDescription(t *testing.T) {
	srv, got := newReceiver(t, http.StatusNoContent, nil)
	c := newTestConnector(srv.URL)

	cfg := fmt.Sprintf(`{"webhook_url":%q,"template":"{{.Name}} in {{.Category}}"}`, srv.URL)
	if err := c.Deliver(context.Background(), instance(cfg), announcement()); err != nil {
		t.Fatalf("Deliver: %v", err)
	}

	var body payload
	if err := json.Unmarshal(got.body, &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Embeds[0].Description != "Some.Release-GROUP in Movies" {
		t.Fatalf("description = %q", body.Embeds[0].Description)
	}
}

func TestDeliverFailsOnNon2xx(t *testing.T) {
	srv, _ := newReceiver(t, http.StatusBadRequest, nil)
	c := newTestConnector(srv.URL)

	cfg := fmt.Sprintf(`{"webhook_url":%q}`, srv.URL)
	err := c.Deliver(context.Background(), instance(cfg), announcement())
	if err == nil {
		t.Fatal("expected a 400 to fail the delivery")
	}
	if !strings.Contains(err.Error(), "400") {
		t.Fatalf("error = %v, want it to name the status", err)
	}
}

func TestDeliverReportsRateLimiting(t *testing.T) {
	srv, _ := newReceiver(t, http.StatusTooManyRequests, map[string]string{"Retry-After": "3"})
	c := newTestConnector(srv.URL)

	cfg := fmt.Sprintf(`{"webhook_url":%q}`, srv.URL)
	err := c.Deliver(context.Background(), instance(cfg), announcement())
	if err == nil {
		t.Fatal("expected a 429 to fail the delivery so the pipeline backs off")
	}
	if !strings.Contains(err.Error(), "rate limited") {
		t.Fatalf("error = %v, want it to say rate limited", err)
	}
}

// The error is what lands in the delivery log for the retention window, and the
// URL is the credential — so it must not be in there, before or after redaction.
func TestDeliverErrorNeverContainsTheWebhookURL(t *testing.T) {
	for name, status := range map[string]int{"400": http.StatusBadRequest, "429": http.StatusTooManyRequests} {
		t.Run(name, func(t *testing.T) {
			srv, _ := newReceiver(t, status, nil)
			c := newTestConnector(srv.URL)
			cfg := json.RawMessage(fmt.Sprintf(`{"webhook_url":%q}`, srv.URL))

			err := c.Deliver(context.Background(), connector.Instance{ID: 1, Config: cfg}, announcement())
			if err == nil {
				t.Fatal("expected a failure")
			}
			if strings.Contains(err.Error(), srv.URL) {
				t.Fatalf("error leaked the webhook URL: %v", err)
			}
			if redacted := connector.RedactError(err, cfg, c.SecretFields()); strings.Contains(redacted, srv.URL) {
				t.Fatalf("redacted error leaked the webhook URL: %s", redacted)
			}
		})
	}
}

// A transport failure puts the URL in *url.Error, which is the subtlest way for
// the credential to reach the log.
func TestDeliverTransportErrorNeverContainsTheWebhookURL(t *testing.T) {
	c := New(httpguard.NewClient(func() bool { return true }, 500*time.Millisecond))
	c.requireHTTPS = false
	c.allowedHosts = []string{"127.0.0.1"}

	// Port 1 on loopback: nothing listens, so the dial fails fast.
	const target = "http://127.0.0.1:1/api/webhooks/1/supersecrettoken"
	cfg := json.RawMessage(fmt.Sprintf(`{"webhook_url":%q}`, target))

	err := c.Deliver(context.Background(), connector.Instance{ID: 1, Config: cfg}, announcement())
	if err == nil {
		t.Fatal("expected an unreachable endpoint to fail")
	}
	if strings.Contains(err.Error(), "supersecrettoken") || strings.Contains(err.Error(), target) {
		t.Fatalf("transport error leaked the webhook URL: %v", err)
	}
}

func TestValidateConfig(t *testing.T) {
	c := New(nil) // the real, strict policy

	valid := []string{
		`{"webhook_url":"https://discord.com/api/webhooks/1/abc"}`,
		`{"webhook_url":"https://discordapp.com/api/webhooks/1/abc","username":"Tracker"}`,
		`{"webhook_url":"https://ptb.discord.com/api/webhooks/1/abc"}`,
		`{"webhook_url":"https://discord.com/api/webhooks/1/abc","template":"{{.Name}}","rate_per_min":5}`,
	}
	for _, cfg := range valid {
		if err := c.ValidateConfig(json.RawMessage(cfg)); err != nil {
			t.Errorf("ValidateConfig(%s) = %v, want nil", cfg, err)
		}
	}

	invalid := map[string]string{
		"missing url": `{}`,
		// The URL is a bearer credential, so plaintext would hand it to anyone
		// on the path.
		"http not https": `{"webhook_url":"http://discord.com/api/webhooks/1/abc"}`,
		"wrong host":     `{"webhook_url":"https://evil.test/api/webhooks/1/abc"}`,
		"lookalike host": `{"webhook_url":"https://discord.com.evil.test/hook"}`,
		"credentials":    `{"webhook_url":"https://u:p@discord.com/api/webhooks/1/abc"}`,
		"bad avatar":     `{"webhook_url":"https://discord.com/api/webhooks/1/abc","avatar_url":"javascript:alert(1)"}`,
		"bad template":   `{"webhook_url":"https://discord.com/api/webhooks/1/abc","template":"{{.Nope}}"}`,
		"bad rate":       `{"webhook_url":"https://discord.com/api/webhooks/1/abc","rate_per_min":0}`,
		"malformed json": `nope`,
	}
	for name, cfg := range invalid {
		t.Run(name, func(t *testing.T) {
			if err := c.ValidateConfig(json.RawMessage(cfg)); err == nil {
				t.Fatalf("ValidateConfig(%s) = nil, want an error", cfg)
			}
		})
	}
}

// Discord allows roughly five requests per two seconds per webhook, and a drain
// can send a whole minute's budget back to back. Being rate limited is ordinary
// operation, so it must not spend one of the delivery's five attempts —
// otherwise a bulk import dead-letters half its announcements.
func TestDeliverRateLimitIsNotReadyRatherThanAFailure(t *testing.T) {
	srv, _ := newReceiver(t, http.StatusTooManyRequests, map[string]string{"Retry-After": "3"})
	c := newTestConnector(srv.URL)

	cfg := fmt.Sprintf(`{"webhook_url":%q}`, srv.URL)
	err := c.Deliver(context.Background(), instance(cfg), announcement())
	if !errors.Is(err, connector.ErrNotReady) {
		t.Fatalf("err = %v, want connector.ErrNotReady so the attempt is not burned", err)
	}
	if !strings.Contains(err.Error(), "retry after 3s") {
		t.Fatalf("error = %v, want it to carry Discord's own hint", err)
	}
}

// A bare status code is unactionable; Discord's body names the offending field.
func TestDeliverIncludesDiscordsOwnErrorMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"embeds":["0.description: Must be 4096 or fewer in length."]}`))
	}))
	defer srv.Close()

	c := newTestConnector(srv.URL)
	cfg := fmt.Sprintf(`{"webhook_url":%q}`, srv.URL)

	err := c.Deliver(context.Background(), instance(cfg), announcement())
	if err == nil {
		t.Fatal("expected a 400 to fail the delivery")
	}
	if !strings.Contains(err.Error(), "4096 or fewer") {
		t.Fatalf("error = %v, want Discord's own diagnosis included", err)
	}
}

// The default embed description must not simply repeat the title, URL and the
// category/size the embed already shows as fields.
func TestDeliverDefaultDescriptionIsTheKindsOwn(t *testing.T) {
	srv, got := newReceiver(t, http.StatusNoContent, nil)
	c := newTestConnector(srv.URL)

	cfg := fmt.Sprintf(`{"webhook_url":%q}`, srv.URL)
	if err := c.Deliver(context.Background(), instance(cfg), announcement()); err != nil {
		t.Fatalf("Deliver: %v", err)
	}

	var body payload
	if err := json.Unmarshal(got.body, &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Embeds[0].Description != "Some.Release-GROUP" {
		t.Fatalf("description = %q, want the Discord default rather than the shared line",
			body.Embeds[0].Description)
	}
}

// The destination policy is re-checked on delivery, not only at save time: a row
// restored from a backup could carry an http:// URL, and this credential must
// never travel in the clear.
func TestDeliverRefusesAStoredNonHTTPSURL(t *testing.T) {
	srv, got := newReceiver(t, http.StatusNoContent, nil)
	c := New(httpguard.NewClient(func() bool { return true }, 5*time.Second)) // strict policy

	cfg := fmt.Sprintf(`{"webhook_url":%q}`, srv.URL) // httptest is http://
	err := c.Deliver(context.Background(), instance(cfg), announcement())
	if !errors.Is(err, connector.ErrPermanent) {
		t.Fatalf("err = %v, want ErrPermanent: retrying will not make it https", err)
	}
	if got.method != "" {
		t.Fatal("nothing may be sent to a destination that failed the policy check")
	}
}

func TestDeliverRefusesAStoredNonDiscordHost(t *testing.T) {
	srv, got := newReceiver(t, http.StatusNoContent, nil)
	c := New(httpguard.NewClient(func() bool { return true }, 5*time.Second))
	c.requireHTTPS = false // isolate the host check

	cfg := fmt.Sprintf(`{"webhook_url":%q}`, srv.URL)
	if err := c.Deliver(context.Background(), instance(cfg), announcement()); !errors.Is(err, connector.ErrPermanent) {
		t.Fatalf("err = %v, want ErrPermanent for a non-Discord host", err)
	}
	if got.method != "" {
		t.Fatal("nothing may be sent to a host outside the allowlist")
	}
}

// The validation message reaches the admin API, and the URL is the credential.
func TestValidateConfigErrorNeverEchoesTheURL(t *testing.T) {
	c := New(nil)
	const secret = "https://discord.com/api/webhooks/1/s3cr3tHookToken%zz"

	err := c.ValidateConfig(json.RawMessage(fmt.Sprintf(`{"webhook_url":%q}`, secret)))
	if err == nil {
		t.Fatal("expected a malformed URL to be rejected")
	}
	if strings.Contains(err.Error(), "s3cr3tHookToken") {
		t.Fatalf("validation error echoed the credential: %v", err)
	}
}
