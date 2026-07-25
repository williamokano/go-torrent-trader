package irc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/williamokano/go-torrent-trader/backend/internal/connector"
)

// fakeConn stands in for a live IRC connection.
type fakeConn struct {
	mu           sync.Mutex
	connected    bool
	joined       []string
	sent         []sentLine
	sendErr      error
	onConnect    func()
	onDisconnect func()
	quit         bool
	loopDone     chan struct{}
}

type sentLine struct {
	target string
	text   string
}

func newFakeConn() *fakeConn {
	return &fakeConn{connected: true, loopDone: make(chan struct{})}
}

func (f *fakeConn) Connect() error {
	if f.onConnect != nil {
		f.onConnect()
	}
	return nil
}

func (f *fakeConn) Loop() { <-f.loopDone }

func (f *fakeConn) Join(channel string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.joined = append(f.joined, channel)
	return nil
}

func (f *fakeConn) Privmsg(target, message string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.sendErr != nil {
		return f.sendErr
	}
	f.sent = append(f.sent, sentLine{target: target, text: message})
	return nil
}

func (f *fakeConn) Connected() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.connected
}

func (f *fakeConn) Quit() {
	f.mu.Lock()
	f.quit = true
	f.mu.Unlock()
	close(f.loopDone)
}

func (f *fakeConn) OnConnect(fn func())    { f.onConnect = fn }
func (f *fakeConn) OnDisconnect(fn func()) { f.onDisconnect = fn }

func (f *fakeConn) lines() []sentLine {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]sentLine, len(f.sent))
	copy(out, f.sent)
	return out
}

// newTestConnector wires the connector to a fake client already registered as
// the live connection for instance 1, so Deliver can be exercised directly.
// newTestConnector wires the connector to a fake client already published and
// registered for instance 1, so Deliver can be exercised directly.
func newTestConnector() (*Connector, *fakeConn) {
	conn := newFakeConn()
	c := New()
	c.dial = func(Config) ircConn { return conn }
	c.sendDelay = 0
	entry := &client{conn: conn}
	entry.ready.Store(true)
	c.clients[1] = entry
	return c, conn
}

func instance(config string) connector.Instance {
	return connector.Instance{ID: 1, Name: "Freenode", Config: json.RawMessage(config)}
}

func announcement() connector.Announcement {
	return connector.Announcement{
		Event:      connector.EventTorrentPublished,
		TorrentID:  7,
		Name:       "Some.Release-GROUP",
		CategoryID: 2,
		Category:   "Movies",
		Size:       2 * 1024 * 1024 * 1024,
		Uploader:   "alice",
		URL:        "https://tracker.test/torrent/7",
	}
}

const twoChannelConfig = `{
	"server":"irc.test","port":6697,"tls":true,"nick":"announce",
	"channels":[{"name":"#all"},{"name":"#movies","categories":[2]},{"name":"#tv","categories":[3]}]
}`

func TestKindAndShape(t *testing.T) {
	c, _ := newTestConnector()
	if c.Kind() != "irc" {
		t.Fatalf("Kind() = %q, want irc", c.Kind())
	}
	if c.Singleton() {
		t.Fatal("several IRC networks are a normal configuration")
	}
	if !c.Coalescable() {
		t.Fatal("an IRC channel is read by people, so it should coalesce")
	}
	secrets := c.SecretFields()
	if len(secrets) != 2 || secrets[0] != "sasl_pass" || secrets[1] != "nickserv_pass" {
		t.Fatalf("SecretFields() = %v, want both passwords", secrets)
	}
}

// One connection can feed several channels, each narrowed to its own
// categories — the reason routing lives here rather than in instance filters.
func TestDeliverRoutesByChannelCategory(t *testing.T) {
	c, conn := newTestConnector()

	if err := c.Deliver(context.Background(), instance(twoChannelConfig), announcement()); err != nil {
		t.Fatalf("Deliver: %v", err)
	}

	var targets []string
	for _, line := range conn.lines() {
		targets = append(targets, line.target)
	}
	if len(targets) != 2 || targets[0] != "#all" || targets[1] != "#movies" {
		t.Fatalf("sent to %v, want [#all #movies] — #tv is category 3 only", targets)
	}
}

func TestDeliverSkipsWhenNoChannelMatches(t *testing.T) {
	c, conn := newTestConnector()

	a := announcement()
	a.CategoryID = 99
	cfg := `{"server":"irc.test","port":6697,"nick":"announce","channels":[{"name":"#movies","categories":[2]}]}`
	if err := c.Deliver(context.Background(), instance(cfg), a); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if len(conn.lines()) != 0 {
		t.Fatal("no channel wanted this category, so nothing should have been sent")
	}
}

func TestDeliverRendersTheDefaultLine(t *testing.T) {
	c, conn := newTestConnector()

	a := announcement()
	a.Freeleech = true
	cfg := `{"server":"irc.test","port":6697,"nick":"announce","channels":[{"name":"#all"}]}`
	if err := c.Deliver(context.Background(), instance(cfg), a); err != nil {
		t.Fatalf("Deliver: %v", err)
	}

	line := conn.lines()[0].text
	for _, want := range []string{"Movies", "Some.Release-GROUP", "2.00 GiB", "[FL]", "https://tracker.test/torrent/7"} {
		if !strings.Contains(line, want) {
			t.Fatalf("line %q is missing %q", line, want)
		}
	}
}

// The one that matters: a torrent name is chosen by whoever uploaded it, and
// IRC is newline-delimited — a name carrying CR/LF would end the PRIVMSG and
// let the rest be read as a fresh command from our authenticated bot.
func TestDeliverStripsCRLFInjection(t *testing.T) {
	c, conn := newTestConnector()

	a := announcement()
	a.Name = "innocent\r\nJOIN #evil\r\nPRIVMSG #ops :owned"

	cfg := `{"server":"irc.test","port":6697,"nick":"announce","channels":[{"name":"#all"}]}`
	if err := c.Deliver(context.Background(), instance(cfg), a); err != nil {
		t.Fatalf("Deliver: %v", err)
	}

	lines := conn.lines()
	if len(lines) != 1 {
		t.Fatalf("sent %d messages, want exactly 1: a name must not be able to split the line", len(lines))
	}
	if strings.ContainsAny(lines[0].text, "\r\n") {
		t.Fatalf("line still contains a newline: %q", lines[0].text)
	}
	if strings.Contains(lines[0].text, "JOIN #evil") && strings.Contains(lines[0].text, "\n") {
		t.Fatalf("injected command survived as a separate line: %q", lines[0].text)
	}
}

// A custom template must not be able to reintroduce the hole, which is why the
// fields are sanitised rather than the rendered output.
func TestDeliverStripsControlCharactersEvenWithACustomTemplate(t *testing.T) {
	c, conn := newTestConnector()

	a := announcement()
	a.Name = "bad\r\nname"
	a.Uploader = "up\nloader"

	cfg := `{"server":"irc.test","port":6697,"nick":"announce","channels":[{"name":"#all"}],
		"template":"{{.Name}} by {{.Uploader}}"}`
	if err := c.Deliver(context.Background(), instance(cfg), a); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if strings.ContainsAny(conn.lines()[0].text, "\r\n") {
		t.Fatalf("custom template leaked a newline: %q", conn.lines()[0].text)
	}
}

func TestDeliverRendersAnonymousUploader(t *testing.T) {
	c, conn := newTestConnector()

	a := announcement()
	a.Anonymous = true
	a.Uploader = connector.AnonymousUploader

	cfg := `{"server":"irc.test","port":6697,"nick":"announce","channels":[{"name":"#all"}],
		"template":"{{.Name}} by {{.Uploader}}"}`
	if err := c.Deliver(context.Background(), instance(cfg), a); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	line := conn.lines()[0].text
	if !strings.Contains(line, connector.AnonymousUploader) {
		t.Fatalf("line %q should say Anonymous", line)
	}
	if strings.Contains(line, "alice") {
		t.Fatalf("line leaked the real uploader: %q", line)
	}
}

// The default template puts the URL last, so truncating the finished line would
// drop the one part a reader needs. The name is shortened instead.
func TestDeliverTruncatesTheNameAndKeepsTheURL(t *testing.T) {
	c, conn := newTestConnector()

	a := announcement()
	a.Name = strings.Repeat("x", 900)

	cfg := `{"server":"irc.test","port":6697,"nick":"announce","channels":[{"name":"#all"}]}`
	if err := c.Deliver(context.Background(), instance(cfg), a); err != nil {
		t.Fatalf("Deliver: %v", err)
	}

	line := conn.lines()[0].text
	if got := len(line); got > maxLineBytes {
		t.Fatalf("line is %d bytes, want at most %d", got, maxLineBytes)
	}
	if !strings.Contains(line, a.URL) {
		t.Fatalf("truncation dropped the URL, which is the point of the line: %q", line)
	}
	if !strings.Contains(line, "Movies") {
		t.Fatalf("truncation dropped the category: %q", line)
	}
}

// A summary stands for a batch that may span several categories, so routing it
// by the representative row's category would silently drop it for every other
// channel — and the rows would still be closed as "coalesced", i.e. success.
func TestDeliverCoalescedGoesToEveryChannel(t *testing.T) {
	c, conn := newTestConnector()

	a := announcement()
	a.Coalesced = 4
	a.CategoryID = 2 // matches only #movies

	if err := c.Deliver(context.Background(), instance(twoChannelConfig), a); err != nil {
		t.Fatalf("Deliver: %v", err)
	}

	var targets []string
	for _, line := range conn.lines() {
		targets = append(targets, line.target)
	}
	if len(targets) != 3 {
		t.Fatalf("summary went to %v, want all three channels", targets)
	}
}

func TestDeliverCoalescedDoesNotNameOneTorrent(t *testing.T) {
	c, conn := newTestConnector()

	a := announcement()
	a.Coalesced = 5

	cfg := `{"server":"irc.test","port":6697,"nick":"announce","channels":[{"name":"#all"}]}`
	if err := c.Deliver(context.Background(), instance(cfg), a); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	line := conn.lines()[0].text
	if !strings.Contains(line, "5 new torrents") {
		t.Fatalf("line %q, want a +N summary", line)
	}
	if strings.Contains(line, "Some.Release-GROUP") {
		t.Fatalf("a summary must not name one torrent: %q", line)
	}
}

// A reconnect is routine. Reporting it as a delivery failure would burn the
// five-attempt budget and dead-letter a queue of perfectly good announcements,
// so it is reported as "not ready" instead.
func TestDeliverWithoutAConnectionIsNotReady(t *testing.T) {
	c := New()
	cfg := `{"server":"irc.test","port":6697,"nick":"announce","channels":[{"name":"#all"}]}`

	err := c.Deliver(context.Background(), instance(cfg), announcement())
	if !errors.Is(err, connector.ErrNotReady) {
		t.Fatalf("err = %v, want connector.ErrNotReady", err)
	}
}

// The library reports its socket as connected during the CAP/SASL handshake,
// when the server answers a PRIVMSG with ERR_NOTREGISTERED and throws it away —
// while Privmsg still returns nil, so the row would be marked sent and the
// announcement lost with no trace. Readiness is therefore tracked separately.
func TestDeliverBeforeRegistrationIsNotReady(t *testing.T) {
	conn := newFakeConn()
	c := New()
	c.sendDelay = 0
	entry := &client{conn: conn} // socket up, registration not finished
	c.clients[1] = entry

	if !conn.Connected() {
		t.Fatal("test setup: the fake socket should report connected")
	}

	cfg := `{"server":"irc.test","port":6697,"nick":"announce","channels":[{"name":"#all"}]}`
	err := c.Deliver(context.Background(), instance(cfg), announcement())
	if !errors.Is(err, connector.ErrNotReady) {
		t.Fatalf("err = %v, want ErrNotReady while the socket is up but unregistered", err)
	}
	if len(conn.lines()) != 0 {
		t.Fatal("nothing may be written before registration completes")
	}
}

func TestDeliverAfterDisconnectIsNotReady(t *testing.T) {
	c, _ := newTestConnector()
	c.clients[1].ready.Store(false)

	cfg := `{"server":"irc.test","port":6697,"nick":"announce","channels":[{"name":"#all"}]}`
	err := c.Deliver(context.Background(), instance(cfg), announcement())
	if !errors.Is(err, connector.ErrNotReady) {
		t.Fatalf("err = %v, want connector.ErrNotReady", err)
	}
}

func TestDeliverPropagatesSendFailure(t *testing.T) {
	c, conn := newTestConnector()
	conn.sendErr = errors.New("socket closed")

	cfg := `{"server":"irc.test","port":6697,"nick":"announce","channels":[{"name":"#all"}]}`
	if err := c.Deliver(context.Background(), instance(cfg), announcement()); err == nil {
		t.Fatal("expected a send failure to surface so the delivery retries")
	}
}

// The admin badge should mean "able to announce", which is registration, not
// merely a socket.
func TestConnectedReportsRegistration(t *testing.T) {
	c, _ := newTestConnector()

	if !c.Connected(1) {
		t.Fatal("expected instance 1 to report connected")
	}
	c.clients[1].ready.Store(false)
	if c.Connected(1) {
		t.Fatal("an unregistered client must not report connected")
	}
	if c.Connected(2) {
		t.Fatal("an instance with no client must not report connected")
	}
}

func TestValidateConfig(t *testing.T) {
	c := New()

	valid := []string{
		`{"server":"irc.test","port":6697,"tls":true,"nick":"announce","channels":[{"name":"#all"}]}`,
		`{"server":"irc.test","port":6667,"nick":"bot","channels":[{"name":"&local","categories":[1,2]}],"rate_per_min":10}`,
		`{"server":"irc.test","port":6697,"nick":"bot","channels":[{"name":"#a"}],"template":"{{.Name}}"}`,
	}
	for _, cfg := range valid {
		if err := c.ValidateConfig(json.RawMessage(cfg)); err != nil {
			t.Errorf("ValidateConfig(%s) = %v, want nil", cfg, err)
		}
	}

	invalid := map[string]string{
		"no server":       `{"port":6697,"nick":"a","channels":[{"name":"#a"}]}`,
		"no nick":         `{"server":"irc.test","port":6697,"channels":[{"name":"#a"}]}`,
		"port zero":       `{"server":"irc.test","port":0,"nick":"a","channels":[{"name":"#a"}]}`,
		"port too high":   `{"server":"irc.test","port":70000,"nick":"a","channels":[{"name":"#a"}]}`,
		"no channels":     `{"server":"irc.test","port":6697,"nick":"a","channels":[]}`,
		"bad channel":     `{"server":"irc.test","port":6697,"nick":"a","channels":[{"name":"movies"}]}`,
		"channel newline": `{"server":"irc.test","port":6697,"nick":"a","channels":[{"name":"#a\nJOIN #b"}]}`,
		"duplicate":       `{"server":"irc.test","port":6697,"nick":"a","channels":[{"name":"#a"},{"name":"#a"}]}`,
		"nick with space": `{"server":"irc.test","port":6697,"nick":"a b","channels":[{"name":"#a"}]}`,
		"bad template":    `{"server":"irc.test","port":6697,"nick":"a","channels":[{"name":"#a"}],"template":"{{.Nope}}"}`,
		"bad rate":        `{"server":"irc.test","port":6697,"nick":"a","channels":[{"name":"#a"}],"rate_per_min":0}`,
		// More channels than the per-delivery timeout can pace through would
		// dead-letter every announcement, so it is refused at save time.
		"too many channels": `{"server":"irc.test","port":6697,"nick":"a","channels":[
			{"name":"#a"},{"name":"#b"},{"name":"#c"},{"name":"#d"},{"name":"#e"},
			{"name":"#f"},{"name":"#g"},{"name":"#h"},{"name":"#i"}]}`,
		"malformed": `nope`,
	}
	for name, cfg := range invalid {
		t.Run(name, func(t *testing.T) {
			if err := c.ValidateConfig(json.RawMessage(cfg)); err == nil {
				t.Fatalf("ValidateConfig(%s) = nil, want an error", cfg)
			}
		})
	}
}

// Start joins on connect (so a reconnect re-joins), publishes the client for
// Deliver, and shuts down cleanly when its context ends.
func TestStartJoinsChannelsAndStopsOnContextCancel(t *testing.T) {
	conn := newFakeConn()
	c := New()
	c.dial = func(Config) ircConn { return conn }

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- c.Start(ctx, instance(twoChannelConfig))
	}()

	waitUntil(t, "the client to register", func() bool { return c.Connected(1) })

	conn.mu.Lock()
	joined := append([]string(nil), conn.joined...)
	conn.mu.Unlock()
	if len(joined) != 3 {
		t.Fatalf("joined %v, want all three configured channels", joined)
	}

	cancel()
	select {
	case <-errCh:
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after its context was cancelled")
	}

	conn.mu.Lock()
	quit := conn.quit
	conn.mu.Unlock()
	if !quit {
		t.Fatal("expected a QUIT to be sent on shutdown")
	}
	// The client must be forgotten, or Deliver would keep writing to a dead one.
	if c.Connected(1) {
		t.Fatal("client should have been dropped after Start returned")
	}
}

// A NickServ password that is collected but never sent is worse than not
// offering the field: on a +r channel the server silently drops the bot's
// messages while Privmsg returns nil, so every delivery is marked sent and
// nothing arrives.
func TestStartIdentifiesToNickServ(t *testing.T) {
	conn := newFakeConn()
	c := New()
	c.dial = func(Config) ircConn { return conn }

	cfg := `{"server":"irc.test","port":6697,"nick":"announce","nickserv_pass":"hunter2",
		"channels":[{"name":"#all"}]}`

	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = c.Start(ctx, instance(cfg)) }()

	waitUntil(t, "identification", func() bool {
		for _, line := range conn.lines() {
			if line.target == "NickServ" {
				return true
			}
		}
		return false
	})
	cancel()

	for _, line := range conn.lines() {
		if line.target == "NickServ" {
			if line.text != "IDENTIFY hunter2" {
				t.Fatalf("sent %q, want an IDENTIFY", line.text)
			}
			return
		}
	}
}

// Registration, not the socket, is what makes an instance deliverable — and a
// disconnect has to take it back.
func TestStartTracksRegistrationLifecycle(t *testing.T) {
	conn := newFakeConn()
	c := New()
	c.dial = func(Config) ircConn { return conn }

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = c.Start(ctx, instance(twoChannelConfig)) }()

	waitUntil(t, "registration", func() bool { return c.Connected(1) })

	conn.onDisconnect()
	if c.Connected(1) {
		t.Fatal("a disconnect must make the instance stop reporting connected")
	}

	// A reconnect re-registers, re-joins and becomes deliverable again.
	conn.onConnect()
	if !c.Connected(1) {
		t.Fatal("re-registration should make the instance deliverable again")
	}
}

func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for !cond() {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for %s", what)
		case <-time.After(time.Millisecond):
		}
	}
}

func TestStartRejectsBadConfig(t *testing.T) {
	c := New()
	if err := c.Start(context.Background(), instance(`nope`)); err == nil {
		t.Fatal("expected Start to reject an unparseable config")
	}
}

// The line-level pass in renderLine would hide a missing field-level pass, so
// this asserts the field-level guard on its own. It is the one that matters:
// a custom template can put a name anywhere, and only sanitised inputs make
// every template safe.
func TestSanitizeAnnouncementCleansEveryTemplateField(t *testing.T) {
	dirty := connector.Announcement{
		Name:     "na\r\nme",
		Category: "ca\nt",
		Uploader: "up\rloader",
		URL:      "https://tracker.test/\r\ntorrent/7",
	}

	clean := sanitizeAnnouncement(dirty)

	for field, value := range map[string]string{
		"Name":     clean.Name,
		"Category": clean.Category,
		"Uploader": clean.Uploader,
		"URL":      clean.URL,
	} {
		if strings.ContainsAny(value, "\r\n") {
			t.Errorf("%s still contains a line break: %q", field, value)
		}
	}
	if clean.Name != "name" {
		t.Errorf("Name = %q, want the breaks removed and nothing else", clean.Name)
	}
}

func TestSanitizeLine(t *testing.T) {
	cases := map[string]string{
		"plain":          "plain",
		"with\rcr":       "withcr",
		"with\nlf":       "withlf",
		"with\x00null":   "withnull",
		"with\x03colour": "withcolour",
		"unicode ✓ kept": "unicode ✓ kept",
	}
	for in, want := range cases {
		if got := sanitizeLine(in); got != want {
			t.Errorf("sanitizeLine(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTruncateTextKeepsRunesIntact(t *testing.T) {
	// Multi-byte runes right at the boundary must not be cut in half.
	in := strings.Repeat("é", 300)
	got := truncateText(in, 100)
	if len(got) > 100 {
		t.Fatalf("len = %d, want at most 100", len(got))
	}
	if !utf8.ValidString(got) {
		t.Fatalf("truncation produced invalid UTF-8: %q", got)
	}
	if truncateText("short", 100) != "short" {
		t.Fatal("a line within budget must be returned unchanged")
	}
	// A budget too small for the ellipsis must not panic.
	if got := truncateText("abcdef", 2); len(got) > 2 {
		t.Fatalf("truncateText(_, 2) = %q", got)
	}
	if truncateText("abc", 0) != "" {
		t.Fatal("a zero budget yields nothing")
	}
}

func TestNewErgoConnAppliesTLSAndSASL(t *testing.T) {
	conn := newErgoConn(Config{
		Server: "irc.test", Port: 6697, TLS: true, Nick: "bot",
		SASLUser: "bot", SASLPass: "hunter2",
	})
	ergo, ok := conn.(*ergoConn)
	if !ok {
		t.Fatalf("conn = %T, want *ergoConn", conn)
	}
	if ergo.conn.Server != fmt.Sprintf("irc.test:%d", 6697) {
		t.Fatalf("Server = %q", ergo.conn.Server)
	}
	if !ergo.conn.UseTLS || ergo.conn.TLSConfig == nil {
		t.Fatal("TLS not configured")
	}
	if !ergo.conn.UseSASL || ergo.conn.SASLLogin != "bot" {
		t.Fatal("SASL not configured")
	}
}
