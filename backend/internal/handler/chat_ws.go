package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

// ChatClient represents a connected WebSocket client.
type ChatClient struct {
	conn   *websocket.Conn
	userID int64
	perms  model.Permissions
}

// ChatBroadcast is a message to broadcast to all connected clients.
type ChatBroadcast struct {
	Data []byte
}

// ChatHub manages WebSocket connections for the shoutbox.
type ChatHub struct {
	chatSvc      *service.ChatService
	sessionStore service.SessionStore

	clients    map[*ChatClient]struct{}
	broadcast  chan ChatBroadcast
	register   chan *ChatClient
	unregister chan *ChatClient
	mu         sync.RWMutex
}

// NewChatHub creates a new ChatHub.
func NewChatHub(chatSvc *service.ChatService, sessionStore service.SessionStore) *ChatHub {
	return &ChatHub{
		chatSvc:      chatSvc,
		sessionStore: sessionStore,
		clients:      make(map[*ChatClient]struct{}),
		broadcast:    make(chan ChatBroadcast, 256),
		register:     make(chan *ChatClient),
		unregister:   make(chan *ChatClient),
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
				_ = client.conn.Close()
			}
			h.mu.Unlock()
			slog.Debug("chat client disconnected", "user_id", client.userID)

		case msg := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				if err := client.conn.WriteMessage(websocket.TextMessage, msg.Data); err != nil {
					slog.Debug("failed to write to client, removing", "user_id", client.userID, "error", err)
					_ = client.conn.Close()
					delete(h.clients, client)
				}
			}
			h.mu.RUnlock()
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

// wsIncoming represents an incoming WebSocket message from a client.
type wsIncoming struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// HandleWebSocket handles the WebSocket upgrade and client lifecycle.
// unwrapHijacker walks the ResponseWriter wrapper chain to find one that
// implements http.Hijacker. Chi's Recoverer middleware wraps the writer,
// stripping the Hijacker interface that WebSocket upgrade requires.
func unwrapHijacker(w http.ResponseWriter) http.ResponseWriter {
	if _, ok := w.(http.Hijacker); ok {
		return w
	}
	// Chi and other middleware use Unwrap() to expose the inner writer.
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

	// Unwrap the ResponseWriter to get past middleware wrappers (e.g.
	// Chi's Recoverer) that strip the http.Hijacker interface.
	conn, err := upgrader.Upgrade(unwrapHijacker(w), r, nil)
	if err != nil {
		slog.Error("websocket upgrade failed", "error", err)
		return
	}

	client := &ChatClient{
		conn:   conn,
		userID: sess.UserID,
		perms:  sess.Permissions,
	}

	h.register <- client

	// Send backfill of recent messages.
	h.sendBackfill(conn)

	// Read messages from client.
	go h.readPump(client)
}

func (h *ChatHub) sendBackfill(conn *websocket.Conn) {
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
	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		slog.Error("failed to send backfill", "error", err)
	}
}

func (h *ChatHub) readPump(client *ChatClient) {
	defer func() {
		h.unregister <- client
	}()

	// Set read deadline and pong handler for keepalive.
	_ = client.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	client.conn.SetPongHandler(func(string) error {
		_ = client.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	// Start ping ticker.
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			if err := client.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}()

	for {
		_, rawMsg, err := client.conn.ReadMessage()
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

		switch incoming.Type {
		case "message":
			h.handleIncomingMessage(client, incoming.Text)
		}
	}
}

func (h *ChatHub) handleIncomingMessage(client *ChatClient, text string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	msg, err := h.chatSvc.SendMessage(ctx, client.userID, text)
	if err != nil {
		// Send error back to the sender.
		errPayload, _ := json.Marshal(map[string]interface{}{
			"type":    "error",
			"message": err.Error(),
		})
		_ = client.conn.WriteMessage(websocket.TextMessage, errPayload)
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
