package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/connector"
	"github.com/williamokano/go-torrent-trader/backend/internal/connector/httpguard"
)

// captured is what the fake receiver saw.
type captured struct {
	method  string
	headers http.Header
	body    []byte
}

// newReceiver starts a fake webhook endpoint. httptest binds loopback, so the
// guard is built with private networks permitted — the blocking behaviour is
// covered in the httpguard package's own tests.
func newReceiver(t *testing.T, status int) (*httptest.Server, *captured) {
	t.Helper()
	got := &captured{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got.method = r.Method
		got.headers = r.Header.Clone()
		got.body = body
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv, got
}

func newTestConnector() *Connector {
	c := New(httpguard.NewClient(func() bool { return true }, 5*time.Second))
	c.now = func() time.Time { return time.Unix(1700000000, 0) }
	return c
}

func announcement() connector.Announcement {
	return connector.Announcement{
		Event:       connector.EventTorrentPublished,
		Title:       "Some.Release-GROUP",
		Body:        "New torrent: Some.Release-GROUP",
		TorrentID:   7,
		Name:        "Some.Release-GROUP",
		InfoHashHex: "abc123",
		CategoryID:  3,
		Category:    "Movies",
		Size:        2 * 1024 * 1024 * 1024,
		FileCount:   1,
		Uploader:    "alice",
		URL:         "https://tracker.test/torrent/7",
		DeliveryKey: "torrent.published:7",
	}
}

func TestKindAndShape(t *testing.T) {
	c := newTestConnector()
	if c.Kind() != "webhook" {
		t.Fatalf("Kind() = %q, want webhook", c.Kind())
	}
	if c.Singleton() {
		t.Fatal("webhook must not be a singleton: several endpoints are normal")
	}
	if got := c.SecretFields(); len(got) != 1 || got[0] != "hmac_secret" {
		t.Fatalf("SecretFields() = %v, want [hmac_secret]", got)
	}
}

func TestDeliverPostsCanonicalAnnouncementJSON(t *testing.T) {
	srv, got := newReceiver(t, http.StatusOK)
	c := newTestConnector()

	cfg := fmt.Sprintf(`{"url":%q}`, srv.URL)
	inst := connector.Instance{ID: 1, Config: json.RawMessage(cfg)}
	if err := c.Deliver(context.Background(), inst, announcement()); err != nil {
		t.Fatalf("Deliver: %v", err)
	}

	if got.method != http.MethodPost {
		t.Fatalf("method = %s, want POST", got.method)
	}
	if ct := got.headers.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}
	if ev := got.headers.Get(HeaderEvent); ev != connector.EventTorrentPublished {
		t.Fatalf("%s = %q, want %q", HeaderEvent, ev, connector.EventTorrentPublished)
	}
	if d := got.headers.Get(HeaderDelivery); d != "torrent.published:7" {
		t.Fatalf("%s = %q, want the delivery key", HeaderDelivery, d)
	}
	ts, err := strconv.ParseInt(got.headers.Get(HeaderTimestamp), 10, 64)
	if err != nil || ts != 1700000000 {
		t.Fatalf("%s = %q, want a unix timestamp", HeaderTimestamp, got.headers.Get(HeaderTimestamp))
	}
	if got.headers.Get(HeaderSignature) != "" {
		t.Fatal("no signature header expected when no hmac_secret is configured")
	}

	var round connector.Announcement
	if err := json.Unmarshal(got.body, &round); err != nil {
		t.Fatalf("body is not an Announcement: %v (%s)", err, got.body)
	}
	if round.Name != "Some.Release-GROUP" || round.TorrentID != 7 || round.Category != "Movies" {
		t.Fatalf("body round-tripped to %+v", round)
	}
}

func TestDeliverSignsWithHMACWhenConfigured(t *testing.T) {
	srv, got := newReceiver(t, http.StatusOK)
	c := newTestConnector()

	const secret = "s3cr3t-value"
	cfg := fmt.Sprintf(`{"url":%q,"hmac_secret":%q}`, srv.URL, secret)
	inst := connector.Instance{ID: 1, Config: json.RawMessage(cfg)}
	if err := c.Deliver(context.Background(), inst, announcement()); err != nil {
		t.Fatalf("Deliver: %v", err)
	}

	timestamp := got.headers.Get(HeaderTimestamp)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(got.body)
	want := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if signature := got.headers.Get(HeaderSignature); !hmac.Equal([]byte(signature), []byte(want)) {
		t.Fatalf("%s = %q, want %q", HeaderSignature, signature, want)
	}
	// The secret itself must never travel — only a signature derived from it.
	if strings.Contains(string(got.body), secret) {
		t.Fatal("the request body contains the HMAC secret")
	}
	for name, values := range got.headers {
		for _, v := range values {
			if strings.Contains(v, secret) {
				t.Fatalf("header %s contains the HMAC secret", name)
			}
		}
	}
}

// The critical anonymity assertion: the real uploader name must appear nowhere
// in what leaves the process.
func TestDeliverNeverLeaksAnonymousUploader(t *testing.T) {
	srv, got := newReceiver(t, http.StatusOK)
	c := newTestConnector()

	a := announcement()
	a.Anonymous = true
	a.Uploader = connector.AnonymousUploader

	cfg := fmt.Sprintf(`{"url":%q}`, srv.URL)
	if err := c.Deliver(context.Background(), connector.Instance{ID: 1, Config: json.RawMessage(cfg)}, a); err != nil {
		t.Fatalf("Deliver: %v", err)
	}

	body := string(got.body)
	if !strings.Contains(body, `"uploader":"Anonymous"`) {
		t.Fatalf("body = %s, want uploader Anonymous", body)
	}
	if strings.Contains(body, "alice") {
		t.Fatalf("body leaked the real uploader: %s", body)
	}
}

func TestDeliverHonoursPutMethod(t *testing.T) {
	srv, got := newReceiver(t, http.StatusOK)
	c := newTestConnector()

	cfg := fmt.Sprintf(`{"url":%q,"method":"put"}`, srv.URL)
	if err := c.Deliver(context.Background(), connector.Instance{ID: 1, Config: json.RawMessage(cfg)}, announcement()); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if got.method != http.MethodPut {
		t.Fatalf("method = %s, want PUT", got.method)
	}
}

func TestDeliverSendsConfiguredHeaders(t *testing.T) {
	srv, got := newReceiver(t, http.StatusOK)
	c := newTestConnector()

	cfg := fmt.Sprintf(`{"url":%q,"headers":{"X-Tracker":"test"}}`, srv.URL)
	if err := c.Deliver(context.Background(), connector.Instance{ID: 1, Config: json.RawMessage(cfg)}, announcement()); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if v := got.headers.Get("X-Tracker"); v != "test" {
		t.Fatalf("X-Tracker = %q, want test", v)
	}
}

// A configured header must not be able to override the signature or the event
// headers the receiver relies on.
func TestConfiguredHeadersCannotOverrideAnnounceHeaders(t *testing.T) {
	srv, got := newReceiver(t, http.StatusOK)
	c := newTestConnector()

	cfg := fmt.Sprintf(`{"url":%q,"headers":{%q:"spoofed"}}`, srv.URL, HeaderEvent)
	if err := c.Deliver(context.Background(), connector.Instance{ID: 1, Config: json.RawMessage(cfg)}, announcement()); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if v := got.headers.Get(HeaderEvent); v != connector.EventTorrentPublished {
		t.Fatalf("%s = %q, want the real event type", HeaderEvent, v)
	}
}

func TestDeliverFailsOnNon2xx(t *testing.T) {
	srv, _ := newReceiver(t, http.StatusInternalServerError)
	c := newTestConnector()

	cfg := fmt.Sprintf(`{"url":%q}`, srv.URL)
	err := c.Deliver(context.Background(), connector.Instance{ID: 1, Config: json.RawMessage(cfg)}, announcement())
	if err == nil {
		t.Fatal("expected a 500 response to fail the delivery")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("error = %v, want it to name the status", err)
	}
}

// The error is what lands in the delivery log; it must not carry the secret or
// the endpoint's full URL.
func TestDeliverErrorDoesNotEchoSecret(t *testing.T) {
	srv, _ := newReceiver(t, http.StatusForbidden)
	c := newTestConnector()

	const secret = "s3cr3t-value"
	cfg := json.RawMessage(fmt.Sprintf(`{"url":%q,"hmac_secret":%q}`, srv.URL, secret))
	err := c.Deliver(context.Background(), connector.Instance{ID: 1, Config: cfg}, announcement())
	if err == nil {
		t.Fatal("expected a 403 response to fail the delivery")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error leaked the secret: %v", err)
	}
	if redacted := connector.RedactError(err, cfg, c.SecretFields()); strings.Contains(redacted, secret) {
		t.Fatalf("redacted error leaked the secret: %s", redacted)
	}
}

func TestValidateConfig(t *testing.T) {
	c := newTestConnector()

	valid := []string{
		`{"url":"https://example.test/hook"}`,
		`{"url":"http://example.test/hook","method":"PUT"}`,
		`{"url":"https://example.test/hook","headers":{"X-A":"b"}}`,
		`{"url":"https://example.test/hook","hmac_secret":"x","rate_per_min":5}`,
	}
	for _, cfg := range valid {
		if err := c.ValidateConfig(json.RawMessage(cfg)); err != nil {
			t.Errorf("ValidateConfig(%s) = %v, want nil", cfg, err)
		}
	}

	invalid := map[string]string{
		"missing url":        `{}`,
		"bad scheme":         `{"url":"file:///etc/passwd"}`,
		"credentials in url": `{"url":"https://u:p@example.test/hook"}`,
		"bad method":         `{"url":"https://example.test/hook","method":"DELETE"}`,
		"header injection":   `{"url":"https://example.test/hook","headers":{"X-A":"b\r\nX-Evil: yes"}}`,
		"bad header name":    `{"url":"https://example.test/hook","headers":{"X A":"b"}}`,
		"empty header name":  `{"url":"https://example.test/hook","headers":{"":"b"}}`,
		"bad rate":           `{"url":"https://example.test/hook","rate_per_min":0}`,
		"malformed json":     `nope`,
	}
	for name, cfg := range invalid {
		t.Run(name, func(t *testing.T) {
			if err := c.ValidateConfig(json.RawMessage(cfg)); err == nil {
				t.Fatalf("ValidateConfig(%s) = nil, want an error", cfg)
			}
		})
	}
}

func TestDeliverFailsWhenEndpointIsUnreachable(t *testing.T) {
	c := New(httpguard.NewClient(func() bool { return true }, 500*time.Millisecond))

	// Port 1 on loopback: nothing listens there, so the dial fails fast.
	cfg := json.RawMessage(`{"url":"http://127.0.0.1:1/hook"}`)
	if err := c.Deliver(context.Background(), connector.Instance{ID: 1, Config: cfg}, announcement()); err == nil {
		t.Fatal("expected an unreachable endpoint to fail the delivery")
	}
}

func TestSignIsStable(t *testing.T) {
	a := Sign("secret", "1700000000", []byte(`{"a":1}`))
	b := Sign("secret", "1700000000", []byte(`{"a":1}`))
	if a != b {
		t.Fatal("signing must be deterministic")
	}
	if !strings.HasPrefix(a, "sha256=") {
		t.Fatalf("signature = %q, want a sha256= prefix", a)
	}
	if Sign("other", "1700000000", []byte(`{"a":1}`)) == a {
		t.Fatal("a different secret must produce a different signature")
	}
	if Sign("secret", "1700000001", []byte(`{"a":1}`)) == a {
		t.Fatal("the timestamp must be part of the signed material")
	}
}
