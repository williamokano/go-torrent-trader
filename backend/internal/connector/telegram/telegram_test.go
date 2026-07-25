package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/connector"
	"github.com/williamokano/go-torrent-trader/backend/internal/connector/httpguard"
)

const testToken = "123456:AAHsuperSecretBotToken"

type sentMessage struct {
	path   string
	chatID string
	text   string
	parse  string
	noPrev bool
}

type fakeAPI struct {
	mu       sync.Mutex
	messages []sentMessage
	// statusFor lets a test fail one specific chat.
	statusFor map[string]int
}

// newFakeAPI stands in for api.telegram.org. The token is in the request path,
// so intercepting a request means replacing the base URL — which is why the
// connector has one.
func newFakeAPI(t *testing.T) (*httptest.Server, *fakeAPI) {
	t.Helper()
	api := &fakeAPI{statusFor: map[string]int{}}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var parsed struct {
			ChatID    string `json:"chat_id"`
			Text      string `json:"text"`
			ParseMode string `json:"parse_mode"`
			NoPreview bool   `json:"disable_web_page_preview"`
		}
		_ = json.Unmarshal(body, &parsed)

		api.mu.Lock()
		api.messages = append(api.messages, sentMessage{
			path:   r.URL.Path,
			chatID: parsed.ChatID,
			text:   parsed.Text,
			parse:  parsed.ParseMode,
			noPrev: parsed.NoPreview,
		})
		status := api.statusFor[parsed.ChatID]
		api.mu.Unlock()

		if status != 0 {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"ok":false,"description":"chat not found"}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)
	return srv, api
}

func (a *fakeAPI) sent() []sentMessage {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]sentMessage, len(a.messages))
	copy(out, a.messages)
	return out
}

func newTestConnector(apiBase string) *Connector {
	c := New(httpguard.NewClient(func() bool { return true }, 5*time.Second))
	c.apiBase = apiBase
	return c
}

func instance(cfg string) connector.Instance {
	return connector.Instance{ID: 1, Name: "Telegram", Config: json.RawMessage(cfg)}
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

func config(chatIDs ...string) string {
	quoted := make([]string, len(chatIDs))
	for i, id := range chatIDs {
		quoted[i] = fmt.Sprintf("%q", id)
	}
	return fmt.Sprintf(`{"bot_token":%q,"chat_ids":[%s]}`, testToken, strings.Join(quoted, ","))
}

func TestKindAndShape(t *testing.T) {
	c := newTestConnector("https://api.telegram.org")

	if c.Kind() != "telegram" {
		t.Fatalf("Kind() = %q, want telegram", c.Kind())
	}
	if c.Singleton() {
		t.Fatal("several bots/chats are a normal configuration")
	}
	if !c.Coalescable() {
		t.Fatal("a Telegram chat is read by people, so it should coalesce")
	}
	if got := c.SecretFields(); len(got) != 1 || got[0] != "bot_token" {
		t.Fatalf("SecretFields() = %v, want [bot_token]", got)
	}
}

func TestDeliverSendsOneMessagePerChat(t *testing.T) {
	srv, api := newFakeAPI(t)
	c := newTestConnector(srv.URL)

	if err := c.Deliver(context.Background(), instance(config("-1001", "@channel")), announcement()); err != nil {
		t.Fatalf("Deliver: %v", err)
	}

	sent := api.sent()
	if len(sent) != 2 {
		t.Fatalf("sent %d messages, want one per chat", len(sent))
	}
	if sent[0].chatID != "-1001" || sent[1].chatID != "@channel" {
		t.Fatalf("chat ids = %q, %q", sent[0].chatID, sent[1].chatID)
	}
	for _, message := range sent {
		if message.parse != "HTML" {
			t.Fatalf("parse_mode = %q, want HTML", message.parse)
		}
		if !message.noPrev {
			t.Fatal("link previews should be disabled; the message already carries the link")
		}
		if message.path != "/bot"+testToken+"/sendMessage" {
			t.Fatalf("path = %q, want the sendMessage endpoint", message.path)
		}
		if !strings.Contains(message.text, "Some.Release-GROUP") {
			t.Fatalf("text = %q", message.text)
		}
		if !strings.Contains(message.text, "2.00 GiB") {
			t.Fatalf("text = %q, want the human size", message.text)
		}
	}
}

// parse_mode is HTML, and torrent names contain & and < routinely. Unescaped,
// Telegram rejects the whole message — so the announcement would never arrive.
func TestDeliverEscapesHTMLInInterpolatedFields(t *testing.T) {
	srv, api := newFakeAPI(t)
	c := newTestConnector(srv.URL)

	a := announcement()
	a.Name = "<b>x & y</b>"
	a.Uploader = "a<script>lert"

	if err := c.Deliver(context.Background(), instance(config("-1001")), a); err != nil {
		t.Fatalf("Deliver: %v", err)
	}

	text := api.sent()[0].text
	if strings.Contains(text, "<b>x & y</b>") {
		t.Fatalf("text = %q, want the name escaped", text)
	}
	if !strings.Contains(text, "&lt;b&gt;x &amp; y&lt;/b&gt;") {
		t.Fatalf("text = %q, want HTML-escaped name", text)
	}
	if strings.Contains(text, "<script>") {
		t.Fatalf("text = %q, want the uploader escaped", text)
	}
	// The template's own markup must survive: escaping the inputs is what makes
	// that possible.
	if !strings.Contains(text, "<b>") || !strings.Contains(text, "<a href=") {
		t.Fatalf("text = %q, want the template's own HTML intact", text)
	}
}

// A custom template must not be able to reintroduce the hole, which is why the
// fields are escaped rather than the rendered output.
func TestDeliverEscapesEvenWithACustomTemplate(t *testing.T) {
	srv, api := newFakeAPI(t)
	c := newTestConnector(srv.URL)

	a := announcement()
	a.Name = "x & <y>"

	cfg := fmt.Sprintf(`{"bot_token":%q,"chat_ids":["-1001"],"template":"{{.Name}}"}`, testToken)
	if err := c.Deliver(context.Background(), instance(cfg), a); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if got := api.sent()[0].text; got != "x &amp; &lt;y&gt;" {
		t.Fatalf("text = %q, want it escaped", got)
	}
}

func TestDeliverRendersAnonymousUploader(t *testing.T) {
	srv, api := newFakeAPI(t)
	c := newTestConnector(srv.URL)

	a := announcement()
	a.Anonymous = true
	a.Uploader = connector.AnonymousUploader

	cfg := fmt.Sprintf(`{"bot_token":%q,"chat_ids":["-1001"],"template":"{{.Name}} by {{.Uploader}}"}`, testToken)
	if err := c.Deliver(context.Background(), instance(cfg), a); err != nil {
		t.Fatalf("Deliver: %v", err)
	}

	text := api.sent()[0].text
	if strings.Contains(text, "alice") {
		t.Fatalf("text leaked the real uploader: %q", text)
	}
	if !strings.Contains(text, connector.AnonymousUploader) {
		t.Fatalf("text = %q, want Anonymous", text)
	}
}

func TestDeliverCoalescedDoesNotNameOneTorrent(t *testing.T) {
	srv, api := newFakeAPI(t)
	c := newTestConnector(srv.URL)

	a := announcement()
	a.Coalesced = 4

	if err := c.Deliver(context.Background(), instance(config("-1001")), a); err != nil {
		t.Fatalf("Deliver: %v", err)
	}

	text := api.sent()[0].text
	if !strings.Contains(text, "4 new torrents") {
		t.Fatalf("text = %q, want a +N summary", text)
	}
	if strings.Contains(text, "Some.Release-GROUP") {
		t.Fatalf("a summary must not name one torrent: %q", text)
	}
}

// One bad chat id must fail the delivery so the pipeline retries — otherwise a
// misconfigured chat silently receives nothing forever.
func TestDeliverFailsWhenOneChatFails(t *testing.T) {
	srv, api := newFakeAPI(t)
	api.statusFor["-1002"] = http.StatusBadRequest
	c := newTestConnector(srv.URL)

	err := c.Deliver(context.Background(), instance(config("-1001", "-1002")), announcement())
	if err == nil {
		t.Fatal("expected a failing chat to fail the delivery")
	}
	if !strings.Contains(err.Error(), "-1002") {
		t.Fatalf("error = %v, want it to name the failing chat", err)
	}
	// The other chat was still attempted, so a single bad id does not stop the
	// rest from being told.
	if len(api.sent()) != 2 {
		t.Fatalf("attempted %d chats, want both", len(api.sent()))
	}
}

// The token is in the request path, so this is the sharpest way for it to reach
// the delivery log.
func TestDeliverErrorNeverContainsTheToken(t *testing.T) {
	srv, api := newFakeAPI(t)
	api.statusFor["-1001"] = http.StatusUnauthorized
	c := newTestConnector(srv.URL)

	cfg := json.RawMessage(config("-1001"))
	err := c.Deliver(context.Background(), connector.Instance{ID: 1, Config: cfg}, announcement())
	if err == nil {
		t.Fatal("expected a 401 to fail the delivery")
	}
	if strings.Contains(err.Error(), testToken) {
		t.Fatalf("error leaked the bot token: %v", err)
	}
	if strings.Contains(err.Error(), srv.URL) {
		t.Fatalf("error leaked the request URL: %v", err)
	}
	if redacted := connector.RedactError(err, cfg, c.SecretFields()); strings.Contains(redacted, testToken) {
		t.Fatalf("redacted error leaked the bot token: %s", redacted)
	}
}

// A transport failure puts the whole URL — and therefore the token — into
// *url.Error.
func TestDeliverTransportErrorNeverContainsTheToken(t *testing.T) {
	c := New(httpguard.NewClient(func() bool { return true }, 500*time.Millisecond))
	// Port 1 on loopback: nothing listens, so the dial fails fast.
	c.apiBase = "http://127.0.0.1:1"

	cfg := json.RawMessage(config("-1001"))
	err := c.Deliver(context.Background(), connector.Instance{ID: 1, Config: cfg}, announcement())
	if err == nil {
		t.Fatal("expected an unreachable API to fail")
	}
	if strings.Contains(err.Error(), testToken) {
		t.Fatalf("transport error leaked the bot token: %v", err)
	}
	if redacted := connector.RedactError(err, cfg, c.SecretFields()); strings.Contains(redacted, testToken) {
		t.Fatalf("redacted transport error leaked the bot token: %s", redacted)
	}
}

func TestValidateConfig(t *testing.T) {
	c := newTestConnector("https://api.telegram.org")

	valid := []string{
		config("-1001"),
		config("-1001", "@channel"),
		fmt.Sprintf(`{"bot_token":%q,"chat_ids":["-1001"],"template":"{{.Name}}","rate_per_min":5}`, testToken),
	}
	for _, cfg := range valid {
		if err := c.ValidateConfig(json.RawMessage(cfg)); err != nil {
			t.Errorf("ValidateConfig(%s) = %v, want nil", cfg, err)
		}
	}

	invalid := map[string]string{
		"no token":       `{"chat_ids":["-1001"]}`,
		"no chats":       fmt.Sprintf(`{"bot_token":%q,"chat_ids":[]}`, testToken),
		"empty chat":     fmt.Sprintf(`{"bot_token":%q,"chat_ids":["  "]}`, testToken),
		"duplicate chat": fmt.Sprintf(`{"bot_token":%q,"chat_ids":["-1001","-1001"]}`, testToken),
		// The token goes straight into a URL path, so anything that could alter
		// the request shape is refused rather than escaped.
		"token with slash":   `{"bot_token":"abc/def","chat_ids":["-1001"]}`,
		"token with newline": `{"bot_token":"abc\ndef","chat_ids":["-1001"]}`,
		"token with query":   `{"bot_token":"abc?x=1","chat_ids":["-1001"]}`,
		"bad template":       fmt.Sprintf(`{"bot_token":%q,"chat_ids":["-1001"],"template":"{{.Nope}}"}`, testToken),
		"bad rate":           fmt.Sprintf(`{"bot_token":%q,"chat_ids":["-1001"],"rate_per_min":0}`, testToken),
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

func TestDefaultTemplateRenders(t *testing.T) {
	// The default is HTML, so a broken one would make every message fail at the
	// API rather than at save time.
	if err := connector.ValidateTemplate(DefaultTemplate); err != nil {
		t.Fatalf("the default template must be valid: %v", err)
	}
	if err := connector.ValidateTemplate(CoalescedTemplate); err != nil {
		t.Fatalf("the coalesced template must be valid: %v", err)
	}
}

// A permanently-broken chat would otherwise make every healthy chat receive the
// same announcement five times, for every torrent, forever.
func TestDeliverDeadLettersWhenEveryChatFailsPermanently(t *testing.T) {
	srv, api := newFakeAPI(t)
	api.statusFor["-1001"] = http.StatusBadRequest // "chat not found"
	c := newTestConnector(srv.URL)

	err := c.Deliver(context.Background(), instance(config("-1001")), announcement())
	if !errors.Is(err, connector.ErrPermanent) {
		t.Fatalf("err = %v, want ErrPermanent so the row dead-letters at once", err)
	}
	if !strings.Contains(err.Error(), "chat not found") {
		t.Fatalf("error = %v, want Telegram's own description", err)
	}
	_ = api
}

// But a mixed failure is still retried, because the transient one might succeed.
func TestDeliverRetriesWhenAnyChatFailureIsTransient(t *testing.T) {
	srv, api := newFakeAPI(t)
	api.statusFor["-1001"] = http.StatusBadRequest          // permanent
	api.statusFor["-1002"] = http.StatusInternalServerError // transient
	c := newTestConnector(srv.URL)

	err := c.Deliver(context.Background(), instance(config("-1001", "-1002")), announcement())
	if err == nil {
		t.Fatal("expected the delivery to fail")
	}
	if errors.Is(err, connector.ErrPermanent) {
		t.Fatalf("err = %v, want an ordinary retryable failure when one chat may recover", err)
	}
}

// Telegram's rate limit is ordinary operation, not a fault of this delivery.
func TestDeliverRateLimitIsNotReady(t *testing.T) {
	srv, api := newFakeAPI(t)
	api.statusFor["-1001"] = http.StatusTooManyRequests
	c := newTestConnector(srv.URL)

	err := c.Deliver(context.Background(), instance(config("-1001")), announcement())
	if !errors.Is(err, connector.ErrNotReady) {
		t.Fatalf("err = %v, want connector.ErrNotReady so the attempt is not burned", err)
	}
}

// Release names contain apostrophes and quotes routinely; html.EscapeString
// renders them as numeric references, and the quote matters because the default
// template puts the URL inside href="…".
func TestDeliverEscapesQuotesAndApostrophes(t *testing.T) {
	srv, api := newFakeAPI(t)
	c := newTestConnector(srv.URL)

	a := announcement()
	a.Name = `Don't "Look" & <Run>`

	if err := c.Deliver(context.Background(), instance(config("-1001")), a); err != nil {
		t.Fatalf("Deliver: %v", err)
	}

	text := api.sent()[0].text
	for _, raw := range []string{`Don't`, `"Look"`, `<Run>`} {
		if strings.Contains(text, raw) {
			t.Fatalf("text = %q, want %q escaped", text, raw)
		}
	}
	if !strings.Contains(text, "&#39;") || !strings.Contains(text, "&#34;") {
		t.Fatalf("text = %q, want numeric references for the quote characters", text)
	}
}

// A stored config that could never work is refused before anything is sent, and
// an empty chat list must not read as success.
func TestDeliverRefusesAnUnusableStoredConfig(t *testing.T) {
	srv, api := newFakeAPI(t)
	c := newTestConnector(srv.URL)

	cases := map[string]string{
		"no chats":  fmt.Sprintf(`{"bot_token":%q,"chat_ids":[]}`, testToken),
		"bad token": `{"bot_token":"abc/def","chat_ids":["-1001"]}`,
	}
	for name, cfg := range cases {
		t.Run(name, func(t *testing.T) {
			err := c.Deliver(context.Background(), instance(cfg), announcement())
			if !errors.Is(err, connector.ErrPermanent) {
				t.Fatalf("err = %v, want ErrPermanent", err)
			}
		})
	}
	if len(api.sent()) != 0 {
		t.Fatal("nothing may be sent for a config that cannot work")
	}
}

// A token pasted with a trailing space would otherwise produce /bot123:ABC%20/
// and 404 forever.
func TestDeliverTrimsTheToken(t *testing.T) {
	srv, api := newFakeAPI(t)
	c := newTestConnector(srv.URL)

	cfg := fmt.Sprintf(`{"bot_token":"  %s  ","chat_ids":["-1001"]}`, testToken)
	if err := c.Deliver(context.Background(), instance(cfg), announcement()); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if got := api.sent()[0].path; got != "/bot"+testToken+"/sendMessage" {
		t.Fatalf("path = %q, want the trimmed token", got)
	}
}
