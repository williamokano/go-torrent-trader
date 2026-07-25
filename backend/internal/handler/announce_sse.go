package handler

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

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
	// sseUnregisterGrace is how long a departing client waits to be reaped by
	// the hub before removing itself.
	sseUnregisterGrace = time.Second
)

// sseClient is one connected browser.
type sseClient struct {
	userID int64
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

	clients    map[*sseClient]struct{}
	register   chan *sseClient
	unregister chan *sseClient
	broadcast  chan []byte
	mu         sync.RWMutex

	// heartbeat is a field so tests need not wait twenty-five seconds.
	heartbeat time.Duration
}

// NewAnnounceHub creates an AnnounceHub.
func NewAnnounceHub(sessionStore service.SessionStore) *AnnounceHub {
	return &AnnounceHub{
		sessionStore: sessionStore,
		clients:      make(map[*sseClient]struct{}),
		register:     make(chan *sseClient, 64),
		unregister:   make(chan *sseClient, 64),
		broadcast:    make(chan []byte, 256),
		heartbeat:    sseHeartbeat,
	}
}

// Run starts the hub loop. Call it in a goroutine.
func (h *AnnounceHub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = struct{}{}
			h.mu.Unlock()

		case client := <-h.unregister:
			h.remove(client)

		case frame := <-h.broadcast:
			h.mu.RLock()
			var slow []*sseClient
			for client := range h.clients {
				select {
				case client.send <- frame:
				default:
					// Buffer full: this client is not keeping up. Drop it rather
					// than let it stall the fan-out to everyone else.
					slow = append(slow, client)
				}
			}
			h.mu.RUnlock()

			for _, client := range slow {
				h.remove(client)
			}
		}
	}
}

// remove reaps a client. Safe to call from either the hub or a handler.
func (h *AnnounceHub) remove(client *sseClient) {
	h.mu.Lock()
	_, present := h.clients[client]
	delete(h.clients, client)
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
func (h *AnnounceHub) Broadcast(id string, payload []byte) {
	frame := sseFrame("announcement", id, payload)
	select {
	case h.broadcast <- frame:
	default:
		slog.Warn("announce stream: broadcast dropped, hub buffer full")
	}
}

// clientCount reports how many clients are connected, for tests.
func (h *AnnounceHub) clientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// HandleStream serves GET /api/v1/announce-stream.
//
// Authentication is by query parameter because EventSource cannot set headers —
// the same reason /ws/chat does it, and the token is validated against live
// sessions on every connect, so logging out stops the feed at the next
// reconnect.
func (h *AnnounceHub) HandleStream(w http.ResponseWriter, r *http.Request) {
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

	flusher, ok := w.(http.Flusher)
	if !ok {
		// Without flushing, every frame would sit in the buffer and the stream
		// would never arrive.
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "streaming is not supported")
		return
	}

	if h.countForUser(session.UserID) >= sseMaxPerUser {
		ErrorResponse(w, http.StatusTooManyRequests, "too_many_streams",
			fmt.Sprintf("at most %d live feeds per user", sseMaxPerUser))
		return
	}

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

	client := &sseClient{userID: session.UserID, send: make(chan []byte, sseClientBuffer)}
	h.register <- client
	defer h.detach(client)

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

// detach removes a departing client, preferring the hub so registration and
// removal stay serialised, but never waiting on it indefinitely.
func (h *AnnounceHub) detach(client *sseClient) {
	select {
	case h.unregister <- client:
	case <-time.After(sseUnregisterGrace):
		h.remove(client)
	}
}

func (h *AnnounceHub) countForUser(userID int64) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	var count int
	for client := range h.clients {
		if client.userID == userID {
			count++
		}
	}
	return count
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
	for _, line := range strings.Split(string(payload), "\n") {
		b.WriteString("data: ")
		b.WriteString(strings.TrimRight(line, "\r"))
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
