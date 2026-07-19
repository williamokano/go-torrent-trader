package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/williamokano/go-torrent-trader/backend/internal/middleware"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// --- mocks ------------------------------------------------------------------

type notifStoreStub struct {
	list             []model.Notification
	total            int64
	unread           int
	listErr          error
	unreadErr        error
	markErr          error
	markAllErr       error
	markedRead       []int64
	markedAllTo      int64
	markGroupResult  int64
	markGroupErr     error
	markGroupUserID  int64
	markGroupTopicID int64
	markGroupCalled  bool
}

func (m *notifStoreStub) Create(_ context.Context, _ *model.Notification) error { return nil }
func (m *notifStoreStub) GetByID(_ context.Context, _ int64) (*model.Notification, error) {
	return nil, nil
}
func (m *notifStoreStub) List(_ context.Context, _ int64, _ repository.ListNotificationsOptions) ([]model.Notification, int64, error) {
	return m.list, m.total, m.listErr
}
func (m *notifStoreStub) MarkRead(_ context.Context, _, id int64) error {
	m.markedRead = append(m.markedRead, id)
	return m.markErr
}
func (m *notifStoreStub) MarkAllRead(_ context.Context, userID int64) error {
	m.markedAllTo = userID
	return m.markAllErr
}
func (m *notifStoreStub) MarkTopicReplyGroupRead(_ context.Context, userID, topicID int64) (int64, error) {
	m.markGroupCalled = true
	m.markGroupUserID = userID
	m.markGroupTopicID = topicID
	return m.markGroupResult, m.markGroupErr
}
func (m *notifStoreStub) CountUnread(_ context.Context, _ int64) (int, error) {
	return m.unread, m.unreadErr
}
func (m *notifStoreStub) DeleteOld(_ context.Context, _ time.Time) (int64, error) { return 0, nil }
func (m *notifStoreStub) CountUnreadSince(_ context.Context, _ int64, _ time.Time) (int, error) {
	return 0, nil
}
func (m *notifStoreStub) ListUnreadSince(_ context.Context, _ int64, _ time.Time, _ int) ([]model.Notification, error) {
	return nil, nil
}

type notifDigestPrefStub struct {
	frequency string
	getErr    error
	setErr    error
	set       map[int64]string // userID -> frequency
}

func (m *notifDigestPrefStub) GetFrequency(_ context.Context, _ int64) (string, error) {
	if m.frequency == "" {
		return model.DigestOff, m.getErr
	}
	return m.frequency, m.getErr
}
func (m *notifDigestPrefStub) SetFrequency(_ context.Context, userID int64, frequency string) error {
	if m.set == nil {
		m.set = map[int64]string{}
	}
	m.set[userID] = frequency
	return m.setErr
}
func (m *notifDigestPrefStub) ListDue(_ context.Context, _ string, _ time.Time) ([]model.DigestRecipient, error) {
	return nil, nil
}
func (m *notifDigestPrefStub) MarkSent(_ context.Context, _ int64, _ time.Time) error { return nil }

type notifPrefStub struct {
	all    []model.NotificationPreference
	getErr error
	setErr error
	set    map[string]bool
}

func (m *notifPrefStub) Get(_ context.Context, _ int64, _ string) (*model.NotificationPreference, error) {
	return nil, nil
}
func (m *notifPrefStub) GetAll(_ context.Context, _ int64) ([]model.NotificationPreference, error) {
	return m.all, m.getErr
}
func (m *notifPrefStub) Set(_ context.Context, _ int64, notifType string, enabled bool) error {
	if m.set == nil {
		m.set = map[string]bool{}
	}
	m.set[notifType] = enabled
	return m.setErr
}
func (m *notifPrefStub) IsEnabled(_ context.Context, _ int64, _ string) (bool, error) {
	return true, nil
}

type topicSubStub struct {
	subscribed     bool
	subscribeErr   error
	unsubscribeErr error
	isSubErr       error
	unsubscribed   []int64
}

func (m *topicSubStub) Subscribe(_ context.Context, _, _ int64) error { return m.subscribeErr }
func (m *topicSubStub) Unsubscribe(_ context.Context, _, topicID int64) error {
	m.unsubscribed = append(m.unsubscribed, topicID)
	return m.unsubscribeErr
}
func (m *topicSubStub) IsSubscribed(_ context.Context, _, _ int64) (bool, error) {
	return m.subscribed, m.isSubErr
}
func (m *topicSubStub) ListSubscribers(_ context.Context, _ int64) ([]int64, error) {
	return nil, nil
}

// topicLookupStub / forumLookupStub back SubscribeWithAccessCheck's access
// check. Only the methods that path touches carry behaviour.
type topicLookupStub struct {
	topic *model.ForumTopic
	err   error
}

func (m *topicLookupStub) GetByID(_ context.Context, _ int64) (*model.ForumTopic, error) {
	return m.topic, m.err
}
func (m *topicLookupStub) ListByForum(_ context.Context, _ int64, _, _ int) ([]model.ForumTopic, int64, error) {
	return nil, 0, nil
}
func (m *topicLookupStub) Create(_ context.Context, _ *model.ForumTopic) error        { return nil }
func (m *topicLookupStub) IncrementViewCount(_ context.Context, _ int64) error        { return nil }
func (m *topicLookupStub) IncrementPostCount(_ context.Context, _ int64, _ int) error { return nil }
func (m *topicLookupStub) UpdateLastPost(_ context.Context, _ int64, _ int64, _ time.Time) error {
	return nil
}
func (m *topicLookupStub) RecalculateLastPost(_ context.Context, _ int64) error    { return nil }
func (m *topicLookupStub) SetLocked(_ context.Context, _ int64, _ bool) error      { return nil }
func (m *topicLookupStub) SetPinned(_ context.Context, _ int64, _ bool) error      { return nil }
func (m *topicLookupStub) UpdateTitle(_ context.Context, _ int64, _ string) error  { return nil }
func (m *topicLookupStub) UpdateForumID(_ context.Context, _ int64, _ int64) error { return nil }
func (m *topicLookupStub) Delete(_ context.Context, _ int64) error                 { return nil }

type forumLookupStub struct {
	forum *model.Forum
	err   error
}

func (m *forumLookupStub) GetByID(_ context.Context, _ int64) (*model.Forum, error) {
	return m.forum, m.err
}
func (m *forumLookupStub) List(_ context.Context) ([]model.Forum, error) { return nil, nil }
func (m *forumLookupStub) ListByCategory(_ context.Context, _ int64) ([]model.Forum, error) {
	return nil, nil
}
func (m *forumLookupStub) Create(_ context.Context, _ *model.Forum) error              { return nil }
func (m *forumLookupStub) Update(_ context.Context, _ *model.Forum) error              { return nil }
func (m *forumLookupStub) Delete(_ context.Context, _ int64) error                     { return nil }
func (m *forumLookupStub) IncrementTopicCount(_ context.Context, _ int64, _ int) error { return nil }
func (m *forumLookupStub) IncrementPostCount(_ context.Context, _ int64, _ int) error  { return nil }
func (m *forumLookupStub) UpdateLastPost(_ context.Context, _ int64, _ int64) error    { return nil }
func (m *forumLookupStub) RecalculateLastPost(_ context.Context, _ int64) error        { return nil }
func (m *forumLookupStub) RecalculateCounts(_ context.Context, _ int64) error          { return nil }
func (m *forumLookupStub) CountTopicsByForum(_ context.Context, _ int64) (int64, error) {
	return 0, nil
}

// --- harness ----------------------------------------------------------------

type notifDeps struct {
	store  *notifStoreStub
	prefs  *notifPrefStub
	digest *notifDigestPrefStub
	subs   *topicSubStub
	topics *topicLookupStub
	forums *forumLookupStub
}

func newNotifService(d *notifDeps) *service.NotificationService {
	// Passing the typed nils through as interfaces would make the service's
	// `!= nil` checks pass and then panic, so hand over untyped nil when a
	// lookup is not part of the scenario.
	var topics repository.ForumTopicRepository
	if d.topics != nil {
		topics = d.topics
	}
	var forums repository.ForumRepository
	if d.forums != nil {
		forums = d.forums
	}
	var digest repository.NotificationDigestPreferenceRepository
	if d.digest != nil {
		digest = d.digest
	}
	return service.NewNotificationService(d.store, d.prefs, digest, d.subs, topics, forums, nil)
}

func authed(r *http.Request, userID int64, level int) *http.Request {
	ctx := context.WithValue(r.Context(), middleware.UserIDKey, userID)
	ctx = context.WithValue(ctx, middleware.PermissionsKey, model.Permissions{Level: level})
	return r.WithContext(ctx)
}

func withURLParam(r *http.Request, key, val string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, val)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

func decodeBody(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var body map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	return body
}

// --- notification handler ---------------------------------------------------

func TestListNotificationsRequiresAuth(t *testing.T) {
	h := NewNotificationHandler(newNotifService(&notifDeps{
		store: &notifStoreStub{}, prefs: &notifPrefStub{}, subs: &topicSubStub{},
	}))

	// No user in context.
	req := httptest.NewRequest("GET", "/api/v1/notifications", nil)
	w := httptest.NewRecorder()
	h.HandleListNotifications(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 for an unauthenticated request", w.Code)
	}
}

func TestListNotificationsReturnsItems(t *testing.T) {
	store := &notifStoreStub{
		list:  []model.Notification{{ID: 1, UserID: 7, Type: model.NotifForumReply}},
		total: 1,
	}
	h := NewNotificationHandler(newNotifService(&notifDeps{
		store: store, prefs: &notifPrefStub{}, subs: &topicSubStub{},
	}))

	req := authed(httptest.NewRequest("GET", "/api/v1/notifications", nil), 7, 1)
	w := httptest.NewRecorder()
	h.HandleListNotifications(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := decodeBody(t, w)
	items, ok := body["notifications"].([]interface{})
	if !ok || len(items) != 1 {
		t.Fatalf("notifications = %v, want 1 item", body["notifications"])
	}
	if body["total"].(float64) != 1 {
		t.Errorf("total = %v, want 1", body["total"])
	}
}

func TestListNotificationsSurfacesStoreFailure(t *testing.T) {
	h := NewNotificationHandler(newNotifService(&notifDeps{
		store: &notifStoreStub{listErr: errors.New("db down")},
		prefs: &notifPrefStub{}, subs: &topicSubStub{},
	}))

	req := authed(httptest.NewRequest("GET", "/api/v1/notifications", nil), 7, 1)
	w := httptest.NewRecorder()
	h.HandleListNotifications(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 when the store fails — it must not report success", w.Code)
	}
}

func TestUnreadCountReturnsCount(t *testing.T) {
	h := NewNotificationHandler(newNotifService(&notifDeps{
		store: &notifStoreStub{unread: 4}, prefs: &notifPrefStub{}, subs: &topicSubStub{},
	}))

	req := authed(httptest.NewRequest("GET", "/api/v1/notifications/unread", nil), 7, 1)
	w := httptest.NewRecorder()
	h.HandleUnreadCount(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if got := decodeBody(t, w)["count"].(float64); got != 4 {
		t.Errorf("count = %v, want 4", got)
	}
}

func TestMarkReadRejectsInvalidID(t *testing.T) {
	h := NewNotificationHandler(newNotifService(&notifDeps{
		store: &notifStoreStub{}, prefs: &notifPrefStub{}, subs: &topicSubStub{},
	}))

	req := authed(httptest.NewRequest("POST", "/api/v1/notifications/abc/read", nil), 7, 1)
	req = withURLParam(req, "id", "abc")
	w := httptest.NewRecorder()
	h.HandleMarkRead(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for a non-numeric id", w.Code)
	}
}

func TestMarkReadMarksTheRequestedNotification(t *testing.T) {
	store := &notifStoreStub{}
	h := NewNotificationHandler(newNotifService(&notifDeps{
		store: store, prefs: &notifPrefStub{}, subs: &topicSubStub{},
	}))

	req := authed(httptest.NewRequest("POST", "/api/v1/notifications/42/read", nil), 7, 1)
	req = withURLParam(req, "id", "42")
	w := httptest.NewRecorder()
	h.HandleMarkRead(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", w.Code)
	}
	if len(store.markedRead) != 1 || store.markedRead[0] != 42 {
		t.Errorf("marked read = %v, want [42]", store.markedRead)
	}
}

func TestMarkAllReadTargetsTheAuthenticatedUser(t *testing.T) {
	store := &notifStoreStub{}
	h := NewNotificationHandler(newNotifService(&notifDeps{
		store: store, prefs: &notifPrefStub{}, subs: &topicSubStub{},
	}))

	req := authed(httptest.NewRequest("POST", "/api/v1/notifications/read-all", nil), 7, 1)
	w := httptest.NewRecorder()
	h.HandleMarkAllRead(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", w.Code)
	}
	if store.markedAllTo != 7 {
		t.Errorf("marked all read for user %d, want the authenticated user 7", store.markedAllTo)
	}
}

func TestUpdatePreferencesRejectsBadJSON(t *testing.T) {
	h := NewNotificationHandler(newNotifService(&notifDeps{
		store: &notifStoreStub{}, prefs: &notifPrefStub{}, subs: &topicSubStub{},
	}))

	req := authed(httptest.NewRequest("PUT", "/api/v1/notifications/preferences", strings.NewReader("not json")), 7, 1)
	w := httptest.NewRecorder()
	h.HandleUpdatePreferences(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for a malformed body", w.Code)
	}
}

// --- topic subscription handler ---------------------------------------------

func TestSubscribeRejectsInvalidTopicID(t *testing.T) {
	h := NewTopicSubscriptionHandler(newNotifService(&notifDeps{
		store: &notifStoreStub{}, prefs: &notifPrefStub{}, subs: &topicSubStub{},
	}))

	req := authed(httptest.NewRequest("POST", "/api/v1/forums/topics/0/subscribe", nil), 7, 1)
	req = withURLParam(req, "id", "0")
	w := httptest.NewRecorder()
	h.HandleSubscribe(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 — a topic id of 0 is not valid", w.Code)
	}
}

func TestSubscribeReturns404WhenTopicMissing(t *testing.T) {
	h := NewTopicSubscriptionHandler(newNotifService(&notifDeps{
		store:  &notifStoreStub{},
		prefs:  &notifPrefStub{},
		subs:   &topicSubStub{},
		topics: &topicLookupStub{err: errors.New("no rows")},
	}))

	req := authed(httptest.NewRequest("POST", "/api/v1/forums/topics/5/subscribe", nil), 7, 1)
	req = withURLParam(req, "id", "5")
	w := httptest.NewRecorder()
	h.HandleSubscribe(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 for a missing topic", w.Code)
	}
}

// A user must not be able to subscribe to a topic in a forum above their group
// level — otherwise subscription becomes a side channel for restricted content.
func TestSubscribeForbiddenWhenForumOutranksUser(t *testing.T) {
	h := NewTopicSubscriptionHandler(newNotifService(&notifDeps{
		store:  &notifStoreStub{},
		prefs:  &notifPrefStub{},
		subs:   &topicSubStub{},
		topics: &topicLookupStub{topic: &model.ForumTopic{ID: 5, ForumID: 3}},
		forums: &forumLookupStub{forum: &model.Forum{ID: 3, MinGroupLevel: 10}},
	}))

	req := authed(httptest.NewRequest("POST", "/api/v1/forums/topics/5/subscribe", nil), 7, 1) // level 1 < 10
	req = withURLParam(req, "id", "5")
	w := httptest.NewRecorder()
	h.HandleSubscribe(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 — the forum requires a higher group level", w.Code)
	}
}

func TestSubscribeSucceedsWhenUserMeetsForumLevel(t *testing.T) {
	h := NewTopicSubscriptionHandler(newNotifService(&notifDeps{
		store:  &notifStoreStub{},
		prefs:  &notifPrefStub{},
		subs:   &topicSubStub{},
		topics: &topicLookupStub{topic: &model.ForumTopic{ID: 5, ForumID: 3}},
		forums: &forumLookupStub{forum: &model.Forum{ID: 3, MinGroupLevel: 1}},
	}))

	req := authed(httptest.NewRequest("POST", "/api/v1/forums/topics/5/subscribe", nil), 7, 5)
	req = withURLParam(req, "id", "5")
	w := httptest.NewRecorder()
	h.HandleSubscribe(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if decodeBody(t, w)["subscribed"] != true {
		t.Error("subscribed = false, want true")
	}
}

func TestUnsubscribeRemovesTheSubscription(t *testing.T) {
	subs := &topicSubStub{}
	h := NewTopicSubscriptionHandler(newNotifService(&notifDeps{
		store: &notifStoreStub{}, prefs: &notifPrefStub{}, subs: subs,
	}))

	req := authed(httptest.NewRequest("DELETE", "/api/v1/forums/topics/5/subscribe", nil), 7, 1)
	req = withURLParam(req, "id", "5")
	w := httptest.NewRecorder()
	h.HandleUnsubscribe(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if len(subs.unsubscribed) != 1 || subs.unsubscribed[0] != 5 {
		t.Errorf("unsubscribed from %v, want [5]", subs.unsubscribed)
	}
	if decodeBody(t, w)["subscribed"] != false {
		t.Error("subscribed = true, want false after unsubscribing")
	}
}

func TestGetSubscriptionReportsCurrentState(t *testing.T) {
	h := NewTopicSubscriptionHandler(newNotifService(&notifDeps{
		store: &notifStoreStub{}, prefs: &notifPrefStub{},
		subs: &topicSubStub{subscribed: true},
	}))

	req := authed(httptest.NewRequest("GET", "/api/v1/forums/topics/5/subscription", nil), 7, 1)
	req = withURLParam(req, "id", "5")
	w := httptest.NewRecorder()
	h.HandleGetSubscription(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if decodeBody(t, w)["subscribed"] != true {
		t.Error("subscribed = false, want true")
	}
}

func TestGetSubscriptionRequiresAuth(t *testing.T) {
	h := NewTopicSubscriptionHandler(newNotifService(&notifDeps{
		store: &notifStoreStub{}, prefs: &notifPrefStub{}, subs: &topicSubStub{},
	}))

	req := withURLParam(httptest.NewRequest("GET", "/api/v1/forums/topics/5/subscription", nil), "id", "5")
	w := httptest.NewRecorder()
	h.HandleGetSubscription(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}
