package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
	"github.com/williamokano/go-torrent-trader/backend/internal/testutil"
)

// These drive the chat hub over a real WebSocket connection: a genuine
// httptest server, a genuine gorilla dialer, real upgrade/auth/pumps. Unit-
// testing the handler function directly would skip the upgrade — which is
// where the origin check and the token auth actually live.

type wsHarness struct {
	hub      *ChatHub
	srv      *httptest.Server
	sessions *testutil.MemorySessionStore
	messages *stubChatMessageRepo
	mutes    *stubChatMuteRepo
	users    *stubUserRepo
	bus      event.Bus
}

func newWSHarness(t *testing.T, allowedOrigins []string, users ...*model.User) *wsHarness {
	t.Helper()

	if len(users) == 0 {
		users = []*model.User{{ID: 7, Username: "member", CanChat: true}}
	}

	messages := &stubChatMessageRepo{}
	mutes := &stubChatMuteRepo{}
	userRepo := newStubUserRepo(users...)
	bus := event.NewInMemoryBus()
	sessions := testutil.NewMemorySessionStore()

	chatSvc := service.NewChatService(messages, mutes, userRepo, bus, nil)
	settingsSvc := service.NewSiteSettingsService(newStubSiteSettingsRepo(), bus)

	hub := NewChatHub(chatSvc, sessions, settingsSvc, bus, allowedOrigins)
	go hub.Run()

	srv := httptest.NewServer(http.HandlerFunc(hub.HandleWebSocket))
	t.Cleanup(srv.Close)

	return &wsHarness{
		hub: hub, srv: srv, sessions: sessions,
		messages: messages, mutes: mutes, users: userRepo, bus: bus,
	}
}

// session registers an access token the hub will accept.
func (h *wsHarness) session(t *testing.T, token string, userID int64, perms model.Permissions) {
	t.Helper()
	err := h.sessions.Create(&service.Session{
		UserID:       userID,
		AccessToken:  token,
		RefreshToken: token + "-refresh",
		Permissions:  perms,
		// The store treats a zero ExpiresAt as already expired.
		ExpiresAt:        time.Now().Add(time.Hour),
		RefreshExpiresAt: time.Now().Add(24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("creating session: %v", err)
	}
}

// dial opens a real WebSocket connection. origin == "" omits the header, which
// is how a non-browser client connects.
func (h *wsHarness) dial(t *testing.T, token, origin string) (*websocket.Conn, *http.Response, error) {
	t.Helper()

	// The path matters: a query string with no path yields an invalid request
	// URI and the handshake fails before the handler is ever reached.
	url := "ws" + strings.TrimPrefix(h.srv.URL, "http") + "/ws?token=" + token
	header := http.Header{}
	if origin != "" {
		header.Set("Origin", origin)
	}

	conn, resp, err := websocket.DefaultDialer.Dial(url, header)
	if conn != nil {
		t.Cleanup(func() { _ = conn.Close() })
	}
	return conn, resp, err
}

// mustDial fails the test if the connection cannot be established.
func (h *wsHarness) mustDial(t *testing.T, token string) *websocket.Conn {
	t.Helper()
	conn, _, err := h.dial(t, token, "")
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	return conn
}

// readFrame reads one JSON frame, failing rather than hanging forever.
func readFrame(t *testing.T, conn *websocket.Conn) map[string]interface{} {
	t.Helper()

	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, raw, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("reading frame: %v", err)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("frame is not JSON: %v (%s)", err, raw)
	}
	return payload
}

// readFrameOfType skips frames until the wanted type arrives — a connection
// always begins with a backfill, and a muted user also gets a mute frame.
func readFrameOfType(t *testing.T, conn *websocket.Conn, want string) map[string]interface{} {
	t.Helper()

	// Generous: a rate-limit burst puts the frame we want behind every message
	// that was accepted before the limit tripped.
	const maxFrames = 40
	for i := 0; i < maxFrames; i++ {
		frame := readFrame(t, conn)
		if frame["type"] == want {
			return frame
		}
	}
	t.Fatalf("no %q frame after %d frames", want, maxFrames)
	return nil
}

func sendText(t *testing.T, conn *websocket.Conn, text string) {
	t.Helper()
	raw, err := json.Marshal(map[string]string{"type": "message", "text": text})
	if err != nil {
		t.Fatalf("marshalling outgoing: %v", err)
	}
	if err := conn.WriteMessage(websocket.TextMessage, raw); err != nil {
		t.Fatalf("writing frame: %v", err)
	}
}

// --- auth on the upgrade ----------------------------------------------------

// The token is the only thing standing between an anonymous socket and the
// chat. Both rejections must happen BEFORE the upgrade, so the client never
// gets a usable connection.

func TestWebSocketRejectsMissingToken(t *testing.T) {
	h := newWSHarness(t, nil)

	_, resp, err := h.dial(t, "", "")
	if err == nil {
		t.Fatal("connected with no token")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %v, want 401", resp)
	}
}

func TestWebSocketRejectsUnknownToken(t *testing.T) {
	h := newWSHarness(t, nil)

	_, resp, err := h.dial(t, "not-a-real-token", "")
	if err == nil {
		t.Fatal("connected with an unknown token")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %v, want 401", resp)
	}
}

// A session that has been revoked must not keep working: the token is looked
// up on every connect.
func TestWebSocketRejectsRevokedSession(t *testing.T) {
	h := newWSHarness(t, nil)
	h.session(t, "tok", 7, model.Permissions{Level: 1})
	h.sessions.Delete("tok")

	_, resp, err := h.dial(t, "tok", "")
	if err == nil {
		t.Fatal("connected with a revoked session")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %v, want 401", resp)
	}
}

// --- origin check -----------------------------------------------------------

// A browser sends Origin. With no allow-list configured the safe default is to
// refuse — otherwise any site could open a socket as the logged-in user.
func TestWebSocketRejectsBrowserOriginWhenNoneConfigured(t *testing.T) {
	h := newWSHarness(t, nil)
	h.session(t, "tok", 7, model.Permissions{Level: 1})

	_, _, err := h.dial(t, "tok", "https://evil.example")
	if err == nil {
		t.Fatal("upgraded a browser request with no configured origins — any site could hijack the session")
	}
}

func TestWebSocketRejectsOriginNotOnAllowList(t *testing.T) {
	h := newWSHarness(t, []string{"https://tracker.example"})
	h.session(t, "tok", 7, model.Permissions{Level: 1})

	_, _, err := h.dial(t, "tok", "https://evil.example")
	if err == nil {
		t.Fatal("upgraded a request from an origin that is not on the allow-list")
	}
}

func TestWebSocketAcceptsAllowedOrigin(t *testing.T) {
	h := newWSHarness(t, []string{"https://tracker.example"})
	h.session(t, "tok", 7, model.Permissions{Level: 1})

	conn, _, err := h.dial(t, "tok", "https://tracker.example")
	if err != nil {
		t.Fatalf("allowed origin was refused: %v", err)
	}
	if frame := readFrameOfType(t, conn, "backfill"); frame == nil {
		t.Error("no backfill after connecting")
	}
}

// Origin matching is case-insensitive per RFC 6454.
func TestWebSocketAllowedOriginIsCaseInsensitive(t *testing.T) {
	h := newWSHarness(t, []string{"https://Tracker.Example"})
	h.session(t, "tok", 7, model.Permissions{Level: 1})

	if _, _, err := h.dial(t, "tok", "https://tracker.example"); err != nil {
		t.Errorf("origin match should be case-insensitive: %v", err)
	}
}

// A non-browser client (no Origin header) is allowed through.
func TestWebSocketAcceptsClientWithNoOriginHeader(t *testing.T) {
	h := newWSHarness(t, nil)
	h.session(t, "tok", 7, model.Permissions{Level: 1})

	if _, _, err := h.dial(t, "tok", ""); err != nil {
		t.Errorf("a client sending no Origin should connect: %v", err)
	}
}

// --- connect: backfill and mute status --------------------------------------

func TestWebSocketSendsBackfillOnConnect(t *testing.T) {
	h := newWSHarness(t, nil)
	h.messages.recent = []model.ChatMessage{
		{ID: 1, UserID: 7, Username: "member", Message: "earlier", CreatedAt: time.Now()},
	}
	h.session(t, "tok", 7, model.Permissions{Level: 1})

	conn := h.mustDial(t, "tok")
	frame := readFrameOfType(t, conn, "backfill")

	msgs, ok := frame["messages"].([]interface{})
	if !ok || len(msgs) != 1 {
		t.Fatalf("backfill messages = %v, want 1", frame["messages"])
	}
	first := msgs[0].(map[string]interface{})
	if first["message"] != "earlier" {
		t.Errorf("backfill message = %v, want %q", first["message"], "earlier")
	}
}

// A user who is muted must be told so on connect — otherwise the UI lets them
// type into a socket that will silently reject everything.
func TestWebSocketSendsMuteStatusOnConnectWhenMuted(t *testing.T) {
	h := newWSHarness(t, nil)
	h.mutes.active = &model.ChatMute{
		UserID: 7, Reason: "spam", ExpiresAt: time.Now().Add(time.Hour),
	}
	h.session(t, "tok", 7, model.Permissions{Level: 1})

	conn := h.mustDial(t, "tok")
	frame := readFrameOfType(t, conn, "mute")

	if frame["reason"] != "spam" {
		t.Errorf("mute reason = %v, want spam", frame["reason"])
	}
	if frame["expires_at"] == nil {
		t.Error("mute frame carries no expires_at")
	}
}

// --- sending messages -------------------------------------------------------

func TestWebSocketBroadcastsMessageToAllClients(t *testing.T) {
	h := newWSHarness(t, nil,
		&model.User{ID: 7, Username: "sender", CanChat: true},
		&model.User{ID: 8, Username: "listener", CanChat: true},
	)
	h.session(t, "sender-tok", 7, model.Permissions{Level: 1})
	h.session(t, "listener-tok", 8, model.Permissions{Level: 1})

	sender := h.mustDial(t, "sender-tok")
	listener := h.mustDial(t, "listener-tok")
	readFrameOfType(t, sender, "backfill")
	readFrameOfType(t, listener, "backfill")

	sendText(t, sender, "hello room")

	// The sender sees their own message, and so does everyone else.
	for name, conn := range map[string]*websocket.Conn{"sender": sender, "listener": listener} {
		frame := readFrameOfType(t, conn, "message")
		if frame["message"] != "hello room" {
			t.Errorf("%s received %v, want %q", name, frame["message"], "hello room")
		}
	}
}

// A muted user's message must be refused, and they must be told why rather than
// having it silently dropped.
func TestWebSocketRefusesMessageFromMutedUser(t *testing.T) {
	h := newWSHarness(t, nil)
	h.mutes.active = &model.ChatMute{
		UserID: 7, Reason: "spam", ExpiresAt: time.Now().Add(time.Hour),
	}
	h.session(t, "tok", 7, model.Permissions{Level: 1})

	conn := h.mustDial(t, "tok")
	sendText(t, conn, "let me through")

	frame := readFrameOfType(t, conn, "error")
	if frame["message"] == nil || frame["message"] == "" {
		t.Error("error frame carries no reason")
	}
	if len(h.messages.created()) != 0 {
		t.Errorf("a muted user's message was persisted: %v", h.messages.created())
	}
}

// A user without chat privileges is refused the same way.
func TestWebSocketRefusesMessageFromUserWhoCannotChat(t *testing.T) {
	h := newWSHarness(t, nil, &model.User{ID: 7, Username: "silenced", CanChat: false})
	h.session(t, "tok", 7, model.Permissions{Level: 1})

	conn := h.mustDial(t, "tok")
	sendText(t, conn, "hello")

	readFrameOfType(t, conn, "error")
	if len(h.messages.created()) != 0 {
		t.Error("a user without CanChat had their message persisted")
	}
}

// Frames that are not JSON, or not of type "message", are ignored rather than
// killing the connection.
func TestWebSocketIgnoresMalformedAndUnknownFrames(t *testing.T) {
	h := newWSHarness(t, nil)
	h.session(t, "tok", 7, model.Permissions{Level: 1})

	conn := h.mustDial(t, "tok")
	readFrameOfType(t, conn, "backfill")

	if err := conn.WriteMessage(websocket.TextMessage, []byte("{not json")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"typing"}`)); err != nil {
		t.Fatalf("write: %v", err)
	}

	// The connection must still work afterwards.
	sendText(t, conn, "still here")
	frame := readFrameOfType(t, conn, "message")
	if frame["message"] != "still here" {
		t.Errorf("connection did not survive junk frames: %v", frame["message"])
	}
}

// --- rate limiting ----------------------------------------------------------

// Defaults are 10 messages per 10s window. The 11th earns a strike and an error
// frame rather than being broadcast.
func TestWebSocketRateLimitsBurst(t *testing.T) {
	h := newWSHarness(t, nil)
	h.session(t, "tok", 7, model.Permissions{Level: 1})

	conn := h.mustDial(t, "tok")
	readFrameOfType(t, conn, "backfill")

	for i := 0; i < 11; i++ {
		sendText(t, conn, "spam")
	}

	// Drain until the rate-limit error surfaces.
	var sawError bool
	for i := 0; i < 20 && !sawError; i++ {
		if readFrame(t, conn)["type"] == "error" {
			sawError = true
		}
	}
	if !sawError {
		t.Error("11th message in the window was not rate-limited")
	}
	if got := len(h.messages.created()); got > 10 {
		t.Errorf("%d messages persisted, want at most the 10 the limit allows", got)
	}
}

// --- hub fan-out ------------------------------------------------------------

// SendToUser is how notifications reach one person. It must not leak to others.
func TestSendToUserReachesOnlyThatUser(t *testing.T) {
	h := newWSHarness(t, nil,
		&model.User{ID: 7, Username: "target", CanChat: true},
		&model.User{ID: 8, Username: "bystander", CanChat: true},
	)
	h.session(t, "target-tok", 7, model.Permissions{Level: 1})
	h.session(t, "bystander-tok", 8, model.Permissions{Level: 1})

	target := h.mustDial(t, "target-tok")
	bystander := h.mustDial(t, "bystander-tok")
	readFrameOfType(t, target, "backfill")
	readFrameOfType(t, bystander, "backfill")

	h.hub.SendToUser(7, []byte(`{"type":"notification","id":1}`))

	if frame := readFrameOfType(t, target, "notification"); frame["id"] != float64(1) {
		t.Errorf("target got %v", frame)
	}

	// The bystander must receive nothing at all.
	_ = bystander.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	if _, raw, err := bystander.ReadMessage(); err == nil {
		t.Errorf("a notification for user 7 was delivered to user 8: %s", raw)
	}
}

func TestBroadcastDeleteReachesEveryone(t *testing.T) {
	h := newWSHarness(t, nil)
	h.session(t, "tok", 7, model.Permissions{Level: 1})

	conn := h.mustDial(t, "tok")
	readFrameOfType(t, conn, "backfill")

	h.hub.BroadcastDelete(42)

	frame := readFrameOfType(t, conn, "delete")
	if frame["id"] != float64(42) {
		t.Errorf("delete id = %v, want 42", frame["id"])
	}
}

func TestBroadcastDeleteUserReachesEveryone(t *testing.T) {
	h := newWSHarness(t, nil)
	h.session(t, "tok", 7, model.Permissions{Level: 1})

	conn := h.mustDial(t, "tok")
	readFrameOfType(t, conn, "backfill")

	h.hub.BroadcastDeleteUser(9)

	frame := readFrameOfType(t, conn, "delete_user")
	if frame["user_id"] != float64(9) {
		t.Errorf("user_id = %v, want 9", frame["user_id"])
	}
}

// --- spam escalation and session revalidation --------------------------------

// Repeated rate-limit strikes escalate to an automatic mute. Without this a
// spammer just eats error frames and keeps going.
func TestWebSocketAutoMutesAfterRepeatedStrikes(t *testing.T) {
	h := newWSHarness(t, nil)
	h.session(t, "tok", 7, model.Permissions{Level: 1})

	conn := h.mustDial(t, "tok")
	readFrameOfType(t, conn, "backfill")

	// Defaults: 10 messages per window, 3 strikes before a mute. Every message
	// past the limit earns a strike, so this crosses the threshold.
	for i := 0; i < 14; i++ {
		sendText(t, conn, "spam")
	}

	frame := readFrameOfType(t, conn, "mute")
	if frame["reason"] == nil || frame["reason"] == "" {
		t.Error("auto-mute frame carries no reason")
	}

	h.mutes.mu.Lock()
	defer h.mutes.mu.Unlock()
	if len(h.mutes.created) == 0 {
		t.Error("the user was told they were muted, but no mute was persisted")
	}
}

// The token is re-checked periodically on a live socket. A session revoked
// mid-connection (logout, ban) must not keep chatting until they disconnect.
func TestWebSocketClosesWhenSessionRevokedMidConnection(t *testing.T) {
	h := newWSHarness(t, nil)
	h.session(t, "tok", 7, model.Permissions{Level: 1})

	conn := h.mustDial(t, "tok")
	readFrameOfType(t, conn, "backfill")

	// Revoke, then keep sending: the revalidation check fires every 5 messages.
	h.sessions.Delete("tok")
	for i := 0; i < 6; i++ {
		if err := writeText(conn, "still here?"); err != nil {
			break // the server has already hung up
		}
	}

	// The server must close the connection rather than keep serving it.
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return // closed, as required
		}
	}
}

// writeText is sendText without the fatal — the peer may have already closed.
func writeText(conn *websocket.Conn, text string) error {
	raw, err := json.Marshal(map[string]string{"type": "message", "text": text})
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.TextMessage, raw)
}
