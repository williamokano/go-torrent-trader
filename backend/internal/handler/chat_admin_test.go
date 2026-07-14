package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

func newChatAdminHandler(d *chatDeps) *ChatAdminHandler {
	return NewChatAdminHandler(d.chatService(), d.hub)
}

// --- list active mutes ------------------------------------------------------

// Non-staff must not be able to enumerate who is muted.
func TestListActiveMutesForbiddenForNonStaff(t *testing.T) {
	d := newChatDeps()
	h := newChatAdminHandler(d)

	req := authed(httptest.NewRequest(http.MethodGet, "/api/v1/admin/chat/mutes", nil), 7, 100)
	w := httptest.NewRecorder()
	h.HandleListActiveMutes(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 for a non-staff caller", w.Code)
	}
}

func TestListActiveMutesReturnsItems(t *testing.T) {
	d := newChatDeps()
	mutedBy := int64(1)
	byName := "admin"
	d.mutes.list = []repository.ChatMuteWithNames{{
		ChatMute: model.ChatMute{
			ID: 3, UserID: 7, MutedBy: &mutedBy, Reason: "flooding",
			ExpiresAt: time.Now().Add(time.Hour), CreatedAt: time.Now(),
		},
		Username:    "member",
		MutedByName: &byName,
	}}
	d.mutes.total = 1
	h := newChatAdminHandler(d)

	req := staffAuthed(httptest.NewRequest(http.MethodGet, "/api/v1/admin/chat/mutes", nil), 1)
	w := httptest.NewRecorder()
	h.HandleListActiveMutes(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	body := decodeBody(t, w)
	items, ok := body["mutes"].([]interface{})
	if !ok || len(items) != 1 {
		t.Fatalf("mutes = %v, want 1 item", body["mutes"])
	}
	mute := items[0].(map[string]interface{})
	for _, key := range []string{"id", "user_id", "username", "muted_by", "muted_by_name", "reason", "expires_at", "created_at"} {
		if _, present := mute[key]; !present {
			t.Errorf("mute response is missing key %q the frontend depends on", key)
		}
	}
	if mute["username"] != "member" {
		t.Errorf("username = %v, want \"member\"", mute["username"])
	}
	if body["total"].(float64) != 1 {
		t.Errorf("total = %v, want 1", body["total"])
	}
}

func TestListActiveMutesPaginationDefaultsAndClamping(t *testing.T) {
	cases := []struct {
		query           string
		wantPage        int
		wantPerPage     int
		clampExplanaton string
	}{
		{"", 1, 25, "defaults"},
		{"?page=3&per_page=50", 3, 50, "explicit values are honoured"},
		{"?page=0&per_page=0", 1, 25, "non-positive values fall back to the defaults"},
		{"?per_page=500", 1, 25, "per_page above the 100 cap falls back to the default"},
		{"?page=abc&per_page=abc", 1, 25, "unparseable values fall back to the defaults"},
	}
	for _, tc := range cases {
		t.Run(tc.clampExplanaton, func(t *testing.T) {
			d := newChatDeps()
			h := newChatAdminHandler(d)

			req := staffAuthed(httptest.NewRequest(http.MethodGet, "/api/v1/admin/chat/mutes"+tc.query, nil), 1)
			w := httptest.NewRecorder()
			h.HandleListActiveMutes(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", w.Code)
			}
			if d.mutes.lastPage != tc.wantPage || d.mutes.lastPer != tc.wantPerPage {
				t.Errorf("page/per_page = %d/%d, want %d/%d",
					d.mutes.lastPage, d.mutes.lastPer, tc.wantPage, tc.wantPerPage)
			}
		})
	}
}

func TestListActiveMutesSurfacesStoreFailure(t *testing.T) {
	d := newChatDeps()
	d.mutes.listErr = errStub
	h := newChatAdminHandler(d)

	req := staffAuthed(httptest.NewRequest(http.MethodGet, "/api/v1/admin/chat/mutes", nil), 1)
	w := httptest.NewRecorder()
	h.HandleListActiveMutes(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

// --- delete a single message ------------------------------------------------

func TestAdminDeleteChatMessageRequiresAuth(t *testing.T) {
	d := newChatDeps()
	h := newChatAdminHandler(d)

	req := withURLParam(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/chat/messages/1", nil), "id", "1")
	w := httptest.NewRecorder()
	h.HandleDeleteMessage(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestAdminDeleteChatMessageRejectsInvalidID(t *testing.T) {
	d := newChatDeps()
	h := newChatAdminHandler(d)

	req := staffAuthed(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/chat/messages/x", nil), 1)
	req = withURLParam(req, "id", "x")
	w := httptest.NewRecorder()
	h.HandleDeleteMessage(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestAdminDeleteChatMessageForbiddenForNonStaff(t *testing.T) {
	d := newChatDeps()
	h := newChatAdminHandler(d)

	req := authed(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/chat/messages/5", nil), 7, 100)
	req = withURLParam(req, "id", "5")
	w := httptest.NewRecorder()
	h.HandleDeleteMessage(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", w.Code)
	}
	assertNoBroadcast(t, d.hub)
}

func TestAdminDeleteChatMessageBroadcastsRemoval(t *testing.T) {
	d := newChatDeps()
	h := newChatAdminHandler(d)

	req := staffAuthed(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/chat/messages/5", nil), 1)
	req = withURLParam(req, "id", "5")
	w := httptest.NewRecorder()
	h.HandleDeleteMessage(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", w.Code)
	}
	if len(d.messages.deleted) != 1 || d.messages.deleted[0] != 5 {
		t.Errorf("deleted = %v, want [5]", d.messages.deleted)
	}
	payload := nextBroadcast(t, d.hub)
	if payload["type"] != "delete" || payload["id"] != float64(5) {
		t.Errorf("broadcast = %v, want a delete event for id 5", payload)
	}
}

func TestAdminDeleteChatMessageSurfacesStoreFailure(t *testing.T) {
	d := newChatDeps()
	d.messages.deleteErr = errStub
	h := newChatAdminHandler(d)

	req := staffAuthed(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/chat/messages/5", nil), 1)
	req = withURLParam(req, "id", "5")
	w := httptest.NewRecorder()
	h.HandleDeleteMessage(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
	assertNoBroadcast(t, d.hub)
}

// --- purge a user's messages ------------------------------------------------

func TestDeleteUserMessagesRequiresAuth(t *testing.T) {
	d := newChatDeps()
	h := newChatAdminHandler(d)

	req := withURLParam(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/chat/users/7/messages", nil), "id", "7")
	w := httptest.NewRecorder()
	h.HandleDeleteUserMessages(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestDeleteUserMessagesRejectsInvalidID(t *testing.T) {
	d := newChatDeps()
	h := newChatAdminHandler(d)

	req := staffAuthed(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/chat/users/0/messages", nil), 1)
	req = withURLParam(req, "id", "0")
	w := httptest.NewRecorder()
	h.HandleDeleteUserMessages(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestDeleteUserMessagesForbiddenForNonStaff(t *testing.T) {
	d := newChatDeps()
	h := newChatAdminHandler(d)

	req := authed(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/chat/users/7/messages", nil), 9, 100)
	req = withURLParam(req, "id", "7")
	w := httptest.NewRecorder()
	h.HandleDeleteUserMessages(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", w.Code)
	}
	if len(d.messages.deletedBy) != 0 {
		t.Errorf("purged %v despite the refusal", d.messages.deletedBy)
	}
	assertNoBroadcast(t, d.hub)
}

func TestDeleteUserMessagesReportsCountAndBroadcasts(t *testing.T) {
	d := newChatDeps()
	d.messages.userCount = 12
	h := newChatAdminHandler(d)

	req := staffAuthed(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/chat/users/7/messages", nil), 1)
	req = withURLParam(req, "id", "7")
	w := httptest.NewRecorder()
	h.HandleDeleteUserMessages(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if got := decodeBody(t, w)["deleted"]; got != float64(12) {
		t.Errorf("deleted = %v, want 12", got)
	}
	payload := nextBroadcast(t, d.hub)
	if payload["type"] != "delete_user" || payload["user_id"] != float64(7) {
		t.Errorf("broadcast = %v, want a delete_user event for user 7", payload)
	}
}

func TestDeleteUserMessagesSurfacesStoreFailure(t *testing.T) {
	d := newChatDeps()
	d.messages.bulkErr = errStub
	h := newChatAdminHandler(d)

	req := staffAuthed(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/chat/users/7/messages", nil), 1)
	req = withURLParam(req, "id", "7")
	w := httptest.NewRecorder()
	h.HandleDeleteUserMessages(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
	assertNoBroadcast(t, d.hub)
}

// --- mute -------------------------------------------------------------------

func TestMuteUserRequiresAuth(t *testing.T) {
	d := newChatDeps()
	h := newChatAdminHandler(d)

	req := withURLParam(httptest.NewRequest(http.MethodPost, "/api/v1/admin/chat/users/7/mute", strings.NewReader(`{}`)), "id", "7")
	w := httptest.NewRecorder()
	h.HandleMuteUser(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestMuteUserRejectsInvalidID(t *testing.T) {
	d := newChatDeps()
	h := newChatAdminHandler(d)

	req := staffAuthed(httptest.NewRequest(http.MethodPost, "/api/v1/admin/chat/users/x/mute", strings.NewReader(`{"duration_minutes":10}`)), 1)
	req = withURLParam(req, "id", "x")
	w := httptest.NewRecorder()
	h.HandleMuteUser(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestMuteUserRejectsBadJSON(t *testing.T) {
	d := newChatDeps()
	h := newChatAdminHandler(d)

	req := staffAuthed(httptest.NewRequest(http.MethodPost, "/api/v1/admin/chat/users/7/mute", strings.NewReader("{")), 1)
	req = withURLParam(req, "id", "7")
	w := httptest.NewRecorder()
	h.HandleMuteUser(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestMuteUserRejectsNonPositiveDuration(t *testing.T) {
	d := newChatDeps()
	h := newChatAdminHandler(d)

	for _, body := range []string{`{"duration_minutes":0}`, `{"duration_minutes":-5}`, `{}`} {
		req := staffAuthed(httptest.NewRequest(http.MethodPost, "/api/v1/admin/chat/users/7/mute", strings.NewReader(body)), 1)
		req = withURLParam(req, "id", "7")
		w := httptest.NewRecorder()
		h.HandleMuteUser(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("body %s: status = %d, want 400", body, w.Code)
		}
	}
	if len(d.mutes.created) != 0 {
		t.Errorf("created %d mutes despite the invalid duration", len(d.mutes.created))
	}
}

// The service caps a mute at 30 days; anything longer is a client error.
func TestMuteUserRejectsExcessiveDuration(t *testing.T) {
	d := newChatDeps()
	h := newChatAdminHandler(d)

	req := staffAuthed(httptest.NewRequest(http.MethodPost, "/api/v1/admin/chat/users/7/mute", strings.NewReader(`{"duration_minutes":43201}`)), 1)
	req = withURLParam(req, "id", "7")
	w := httptest.NewRecorder()
	h.HandleMuteUser(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for a mute longer than the 30-day cap", w.Code)
	}
}

func TestMuteUserForbiddenForNonStaff(t *testing.T) {
	d := newChatDeps()
	h := newChatAdminHandler(d)

	req := authed(httptest.NewRequest(http.MethodPost, "/api/v1/admin/chat/users/7/mute", strings.NewReader(`{"duration_minutes":10}`)), 9, 100)
	req = withURLParam(req, "id", "7")
	w := httptest.NewRecorder()
	h.HandleMuteUser(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", w.Code)
	}
	if len(d.mutes.created) != 0 {
		t.Error("a non-staff caller managed to create a mute")
	}
}

func TestMuteUserCreatesMute(t *testing.T) {
	d := newChatDeps()
	h := newChatAdminHandler(d)

	body := `{"duration_minutes":30,"reason":"  flooding  "}`
	req := staffAuthed(httptest.NewRequest(http.MethodPost, "/api/v1/admin/chat/users/7/mute", strings.NewReader(body)), 1)
	req = withURLParam(req, "id", "7")
	w := httptest.NewRecorder()
	h.HandleMuteUser(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
	}
	if len(d.mutes.created) != 1 {
		t.Fatalf("created %d mutes, want 1", len(d.mutes.created))
	}
	mute := d.mutes.created[0]
	if mute.UserID != 7 {
		t.Errorf("user_id = %d, want 7", mute.UserID)
	}
	if mute.MutedBy == nil || *mute.MutedBy != 1 {
		t.Errorf("muted_by = %v, want the authenticated actor 1", mute.MutedBy)
	}
	if mute.Reason != "flooding" {
		t.Errorf("reason = %q, want it trimmed to \"flooding\"", mute.Reason)
	}
	if remaining := time.Until(mute.ExpiresAt); remaining < 29*time.Minute || remaining > 31*time.Minute {
		t.Errorf("expires in %v, want roughly 30 minutes", remaining)
	}

	resp := decodeBody(t, w)
	for _, key := range []string{"id", "user_id", "muted_by", "reason", "expires_at", "created_at"} {
		if _, present := resp[key]; !present {
			t.Errorf("mute response is missing key %q", key)
		}
	}
}

func TestMuteUserSurfacesStoreFailure(t *testing.T) {
	d := newChatDeps()
	d.mutes.createErr = errStub
	h := newChatAdminHandler(d)

	req := staffAuthed(httptest.NewRequest(http.MethodPost, "/api/v1/admin/chat/users/7/mute", strings.NewReader(`{"duration_minutes":10}`)), 1)
	req = withURLParam(req, "id", "7")
	w := httptest.NewRecorder()
	h.HandleMuteUser(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

// --- unmute -----------------------------------------------------------------

func TestUnmuteUserRequiresAuth(t *testing.T) {
	d := newChatDeps()
	h := newChatAdminHandler(d)

	req := withURLParam(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/chat/users/7/mute", nil), "id", "7")
	w := httptest.NewRecorder()
	h.HandleUnmuteUser(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestUnmuteUserRejectsInvalidID(t *testing.T) {
	d := newChatDeps()
	h := newChatAdminHandler(d)

	req := staffAuthed(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/chat/users/-1/mute", nil), 1)
	req = withURLParam(req, "id", "-1")
	w := httptest.NewRecorder()
	h.HandleUnmuteUser(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestUnmuteUserForbiddenForNonStaff(t *testing.T) {
	d := newChatDeps()
	h := newChatAdminHandler(d)

	req := authed(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/chat/users/7/mute", nil), 9, 100)
	req = withURLParam(req, "id", "7")
	w := httptest.NewRecorder()
	h.HandleUnmuteUser(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", w.Code)
	}
	if len(d.mutes.unmuted) != 0 {
		t.Error("a non-staff caller managed to lift a mute")
	}
}

func TestUnmuteUserClearsMute(t *testing.T) {
	d := newChatDeps()
	h := newChatAdminHandler(d)

	req := staffAuthed(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/chat/users/7/mute", nil), 1)
	req = withURLParam(req, "id", "7")
	w := httptest.NewRecorder()
	h.HandleUnmuteUser(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", w.Code)
	}
	if len(d.mutes.unmuted) != 1 || d.mutes.unmuted[0] != 7 {
		t.Errorf("unmuted = %v, want [7]", d.mutes.unmuted)
	}
}

func TestUnmuteUserSurfacesStoreFailure(t *testing.T) {
	d := newChatDeps()
	d.mutes.deleteErr = errStub
	h := newChatAdminHandler(d)

	req := staffAuthed(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/chat/users/7/mute", nil), 1)
	req = withURLParam(req, "id", "7")
	w := httptest.NewRecorder()
	h.HandleUnmuteUser(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}
