package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/williamokano/go-torrent-trader/backend/internal/cluster"
	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = 30 * time.Second

	// Maximum message size allowed from peer (10 KB).
	maxMessageSize = 10 * 1024

	// Re-validate session token every N messages.
	revalidateEveryN = 5

	// Default rate limit settings (used if site settings unavailable).
	defaultRateLimitWindowSecs = 10
	defaultRateLimitMaxMsgs    = 10
	defaultSpamStrikeCount     = 3
	defaultSpamMuteMinutes     = 5
	defaultStrikeResetSeconds  = 60
)

// ChatClient represents a connected WebSocket client.
type ChatClient struct {
	hub            *ChatHub
	conn           *websocket.Conn
	closeOnce      sync.Once // Ensures conn.Close() is called exactly once.
	userID         int64
	accessToken    string // For periodic re-validation.
	perms          model.Permissions
	send           chan []byte // Buffered channel of outbound messages.
	lastMsg        time.Time   // Rate limiting: time of last sent message.
	msgCount       int         // Rate limiting: messages in current window.
	strikeCount    int         // Anti-spam: consecutive rate limit violations.
	lastStrikeTime time.Time   // Anti-spam: when the last strike occurred.
}

// closeConn safely closes the WebSocket connection exactly once.
func (c *ChatClient) closeConn() {
	c.closeOnce.Do(func() {
		_ = c.conn.Close()
	})
}

// ChatBroadcast is a message to broadcast to all connected clients.
type ChatBroadcast struct {
	Data []byte
}

// chatSpamSettings holds the anti-spam configuration loaded from site settings.
type chatSpamSettings struct {
	rateLimitWindow    time.Duration
	rateLimitMaxMsgs   int
	spamStrikeCount    int
	spamMuteMinutes    int
	strikeResetSeconds int
	rateLimitMessage   string
	spamMuteMessage    string
}

// Cluster topics the chat hub publishes and applies. Each carries a change one
// node made to its own connections, for every other node to repeat on theirs.
const (
	topicChatBroadcast  = "chat.broadcast"
	topicChatToUser     = "chat.to_user"
	topicChatDisconnect = "chat.disconnect"
)

// chatToUser is the payload of topicChatToUser.
type chatToUser struct {
	UserID int64           `json:"user_id"`
	Data   json.RawMessage `json:"data"`
}

// chatUser is the payload of topicChatDisconnect.
type chatUser struct {
	UserID int64 `json:"user_id"`
}

// ChatHub manages WebSocket connections for the shoutbox.
//
// Its clients are the ones connected to this process. Everything that reaches
// clients — a message, a delete, a mute, a disconnect — is applied here and
// published through cluster, so the other replicas apply it to theirs (#295).
type ChatHub struct {
	chatSvc         *service.ChatService
	sessionStore    service.SessionStore
	siteSettingsSvc *service.SiteSettingsService
	eventBus        event.Bus
	allowedOrigins  []string

	// cluster is nil on a single node. Set once by UseCluster, before Run.
	cluster cluster.Publisher
	// resyncSpread is a field so a test need not wait out the real one.
	resyncSpread time.Duration

	spamSettings chatSpamSettings
	settingsMu   sync.RWMutex
	// reloadMu serialises loadSpamSettings, so of two reloads racing, the one
	// that reads last is the one that is kept.
	reloadMu sync.Mutex
	// reloadSettings wakes the reloader UseCluster starts. Depth 1: a burst of
	// changes needs one reload.
	reloadSettings chan struct{}

	clients    map[*ChatClient]struct{}
	broadcast  chan ChatBroadcast
	register   chan *ChatClient
	unregister chan *ChatClient
	mu         sync.RWMutex
}

// NewChatHub creates a new ChatHub.
func NewChatHub(chatSvc *service.ChatService, sessionStore service.SessionStore, siteSettingsSvc *service.SiteSettingsService, eventBus event.Bus, allowedOrigins []string) *ChatHub {
	h := &ChatHub{
		chatSvc:         chatSvc,
		sessionStore:    sessionStore,
		siteSettingsSvc: siteSettingsSvc,
		eventBus:        eventBus,
		allowedOrigins:  allowedOrigins,
		clients:         make(map[*ChatClient]struct{}),
		broadcast:       make(chan ChatBroadcast, 256),
		register:        make(chan *ChatClient, 64),
		unregister:      make(chan *ChatClient, 64),
		resyncSpread:    resyncSpread,
		reloadSettings:  make(chan struct{}, 1),
	}
	h.loadSpamSettings()
	return h
}

// loadSpamSettings reads anti-spam configuration from site settings.
func (h *ChatHub) loadSpamSettings() {
	h.reloadMu.Lock()
	defer h.reloadMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	windowSecs := h.siteSettingsSvc.GetInt(ctx, service.SettingChatRateLimitWindow, defaultRateLimitWindowSecs)
	maxMsgs := h.siteSettingsSvc.GetInt(ctx, service.SettingChatRateLimitMax, defaultRateLimitMaxMsgs)
	strikeCount := h.siteSettingsSvc.GetInt(ctx, service.SettingChatSpamStrikeCount, defaultSpamStrikeCount)
	muteMinutes := h.siteSettingsSvc.GetInt(ctx, service.SettingChatSpamMuteMinutes, defaultSpamMuteMinutes)
	strikeReset := h.siteSettingsSvc.GetInt(ctx, service.SettingChatStrikeResetSeconds, defaultStrikeResetSeconds)
	rateLimitMsg := h.siteSettingsSvc.GetString(ctx, service.SettingChatRateLimitMessage, "Slow down! You are sending messages too fast.")
	spamMuteMsg := h.siteSettingsSvc.GetString(ctx, service.SettingChatSpamMuteMessage, "You have been automatically muted for flooding the chat.")

	h.settingsMu.Lock()
	h.spamSettings = chatSpamSettings{
		rateLimitWindow:    time.Duration(windowSecs) * time.Second,
		rateLimitMaxMsgs:   maxMsgs,
		spamStrikeCount:    strikeCount,
		spamMuteMinutes:    muteMinutes,
		strikeResetSeconds: strikeReset,
		rateLimitMessage:   rateLimitMsg,
		spamMuteMessage:    spamMuteMsg,
	}
	h.settingsMu.Unlock()

	slog.Info("chat anti-spam settings loaded",
		"rate_limit_window_secs", windowSecs,
		"rate_limit_max_msgs", maxMsgs,
		"spam_strike_count", strikeCount,
		"spam_mute_minutes", muteMinutes,
	)
}

// getSpamSettings returns a snapshot of the current anti-spam settings.
func (h *ChatHub) getSpamSettings() chatSpamSettings {
	h.settingsMu.RLock()
	defer h.settingsMu.RUnlock()
	return h.spamSettings
}

// UseCluster connects the hub to the other replicas: what this node changes is
// published, and what they publish is applied to the clients connected here.
// Call it once, before Run.
//
// After the cluster listener reconnects, every client here is disconnected.
// Messages sent while it was down never arrived, and the reconnect gives each
// browser a fresh backfill and a fresh session check — the cheapest way to make
// sure nothing missed in the gap stays missing.
func (h *ChatHub) UseCluster(node cluster.Node) {
	h.cluster = node
	node.Subscribe(topicChatBroadcast, func(payload json.RawMessage) {
		h.broadcastLocal(payload)
	})
	node.Subscribe(topicChatToUser, func(payload json.RawMessage) {
		if msg, ok := cluster.Decode[chatToUser](topicChatToUser, payload); ok {
			h.sendToUserLocal(msg.UserID, msg.Data)
		}
	})
	node.Subscribe(topicChatDisconnect, func(payload json.RawMessage) {
		if msg, ok := cluster.Decode[chatUser](topicChatDisconnect, payload); ok {
			if dropped := h.disconnectUserLocal(msg.UserID); dropped > 0 {
				slog.Info("chat: closed connections on another node's request",
					"user_id", msg.UserID, "connections", dropped)
			}
		}
	})
	node.Subscribe(service.TopicSiteSettingChanged, func(payload json.RawMessage) {
		if msg, ok := cluster.Decode[service.SiteSettingChange](service.TopicSiteSettingChanged, payload); ok &&
			isChatSpamSetting(msg.Key) {
			h.requestSettingsReload()
		}
	})
	node.OnResync(func() {
		// A change missed in the gap would otherwise last until a restart:
		// these settings are only ever reloaded when told to.
		h.requestSettingsReload()
		if dropped := h.disconnectAllLocal(h.resyncSpread); dropped > 0 {
			slog.Info("chat: closing every connection to resync after a cluster gap", "connections", dropped)
		}
	})
	go func() {
		// Off the listener's goroutine, because a reload reads the database.
		for range h.reloadSettings {
			h.loadSpamSettings()
		}
	}()
}

// requestSettingsReload asks the reloader for a reload, without blocking.
func (h *ChatHub) requestSettingsReload() {
	select {
	case h.reloadSettings <- struct{}{}:
	default:
	}
}

// resyncSpread is how long a resync takes to close every connection. A gap in
// the broadcast usually means the database restarted, and every replica
// resyncs at once: closing everything at the same instant would bring every
// browser back together, each with a session lookup and a backfill query,
// onto a database that has just come up.
const resyncSpread = 10 * time.Second

// closeWithin runs closeFn after a random delay of up to spread, or at once
// when spread is zero.
func closeWithin(spread time.Duration, closeFn func()) {
	if spread <= 0 {
		closeFn()
		return
	}
	time.AfterFunc(rand.N(spread), closeFn)
}

// publish hands a change to the other replicas, when there are any.
func (h *ChatHub) publish(topic string, v any) {
	if h.cluster != nil {
		h.cluster.Publish(topic, v)
	}
}

// isChatSpamSetting reports whether a site setting feeds chatSpamSettings.
func isChatSpamSetting(key string) bool {
	switch key {
	case service.SettingChatRateLimitWindow,
		service.SettingChatRateLimitMax,
		service.SettingChatSpamStrikeCount,
		service.SettingChatSpamMuteMinutes,
		service.SettingChatStrikeResetSeconds,
		service.SettingChatRateLimitMessage,
		service.SettingChatSpamMuteMessage:
		return true
	}
	return false
}

// Run starts the hub event loop. Should be called in a goroutine.
func (h *ChatHub) Run() {
	// Subscribe to site setting changes for real-time config updates.
	h.eventBus.Subscribe(event.SiteSettingChanged, func(_ context.Context, evt event.Event) error {
		// The other replicas hear about it from SiteSettingsService.Set, which
		// publishes every change (see UseCluster).
		e := evt.(*event.SiteSettingChangedEvent)
		if isChatSpamSetting(e.Key) {
			slog.Info("chat setting changed, reloading", "key", e.Key)
			h.loadSpamSettings()
		}
		return nil
	})

	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = struct{}{}
			h.mu.Unlock()
			slog.Debug("chat client connected", "user_id", client.userID)

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()
			slog.Debug("chat client disconnected", "user_id", client.userID)

		case msg := <-h.broadcast:
			h.mu.RLock()
			var deadClients []*ChatClient
			for client := range h.clients {
				select {
				case client.send <- msg.Data:
				default:
					// Client's send buffer is full — drop it.
					deadClients = append(deadClients, client)
				}
			}
			h.mu.RUnlock()

			// Clean up slow/dead clients outside the read lock.
			if len(deadClients) > 0 {
				h.mu.Lock()
				for _, client := range deadClients {
					if _, ok := h.clients[client]; ok {
						delete(h.clients, client)
						close(client.send)
					}
				}
				h.mu.Unlock()
			}
		}
	}
}

// Broadcast pushes a pre-marshaled payload to every connected chat client.
//
// It exists so the chat notification connector — which runs on the worker side —
// can reach WS clients without importing the hub's internals. That works because
// the HTTP server and the worker share a process (see main.go), the same
// assumption the existing SendToUser bridge already relies on.
//
// The send is non-blocking. Its callers are a worker task holding one of ten
// worker slots and an admin's test-send HTTP request; a plain channel send
// ignores their contexts, so if Run() were not draining — a burst deeper than
// the buffer, or a deployment where the hub goroutine never started — either
// would park forever. Dropping a shoutbox line is the right trade against that.
func (h *ChatHub) Broadcast(payload []byte) {
	h.broadcastLocal(payload)
	h.publish(topicChatBroadcast, json.RawMessage(payload))
}

// broadcastLocal queues a payload for the clients connected to this node,
// without blocking (see Broadcast).
func (h *ChatHub) broadcastLocal(payload []byte) {
	select {
	case h.broadcast <- ChatBroadcast{Data: payload}:
	default:
		slog.Warn("chat broadcast dropped: hub buffer full")
	}
}

// SendToUser sends a payload to all connected clients belonging to the given
// user, on every node: a member with two tabs may be connected to two replicas.
func (h *ChatHub) SendToUser(userID int64, payload []byte) {
	h.sendToUserLocal(userID, payload)
	h.publish(topicChatToUser, chatToUser{UserID: userID, Data: payload})
}

func (h *ChatHub) sendToUserLocal(userID int64, payload []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for client := range h.clients {
		if client.userID == userID {
			select {
			case client.send <- payload:
			default:
				// Buffer full — skip (will be cleaned up on next broadcast).
			}
		}
	}
}

// DisconnectUser closes every chat connection the user has, here and on every
// other node, returning how many it closed on this one. Closing the socket ends
// the client's read loop, which unregisters it the usual way; closeConn makes a
// second close harmless.
func (h *ChatHub) DisconnectUser(userID int64) int {
	dropped := h.disconnectUserLocal(userID)
	h.publish(topicChatDisconnect, chatUser{UserID: userID})
	return dropped
}

func (h *ChatHub) disconnectUserLocal(userID int64) int {
	h.mu.RLock()
	var mine []*ChatClient
	for client := range h.clients {
		if client.userID == userID {
			mine = append(mine, client)
		}
	}
	h.mu.RUnlock()
	for _, client := range mine {
		client.closeConn()
	}
	return len(mine)
}

// disconnectAllLocal closes every connection on this node, each at a random
// moment within spread; see UseCluster.
func (h *ChatHub) disconnectAllLocal(spread time.Duration) int {
	h.mu.RLock()
	all := make([]*ChatClient, 0, len(h.clients))
	for client := range h.clients {
		all = append(all, client)
	}
	h.mu.RUnlock()
	for _, client := range all {
		closeWithin(spread, client.closeConn)
	}
	return len(all)
}

// WatchBans closes a member's chat connections the moment they are banned
// (#291). The connection re-checks its session only every few messages it
// sends, so without this a banned member could send a few more lines, and one
// who only listens would stay connected indefinitely.
func (h *ChatHub) WatchBans(bus event.Bus) {
	bus.Subscribe(event.UserBanned, func(_ context.Context, evt event.Event) error {
		if e, ok := evt.(*event.UserBannedEvent); ok {
			if dropped := h.DisconnectUser(e.UserID); dropped > 0 {
				slog.Info("chat: closed a banned member's connections", "user_id", e.UserID, "connections", dropped)
			}
		}
		return nil
	})
}

// BroadcastDelete sends a delete event to all connected clients.
func (h *ChatHub) BroadcastDelete(id int64) {
	payload := map[string]interface{}{
		"type": "delete",
		"id":   id,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		slog.Error("failed to marshal delete broadcast", "error", err)
		return
	}
	h.broadcastAll(data)
}

// BroadcastDeleteUser sends a delete_user event to all connected clients,
// instructing them to remove all messages from the given user.
func (h *ChatHub) BroadcastDeleteUser(userID int64) {
	payload := map[string]interface{}{
		"type":    "delete_user",
		"user_id": userID,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		slog.Error("failed to marshal delete_user broadcast", "error", err)
		return
	}
	h.broadcastAll(data)
}

// wsIncoming represents an incoming WebSocket message from a client.
type wsIncoming struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// unwrapHijacker walks the ResponseWriter wrapper chain to find one that
// implements http.Hijacker.
func unwrapHijacker(w http.ResponseWriter) http.ResponseWriter {
	if _, ok := w.(http.Hijacker); ok {
		return w
	}
	type unwrapper interface {
		Unwrap() http.ResponseWriter
	}
	if u, ok := w.(unwrapper); ok {
		return unwrapHijacker(u.Unwrap())
	}
	return w
}

func (h *ChatHub) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Authenticate via query param token.
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "missing token", http.StatusUnauthorized)
		return
	}

	sess, err := h.sessionStore.GetByAccessToken(token)
	if err != nil {
		slog.Error("chat: session store unavailable", "error", err)
		http.Error(w, "sessions are temporarily unavailable", http.StatusServiceUnavailable)
		return
	}
	if sess == nil {
		http.Error(w, "invalid or expired token", http.StatusUnauthorized)
		return
	}

	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			origin := r.Header.Get("Origin")
			// Allow non-browser clients (no Origin header).
			if origin == "" {
				return true
			}
			// If no origins configured, reject browser requests (safe default).
			if len(h.allowedOrigins) == 0 {
				return false
			}
			for _, allowed := range h.allowedOrigins {
				if strings.EqualFold(origin, allowed) {
					return true
				}
			}
			return false
		},
	}

	conn, err := upgrader.Upgrade(unwrapHijacker(w), r, nil)
	if err != nil {
		slog.Error("websocket upgrade failed", "error", err)
		return
	}

	client := &ChatClient{
		hub:         h,
		conn:        conn,
		userID:      sess.UserID,
		accessToken: token,
		perms:       sess.Permissions,
		send:        make(chan []byte, 1024),
	}

	h.register <- client

	// Send backfill before starting pumps.
	h.sendBackfill(client)

	// Send mute status if the user has an active mute.
	h.sendMuteStatus(client)

	// Start the write pump (single writer goroutine per connection).
	go client.writePump()

	// readPump runs in the current goroutine (HandleWebSocket exits when it returns).
	go client.readPump()
}

func (h *ChatHub) sendBackfill(client *ChatClient) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	msgs, err := h.chatSvc.ListRecent(ctx, 50)
	if err != nil {
		slog.Error("failed to load backfill messages", "error", err)
		return
	}

	items := make([]map[string]interface{}, len(msgs))
	for i := range msgs {
		items[i] = chatMessagePayload(&msgs[i])
	}

	payload := map[string]interface{}{
		"type":     "backfill",
		"messages": items,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		slog.Error("failed to marshal backfill", "error", err)
		return
	}

	// Send via the client's send channel (writePump handles the actual write).
	select {
	case client.send <- data:
	default:
		slog.Debug("failed to send backfill, client send buffer full")
	}
}

func (h *ChatHub) sendMuteStatus(client *ChatClient) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	mute, err := h.chatSvc.GetActiveMute(ctx, client.userID)
	if err != nil {
		slog.Error("failed to check mute status on connect", "error", err)
		return
	}
	if mute == nil {
		return
	}

	payload := map[string]interface{}{
		"type":       "mute",
		"expires_at": mute.ExpiresAt.Format(time.RFC3339),
		"reason":     mute.Reason,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		slog.Error("failed to marshal mute status", "error", err)
		return
	}

	select {
	case client.send <- data:
	default:
		slog.Debug("failed to send mute status, client send buffer full")
	}
}

// readPump reads messages from the WebSocket connection.
// There is at most one reader per connection.
func (c *ChatClient) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.closeConn()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	var msgsSinceValidation int
	for {
		_, rawMsg, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				slog.Debug("websocket read error", "error", err)
			}
			return
		}

		var incoming wsIncoming
		if err := json.Unmarshal(rawMsg, &incoming); err != nil {
			continue
		}

		if incoming.Type != "message" {
			continue
		}

		// Periodic session re-validation.
		msgsSinceValidation++
		if msgsSinceValidation >= revalidateEveryN {
			msgsSinceValidation = 0
			// A store error closes the connection too: chatting on a session
			// nobody can confirm is not allowed, and the client reconnects —
			// through the 503 above until the store is back.
			if sess, err := c.hub.sessionStore.GetByAccessToken(c.accessToken); err != nil || sess == nil {
				slog.Debug("websocket session no longer valid, closing", "user_id", c.userID, "error", err)
				return
			}
		}

		// Rate limiting with anti-spam strike tracking.
		settings := c.hub.getSpamSettings()
		now := time.Now()
		if now.Sub(c.lastMsg) > settings.rateLimitWindow {
			c.msgCount = 0
			c.lastMsg = now
		}
		c.msgCount++
		if c.msgCount > settings.rateLimitMaxMsgs {
			c.strikeCount++
			c.lastStrikeTime = now
			slog.Debug("chat rate limit exceeded",
				"user_id", c.userID,
				"strike", c.strikeCount,
				"threshold", settings.spamStrikeCount,
			)

			// Check if strikes exceed threshold for auto-mute.
			if c.strikeCount >= settings.spamStrikeCount {
				c.strikeCount = 0
				c.hub.autoMuteUser(c, settings.spamMuteMinutes, settings.spamMuteMessage)
			} else {
				errPayload, _ := json.Marshal(map[string]interface{}{
					"type":    "error",
					"message": settings.rateLimitMessage,
				})
				select {
				case c.send <- errPayload:
				default:
				}
			}
			continue
		}

		// Reset strikes only after the configured cool-down period of no violations.
		if c.strikeCount > 0 && now.Sub(c.lastStrikeTime) > time.Duration(settings.strikeResetSeconds)*time.Second {
			c.strikeCount = 0
		}

		c.hub.handleIncomingMessage(c, incoming.Text)
	}
}

// writePump pumps messages from the send channel to the WebSocket connection.
// There is exactly one writePump per connection, ensuring no concurrent writes.
func (c *ChatClient) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.closeConn()
	}()

	for {
		select {
		case message, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// Hub closed the channel — send close frame.
				_ = c.conn.WriteMessage(websocket.CloseMessage, nil)
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}

		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// autoMuteUser automatically mutes a user for spam/flooding and notifies them via WebSocket.
func (h *ChatHub) autoMuteUser(client *ChatClient, durationMinutes int, reason string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	mute, err := h.chatSvc.SystemMuteUser(ctx, client.userID, durationMinutes, reason)
	if err != nil {
		slog.Error("failed to auto-mute user for spam",
			"user_id", client.userID,
			"error", err,
		)
		return
	}

	slog.Info("user auto-muted for chat spam",
		"user_id", client.userID,
		"duration_minutes", durationMinutes,
	)

	// Send mute notification to all of this user's connected clients.
	mutePayload, err := json.Marshal(map[string]interface{}{
		"type":       "mute",
		"expires_at": mute.ExpiresAt.Format(time.RFC3339),
		"reason":     mute.Reason,
	})
	if err != nil {
		slog.Error("failed to marshal auto-mute notification", "error", err)
		return
	}
	h.SendToUser(client.userID, mutePayload)
}

func (h *ChatHub) handleIncomingMessage(client *ChatClient, text string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	msg, err := h.chatSvc.SendMessage(ctx, client.userID, text)
	if err != nil {
		errPayload, _ := json.Marshal(map[string]interface{}{
			"type":    "error",
			"message": err.Error(),
		})
		select {
		case client.send <- errPayload:
		default:
		}
		return
	}

	payload := chatMessagePayload(msg)
	payload["type"] = "message"

	data, err := json.Marshal(payload)
	if err != nil {
		slog.Error("failed to marshal chat message", "error", err)
		return
	}

	h.broadcastAll(data)
}

// broadcastAll sends a member-originated payload — a message, a delete — to
// every client on every node. Unlike Broadcast, the local send waits for room
// in the hub's queue: a member's own message is not something to drop.
func (h *ChatHub) broadcastAll(data []byte) {
	h.broadcast <- ChatBroadcast{Data: data}
	h.publish(topicChatBroadcast, json.RawMessage(data))
}

func chatMessagePayload(msg *model.ChatMessage) map[string]interface{} {
	return map[string]interface{}{
		"id":         msg.ID,
		"user_id":    msg.UserID,
		"username":   msg.Username,
		"message":    msg.Message,
		"system":     msg.System,
		"created_at": msg.CreatedAt.Format(time.RFC3339),
	}
}
