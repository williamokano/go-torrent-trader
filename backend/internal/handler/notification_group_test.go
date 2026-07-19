package handler

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

func topicReplyNotif(id, userID, topicID int64, actor string, read bool, createdAt time.Time) model.Notification {
	return model.Notification{
		ID:     id,
		UserID: userID,
		Type:   model.NotifTopicReply,
		Data: json.RawMessage(fmt.Sprintf(
			`{"topic_id": %d, "topic_title": "Topic %d", "actor_username": %q}`,
			topicID, topicID, actor,
		)),
		Read:      read,
		CreatedAt: createdAt,
	}
}

func TestListNotificationsGroupedRequiresAuth(t *testing.T) {
	h := NewNotificationHandler(newNotifService(&notifDeps{
		store: &notifStoreStub{}, prefs: &notifPrefStub{}, subs: &topicSubStub{},
	}))

	req := httptest.NewRequest("GET", "/api/v1/notifications/grouped", nil)
	w := httptest.NewRecorder()
	h.HandleListNotificationsGrouped(w, req)

	if w.Code != 401 {
		t.Errorf("status = %d, want 401 for an unauthenticated request", w.Code)
	}
}

func TestListNotificationsGroupedCollapsesSameTopic(t *testing.T) {
	now := time.Now()
	store := &notifStoreStub{
		list: []model.Notification{
			topicReplyNotif(3, 7, 1, "carol", false, now),
			topicReplyNotif(2, 7, 1, "bob", false, now.Add(-time.Minute)),
			topicReplyNotif(1, 7, 1, "alice", true, now.Add(-2*time.Minute)),
		},
		total: 3,
	}
	h := NewNotificationHandler(newNotifService(&notifDeps{
		store: store, prefs: &notifPrefStub{}, subs: &topicSubStub{},
	}))

	req := authed(httptest.NewRequest("GET", "/api/v1/notifications/grouped", nil), 7, 1)
	w := httptest.NewRecorder()
	h.HandleListNotificationsGrouped(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := decodeBody(t, w)
	if body["total"].(float64) != 1 {
		t.Fatalf("total = %v, want 1 (all three collapse into one topic group)", body["total"])
	}
	groups, ok := body["groups"].([]interface{})
	if !ok || len(groups) != 1 {
		t.Fatalf("groups = %v, want 1 item", body["groups"])
	}
	g := groups[0].(map[string]interface{})
	if g["count"].(float64) != 3 {
		t.Errorf("count = %v, want 3", g["count"])
	}
	if g["unread"] != true {
		t.Errorf("unread = %v, want true (2 of 3 underlying are unread)", g["unread"])
	}
	actors, ok := g["last_actors"].([]interface{})
	if !ok || len(actors) != 3 {
		t.Fatalf("last_actors = %v, want 3 entries", g["last_actors"])
	}
	if actors[0] != "carol" {
		t.Errorf("last_actors[0] = %v, want carol (most recent)", actors[0])
	}
	notifs, ok := g["notifications"].([]interface{})
	if !ok || len(notifs) != 3 {
		t.Fatalf("notifications = %v, want 3 underlying entries for expand-on-click", g["notifications"])
	}
}

func TestListNotificationsGroupedSurfacesStoreFailure(t *testing.T) {
	h := NewNotificationHandler(newNotifService(&notifDeps{
		store: &notifStoreStub{listErr: fmt.Errorf("db down")},
		prefs: &notifPrefStub{}, subs: &topicSubStub{},
	}))

	req := authed(httptest.NewRequest("GET", "/api/v1/notifications/grouped", nil), 7, 1)
	w := httptest.NewRecorder()
	h.HandleListNotificationsGrouped(w, req)

	if w.Code != 500 {
		t.Errorf("status = %d, want 500 when the store fails", w.Code)
	}
}

func TestListNotificationsGroupedNonTopicReplyStaysSingleton(t *testing.T) {
	store := &notifStoreStub{
		list: []model.Notification{
			{ID: 1, UserID: 7, Type: model.NotifMention, Data: json.RawMessage(`{"actor_username":"alice"}`), CreatedAt: time.Now()},
			{ID: 2, UserID: 7, Type: model.NotifPMReceived, Data: json.RawMessage(`{"actor_username":"bob"}`), CreatedAt: time.Now()},
		},
		total: 2,
	}
	h := NewNotificationHandler(newNotifService(&notifDeps{
		store: store, prefs: &notifPrefStub{}, subs: &topicSubStub{},
	}))

	req := authed(httptest.NewRequest("GET", "/api/v1/notifications/grouped", nil), 7, 1)
	w := httptest.NewRecorder()
	h.HandleListNotificationsGrouped(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := decodeBody(t, w)
	if body["total"].(float64) != 2 {
		t.Fatalf("total = %v, want 2 (non-topic_reply types are not collapsed)", body["total"])
	}
}

// --- HandleMarkGroupRead ------------------------------------------------

func TestMarkGroupReadRequiresAuth(t *testing.T) {
	h := NewNotificationHandler(newNotifService(&notifDeps{
		store: &notifStoreStub{}, prefs: &notifPrefStub{}, subs: &topicSubStub{},
	}))

	req := httptest.NewRequest("PUT", "/api/v1/notifications/groups/topic_reply:1/read", nil)
	req = withURLParam(req, "key", "topic_reply:1")
	w := httptest.NewRecorder()
	h.HandleMarkGroupRead(w, req)

	if w.Code != 401 {
		t.Errorf("status = %d, want 401 for an unauthenticated request", w.Code)
	}
}

func TestMarkGroupReadDelegatesToTheStoreForATopicReplyKey(t *testing.T) {
	store := &notifStoreStub{markGroupResult: 3}
	h := NewNotificationHandler(newNotifService(&notifDeps{
		store: store, prefs: &notifPrefStub{}, subs: &topicSubStub{},
	}))

	req := authed(httptest.NewRequest("PUT", "/api/v1/notifications/groups/topic_reply:42/read", nil), 7, 1)
	req = withURLParam(req, "key", "topic_reply:42")
	w := httptest.NewRecorder()
	h.HandleMarkGroupRead(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if !store.markGroupCalled {
		t.Fatal("expected the store's batch mark-read to be called")
	}
	if store.markGroupUserID != 7 {
		t.Errorf("marked for user %d, want the authenticated user 7", store.markGroupUserID)
	}
	if store.markGroupTopicID != 42 {
		t.Errorf("marked topic %d, want 42 (parsed from the group key)", store.markGroupTopicID)
	}
	body := decodeBody(t, w)
	if body["marked"].(float64) != 3 {
		t.Errorf("marked = %v, want 3", body["marked"])
	}
}

// A "single:<id>" key identifies a singleton group, which is never
// batchable (the UI already covers it with the single-notification
// MarkRead endpoint) -- the handler must reject it rather than silently
// no-op or guess at a notification ID to mark.
func TestMarkGroupReadRejectsASingletonKey(t *testing.T) {
	store := &notifStoreStub{}
	h := NewNotificationHandler(newNotifService(&notifDeps{
		store: store, prefs: &notifPrefStub{}, subs: &topicSubStub{},
	}))

	req := authed(httptest.NewRequest("PUT", "/api/v1/notifications/groups/single:9/read", nil), 7, 1)
	req = withURLParam(req, "key", "single:9")
	w := httptest.NewRecorder()
	h.HandleMarkGroupRead(w, req)

	if w.Code != 400 {
		t.Errorf("status = %d, want 400 for a non-batchable group key", w.Code)
	}
	if store.markGroupCalled {
		t.Error("expected the store's batch mark-read to never be called for an unbatchable key")
	}
}

func TestMarkGroupReadRejectsAMalformedKey(t *testing.T) {
	h := NewNotificationHandler(newNotifService(&notifDeps{
		store: &notifStoreStub{}, prefs: &notifPrefStub{}, subs: &topicSubStub{},
	}))

	req := authed(httptest.NewRequest("PUT", "/api/v1/notifications/groups/not-a-key/read", nil), 7, 1)
	req = withURLParam(req, "key", "not-a-key")
	w := httptest.NewRecorder()
	h.HandleMarkGroupRead(w, req)

	if w.Code != 400 {
		t.Errorf("status = %d, want 400 for a malformed group key", w.Code)
	}
}

func TestMarkGroupReadIsIdempotentWhenNothingIsUnread(t *testing.T) {
	store := &notifStoreStub{markGroupResult: 0}
	h := NewNotificationHandler(newNotifService(&notifDeps{
		store: store, prefs: &notifPrefStub{}, subs: &topicSubStub{},
	}))

	req := authed(httptest.NewRequest("PUT", "/api/v1/notifications/groups/topic_reply:42/read", nil), 7, 1)
	req = withURLParam(req, "key", "topic_reply:42")
	w := httptest.NewRecorder()
	h.HandleMarkGroupRead(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200 (a second call with nothing left unread is not an error)", w.Code)
	}
	if decodeBody(t, w)["marked"].(float64) != 0 {
		t.Errorf("marked = %v, want 0", decodeBody(t, w)["marked"])
	}
}

func TestMarkGroupReadSurfacesStoreFailure(t *testing.T) {
	store := &notifStoreStub{markGroupErr: fmt.Errorf("db down")}
	h := NewNotificationHandler(newNotifService(&notifDeps{
		store: store, prefs: &notifPrefStub{}, subs: &topicSubStub{},
	}))

	req := authed(httptest.NewRequest("PUT", "/api/v1/notifications/groups/topic_reply:42/read", nil), 7, 1)
	req = withURLParam(req, "key", "topic_reply:42")
	w := httptest.NewRecorder()
	h.HandleMarkGroupRead(w, req)

	if w.Code != 500 {
		t.Errorf("status = %d, want 500 when the store fails", w.Code)
	}
}
