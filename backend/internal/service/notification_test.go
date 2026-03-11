package service_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// --- mock notification repo ---

type mockNotificationRepo struct {
	mu     sync.Mutex
	notifs []*model.Notification
	nextID int64
}

func newMockNotificationRepo() *mockNotificationRepo {
	return &mockNotificationRepo{nextID: 1}
}

func (m *mockNotificationRepo) Create(_ context.Context, notif *model.Notification) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	notif.ID = m.nextID
	m.nextID++
	notif.CreatedAt = time.Now()
	cp := *notif
	m.notifs = append(m.notifs, &cp)
	return nil
}

func (m *mockNotificationRepo) GetByID(_ context.Context, id int64) (*model.Notification, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, n := range m.notifs {
		if n.ID == id {
			cp := *n
			return &cp, nil
		}
	}
	return nil, sql.ErrNoRows
}

func (m *mockNotificationRepo) List(_ context.Context, userID int64, opts repository.ListNotificationsOptions) ([]model.Notification, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var filtered []model.Notification
	for _, n := range m.notifs {
		if n.UserID != userID {
			continue
		}
		if opts.UnreadOnly && n.Read {
			continue
		}
		filtered = append(filtered, *n)
	}
	total := int64(len(filtered))
	page := opts.Page
	perPage := opts.PerPage
	if page <= 0 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 25
	}
	start := (page - 1) * perPage
	if start >= len(filtered) {
		return nil, total, nil
	}
	end := start + perPage
	if end > len(filtered) {
		end = len(filtered)
	}
	return filtered[start:end], total, nil
}

func (m *mockNotificationRepo) MarkRead(_ context.Context, userID, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, n := range m.notifs {
		if n.ID == id && n.UserID == userID {
			n.Read = true
			return nil
		}
	}
	return sql.ErrNoRows
}

func (m *mockNotificationRepo) MarkAllRead(_ context.Context, userID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, n := range m.notifs {
		if n.UserID == userID {
			n.Read = true
		}
	}
	return nil
}

func (m *mockNotificationRepo) CountUnread(_ context.Context, userID int64) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, n := range m.notifs {
		if n.UserID == userID && !n.Read {
			count++
		}
	}
	return count, nil
}

func (m *mockNotificationRepo) DeleteOld(_ context.Context, before time.Time) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var kept []*model.Notification
	deleted := int64(0)
	for _, n := range m.notifs {
		if n.Read && n.CreatedAt.Before(before) {
			deleted++
		} else {
			kept = append(kept, n)
		}
	}
	m.notifs = kept
	return deleted, nil
}

// --- mock notification preference repo ---

type mockNotifPrefRepo struct {
	mu    sync.Mutex
	prefs map[string]bool // key: "userID:type"
}

func newMockNotifPrefRepo() *mockNotifPrefRepo {
	return &mockNotifPrefRepo{prefs: make(map[string]bool)}
}

func (m *mockNotifPrefRepo) Get(_ context.Context, userID int64, notifType string) (*model.NotificationPreference, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := prefKey(userID, notifType)
	if enabled, ok := m.prefs[k]; ok {
		return &model.NotificationPreference{UserID: userID, NotificationType: notifType, Enabled: enabled}, nil
	}
	return nil, sql.ErrNoRows
}

func (m *mockNotifPrefRepo) GetAll(_ context.Context, userID int64) ([]model.NotificationPreference, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []model.NotificationPreference
	for k, enabled := range m.prefs {
		uid, ntype := parsePrefKey(k)
		if uid == userID {
			result = append(result, model.NotificationPreference{UserID: uid, NotificationType: ntype, Enabled: enabled})
		}
	}
	return result, nil
}

func (m *mockNotifPrefRepo) Set(_ context.Context, userID int64, notifType string, enabled bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.prefs[prefKey(userID, notifType)] = enabled
	return nil
}

func (m *mockNotifPrefRepo) IsEnabled(_ context.Context, userID int64, notifType string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := prefKey(userID, notifType)
	if enabled, ok := m.prefs[k]; ok {
		return enabled, nil
	}
	return true, nil // default enabled
}

func prefKey(userID int64, notifType string) string {
	return fmt.Sprintf("%d:%s", userID, notifType)
}

func parsePrefKey(k string) (int64, string) {
	for i := 0; i < len(k); i++ {
		if k[i] == ':' {
			uid, _ := strconv.ParseInt(k[:i], 10, 64)
			return uid, k[i+1:]
		}
	}
	return 0, k
}

// --- mock topic subscription repo ---

type mockTopicSubRepo struct {
	mu   sync.Mutex
	subs map[string]bool // "userID:topicID"
}

func newMockTopicSubRepo() *mockTopicSubRepo {
	return &mockTopicSubRepo{subs: make(map[string]bool)}
}

func subKey(userID, topicID int64) string {
	return fmt.Sprintf("%d:%d", userID, topicID)
}

func (m *mockTopicSubRepo) Subscribe(_ context.Context, userID, topicID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.subs[subKey(userID, topicID)] = true
	return nil
}

func (m *mockTopicSubRepo) Unsubscribe(_ context.Context, userID, topicID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.subs, subKey(userID, topicID))
	return nil
}

func (m *mockTopicSubRepo) IsSubscribed(_ context.Context, userID, topicID int64) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.subs[subKey(userID, topicID)], nil
}

func (m *mockTopicSubRepo) ListSubscribers(_ context.Context, topicID int64) ([]int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []int64
	for k, v := range m.subs {
		if !v {
			continue
		}
		parts := splitKey(k)
		if len(parts) == 2 {
			uid, _ := strconv.ParseInt(parts[0], 10, 64)
			tid, _ := strconv.ParseInt(parts[1], 10, 64)
			if tid == topicID {
				result = append(result, uid)
			}
		}
	}
	return result, nil
}

func splitKey(k string) []string {
	for i := 0; i < len(k); i++ {
		if k[i] == ':' {
			return []string{k[:i], k[i+1:]}
		}
	}
	return []string{k}
}

// --- tests ---

func newTestNotifService() (*service.NotificationService, *mockNotificationRepo, *mockNotifPrefRepo, *mockTopicSubRepo, *[][]byte) {
	notifRepo := newMockNotificationRepo()
	prefRepo := newMockNotifPrefRepo()
	subRepo := newMockTopicSubRepo()
	var wsPayloads [][]byte
	sendToUser := func(_ int64, payload []byte) {
		wsPayloads = append(wsPayloads, payload)
	}
	svc := service.NewNotificationService(notifRepo, prefRepo, subRepo, nil, nil, sendToUser)
	return svc, notifRepo, prefRepo, subRepo, &wsPayloads
}

func TestNotificationService_Create(t *testing.T) {
	ctx := context.Background()
	svc, repo, _, _, wsPayloads := newTestNotifService()

	data := json.RawMessage(`{"topic_id": 1}`)
	notif, err := svc.Create(ctx, 2, 1, model.NotifForumReply, data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if notif == nil {
		t.Fatal("expected notification, got nil")
	}
	if notif.ID != 1 {
		t.Errorf("expected ID 1, got %d", notif.ID)
	}
	if notif.Type != model.NotifForumReply {
		t.Errorf("expected type %s, got %s", model.NotifForumReply, notif.Type)
	}

	// Should have pushed via WebSocket
	if len(*wsPayloads) != 1 {
		t.Errorf("expected 1 WS payload, got %d", len(*wsPayloads))
	}

	// Verify repo stored it
	_ = repo
	stored, _, err := svc.List(ctx, 2, 1, 25, false)
	if err != nil {
		t.Fatalf("list error: %v", err)
	}
	if stored == nil {
		t.Fatal("expected notifications list, got nil")
	}
}

func TestNotificationService_Create_SkipsSelfNotify(t *testing.T) {
	ctx := context.Background()
	svc, _, _, _, wsPayloads := newTestNotifService()

	notif, err := svc.Create(ctx, 1, 1, model.NotifForumReply, json.RawMessage("{}"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if notif != nil {
		t.Error("expected nil for self-notify")
	}
	if len(*wsPayloads) != 0 {
		t.Error("expected no WS payload for self-notify")
	}
}

func TestNotificationService_Create_RespectsPreferences(t *testing.T) {
	ctx := context.Background()
	svc, _, prefRepo, _, wsPayloads := newTestNotifService()

	// Disable forum_reply for user 2
	if err := prefRepo.Set(ctx, 2, model.NotifForumReply, false); err != nil {
		t.Fatalf("set pref error: %v", err)
	}

	notif, err := svc.Create(ctx, 2, 1, model.NotifForumReply, json.RawMessage("{}"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if notif != nil {
		t.Error("expected nil when disabled")
	}
	if len(*wsPayloads) != 0 {
		t.Error("expected no WS payload when disabled")
	}
}

func TestNotificationService_MarkRead(t *testing.T) {
	ctx := context.Background()
	svc, _, _, _, _ := newTestNotifService()

	// Create a notification
	notif, err := svc.Create(ctx, 2, 1, model.NotifSystem, json.RawMessage("{}"))
	if err != nil {
		t.Fatalf("create error: %v", err)
	}

	// Mark as read
	if err := svc.MarkRead(ctx, 2, notif.ID); err != nil {
		t.Fatalf("mark read error: %v", err)
	}

	// Unread count should be 0
	count, err := svc.UnreadCount(ctx, 2)
	if err != nil {
		t.Fatalf("count error: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 unread, got %d", count)
	}
}

func TestNotificationService_MarkAllRead(t *testing.T) {
	ctx := context.Background()
	svc, _, _, _, _ := newTestNotifService()

	// Create multiple notifications
	for i := 0; i < 3; i++ {
		if _, err := svc.Create(ctx, 2, 1, model.NotifSystem, json.RawMessage("{}")); err != nil {
			t.Fatalf("create error: %v", err)
		}
	}

	count, _ := svc.UnreadCount(ctx, 2)
	if count != 3 {
		t.Fatalf("expected 3 unread, got %d", count)
	}

	if err := svc.MarkAllRead(ctx, 2); err != nil {
		t.Fatalf("mark all read error: %v", err)
	}

	count, _ = svc.UnreadCount(ctx, 2)
	if count != 0 {
		t.Errorf("expected 0 unread after mark all, got %d", count)
	}
}

func TestNotificationService_ListUnreadOnly(t *testing.T) {
	ctx := context.Background()
	svc, _, _, _, _ := newTestNotifService()

	// Create 3 notifications, mark 1 as read
	for i := 0; i < 3; i++ {
		if _, err := svc.Create(ctx, 2, 1, model.NotifSystem, json.RawMessage("{}")); err != nil {
			t.Fatalf("create error: %v", err)
		}
	}
	if err := svc.MarkRead(ctx, 2, 1); err != nil {
		t.Fatalf("mark read error: %v", err)
	}

	// List all
	all, total, err := svc.List(ctx, 2, 1, 25, false)
	if err != nil {
		t.Fatalf("list error: %v", err)
	}
	if total != 3 {
		t.Errorf("expected total 3, got %d", total)
	}
	if len(all) != 3 {
		t.Errorf("expected 3 items, got %d", len(all))
	}

	// List unread only
	unread, unreadTotal, err := svc.List(ctx, 2, 1, 25, true)
	if err != nil {
		t.Fatalf("list unread error: %v", err)
	}
	if unreadTotal != 2 {
		t.Errorf("expected unread total 2, got %d", unreadTotal)
	}
	if len(unread) != 2 {
		t.Errorf("expected 2 unread items, got %d", len(unread))
	}
}

func TestNotificationService_Preferences(t *testing.T) {
	ctx := context.Background()
	svc, _, _, _, _ := newTestNotifService()

	// Get defaults (all enabled)
	prefs, err := svc.GetPreferences(ctx, 1)
	if err != nil {
		t.Fatalf("get prefs error: %v", err)
	}
	if len(prefs) != len(model.AllNotificationTypes) {
		t.Errorf("expected %d preferences, got %d", len(model.AllNotificationTypes), len(prefs))
	}
	for _, p := range prefs {
		if !p.Enabled {
			t.Errorf("expected %s to be enabled by default", p.NotificationType)
		}
	}

	// Disable one
	if err := svc.SetPreference(ctx, 1, model.NotifForumMention, false); err != nil {
		t.Fatalf("set pref error: %v", err)
	}

	prefs, _ = svc.GetPreferences(ctx, 1)
	for _, p := range prefs {
		if p.NotificationType == model.NotifForumMention && p.Enabled {
			t.Error("expected forum_mention to be disabled")
		}
	}

	// Invalid type
	if err := svc.SetPreference(ctx, 1, "invalid_type", true); err == nil {
		t.Error("expected error for invalid notification type")
	}
}

func TestNotificationService_Subscribe(t *testing.T) {
	ctx := context.Background()
	svc, _, _, _, _ := newTestNotifService()

	// Subscribe
	if err := svc.Subscribe(ctx, 1, 10); err != nil {
		t.Fatalf("subscribe error: %v", err)
	}

	// Check
	sub, err := svc.IsSubscribed(ctx, 1, 10)
	if err != nil {
		t.Fatalf("is subscribed error: %v", err)
	}
	if !sub {
		t.Error("expected subscribed")
	}

	// Unsubscribe
	if err := svc.Unsubscribe(ctx, 1, 10); err != nil {
		t.Fatalf("unsubscribe error: %v", err)
	}

	sub, _ = svc.IsSubscribed(ctx, 1, 10)
	if sub {
		t.Error("expected not subscribed after unsubscribe")
	}
}
