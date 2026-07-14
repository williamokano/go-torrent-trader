package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// --- chat repository stubs --------------------------------------------------

type stubChatMessageRepo struct {
	recent    []model.ChatMessage
	before    []model.ChatMessage
	deleted   []int64
	deletedBy []int64
	userCount int64

	lastLimit    int
	lastBeforeID int64

	createErr error
	listErr   error
	deleteErr error
	bulkErr   error
}

func (s *stubChatMessageRepo) Create(_ context.Context, _ *model.ChatMessage) error {
	return s.createErr
}
func (s *stubChatMessageRepo) ListRecent(_ context.Context, limit int) ([]model.ChatMessage, error) {
	s.lastLimit = limit
	return s.recent, s.listErr
}
func (s *stubChatMessageRepo) ListBefore(_ context.Context, beforeID int64, limit int) ([]model.ChatMessage, error) {
	s.lastBeforeID = beforeID
	s.lastLimit = limit
	return s.before, s.listErr
}
func (s *stubChatMessageRepo) Delete(_ context.Context, id int64) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}
	s.deleted = append(s.deleted, id)
	return nil
}
func (s *stubChatMessageRepo) DeleteByUserID(_ context.Context, userID int64) (int64, error) {
	if s.bulkErr != nil {
		return 0, s.bulkErr
	}
	s.deletedBy = append(s.deletedBy, userID)
	return s.userCount, nil
}

type stubChatMuteRepo struct {
	active   *model.ChatMute
	list     []repository.ChatMuteWithNames
	total    int64
	created  []*model.ChatMute
	unmuted  []int64
	lastPage int
	lastPer  int

	createErr error
	getErr    error
	listErr   error
	deleteErr error
}

func (s *stubChatMuteRepo) Create(_ context.Context, mute *model.ChatMute) error {
	if s.createErr != nil {
		return s.createErr
	}
	mute.ID = int64(len(s.created) + 1)
	s.created = append(s.created, mute)
	return nil
}
func (s *stubChatMuteRepo) GetActiveMute(_ context.Context, _ int64) (*model.ChatMute, error) {
	return s.active, s.getErr
}
func (s *stubChatMuteRepo) ListActive(_ context.Context, page, perPage int) ([]repository.ChatMuteWithNames, int64, error) {
	s.lastPage, s.lastPer = page, perPage
	if s.listErr != nil {
		return nil, 0, s.listErr
	}
	return s.list, s.total, nil
}
func (s *stubChatMuteRepo) Delete(_ context.Context, userID int64) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}
	s.unmuted = append(s.unmuted, userID)
	return nil
}
func (s *stubChatMuteRepo) DeleteExpired(_ context.Context) ([]int64, error) { return nil, nil }

// --- harness ----------------------------------------------------------------

type chatDeps struct {
	messages *stubChatMessageRepo
	mutes    *stubChatMuteRepo
	users    *stubUserRepo
	hub      *ChatHub
}

func newChatDeps() *chatDeps {
	messages := &stubChatMessageRepo{}
	mutes := &stubChatMuteRepo{}
	users := newStubUserRepo(&model.User{ID: 7, Username: "member", CanChat: true})
	bus := event.NewInMemoryBus()
	chatSvc := service.NewChatService(messages, mutes, users, bus)

	// A hub that is never Run(): its broadcast channel is buffered, so handlers
	// can publish into it and the test reads the payloads back directly.
	hub := NewChatHub(chatSvc, nil, service.NewSiteSettingsService(newStubSiteSettingsRepo(), bus), bus, nil)

	return &chatDeps{messages: messages, mutes: mutes, users: users, hub: hub}
}

func (d *chatDeps) chatService() *service.ChatService {
	return service.NewChatService(d.messages, d.mutes, d.users, event.NewInMemoryBus())
}

// nextBroadcast returns the next payload the handler pushed to the hub.
func nextBroadcast(t *testing.T, hub *ChatHub) map[string]interface{} {
	t.Helper()
	select {
	case msg := <-hub.broadcast:
		var payload map[string]interface{}
		if err := json.Unmarshal(msg.Data, &payload); err != nil {
			t.Fatalf("decoding broadcast: %v", err)
		}
		return payload
	default:
		t.Fatal("expected a broadcast to connected clients, got none")
		return nil
	}
}

func assertNoBroadcast(t *testing.T, hub *ChatHub) {
	t.Helper()
	select {
	case msg := <-hub.broadcast:
		t.Fatalf("unexpected broadcast after a failed operation: %s", msg.Data)
	default:
	}
}

// --- history ----------------------------------------------------------------

func TestChatHistoryRequiresBeforeID(t *testing.T) {
	d := newChatDeps()
	h := NewChatHandler(d.chatService(), d.hub)

	for _, q := range []string{"", "?before_id=0", "?before_id=-1", "?before_id=abc"} {
		w := httptest.NewRecorder()
		h.HandleHistory(w, httptest.NewRequest(http.MethodGet, "/api/v1/chat/history"+q, nil))

		if w.Code != http.StatusBadRequest {
			t.Errorf("query %q: status = %d, want 400", q, w.Code)
		}
	}
}

func TestChatHistoryReturnsMessagePayloads(t *testing.T) {
	d := newChatDeps()
	d.messages.before = []model.ChatMessage{
		{ID: 4, UserID: 7, Username: "member", Message: "hello", CreatedAt: time.Now()},
	}
	h := NewChatHandler(d.chatService(), d.hub)

	w := httptest.NewRecorder()
	h.HandleHistory(w, httptest.NewRequest(http.MethodGet, "/api/v1/chat/history?before_id=10", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if d.messages.lastBeforeID != 10 {
		t.Errorf("before_id passed to the store = %d, want 10", d.messages.lastBeforeID)
	}
	if d.messages.lastLimit != 50 {
		t.Errorf("limit = %d, want the default 50", d.messages.lastLimit)
	}

	items, ok := decodeBody(t, w)["messages"].([]interface{})
	if !ok || len(items) != 1 {
		t.Fatalf("messages = %v, want 1 item", items)
	}
	msg := items[0].(map[string]interface{})
	for _, key := range []string{"id", "user_id", "username", "message", "created_at"} {
		if _, present := msg[key]; !present {
			t.Errorf("chat payload is missing key %q the frontend depends on", key)
		}
	}
}

// The service caps limit at 100 so a client cannot ask for the whole backlog.
func TestChatHistoryClampsLimit(t *testing.T) {
	d := newChatDeps()
	h := NewChatHandler(d.chatService(), d.hub)

	w := httptest.NewRecorder()
	h.HandleHistory(w, httptest.NewRequest(http.MethodGet, "/api/v1/chat/history?before_id=10&limit=5000", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if d.messages.lastLimit != 100 {
		t.Errorf("limit = %d, want it clamped to 100", d.messages.lastLimit)
	}
}

func TestChatHistorySurfacesStoreFailure(t *testing.T) {
	d := newChatDeps()
	d.messages.listErr = errStub
	h := NewChatHandler(d.chatService(), d.hub)

	w := httptest.NewRecorder()
	h.HandleHistory(w, httptest.NewRequest(http.MethodGet, "/api/v1/chat/history?before_id=10", nil))

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

// --- mute status ------------------------------------------------------------

func TestChatMuteStatusRequiresAuth(t *testing.T) {
	d := newChatDeps()
	h := NewChatHandler(d.chatService(), d.hub)

	w := httptest.NewRecorder()
	h.HandleMuteStatus(w, httptest.NewRequest(http.MethodGet, "/api/v1/chat/mute-status", nil))

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestChatMuteStatusReportsNotMuted(t *testing.T) {
	d := newChatDeps()
	h := NewChatHandler(d.chatService(), d.hub)

	req := authed(httptest.NewRequest(http.MethodGet, "/api/v1/chat/mute-status", nil), 7, 20)
	w := httptest.NewRecorder()
	h.HandleMuteStatus(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := decodeBody(t, w)
	if body["muted"] != false {
		t.Errorf("muted = %v, want false", body["muted"])
	}
	if _, present := body["expires_at"]; present {
		t.Error("expires_at must be absent when the user is not muted")
	}
}

func TestChatMuteStatusReportsActiveMute(t *testing.T) {
	d := newChatDeps()
	expires := time.Now().Add(30 * time.Minute)
	d.mutes.active = &model.ChatMute{ID: 1, UserID: 7, Reason: "flooding", ExpiresAt: expires}
	h := NewChatHandler(d.chatService(), d.hub)

	req := authed(httptest.NewRequest(http.MethodGet, "/api/v1/chat/mute-status", nil), 7, 20)
	w := httptest.NewRecorder()
	h.HandleMuteStatus(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := decodeBody(t, w)
	if body["muted"] != true {
		t.Errorf("muted = %v, want true", body["muted"])
	}
	if body["reason"] != "flooding" {
		t.Errorf("reason = %v, want \"flooding\"", body["reason"])
	}
	if body["expires_at"] != expires.Format(time.RFC3339) {
		t.Errorf("expires_at = %v, want %s", body["expires_at"], expires.Format(time.RFC3339))
	}
}

func TestChatMuteStatusSurfacesStoreFailure(t *testing.T) {
	d := newChatDeps()
	d.mutes.getErr = errStub
	h := NewChatHandler(d.chatService(), d.hub)

	req := authed(httptest.NewRequest(http.MethodGet, "/api/v1/chat/mute-status", nil), 7, 20)
	w := httptest.NewRecorder()
	h.HandleMuteStatus(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

// --- delete -----------------------------------------------------------------

func TestChatDeleteRequiresAuth(t *testing.T) {
	d := newChatDeps()
	h := NewChatHandler(d.chatService(), d.hub)

	req := withURLParam(httptest.NewRequest(http.MethodDelete, "/api/v1/chat/1", nil), "id", "1")
	w := httptest.NewRecorder()
	h.HandleDelete(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestChatDeleteRejectsInvalidID(t *testing.T) {
	d := newChatDeps()
	h := NewChatHandler(d.chatService(), d.hub)

	req := staffAuthed(httptest.NewRequest(http.MethodDelete, "/api/v1/chat/0", nil), 1)
	req = withURLParam(req, "id", "0")
	w := httptest.NewRecorder()
	h.HandleDelete(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

// Deleting a shout is staff-only: a regular member — even one with a high group
// level — must be refused.
func TestChatDeleteForbiddenForNonStaff(t *testing.T) {
	d := newChatDeps()
	h := NewChatHandler(d.chatService(), d.hub)

	req := authed(httptest.NewRequest(http.MethodDelete, "/api/v1/chat/5", nil), 7, 100)
	req = withURLParam(req, "id", "5")
	w := httptest.NewRecorder()
	h.HandleDelete(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 — only staff may delete chat messages", w.Code)
	}
	if len(d.messages.deleted) != 0 {
		t.Errorf("deleted %v despite the refusal", d.messages.deleted)
	}
	assertNoBroadcast(t, d.hub)
}

func TestChatDeleteBroadcastsRemoval(t *testing.T) {
	d := newChatDeps()
	h := NewChatHandler(d.chatService(), d.hub)

	req := staffAuthed(httptest.NewRequest(http.MethodDelete, "/api/v1/chat/5", nil), 1)
	req = withURLParam(req, "id", "5")
	w := httptest.NewRecorder()
	h.HandleDelete(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body: %s", w.Code, w.Body.String())
	}
	if len(d.messages.deleted) != 1 || d.messages.deleted[0] != 5 {
		t.Errorf("deleted = %v, want [5]", d.messages.deleted)
	}

	payload := nextBroadcast(t, d.hub)
	if payload["type"] != "delete" || payload["id"] != float64(5) {
		t.Errorf("broadcast = %v, want a delete event for id 5", payload)
	}
}

func TestChatDeleteReturns404WhenMissing(t *testing.T) {
	d := newChatDeps()
	d.messages.deleteErr = sql.ErrNoRows
	h := NewChatHandler(d.chatService(), d.hub)

	req := staffAuthed(httptest.NewRequest(http.MethodDelete, "/api/v1/chat/5", nil), 1)
	req = withURLParam(req, "id", "5")
	w := httptest.NewRecorder()
	h.HandleDelete(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
	assertNoBroadcast(t, d.hub)
}

func TestChatDeleteSurfacesStoreFailure(t *testing.T) {
	d := newChatDeps()
	d.messages.deleteErr = errStub
	h := NewChatHandler(d.chatService(), d.hub)

	req := staffAuthed(httptest.NewRequest(http.MethodDelete, "/api/v1/chat/5", nil), 1)
	req = withURLParam(req, "id", "5")
	w := httptest.NewRecorder()
	h.HandleDelete(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
	assertNoBroadcast(t, d.hub)
}
