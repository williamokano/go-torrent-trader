package handler

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

func newMessageHandler(messages *stubMessageRepo, users *stubUserRepo) *MessageHandler {
	return NewMessageHandler(service.NewMessageService(messages, users, event.NewInMemoryBus()))
}

// --- send -------------------------------------------------------------------

func TestSendMessageRequiresAuth(t *testing.T) {
	h := newMessageHandler(newStubMessageRepo(), newStubUserRepo())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/messages", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	h.HandleSendMessage(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 for an unauthenticated send", w.Code)
	}
}

func TestSendMessageRejectsBadJSON(t *testing.T) {
	h := newMessageHandler(newStubMessageRepo(), newStubUserRepo())

	req := authed(httptest.NewRequest(http.MethodPost, "/api/v1/messages", strings.NewReader("{oops")), 7, 1)
	w := httptest.NewRecorder()
	h.HandleSendMessage(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for a malformed body", w.Code)
	}
}

func TestSendMessageCreatesMessage(t *testing.T) {
	messages := newStubMessageRepo()
	users := newStubUserRepo(&model.User{ID: 9, Username: "receiver"})
	h := newMessageHandler(messages, users)

	body := `{"receiver_id":9,"subject":"hi","body":"hello there"}`
	req := authed(httptest.NewRequest(http.MethodPost, "/api/v1/messages", strings.NewReader(body)), 7, 1)
	w := httptest.NewRecorder()
	h.HandleSendMessage(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
	}
	if len(messages.created) != 1 {
		t.Fatalf("created %d messages, want 1", len(messages.created))
	}
	created := messages.created[0]
	if created.SenderID != 7 {
		t.Errorf("sender_id = %d, want the authenticated user 7 — the sender must never be taken from the body", created.SenderID)
	}
	if created.ReceiverID != 9 {
		t.Errorf("receiver_id = %d, want 9", created.ReceiverID)
	}

	msg, ok := decodeBody(t, w)["message"].(map[string]interface{})
	if !ok {
		t.Fatal("response has no message object")
	}
	for _, key := range []string{"id", "sender_id", "receiver_id", "subject", "body", "is_read", "created_at"} {
		if _, present := msg[key]; !present {
			t.Errorf("message response is missing key %q the frontend depends on", key)
		}
	}
}

func TestSendMessageRejectsEmptySubject(t *testing.T) {
	h := newMessageHandler(newStubMessageRepo(), newStubUserRepo(&model.User{ID: 9}))

	body := `{"receiver_id":9,"subject":"   ","body":"hello"}`
	req := authed(httptest.NewRequest(http.MethodPost, "/api/v1/messages", strings.NewReader(body)), 7, 1)
	w := httptest.NewRecorder()
	h.HandleSendMessage(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for a blank subject", w.Code)
	}
}

func TestSendMessageRejectsSelfAddressedMessage(t *testing.T) {
	h := newMessageHandler(newStubMessageRepo(), newStubUserRepo(&model.User{ID: 7}))

	body := `{"receiver_id":7,"subject":"hi","body":"hello"}`
	req := authed(httptest.NewRequest(http.MethodPost, "/api/v1/messages", strings.NewReader(body)), 7, 1)
	w := httptest.NewRecorder()
	h.HandleSendMessage(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 when sending a message to yourself", w.Code)
	}
}

func TestSendMessageRejectsUnknownReceiver(t *testing.T) {
	h := newMessageHandler(newStubMessageRepo(), newStubUserRepo())

	body := `{"receiver_id":404,"subject":"hi","body":"hello"}`
	req := authed(httptest.NewRequest(http.MethodPost, "/api/v1/messages", strings.NewReader(body)), 7, 1)
	w := httptest.NewRecorder()
	h.HandleSendMessage(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 when the receiver does not exist", w.Code)
	}
}

func TestSendMessageSurfacesRepositoryFailure(t *testing.T) {
	messages := newStubMessageRepo()
	messages.createErr = errStub
	h := newMessageHandler(messages, newStubUserRepo(&model.User{ID: 9}))

	body := `{"receiver_id":9,"subject":"hi","body":"hello"}`
	req := authed(httptest.NewRequest(http.MethodPost, "/api/v1/messages", strings.NewReader(body)), 7, 1)
	w := httptest.NewRecorder()
	h.HandleSendMessage(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 when the store fails — it must not report success", w.Code)
	}
}

// --- inbox / outbox ---------------------------------------------------------

func TestListInboxRequiresAuth(t *testing.T) {
	h := newMessageHandler(newStubMessageRepo(), newStubUserRepo())

	w := httptest.NewRecorder()
	h.HandleListInbox(w, httptest.NewRequest(http.MethodGet, "/api/v1/messages/inbox", nil))

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestListInboxReturnsItemsAndPaginationDefaults(t *testing.T) {
	messages := newStubMessageRepo()
	messages.inbox = []model.Message{{ID: 1, SenderID: 3, ReceiverID: 7, Subject: "s"}}
	messages.total = 1
	h := newMessageHandler(messages, newStubUserRepo())

	req := authed(httptest.NewRequest(http.MethodGet, "/api/v1/messages/inbox", nil), 7, 1)
	w := httptest.NewRecorder()
	h.HandleListInbox(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := decodeBody(t, w)
	items, ok := body["messages"].([]interface{})
	if !ok || len(items) != 1 {
		t.Fatalf("messages = %v, want 1 item", body["messages"])
	}
	if body["total"].(float64) != 1 {
		t.Errorf("total = %v, want 1", body["total"])
	}
	if body["page"].(float64) != 1 {
		t.Errorf("page = %v, want the default 1", body["page"])
	}
	if body["per_page"].(float64) != 25 {
		t.Errorf("per_page = %v, want the default 25", body["per_page"])
	}
}

func TestListInboxEchoesRequestedPagination(t *testing.T) {
	h := newMessageHandler(newStubMessageRepo(), newStubUserRepo())

	req := authed(httptest.NewRequest(http.MethodGet, "/api/v1/messages/inbox?page=3&per_page=10", nil), 7, 1)
	w := httptest.NewRecorder()
	h.HandleListInbox(w, req)

	body := decodeBody(t, w)
	if body["page"].(float64) != 3 || body["per_page"].(float64) != 10 {
		t.Errorf("page/per_page = %v/%v, want 3/10", body["page"], body["per_page"])
	}
}

func TestListInboxSurfacesStoreFailure(t *testing.T) {
	messages := newStubMessageRepo()
	messages.listErr = errStub
	h := newMessageHandler(messages, newStubUserRepo())

	req := authed(httptest.NewRequest(http.MethodGet, "/api/v1/messages/inbox", nil), 7, 1)
	w := httptest.NewRecorder()
	h.HandleListInbox(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestListOutboxRequiresAuth(t *testing.T) {
	h := newMessageHandler(newStubMessageRepo(), newStubUserRepo())

	w := httptest.NewRecorder()
	h.HandleListOutbox(w, httptest.NewRequest(http.MethodGet, "/api/v1/messages/outbox", nil))

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestListOutboxReturnsSentMessages(t *testing.T) {
	messages := newStubMessageRepo()
	messages.outbox = []model.Message{{ID: 4, SenderID: 7, ReceiverID: 9}}
	messages.total = 1
	h := newMessageHandler(messages, newStubUserRepo())

	req := authed(httptest.NewRequest(http.MethodGet, "/api/v1/messages/outbox", nil), 7, 1)
	w := httptest.NewRecorder()
	h.HandleListOutbox(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	items := decodeBody(t, w)["messages"].([]interface{})
	if len(items) != 1 {
		t.Fatalf("messages = %v, want 1 item", items)
	}
}

func TestListOutboxSurfacesStoreFailure(t *testing.T) {
	messages := newStubMessageRepo()
	messages.listErr = errStub
	h := newMessageHandler(messages, newStubUserRepo())

	req := authed(httptest.NewRequest(http.MethodGet, "/api/v1/messages/outbox", nil), 7, 1)
	w := httptest.NewRecorder()
	h.HandleListOutbox(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

// --- get --------------------------------------------------------------------

func TestGetMessageRequiresAuth(t *testing.T) {
	h := newMessageHandler(newStubMessageRepo(), newStubUserRepo())

	req := withURLParam(httptest.NewRequest(http.MethodGet, "/api/v1/messages/1", nil), "id", "1")
	w := httptest.NewRecorder()
	h.HandleGetMessage(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestGetMessageRejectsInvalidID(t *testing.T) {
	h := newMessageHandler(newStubMessageRepo(), newStubUserRepo())

	for _, id := range []string{"abc", "0", "-3"} {
		req := authed(httptest.NewRequest(http.MethodGet, "/api/v1/messages/"+id, nil), 7, 1)
		req = withURLParam(req, "id", id)
		w := httptest.NewRecorder()
		h.HandleGetMessage(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("id %q: status = %d, want 400", id, w.Code)
		}
	}
}

// A message must not be readable by anyone other than its two participants.
func TestGetMessageHidesOtherPeoplesMail(t *testing.T) {
	messages := newStubMessageRepo()
	messages.byID[1] = &model.Message{ID: 1, SenderID: 3, ReceiverID: 9}
	h := newMessageHandler(messages, newStubUserRepo())

	req := authed(httptest.NewRequest(http.MethodGet, "/api/v1/messages/1", nil), 7, 1) // neither sender nor receiver
	req = withURLParam(req, "id", "1")
	w := httptest.NewRecorder()
	h.HandleGetMessage(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 — a third party must not be able to read the message", w.Code)
	}
}

func TestGetMessageMarksReceiverMailAsRead(t *testing.T) {
	messages := newStubMessageRepo()
	messages.byID[1] = &model.Message{ID: 1, SenderID: 3, ReceiverID: 7, Subject: "s", IsRead: false}
	h := newMessageHandler(messages, newStubUserRepo())

	req := authed(httptest.NewRequest(http.MethodGet, "/api/v1/messages/1", nil), 7, 1)
	req = withURLParam(req, "id", "1")
	w := httptest.NewRecorder()
	h.HandleGetMessage(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if len(messages.markedRead) != 1 || messages.markedRead[0] != 1 {
		t.Errorf("marked read = %v, want [1] — opening an unread message must mark it read", messages.markedRead)
	}
	msg := decodeBody(t, w)["message"].(map[string]interface{})
	if msg["is_read"] != true {
		t.Error("is_read = false, want true in the response of a just-read message")
	}
}

func TestGetMessageReturns404ForMissingMessage(t *testing.T) {
	messages := newStubMessageRepo()
	messages.getErr = sql.ErrNoRows
	h := newMessageHandler(messages, newStubUserRepo())

	req := authed(httptest.NewRequest(http.MethodGet, "/api/v1/messages/1", nil), 7, 1)
	req = withURLParam(req, "id", "1")
	w := httptest.NewRecorder()
	h.HandleGetMessage(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestGetMessageIncludesParentIDOnReplies(t *testing.T) {
	parent := int64(42)
	messages := newStubMessageRepo()
	messages.byID[1] = &model.Message{ID: 1, SenderID: 7, ReceiverID: 9, ParentID: &parent, IsRead: true}
	h := newMessageHandler(messages, newStubUserRepo())

	req := authed(httptest.NewRequest(http.MethodGet, "/api/v1/messages/1", nil), 7, 1)
	req = withURLParam(req, "id", "1")
	w := httptest.NewRecorder()
	h.HandleGetMessage(w, req)

	msg := decodeBody(t, w)["message"].(map[string]interface{})
	if msg["parent_id"] != float64(42) {
		t.Errorf("parent_id = %v, want 42", msg["parent_id"])
	}
}

// --- delete -----------------------------------------------------------------

func TestDeleteMessageRequiresAuth(t *testing.T) {
	h := newMessageHandler(newStubMessageRepo(), newStubUserRepo())

	req := withURLParam(httptest.NewRequest(http.MethodDelete, "/api/v1/messages/1", nil), "id", "1")
	w := httptest.NewRecorder()
	h.HandleDeleteMessage(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestDeleteMessageRejectsInvalidID(t *testing.T) {
	h := newMessageHandler(newStubMessageRepo(), newStubUserRepo())

	req := authed(httptest.NewRequest(http.MethodDelete, "/api/v1/messages/0", nil), 7, 1)
	req = withURLParam(req, "id", "0")
	w := httptest.NewRecorder()
	h.HandleDeleteMessage(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestDeleteMessageSoftDeletesForCaller(t *testing.T) {
	messages := newStubMessageRepo()
	h := newMessageHandler(messages, newStubUserRepo())

	req := authed(httptest.NewRequest(http.MethodDelete, "/api/v1/messages/5", nil), 7, 1)
	req = withURLParam(req, "id", "5")
	w := httptest.NewRecorder()
	h.HandleDeleteMessage(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", w.Code)
	}
	if len(messages.deleted) != 1 || messages.deleted[0] != 5 {
		t.Errorf("deleted = %v, want [5]", messages.deleted)
	}
}

func TestDeleteMessageReturns404WhenMissing(t *testing.T) {
	messages := newStubMessageRepo()
	messages.deleteErr = sql.ErrNoRows
	h := newMessageHandler(messages, newStubUserRepo())

	req := authed(httptest.NewRequest(http.MethodDelete, "/api/v1/messages/5", nil), 7, 1)
	req = withURLParam(req, "id", "5")
	w := httptest.NewRecorder()
	h.HandleDeleteMessage(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestDeleteMessageSurfacesStoreFailure(t *testing.T) {
	messages := newStubMessageRepo()
	messages.deleteErr = errStub
	h := newMessageHandler(messages, newStubUserRepo())

	req := authed(httptest.NewRequest(http.MethodDelete, "/api/v1/messages/5", nil), 7, 1)
	req = withURLParam(req, "id", "5")
	w := httptest.NewRecorder()
	h.HandleDeleteMessage(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

// --- unread count -----------------------------------------------------------

func TestMessageUnreadCountRequiresAuth(t *testing.T) {
	h := newMessageHandler(newStubMessageRepo(), newStubUserRepo())

	w := httptest.NewRecorder()
	h.HandleUnreadCount(w, httptest.NewRequest(http.MethodGet, "/api/v1/messages/unread-count", nil))

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestMessageUnreadCountReturnsCount(t *testing.T) {
	messages := newStubMessageRepo()
	messages.unread = 3
	h := newMessageHandler(messages, newStubUserRepo())

	req := authed(httptest.NewRequest(http.MethodGet, "/api/v1/messages/unread-count", nil), 7, 1)
	w := httptest.NewRecorder()
	h.HandleUnreadCount(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	// The frontend badge reads "unread_count" — not "count".
	if got := decodeBody(t, w)["unread_count"]; got != float64(3) {
		t.Errorf("unread_count = %v, want 3", got)
	}
}

func TestMessageUnreadCountSurfacesStoreFailure(t *testing.T) {
	messages := newStubMessageRepo()
	messages.unreadErr = errStub
	h := newMessageHandler(messages, newStubUserRepo())

	req := authed(httptest.NewRequest(http.MethodGet, "/api/v1/messages/unread-count", nil), 7, 1)
	w := httptest.NewRecorder()
	h.HandleUnreadCount(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}
