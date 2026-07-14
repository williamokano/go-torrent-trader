package handler

import (
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

// Error paths and preference endpoints that notification_test.go leaves open.

func TestListNotificationsHonoursUnreadOnlyAndPagination(t *testing.T) {
	store := &notifStoreStub{list: []model.Notification{{ID: 1, UserID: 7}}, total: 1}
	h := NewNotificationHandler(newNotifService(&notifDeps{
		store: store, prefs: &notifPrefStub{}, subs: &topicSubStub{},
	}))

	req := authed(httptest.NewRequest(http.MethodGet,
		"/api/v1/notifications?unread_only=true&page=3&per_page=10", nil), 7, 1)
	w := httptest.NewRecorder()
	h.HandleListNotifications(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := decodeBody(t, w)
	if body["page"].(float64) != 3 || body["per_page"].(float64) != 10 {
		t.Errorf("page/per_page = %v/%v, want 3/10", body["page"], body["per_page"])
	}
}

func TestUnreadCountRequiresAuth(t *testing.T) {
	h := NewNotificationHandler(newNotifService(&notifDeps{
		store: &notifStoreStub{}, prefs: &notifPrefStub{}, subs: &topicSubStub{},
	}))

	w := httptest.NewRecorder()
	h.HandleUnreadCount(w, httptest.NewRequest(http.MethodGet, "/api/v1/notifications/unread-count", nil))

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestUnreadCountSurfacesStoreFailure(t *testing.T) {
	h := NewNotificationHandler(newNotifService(&notifDeps{
		store: &notifStoreStub{unreadErr: errors.New("db down")},
		prefs: &notifPrefStub{}, subs: &topicSubStub{},
	}))

	req := authed(httptest.NewRequest(http.MethodGet, "/api/v1/notifications/unread-count", nil), 7, 1)
	w := httptest.NewRecorder()
	h.HandleUnreadCount(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestMarkReadRequiresAuth(t *testing.T) {
	h := NewNotificationHandler(newNotifService(&notifDeps{
		store: &notifStoreStub{}, prefs: &notifPrefStub{}, subs: &topicSubStub{},
	}))

	req := withURLParam(httptest.NewRequest(http.MethodPut, "/api/v1/notifications/1/read", nil), "id", "1")
	w := httptest.NewRecorder()
	h.HandleMarkRead(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestMarkReadReturns404WhenNotificationMissing(t *testing.T) {
	h := NewNotificationHandler(newNotifService(&notifDeps{
		store: &notifStoreStub{markErr: sql.ErrNoRows},
		prefs: &notifPrefStub{}, subs: &topicSubStub{},
	}))

	req := authed(httptest.NewRequest(http.MethodPut, "/api/v1/notifications/1/read", nil), 7, 1)
	req = withURLParam(req, "id", "1")
	w := httptest.NewRecorder()
	h.HandleMarkRead(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestMarkReadSurfacesStoreFailure(t *testing.T) {
	h := NewNotificationHandler(newNotifService(&notifDeps{
		store: &notifStoreStub{markErr: errors.New("db down")},
		prefs: &notifPrefStub{}, subs: &topicSubStub{},
	}))

	req := authed(httptest.NewRequest(http.MethodPut, "/api/v1/notifications/1/read", nil), 7, 1)
	req = withURLParam(req, "id", "1")
	w := httptest.NewRecorder()
	h.HandleMarkRead(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestMarkAllReadRequiresAuth(t *testing.T) {
	h := NewNotificationHandler(newNotifService(&notifDeps{
		store: &notifStoreStub{}, prefs: &notifPrefStub{}, subs: &topicSubStub{},
	}))

	w := httptest.NewRecorder()
	h.HandleMarkAllRead(w, httptest.NewRequest(http.MethodPut, "/api/v1/notifications/read-all", nil))

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestMarkAllReadSurfacesStoreFailure(t *testing.T) {
	h := NewNotificationHandler(newNotifService(&notifDeps{
		store: &notifStoreStub{markAllErr: errors.New("db down")},
		prefs: &notifPrefStub{}, subs: &topicSubStub{},
	}))

	req := authed(httptest.NewRequest(http.MethodPut, "/api/v1/notifications/read-all", nil), 7, 1)
	w := httptest.NewRecorder()
	h.HandleMarkAllRead(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

// --- preferences ------------------------------------------------------------

func TestGetPreferencesRequiresAuth(t *testing.T) {
	h := NewNotificationHandler(newNotifService(&notifDeps{
		store: &notifStoreStub{}, prefs: &notifPrefStub{}, subs: &topicSubStub{},
	}))

	w := httptest.NewRecorder()
	h.HandleGetPreferences(w, httptest.NewRequest(http.MethodGet, "/api/v1/notifications/preferences", nil))

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

// Every known notification type must come back, defaulting to enabled, so the
// settings screen can render a complete list.
func TestGetPreferencesReturnsEveryTypeWithDefaults(t *testing.T) {
	h := NewNotificationHandler(newNotifService(&notifDeps{
		store: &notifStoreStub{},
		prefs: &notifPrefStub{all: []model.NotificationPreference{
			{UserID: 7, NotificationType: model.NotifPMReceived, Enabled: false},
		}},
		subs: &topicSubStub{},
	}))

	req := authed(httptest.NewRequest(http.MethodGet, "/api/v1/notifications/preferences", nil), 7, 1)
	w := httptest.NewRecorder()
	h.HandleGetPreferences(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	items, ok := decodeBody(t, w)["preferences"].([]interface{})
	if !ok || len(items) != len(model.AllNotificationTypes) {
		t.Fatalf("preferences = %v, want one entry per known type (%d)", items, len(model.AllNotificationTypes))
	}

	byType := map[string]bool{}
	for _, raw := range items {
		p := raw.(map[string]interface{})
		byType[p["notification_type"].(string)] = p["enabled"].(bool)
	}
	if byType[model.NotifPMReceived] {
		t.Error("the explicitly disabled type came back enabled")
	}
	if !byType[model.NotifForumReply] {
		t.Error("an unset type must default to enabled")
	}
}

func TestGetPreferencesSurfacesStoreFailure(t *testing.T) {
	h := NewNotificationHandler(newNotifService(&notifDeps{
		store: &notifStoreStub{},
		prefs: &notifPrefStub{getErr: errors.New("db down")},
		subs:  &topicSubStub{},
	}))

	req := authed(httptest.NewRequest(http.MethodGet, "/api/v1/notifications/preferences", nil), 7, 1)
	w := httptest.NewRecorder()
	h.HandleGetPreferences(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestUpdatePreferencesRequiresAuth(t *testing.T) {
	h := NewNotificationHandler(newNotifService(&notifDeps{
		store: &notifStoreStub{}, prefs: &notifPrefStub{}, subs: &topicSubStub{},
	}))

	req := httptest.NewRequest(http.MethodPut, "/api/v1/notifications/preferences",
		strings.NewReader(`{"notification_type":"system","enabled":false}`))
	w := httptest.NewRecorder()
	h.HandleUpdatePreferences(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestUpdatePreferencesValidatesBody(t *testing.T) {
	cases := []struct{ name, body string }{
		{"missing notification_type", `{"enabled":true}`},
		{"missing enabled", `{"notification_type":"system"}`},
		{"unknown notification type", `{"notification_type":"carrier_pigeon","enabled":true}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prefs := &notifPrefStub{}
			h := NewNotificationHandler(newNotifService(&notifDeps{
				store: &notifStoreStub{}, prefs: prefs, subs: &topicSubStub{},
			}))

			req := authed(httptest.NewRequest(http.MethodPut, "/api/v1/notifications/preferences",
				strings.NewReader(tc.body)), 7, 1)
			w := httptest.NewRecorder()
			h.HandleUpdatePreferences(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", w.Code)
			}
			if len(prefs.set) != 0 {
				t.Errorf("a preference was written anyway: %v", prefs.set)
			}
		})
	}
}

func TestUpdatePreferencesStoresTheChoice(t *testing.T) {
	prefs := &notifPrefStub{}
	h := NewNotificationHandler(newNotifService(&notifDeps{
		store: &notifStoreStub{}, prefs: prefs, subs: &topicSubStub{},
	}))

	req := authed(httptest.NewRequest(http.MethodPut, "/api/v1/notifications/preferences",
		strings.NewReader(`{"notification_type":"pm_received","enabled":false}`)), 7, 1)
	w := httptest.NewRecorder()
	h.HandleUpdatePreferences(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body: %s", w.Code, w.Body.String())
	}
	enabled, set := prefs.set[model.NotifPMReceived]
	if !set || enabled {
		t.Errorf("stored preferences = %v, want pm_received disabled", prefs.set)
	}
}

func TestUpdatePreferencesSurfacesStoreFailure(t *testing.T) {
	h := NewNotificationHandler(newNotifService(&notifDeps{
		store: &notifStoreStub{},
		prefs: &notifPrefStub{setErr: errors.New("db down")},
		subs:  &topicSubStub{},
	}))

	req := authed(httptest.NewRequest(http.MethodPut, "/api/v1/notifications/preferences",
		strings.NewReader(`{"notification_type":"system","enabled":true}`)), 7, 1)
	w := httptest.NewRecorder()
	h.HandleUpdatePreferences(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

// --- topic subscriptions ----------------------------------------------------

func TestSubscribeRequiresAuth(t *testing.T) {
	h := NewTopicSubscriptionHandler(newNotifService(&notifDeps{
		store: &notifStoreStub{}, prefs: &notifPrefStub{}, subs: &topicSubStub{},
	}))

	req := withURLParam(httptest.NewRequest(http.MethodPost, "/api/v1/forums/topics/5/subscribe", nil), "id", "5")
	w := httptest.NewRecorder()
	h.HandleSubscribe(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestSubscribeSurfacesStoreFailure(t *testing.T) {
	h := NewTopicSubscriptionHandler(newNotifService(&notifDeps{
		store:  &notifStoreStub{},
		prefs:  &notifPrefStub{},
		subs:   &topicSubStub{subscribeErr: errors.New("db down")},
		topics: &topicLookupStub{topic: &model.ForumTopic{ID: 5, ForumID: 3}},
		forums: &forumLookupStub{forum: &model.Forum{ID: 3, MinGroupLevel: 0}},
	}))

	req := authed(httptest.NewRequest(http.MethodPost, "/api/v1/forums/topics/5/subscribe", nil), 7, 5)
	req = withURLParam(req, "id", "5")
	w := httptest.NewRecorder()
	h.HandleSubscribe(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestUnsubscribeRequiresAuth(t *testing.T) {
	h := NewTopicSubscriptionHandler(newNotifService(&notifDeps{
		store: &notifStoreStub{}, prefs: &notifPrefStub{}, subs: &topicSubStub{},
	}))

	req := withURLParam(httptest.NewRequest(http.MethodDelete, "/api/v1/forums/topics/5/subscribe", nil), "id", "5")
	w := httptest.NewRecorder()
	h.HandleUnsubscribe(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestUnsubscribeRejectsInvalidTopicID(t *testing.T) {
	h := NewTopicSubscriptionHandler(newNotifService(&notifDeps{
		store: &notifStoreStub{}, prefs: &notifPrefStub{}, subs: &topicSubStub{},
	}))

	req := authed(httptest.NewRequest(http.MethodDelete, "/api/v1/forums/topics/abc/subscribe", nil), 7, 1)
	req = withURLParam(req, "id", "abc")
	w := httptest.NewRecorder()
	h.HandleUnsubscribe(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestUnsubscribeSurfacesStoreFailure(t *testing.T) {
	h := NewTopicSubscriptionHandler(newNotifService(&notifDeps{
		store: &notifStoreStub{}, prefs: &notifPrefStub{},
		subs: &topicSubStub{unsubscribeErr: errors.New("db down")},
	}))

	req := authed(httptest.NewRequest(http.MethodDelete, "/api/v1/forums/topics/5/subscribe", nil), 7, 1)
	req = withURLParam(req, "id", "5")
	w := httptest.NewRecorder()
	h.HandleUnsubscribe(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestGetSubscriptionRejectsInvalidTopicID(t *testing.T) {
	h := NewTopicSubscriptionHandler(newNotifService(&notifDeps{
		store: &notifStoreStub{}, prefs: &notifPrefStub{}, subs: &topicSubStub{},
	}))

	req := authed(httptest.NewRequest(http.MethodGet, "/api/v1/forums/topics/0/subscription", nil), 7, 1)
	req = withURLParam(req, "id", "0")
	w := httptest.NewRecorder()
	h.HandleGetSubscription(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestGetSubscriptionSurfacesStoreFailure(t *testing.T) {
	h := NewTopicSubscriptionHandler(newNotifService(&notifDeps{
		store: &notifStoreStub{}, prefs: &notifPrefStub{},
		subs: &topicSubStub{isSubErr: errors.New("db down")},
	}))

	req := authed(httptest.NewRequest(http.MethodGet, "/api/v1/forums/topics/5/subscription", nil), 7, 1)
	req = withURLParam(req, "id", "5")
	w := httptest.NewRecorder()
	h.HandleGetSubscription(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}
