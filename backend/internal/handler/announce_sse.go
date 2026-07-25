package handler

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/williamokano/go-torrent-trader/backend/internal/connector/sse"
	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

const (
	// sseClientBuffer is how far behind a client may fall before it is dropped.
	// Small on purpose: a browser that cannot keep up with sixteen queued
	// announcements is not going to catch up, and holding them costs memory on
	// the server for a page nobody is watching.
	sseClientBuffer = 16
	// sseHeartbeat keeps proxies and load balancers from timing out an idle
	// stream. A tracker can be quiet for hours between uploads.
	sseHeartbeat = 25 * time.Second
	// sseRetryHint tells the browser how long to wait before reconnecting.
	// EventSource reconnects on its own; this just paces it.
	sseRetryHint = 5 * time.Second
	// sseMaxPerUser bounds fan-out. Without it one person with a pinned tab in
	// twenty windows multiplies every announcement twenty times.
	sseMaxPerUser = 5
)

// FeedAccess reports whether a member may watch live feeds. A narrow interface
// so the hub does not depend on the whole service layer.
type FeedAccess interface {
	Allowed(ctx context.Context, userID int64) (bool, error)
}

// sseClient is one connected browser watching one feed.
type sseClient struct {
	userID int64
	feed   string
	send   chan []byte
	// closeOnce guards send: the hub closes it when reaping a client, and the
	// handler closes it if the hub is not responding, so both paths have to be
	// safe.
	closeOnce sync.Once
}

func (c *sseClient) close() {
	c.closeOnce.Do(func() { close(c.send) })
}

// AnnounceHub fans announcements out to browsers over server-sent events.
//
// It is modelled directly on ChatHub, and shares its limitation: fan-out only
// reaches clients connected to the node that ran the delivery. With the
// single-process default that is every client. A multi-node deployment would
// need Redis pub/sub between the hubs — deliberately not built yet.
type AnnounceHub struct {
	sessionStore service.SessionStore
	access       FeedAccess

	// clients is keyed by feed slug. A frame produced by one feed must never
	// reach another's watchers: the whole point of several feeds is that each
	// carries only what its own filters allow.
	clients   map[string]map[*sseClient]struct{}
	broadcast chan feedFrame
	mu        sync.RWMutex

	// heartbeat is a field so tests need not wait twenty-five seconds.
	heartbeat time.Duration
}

// feedFrame is one rendered SSE frame together with the feed it belongs to.
type feedFrame struct {
	feed  string
	frame []byte
}

// NewAnnounceHub creates an AnnounceHub.
//
// A nil access check refuses everyone. This is an authorization gate, so a
// wiring mistake has to be a locked door rather than a privilege that quietly
// stops enforcing — and the only way to reach nil is a bug in main.go, which is
// excluded from coverage and so would not be caught by anything else.
func NewAnnounceHub(sessionStore service.SessionStore, access FeedAccess) *AnnounceHub {
	return &AnnounceHub{
		sessionStore: sessionStore,
		access:       access,
		clients:      make(map[string]map[*sseClient]struct{}),
		broadcast:    make(chan feedFrame, 256),
		heartbeat:    sseHeartbeat,
	}
}

// Run fans queued announcements out to connected clients. Call it in a
// goroutine.
//
// Registration and removal are not routed through here: they are plain
// mutex-guarded map operations, and making the cap atomic meant doing the check
// and the insert under one lock anyway. Only the fan-out needs a loop.
func (h *AnnounceHub) Run() {
	for queued := range h.broadcast {
		h.mu.RLock()
		var slow []*sseClient
		for client := range h.clients[queued.feed] {
			select {
			case client.send <- queued.frame:
			default:
				// Buffer full: this client is not keeping up. Collect it and drop
				// it after the loop, rather than let it stall everyone else's
				// fan-out or mutate the map mid-range.
				slow = append(slow, client)
			}
		}
		h.mu.RUnlock()

		for _, client := range slow {
			h.remove(client)
		}
	}
}

// tryRegister admits a client if the user is under their stream cap.
//
// The check and the insert happen under one lock on purpose: counting first and
// registering afterwards is a race two simultaneous connects can both win, and
// the cap exists precisely to stop someone opening streams in bulk.
func (h *AnnounceHub) tryRegister(client *sseClient) bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Counted across every feed, not per feed: the cap exists to bound fan-out,
	// and five pinned tabs cost the same whichever feeds they watch.
	var forUser int
	for _, feed := range h.clients {
		for existing := range feed {
			if existing.userID == client.userID {
				forUser++
			}
		}
	}
	if forUser >= sseMaxPerUser {
		return false
	}

	if h.clients[client.feed] == nil {
		h.clients[client.feed] = make(map[*sseClient]struct{})
	}
	h.clients[client.feed][client] = struct{}{}
	return true
}

// remove reaps a client. Safe to call from either the hub or a handler.
func (h *AnnounceHub) remove(client *sseClient) {
	h.mu.Lock()
	feed := h.clients[client.feed]
	_, present := feed[client]
	delete(feed, client)
	if len(feed) == 0 {
		// Otherwise a feed that is renamed or deleted leaves an empty map behind
		// for the life of the process.
		delete(h.clients, client.feed)
	}
	h.mu.Unlock()

	if present {
		client.close()
	}
}

// Broadcast queues an announcement for every connected client.
//
// The id becomes the SSE event id, so each frame says which announcement it is.
// The send is non-blocking: this runs on a worker goroutine holding one of ten
// asynq slots, and a stalled hub must not be able to park it.
func (h *AnnounceHub) Broadcast(feed, id string, payload []byte) {
	queued := feedFrame{feed: feed, frame: sseFrame("announcement", id, payload)}
	select {
	case h.broadcast <- queued:
	default:
		slog.Warn("announce stream: broadcast dropped, hub buffer full", "feed", feed)
	}
}

// mayWatch answers on the live rows, so a revoked privilege bites on the next
// connect rather than whenever the member next logs in.
//
// A lookup failure denies. The alternative is serving a stream to someone whose
// access could not be confirmed, which is the wrong way round for a check whose
// entire job is to say no.
func (h *AnnounceHub) mayWatch(ctx context.Context, userID int64) bool {
	if h.access == nil {
		slog.Error("announce stream: no feed access check wired, refusing")
		return false
	}
	allowed, err := h.access.Allowed(ctx, userID)
	if err != nil {
		slog.Error("announce stream: could not read feed access", "user_id", userID, "error", err)
		return false
	}
	return allowed
}

// DisconnectUser closes every stream a member has open.
//
// Called when their feed access is revoked: an open stream would otherwise keep
// delivering announcements to someone who has just been told they cannot watch,
// for as long as they leave the tab open.
func (h *AnnounceHub) DisconnectUser(userID int64) int {
	h.mu.Lock()
	var dropped []*sseClient
	for feed, clients := range h.clients {
		for client := range clients {
			if client.userID != userID {
				continue
			}
			dropped = append(dropped, client)
			delete(clients, client)
		}
		if len(clients) == 0 {
			delete(h.clients, feed)
		}
	}
	h.mu.Unlock()

	for _, client := range dropped {
		client.close()
	}
	return len(dropped)
}

// WatchRestrictions closes a member's streams the moment their feed access is
// revoked.
//
// Subscribed to the event rather than called from the admin handler, because the
// handler is not the only thing that revokes: warning escalation applies
// restrictions too, and anything added later will as well. An open stream would
// otherwise keep delivering announcements to someone who has just been told they
// may not watch, for as long as they leave the tab open.
func (h *AnnounceHub) WatchRestrictions(bus event.Bus) {
	bus.Subscribe(event.RestrictionApplied, func(_ context.Context, evt event.Event) error {
		applied, ok := evt.(*event.RestrictionAppliedEvent)
		if !ok || applied.RestrictionType != model.RestrictionTypeFeed {
			return nil
		}
		if dropped := h.DisconnectUser(applied.UserID); dropped > 0 {
			slog.Info("live feed access revoked, closed open streams",
				"user_id", applied.UserID, "streams", dropped)
		}
		return nil
	})
}

// WatchConfigChanges disconnects every watcher whenever a live feed is created,
// edited, toggled or deleted.
//
// A feed's slug is admin-mutable and the hub keys watchers by it, so without
// this a rename frees a slug that another feed can later take — and every
// browser still connected under the old name would start receiving the new
// feed's announcements, with no reconnect and no signal. That is exactly the
// cross-feed leak the rest of this design exists to prevent, arriving
// sequentially instead of concurrently.
//
// Disconnecting is cheap: the browser reconnects within seconds, re-reads the
// feed list, and resolves which feed it is actually watching. It also ends the
// dead stream a renamed or deleted feed would otherwise leave open, heartbeating
// and reporting "live" while carrying nothing forever.
func (h *AnnounceHub) WatchConfigChanges(bus event.Bus) {
	bus.Subscribe(event.ConnectorConfigChanged, func(_ context.Context, evt event.Event) error {
		changed, ok := evt.(*event.ConnectorConfigChangedEvent)
		if !ok || changed.Kind != "sse" {
			return nil
		}
		if dropped := h.DisconnectAll(); dropped > 0 {
			slog.Info("announce stream: live feeds changed, disconnecting watchers to re-resolve",
				"clients", dropped)
		}
		return nil
	})
}

// DisconnectAll closes every stream and reports how many were closed.
//
// Exported because a class losing the feeds has to end the streams its members
// already have open, and the admin group handler is where that is known.
func (h *AnnounceHub) DisconnectAll() int {
	h.mu.Lock()
	clients := make([]*sseClient, 0, len(h.clients))
	for _, feed := range h.clients {
		for client := range feed {
			clients = append(clients, client)
		}
	}
	h.clients = make(map[string]map[*sseClient]struct{})
	h.mu.Unlock()

	// Closed outside the lock: close() is idempotent, and holding the write lock
	// while waking every handler goroutine would stall registration.
	for _, client := range clients {
		client.close()
	}
	return len(clients)
}

// clientCount reports how many clients are connected across every feed, for
// tests.
func (h *AnnounceHub) clientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	var total int
	for _, feed := range h.clients {
		total += len(feed)
	}
	return total
}

// HandleStream serves GET /api/v1/announce-stream/{slug}, and the unslugged
// legacy route as an alias for the default feed.
//
// Authentication is by query parameter because EventSource cannot set headers —
// the same reason /ws/chat does it, and the token is validated against live
// sessions on every connect, so logging out stops the feed at the next
// reconnect.
//
// The feed is not checked against the configured instances. A slug nobody
// publishes to is simply a stream that stays quiet, and refusing to open it
// would mean holding a connector list in the hub and racing every admin edit
// against every open connection.
func (h *AnnounceHub) HandleStream(w http.ResponseWriter, r *http.Request) {
	feed := strings.TrimSpace(chi.URLParam(r, "slug"))
	if feed == "" {
		feed = sse.DefaultSlug
	}
	if err := sse.ValidateSlug(feed); err != nil {
		// A malformed slug can never match a real feed, so this is a 404 rather
		// than a stream that would never carry anything.
		ErrorResponse(w, http.StatusNotFound, "not_found", "no such feed")
		return
	}

	token := r.URL.Query().Get("token")
	if token == "" {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "missing token")
		return
	}
	session := h.sessionStore.GetByAccessToken(token)
	if session == nil {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "invalid or expired token")
		return
	}

	if !h.mayWatch(r.Context(), session.UserID) {
		ErrorResponse(w, http.StatusForbidden, "forbidden", "you do not have access to the live feeds")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		// Without flushing, every frame would sit in the buffer and the stream
		// would never arrive.
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "streaming is not supported")
		return
	}

	client := &sseClient{userID: session.UserID, feed: feed, send: make(chan []byte, sseClientBuffer)}
	if !h.tryRegister(client) {
		ErrorResponse(w, http.StatusTooManyRequests, "too_many_streams",
			fmt.Sprintf("at most %d live feeds per user", sseMaxPerUser))
		return
	}
	defer h.remove(client)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// nginx buffers proxied responses by default, which would hold every frame
	// until the buffer filled.
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	if _, err := io.WriteString(w, fmt.Sprintf("retry: %d\n\n", sseRetryHint.Milliseconds())); err != nil {
		return
	}
	flusher.Flush()

	ticker := time.NewTicker(h.heartbeat)
	defer ticker.Stop()

	for {
		select {
		case frame, open := <-client.send:
			if !open {
				return
			}
			if _, err := w.Write(frame); err != nil {
				return
			}
			flusher.Flush()

		case <-ticker.C:
			// A comment line: valid SSE that the browser ignores, but enough to
			// keep the connection from being reaped as idle.
			if _, err := io.WriteString(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()

		case <-r.Context().Done():
			return
		}
	}
}

// sseFrame renders one event. Data is split across lines because SSE terminates
// a frame on a blank line, so an embedded newline would truncate the payload.
func sseFrame(event, id string, payload []byte) []byte {
	var b strings.Builder
	b.WriteString("event: ")
	b.WriteString(sseSanitize(event))
	b.WriteString("\n")
	if id != "" {
		b.WriteString("id: ")
		b.WriteString(sseSanitize(id))
		b.WriteString("\n")
	}
	// SSE treats CR, LF and CRLF alike as line breaks, so every one has to
	// become its own data: line or the frame terminates early. JSON escaping
	// already rules this out for a marshalled announcement — this is the belt to
	// its braces, since the payload type is only a []byte.
	for _, line := range strings.Split(strings.ReplaceAll(string(payload), "\r\n", "\n"), "\n") {
		b.WriteString("data: ")
		b.WriteString(strings.ReplaceAll(line, "\r", ""))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	return []byte(b.String())
}

// sseSanitize keeps a field value on one line. The id derives from a delivery
// key rather than user input, but a field that could break framing is not worth
// leaving to trust.
func sseSanitize(s string) string {
	return strings.NewReplacer("\r", "", "\n", "").Replace(s)
}
