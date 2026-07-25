package handler

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/connector"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
	"github.com/williamokano/go-torrent-trader/backend/internal/testutil"
)

// streamHarness runs a real HTTP server around the hub, because the behaviour
// under test — frames arriving one at a time on a connection that stays open —
// only exists over a real socket. httptest.ResponseRecorder would buffer.
type streamHarness struct {
	hub      *AnnounceHub
	server   *httptest.Server
	sessions *testutil.MemorySessionStore
}

func newStreamHarness(t *testing.T) *streamHarness {
	t.Helper()

	sessions := testutil.NewMemorySessionStore()
	hub := NewAnnounceHub(sessions)
	hub.heartbeat = 50 * time.Millisecond
	go hub.Run()

	server := httptest.NewServer(http.HandlerFunc(hub.HandleStream))
	t.Cleanup(server.Close)

	return &streamHarness{hub: hub, server: server, sessions: sessions}
}

func (h *streamHarness) login(t *testing.T, userID int64) string {
	t.Helper()
	token := "token-" + time.Now().Format("150405.000000000")
	if err := h.sessions.Create(&service.Session{
		UserID:      userID,
		AccessToken: token,
		Permissions: model.Permissions{},
		ExpiresAt:   time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("creating session: %v", err)
	}
	return token
}

// connect opens a stream and returns a reader plus a func to close it.
func (h *streamHarness) connect(t *testing.T, token string) (*bufio.Reader, func()) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.server.URL+"?token="+token, nil)
	if err != nil {
		cancel()
		t.Fatalf("building request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		t.Fatalf("connecting: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		cancel()
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}
	if resp.Header.Get("X-Accel-Buffering") != "no" {
		t.Error("X-Accel-Buffering: no is required or nginx holds every frame")
	}

	return bufio.NewReader(resp.Body), func() {
		cancel()
		_ = resp.Body.Close()
	}
}

// readSSEFrame reads up to a blank line, i.e. one SSE event.
func readSSEFrame(t *testing.T, r *bufio.Reader) string {
	t.Helper()
	var b strings.Builder
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			t.Fatalf("reading frame: %v (got %q so far)", err, b.String())
		}
		if line == "\n" {
			return b.String()
		}
		b.WriteString(line)
	}
}

func (h *streamHarness) waitForClients(t *testing.T, want int) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for h.hub.clientCount() != want {
		select {
		case <-deadline:
			t.Fatalf("hub has %d clients, want %d", h.hub.clientCount(), want)
		case <-time.After(time.Millisecond):
		}
	}
}

func sampleAnnouncement() connector.Announcement {
	return connector.Announcement{
		Event:      connector.EventTorrentPublished,
		Title:      "Some.Release-GROUP",
		TorrentID:  42,
		Name:       "Some.Release-GROUP",
		CategoryID: 3,
		Category:   "Movies",
		Size:       2 * 1024 * 1024 * 1024,
		Uploader:   "alice",
		URL:        "https://tracker.test/torrent/42",
	}
}

// --- auth ---

func TestAnnounceStreamRejectsMissingToken(t *testing.T) {
	h := newStreamHarness(t)

	resp, err := http.Get(h.server.URL)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestAnnounceStreamRejectsInvalidToken(t *testing.T) {
	h := newStreamHarness(t)

	resp, err := http.Get(h.server.URL + "?token=nonsense")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

// The token is checked against live sessions on every connect, so logging out
// stops the feed at the next reconnect rather than whenever the page is closed.
func TestAnnounceStreamRejectsALoggedOutToken(t *testing.T) {
	h := newStreamHarness(t)
	token := h.login(t, 1)

	h.sessions.Delete(token)

	resp, err := http.Get(h.server.URL + "?token=" + token)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 once the session is gone", resp.StatusCode)
	}
}

// --- streaming ---

func TestAnnounceStreamOpensWithARetryHint(t *testing.T) {
	h := newStreamHarness(t)
	reader, closeStream := h.connect(t, h.login(t, 1))
	defer closeStream()

	frame := readSSEFrame(t, reader)
	if !strings.HasPrefix(frame, "retry:") {
		t.Fatalf("first frame = %q, want a retry hint so the browser paces reconnects", frame)
	}
}

func TestAnnounceStreamDeliversABroadcast(t *testing.T) {
	h := newStreamHarness(t)
	reader, closeStream := h.connect(t, h.login(t, 1))
	defer closeStream()

	readSSEFrame(t, reader) // retry hint
	h.waitForClients(t, 1)

	payload, err := json.Marshal(sampleAnnouncement())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	h.hub.Broadcast("torrent.published:42", payload)

	frame := readSSEFrame(t, reader)
	if !strings.Contains(frame, "event: announcement") {
		t.Fatalf("frame = %q, want an announcement event", frame)
	}
	if !strings.Contains(frame, "id: torrent.published:42") {
		t.Fatalf("frame = %q, want the delivery key as the event id", frame)
	}

	var round connector.Announcement
	if err := json.Unmarshal([]byte(dataOf(frame)), &round); err != nil {
		t.Fatalf("frame data is not an Announcement: %v (%q)", err, frame)
	}
	if round.TorrentID != 42 || round.Name != "Some.Release-GROUP" {
		t.Fatalf("round-tripped to %+v", round)
	}
}

// The anonymity guarantee has to hold on the wire, not just in the payload the
// pipeline stored.
func TestAnnounceStreamNeverNamesAnAnonymousUploader(t *testing.T) {
	h := newStreamHarness(t)
	reader, closeStream := h.connect(t, h.login(t, 1))
	defer closeStream()

	readSSEFrame(t, reader)
	h.waitForClients(t, 1)

	a := sampleAnnouncement()
	a.Anonymous = true
	a.Uploader = connector.AnonymousUploader
	payload, _ := json.Marshal(a)
	h.hub.Broadcast("torrent.published:42", payload)

	frame := readSSEFrame(t, reader)
	if strings.Contains(frame, "alice") {
		t.Fatalf("frame leaked the real uploader: %q", frame)
	}
	if !strings.Contains(frame, connector.AnonymousUploader) {
		t.Fatalf("frame = %q, want Anonymous", frame)
	}
}

func TestAnnounceStreamSendsHeartbeats(t *testing.T) {
	h := newStreamHarness(t)
	reader, closeStream := h.connect(t, h.login(t, 1))
	defer closeStream()

	readSSEFrame(t, reader) // retry hint

	// The heartbeat is a comment line, which is what stops a proxy reaping an
	// idle stream on a quiet tracker.
	frame := readSSEFrame(t, reader)
	if !strings.HasPrefix(frame, ": ping") {
		t.Fatalf("frame = %q, want a heartbeat comment", frame)
	}
}

func TestAnnounceStreamUnregistersOnDisconnect(t *testing.T) {
	h := newStreamHarness(t)
	reader, closeStream := h.connect(t, h.login(t, 1))

	readSSEFrame(t, reader)
	h.waitForClients(t, 1)

	closeStream()

	// The client must be reaped, or every page close would leak a goroutine and
	// a buffer.
	h.waitForClients(t, 0)
}

// One person with the page pinned in twenty windows would otherwise multiply
// every announcement twenty times.
func TestAnnounceStreamCapsStreamsPerUser(t *testing.T) {
	h := newStreamHarness(t)
	token := h.login(t, 1)

	var closers []func()
	defer func() {
		for _, closeStream := range closers {
			closeStream()
		}
	}()

	for i := 0; i < sseMaxPerUser; i++ {
		reader, closeStream := h.connect(t, token)
		closers = append(closers, closeStream)
		readSSEFrame(t, reader)
		h.waitForClients(t, i+1)
	}

	resp, err := http.Get(h.server.URL + "?token=" + token)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429 past the per-user cap", resp.StatusCode)
	}

	// A different user is unaffected.
	other, closeOther := h.connect(t, h.login(t, 2))
	closers = append(closers, closeOther)
	readSSEFrame(t, other)
}

// Counting and then registering is a race two simultaneous connects can both
// win, which is exactly how someone would get past a cap meant to stop bulk
// stream opening.
func TestAnnounceHubCapIsAtomicUnderConcurrentConnects(t *testing.T) {
	hub := NewAnnounceHub(testutil.NewMemorySessionStore())

	const attempts = 50
	var wg sync.WaitGroup
	admitted := make(chan bool, attempts)
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			admitted <- hub.tryRegister(&sseClient{
				userID: 1, send: make(chan []byte, sseClientBuffer),
			})
		}()
	}
	wg.Wait()
	close(admitted)

	var accepted int
	for ok := range admitted {
		if ok {
			accepted++
		}
	}
	if accepted != sseMaxPerUser {
		t.Fatalf("admitted %d streams, want exactly the cap of %d", accepted, sseMaxPerUser)
	}
	if hub.clientCount() != sseMaxPerUser {
		t.Fatalf("hub holds %d clients, want %d", hub.clientCount(), sseMaxPerUser)
	}
}

// A client that stops consuming must be dropped rather than allowed to wedge
// the fan-out to everyone else.
//
// This works on the hub directly: over a real connection the kernel's socket
// buffer absorbs far more than the client buffer, so a stalled reader cannot be
// made to fall behind reliably — the drop logic is what needs asserting, not
// the operating system's buffering.
func TestAnnounceHubDropsAClientThatStopsConsuming(t *testing.T) {
	hub := NewAnnounceHub(testutil.NewMemorySessionStore())
	go hub.Run()

	const frames = sseClientBuffer * 4

	// The stalled client never reads, so its buffer fills. The other is given a
	// buffer bigger than the whole burst, which is what "keeping up" means here
	// — draining it from a goroutine instead would make the test depend on the
	// scheduler running that goroutine between broadcasts, and it would be
	// dropped too on an unlucky run.
	stalled := &sseClient{userID: 1, send: make(chan []byte, sseClientBuffer)}
	keepingUp := &sseClient{userID: 2, send: make(chan []byte, frames+1)}
	if !hub.tryRegister(stalled) || !hub.tryRegister(keepingUp) {
		t.Fatal("test setup: both clients should register")
	}

	payload, _ := json.Marshal(sampleAnnouncement())
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < frames; i++ {
			hub.Broadcast("torrent.published:1", payload)
		}
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Broadcast blocked on a client that stopped consuming")
	}

	waitFor(t, "the stalled client to be dropped", func() bool { return hub.clientCount() == 1 })

	hub.mu.RLock()
	_, survived := hub.clients[keepingUp]
	hub.mu.RUnlock()
	if !survived {
		t.Fatal("the client that kept up should not have been dropped")
	}
	if len(keepingUp.send) == 0 {
		t.Fatal("the surviving client should have received the announcements")
	}
}

// waitFor polls until cond holds or the test gives up.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for !cond() {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for %s", what)
		case <-time.After(time.Millisecond):
		}
	}
}

func TestAnnounceStreamRequiresAFlusher(t *testing.T) {
	h := newStreamHarness(t)
	token := h.login(t, 1)

	// A recorder that deliberately does not implement http.Flusher.
	rec := &noFlushWriter{header: http.Header{}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/announce-stream?token="+token, nil)
	h.hub.HandleStream(rec, req)

	if rec.status != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 when the writer cannot flush", rec.status)
	}
}

type noFlushWriter struct {
	header http.Header
	status int
}

func (w *noFlushWriter) Header() http.Header         { return w.header }
func (w *noFlushWriter) Write(p []byte) (int, error) { return len(p), nil }
func (w *noFlushWriter) WriteHeader(status int)      { w.status = status }

// --- framing ---

func TestSSEFrameSplitsMultilineData(t *testing.T) {
	// A blank line terminates an SSE event, so an embedded newline in the
	// payload would truncate it. Each line gets its own data: prefix.
	frame := string(sseFrame("announcement", "k", []byte("{\n  \"a\": 1\n}")))

	if strings.Count(frame, "data: ") != 3 {
		t.Fatalf("frame = %q, want one data: line per payload line", frame)
	}
	if !strings.HasSuffix(frame, "\n\n") {
		t.Fatalf("frame = %q, want a terminating blank line", frame)
	}
}

// A newline in a field value would start a second SSE field, letting an id
// forge an event type or terminate the frame early.
func TestSSEFrameSanitisesFieldValues(t *testing.T) {
	frame := string(sseFrame("announcement", "bad\nid: spoofed", []byte("{}")))

	var idLines int
	for _, line := range strings.Split(frame, "\n") {
		if strings.HasPrefix(line, "id: ") {
			idLines++
		}
	}
	if idLines != 1 {
		t.Fatalf("frame = %q, want exactly one id field", frame)
	}
	if strings.Contains(frame, "\nid: spoofed") {
		t.Fatalf("frame = %q, want the injected field neutralised", frame)
	}
}

func TestSSEFrameOmitsAnEmptyID(t *testing.T) {
	frame := string(sseFrame("announcement", "", []byte("{}")))
	if strings.Contains(frame, "id:") {
		t.Fatalf("frame = %q, want no id line when there is no id", frame)
	}
}

// dataOf joins the data: lines of a frame back into the payload.
func dataOf(frame string) string {
	var b strings.Builder
	for _, line := range strings.Split(frame, "\n") {
		if after, ok := strings.CutPrefix(line, "data: "); ok {
			b.WriteString(after)
		}
	}
	return b.String()
}

// SSE treats a bare CR as a line break too, so it must not be able to terminate
// a frame early and let the remainder be read as fields.
func TestSSEFrameNeutralisesBareCarriageReturns(t *testing.T) {
	frame := string(sseFrame("announcement", "k", []byte("{\"a\":\"x\rid: spoofed\"}")))

	var dataLines, idLines int
	for _, line := range strings.Split(frame, "\n") {
		if strings.HasPrefix(line, "data: ") {
			dataLines++
		}
		if strings.HasPrefix(line, "id: ") {
			idLines++
		}
	}
	if dataLines != 1 {
		t.Fatalf("frame = %q, want the CR neutralised into a single data line", frame)
	}
	if idLines != 1 {
		t.Fatalf("frame = %q, want exactly the one real id field", frame)
	}
	if strings.Contains(frame, "\r") {
		t.Fatalf("frame = %q, want no carriage returns", frame)
	}
}
