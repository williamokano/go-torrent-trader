package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

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

	// Rate limiting: max messages per window per client.
	rateLimitWindow   = 10 * time.Second
	rateLimitMaxMsgs  = 10 // 10 messages per 10 seconds = ~1/sec sustained

	// Re-validate session token every N messages.
	revalidateEveryN = 5
)

// ChatClient represents a connected WebSocket client.
type ChatClient struct {
	hub         *ChatHub
	conn        *websocket.Conn
	closeOnce   sync.Once // Ensures conn.Close() is called exactly once.
	userID      int64
	accessToken string           // For periodic re-validation.
	perms       model.Permissions
	send        chan []byte       // Buffered channel of outbound messages.
	lastMsg     time.Time         // Rate limiting: time of last sent message.
	msgCount    int               // Rate limiting: messages in current window.
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

// ChatHub manages WebSocket connections for the shoutbox.
type ChatHub struct {
	chatSvc      *service.ChatService
	sessionStore service.SessionStore
	allowedOrigins []string

	clients    map[*ChatClient]struct{}
	broadcast  chan ChatBroadcast
	register   chan *ChatClient
	unregister chan *ChatClient
	mu         sync.RWMutex
}

// NewChatHub creates a new ChatHub.
func NewChatHub(chatSvc *service.ChatService, sessionStore service.SessionStore, allowedOrigins []string) *ChatHub {
	return &ChatHub{
		chatSvc:        chatSvc,
		sessionStore:   sessionStore,
		allowedOrigins: allowedOrigins,
		clients:        make(map[*ChatClient]struct{}),
		broadcast:      make(chan ChatBroadcast, 256),
		register:       make(chan *ChatClient, 64),
		unregister:     make(chan *ChatClient, 64),
	}
}

// Run starts the hub event loop. Should be called in a goroutine.
func (h *ChatHub) Run() {
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

// SendToUser sends a payload to all connected clients belonging to the given user.
func (h *ChatHub) SendToUser(userID int64, payload []byte) {
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
	h.broadcast <- ChatBroadcast{Data: data}
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
	h.broadcast <- ChatBroadcast{Data: data}
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

	sess := h.sessionStore.GetByAccessToken(token)
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
			if sess := c.hub.sessionStore.GetByAccessToken(c.accessToken); sess == nil {
				slog.Debug("websocket session expired, closing", "user_id", c.userID)
				return
			}
		}

		// Rate limiting.
		now := time.Now()
		if now.Sub(c.lastMsg) > rateLimitWindow {
			c.msgCount = 0
			c.lastMsg = now
		}
		c.msgCount++
		if c.msgCount > rateLimitMaxMsgs {
			errPayload, _ := json.Marshal(map[string]interface{}{
				"type":    "error",
				"message": "rate limit exceeded, slow down",
			})
			select {
			case c.send <- errPayload:
			default:
			}
			continue
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

	h.broadcast <- ChatBroadcast{Data: data}
}

func chatMessagePayload(msg *model.ChatMessage) map[string]interface{} {
	return map[string]interface{}{
		"id":         msg.ID,
		"user_id":    msg.UserID,
		"username":   msg.Username,
		"message":    msg.Message,
		"created_at": msg.CreatedAt.Format(time.RFC3339),
	}
}
