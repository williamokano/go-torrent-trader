package postgres

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// --- chat -------------------------------------------------------------------

func TestChatMessageRepoCreateListDelete(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewChatMessageRepo(db)
	u := newUser(t, db)

	var ids []int64
	for i := 0; i < 3; i++ {
		m := &model.ChatMessage{UserID: u.ID, Message: uniq("msg")}
		if err := repo.Create(ctx, m); err != nil {
			t.Fatalf("Create: %v", err)
		}
		ids = append(ids, m.ID)
	}

	recent, err := repo.ListRecent(ctx, 10)
	if err != nil {
		t.Fatalf("ListRecent: %v", err)
	}
	if len(recent) != 3 {
		t.Fatalf("got %d messages, want 3", len(recent))
	}
	if recent[0].Username != u.Username {
		t.Errorf("username = %q, want %q (resolved via JOIN)", recent[0].Username, u.Username)
	}

	// Paging backwards from the newest returns only older messages.
	before, err := repo.ListBefore(ctx, ids[2], 10)
	if err != nil {
		t.Fatalf("ListBefore: %v", err)
	}
	if len(before) != 2 {
		t.Errorf("ListBefore(newest) returned %d, want the 2 older messages", len(before))
	}

	if err := repo.Delete(ctx, ids[0]); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	recent, err = repo.ListRecent(ctx, 10)
	if err != nil {
		t.Fatalf("ListRecent after delete: %v", err)
	}
	if len(recent) != 2 {
		t.Errorf("got %d messages after delete, want 2", len(recent))
	}

	// Purging a user removes everything they said.
	n, err := repo.DeleteByUserID(ctx, u.ID)
	if err != nil {
		t.Fatalf("DeleteByUserID: %v", err)
	}
	if n != 2 {
		t.Errorf("purged %d messages, want 2", n)
	}
}

func TestChatMuteRepoLifecycle(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewChatMuteRepo(db)
	u := newUser(t, db)
	staff := newUser(t, db)

	mute := &model.ChatMute{
		UserID:    u.ID,
		MutedBy:   &staff.ID,
		Reason:    "spam",
		ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := repo.Create(ctx, mute); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetActiveMute(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetActiveMute: %v", err)
	}
	if got.Reason != "spam" {
		t.Errorf("reason = %q, want spam", got.Reason)
	}

	list, total, err := repo.ListActive(ctx, 1, 10)
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if total != 1 || len(list) != 1 {
		t.Errorf("active mutes = %d (total %d), want 1", len(list), total)
	}

	if err := repo.Delete(ctx, u.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	// This repo signals "nothing found" with (nil, nil), not an error.
	gone, err := repo.GetActiveMute(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetActiveMute after delete: %v", err)
	}
	if gone != nil {
		t.Error("GetActiveMute still returns a mute after it was deleted")
	}
}

// The maintenance job clears mutes whose window has elapsed; a mute that is
// still running must survive.
func TestChatMuteRepoDeleteExpired(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewChatMuteRepo(db)
	expiredUser := newUser(t, db)
	activeUser := newUser(t, db)

	if err := repo.Create(ctx, &model.ChatMute{
		UserID: expiredUser.ID, Reason: "old", ExpiresAt: time.Now().Add(-time.Hour),
	}); err != nil {
		t.Fatalf("Create(expired): %v", err)
	}
	if err := repo.Create(ctx, &model.ChatMute{
		UserID: activeUser.ID, Reason: "current", ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("Create(active): %v", err)
	}

	unmuted, err := repo.DeleteExpired(ctx)
	if err != nil {
		t.Fatalf("DeleteExpired: %v", err)
	}
	if len(unmuted) != 1 || unmuted[0] != expiredUser.ID {
		t.Fatalf("unmuted = %v, want only the expired user", unmuted)
	}
	still, err := repo.GetActiveMute(ctx, activeUser.ID)
	if err != nil {
		t.Fatalf("GetActiveMute: %v", err)
	}
	if still == nil {
		t.Error("the still-running mute was removed — only expired mutes may be")
	}
}

// --- activity log -----------------------------------------------------------

func TestActivityLogRepoCreateAndFilter(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewActivityLogRepo(db)
	u := newUser(t, db)

	if err := repo.Create(ctx, &model.ActivityLog{
		EventType: "torrent.uploaded", ActorID: &u.ID, Message: "uploaded something",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	// A system event has no actor — the column is nullable for exactly this.
	if err := repo.Create(ctx, &model.ActivityLog{
		EventType: "system.maintenance", Message: "ran maintenance",
	}); err != nil {
		t.Fatalf("Create(no actor): %v", err)
	}

	all, total, err := repo.List(ctx, repository.ListActivityLogsOptions{Page: 1, PerPage: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 2 || len(all) != 2 {
		t.Errorf("got %d logs (total %d), want 2", len(all), total)
	}

	byType, _, err := repo.List(ctx, repository.ListActivityLogsOptions{
		EventType: ptr("torrent.uploaded"), Page: 1, PerPage: 10,
	})
	if err != nil {
		t.Fatalf("List(type): %v", err)
	}
	if len(byType) != 1 {
		t.Errorf("type filter returned %d, want 1", len(byType))
	}

	byActor, _, err := repo.List(ctx, repository.ListActivityLogsOptions{
		ActorID: ptr(u.ID), Page: 1, PerPage: 10,
	})
	if err != nil {
		t.Fatalf("List(actor): %v", err)
	}
	if len(byActor) != 1 {
		t.Errorf("actor filter returned %d, want 1 (the system event has no actor)", len(byActor))
	}
}

// --- notifications ----------------------------------------------------------

func TestNotificationRepoLifecycle(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewNotificationRepo(db)
	u := newUser(t, db)

	n := &model.Notification{
		UserID: u.ID, Type: model.NotifForumReply, Data: json.RawMessage(`{"post_id":1}`),
	}
	if err := repo.Create(ctx, n); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetByID(ctx, n.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Type != model.NotifForumReply {
		t.Errorf("type = %q", got.Type)
	}

	unread, err := repo.CountUnread(ctx, u.ID)
	if err != nil {
		t.Fatalf("CountUnread: %v", err)
	}
	if unread != 1 {
		t.Errorf("unread = %d, want 1", unread)
	}

	list, total, err := repo.List(ctx, u.ID, repository.ListNotificationsOptions{Page: 1, PerPage: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 1 || len(list) != 1 {
		t.Errorf("got %d notifications (total %d), want 1", len(list), total)
	}

	if err := repo.MarkRead(ctx, u.ID, n.ID); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}
	unread, err = repo.CountUnread(ctx, u.ID)
	if err != nil {
		t.Fatalf("CountUnread after read: %v", err)
	}
	if unread != 0 {
		t.Errorf("unread = %d after MarkRead, want 0", unread)
	}

	// UnreadOnly must now return nothing.
	only, _, err := repo.List(ctx, u.ID, repository.ListNotificationsOptions{
		UnreadOnly: true, Page: 1, PerPage: 10,
	})
	if err != nil {
		t.Fatalf("List(unread only): %v", err)
	}
	if len(only) != 0 {
		t.Errorf("unread-only returned %d after everything was read", len(only))
	}
}

func TestNotificationRepoMarkAllReadAndDeleteOld(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewNotificationRepo(db)
	u := newUser(t, db)

	for i := 0; i < 3; i++ {
		if err := repo.Create(ctx, &model.Notification{
			UserID: u.ID, Type: model.NotifSystem, Data: json.RawMessage(`{}`),
		}); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	if err := repo.MarkAllRead(ctx, u.ID); err != nil {
		t.Fatalf("MarkAllRead: %v", err)
	}
	unread, err := repo.CountUnread(ctx, u.ID)
	if err != nil {
		t.Fatalf("CountUnread: %v", err)
	}
	if unread != 0 {
		t.Errorf("unread = %d after MarkAllRead, want 0", unread)
	}

	// DeleteOld only removes READ notifications past the cutoff. Nothing is old
	// enough yet, so a cutoff in the past must delete nothing.
	deleted, err := repo.DeleteOld(ctx, time.Now().Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("DeleteOld: %v", err)
	}
	if deleted != 0 {
		t.Errorf("deleted %d fresh notifications, want 0", deleted)
	}

	// With a cutoff in the future, every read notification qualifies.
	deleted, err = repo.DeleteOld(ctx, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("DeleteOld(future cutoff): %v", err)
	}
	if deleted != 3 {
		t.Errorf("deleted %d, want all 3 read notifications", deleted)
	}
}

// An unread notification must survive the purge no matter how old it is —
// DeleteOld is scoped to is_read = TRUE.
func TestNotificationRepoDeleteOldSpareUnread(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewNotificationRepo(db)
	u := newUser(t, db)

	if err := repo.Create(ctx, &model.Notification{
		UserID: u.ID, Type: model.NotifSystem, Data: json.RawMessage(`{}`),
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	deleted, err := repo.DeleteOld(ctx, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("DeleteOld: %v", err)
	}
	if deleted != 0 {
		t.Errorf("deleted %d unread notifications, want 0 — the purge must spare unread", deleted)
	}
}

// --- notification preferences -----------------------------------------------

func TestNotificationPreferenceRepo(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewNotificationPreferenceRepo(db)
	u := newUser(t, db)

	// With no row stored, a type defaults to enabled.
	enabled, err := repo.IsEnabled(ctx, u.ID, model.NotifForumReply)
	if err != nil {
		t.Fatalf("IsEnabled: %v", err)
	}
	if !enabled {
		t.Error("a notification type with no stored preference should default to enabled")
	}

	if err := repo.Set(ctx, u.ID, model.NotifForumReply, false); err != nil {
		t.Fatalf("Set: %v", err)
	}
	enabled, err = repo.IsEnabled(ctx, u.ID, model.NotifForumReply)
	if err != nil {
		t.Fatalf("IsEnabled after Set: %v", err)
	}
	if enabled {
		t.Error("preference was disabled but IsEnabled still reports true")
	}

	got, err := repo.Get(ctx, u.ID, model.NotifForumReply)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Enabled {
		t.Error("Get reports enabled for a disabled preference")
	}

	// Setting it again must update the existing row, not add a second.
	if err := repo.Set(ctx, u.ID, model.NotifForumReply, true); err != nil {
		t.Fatalf("Set(true): %v", err)
	}
	all, err := repo.GetAll(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if len(all) != 1 {
		t.Errorf("got %d preference rows, want 1 — Set must upsert", len(all))
	}
}

// --- topic subscriptions ----------------------------------------------------

func TestTopicSubscriptionRepo(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewTopicSubscriptionRepo(db)
	f := newForumFixture(t, db)
	other := newUser(t, db)

	if err := repo.Subscribe(ctx, f.UserID, f.TopicID); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	// Subscribing twice must be idempotent, not a duplicate-key error.
	if err := repo.Subscribe(ctx, f.UserID, f.TopicID); err != nil {
		t.Fatalf("Subscribe (second time) must be idempotent: %v", err)
	}
	if err := repo.Subscribe(ctx, other.ID, f.TopicID); err != nil {
		t.Fatalf("Subscribe(other): %v", err)
	}

	sub, err := repo.IsSubscribed(ctx, f.UserID, f.TopicID)
	if err != nil {
		t.Fatalf("IsSubscribed: %v", err)
	}
	if !sub {
		t.Error("expected the user to be subscribed")
	}

	subs, err := repo.ListSubscribers(ctx, f.TopicID)
	if err != nil {
		t.Fatalf("ListSubscribers: %v", err)
	}
	if len(subs) != 2 {
		t.Errorf("got %d subscribers, want 2", len(subs))
	}

	if err := repo.Unsubscribe(ctx, f.UserID, f.TopicID); err != nil {
		t.Fatalf("Unsubscribe: %v", err)
	}
	sub, err = repo.IsSubscribed(ctx, f.UserID, f.TopicID)
	if err != nil {
		t.Fatalf("IsSubscribed after unsubscribe: %v", err)
	}
	if sub {
		t.Error("still subscribed after Unsubscribe")
	}
}

// --- site settings ----------------------------------------------------------

func TestSiteSettingsRepo(t *testing.T) {
	db := requireDB(t)
	ctx := context.Background()

	repo := NewSiteSettingsRepo(db)

	if err := repo.Set(ctx, "test_key", "value1"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := repo.Get(ctx, "test_key")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Value != "value1" {
		t.Errorf("value = %q, want value1", got.Value)
	}

	// Set must upsert, not fail on the existing key.
	if err := repo.Set(ctx, "test_key", "value2"); err != nil {
		t.Fatalf("Set (overwrite): %v", err)
	}
	got, err = repo.Get(ctx, "test_key")
	if err != nil {
		t.Fatalf("Get after overwrite: %v", err)
	}
	if got.Value != "value2" {
		t.Errorf("value = %q, want value2 — Set must upsert", got.Value)
	}

	all, err := repo.GetAll(ctx)
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if len(all) == 0 {
		t.Error("GetAll returned nothing")
	}
}

// --- transfer history -------------------------------------------------------

func TestTransferHistoryRepoUpsertAccumulates(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewTransferHistoryRepo(db)
	u := newUser(t, db)
	tor := newTorrent(t, db, u.ID)

	th := &model.TransferHistory{
		UserID: u.ID, TorrentID: &tor.ID,
		Uploaded: 100, Downloaded: 50, Seeder: false,
		CompletedAt: time.Now(), LastAnnounce: time.Now(),
	}
	if err := repo.Upsert(ctx, th); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	th.Uploaded = 300
	th.Seeder = true
	if err := repo.Upsert(ctx, th); err != nil {
		t.Fatalf("Upsert (second): %v", err)
	}

	list, total, err := repo.ListByUser(ctx, u.ID, 1, 10)
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if total != 1 || len(list) != 1 {
		t.Fatalf("got %d rows (total %d), want 1 — the second upsert must update", len(list), total)
	}
	if list[0].Uploaded != 300 || !list[0].Seeder {
		t.Errorf("uploaded=%d seeder=%v, want the upsert to have applied", list[0].Uploaded, list[0].Seeder)
	}
	if list[0].TorrentName != tor.Name {
		t.Errorf("torrent name = %q, want %q", list[0].TorrentName, tor.Name)
	}
}

// --- ratings ----------------------------------------------------------------

func TestRatingRepoUpsertAndStats(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewRatingRepo(db)
	a := newUser(t, db)
	b := newUser(t, db)
	tor := newTorrent(t, db, a.ID)

	if err := repo.Upsert(ctx, &model.Rating{TorrentID: tor.ID, UserID: a.ID, Rating: 5}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if err := repo.Upsert(ctx, &model.Rating{TorrentID: tor.ID, UserID: b.ID, Rating: 3}); err != nil {
		t.Fatalf("Upsert(b): %v", err)
	}

	avg, count, err := repo.GetStatsByTorrent(ctx, tor.ID)
	if err != nil {
		t.Fatalf("GetStatsByTorrent: %v", err)
	}
	if count != 2 || avg != 4 {
		t.Errorf("avg=%v count=%d, want 4/2", avg, count)
	}

	// Re-rating must replace that user's score, not add another.
	if err := repo.Upsert(ctx, &model.Rating{TorrentID: tor.ID, UserID: a.ID, Rating: 1}); err != nil {
		t.Fatalf("Upsert (re-rate): %v", err)
	}
	avg, count, err = repo.GetStatsByTorrent(ctx, tor.ID)
	if err != nil {
		t.Fatalf("GetStatsByTorrent after re-rate: %v", err)
	}
	if count != 2 || avg != 2 {
		t.Errorf("avg=%v count=%d, want 2/2 — re-rating must overwrite, not append", avg, count)
	}

	got, err := repo.GetByTorrentAndUser(ctx, tor.ID, a.ID)
	if err != nil {
		t.Fatalf("GetByTorrentAndUser: %v", err)
	}
	if got.Rating != 1 {
		t.Errorf("rating = %d, want 1", got.Rating)
	}
}

// --- reseed requests --------------------------------------------------------

func TestReseedRequestRepo(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewReseedRequestRepo(db)
	u := newUser(t, db)
	other := newUser(t, db)
	tor := newTorrent(t, db, u.ID)

	if err := repo.Create(ctx, &model.ReseedRequest{TorrentID: tor.ID, RequesterID: u.ID}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	exists, err := repo.ExistsByTorrentAndUser(ctx, tor.ID, u.ID)
	if err != nil {
		t.Fatalf("ExistsByTorrentAndUser: %v", err)
	}
	if !exists {
		t.Error("expected the request to be found")
	}

	exists, err = repo.ExistsByTorrentAndUser(ctx, tor.ID, other.ID)
	if err != nil {
		t.Fatalf("ExistsByTorrentAndUser(other): %v", err)
	}
	if exists {
		t.Error("another user's reseed request was reported")
	}

	n, err := repo.CountByTorrent(ctx, tor.ID)
	if err != nil {
		t.Fatalf("CountByTorrent: %v", err)
	}
	if n != 1 {
		t.Errorf("count = %d, want 1", n)
	}
}

// --- dashboard --------------------------------------------------------------

func TestDashboardRepoGetStats(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	u := newUser(t, db)
	newTorrent(t, db, u.ID)

	stats, err := NewDashboardRepo(db).GetStats(ctx)
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	if stats.UsersTotal < 1 || stats.TorrentsTotal < 1 {
		t.Errorf("stats = %+v, want at least the user and torrent just created", stats)
	}
}

// --- forum categories -------------------------------------------------------

func TestForumCategoryRepoCRUD(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewForumCategoryRepo(db)

	cat := &model.ForumCategory{Name: "Support", SortOrder: 2}
	if err := repo.Create(ctx, cat); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetByID(ctx, cat.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Name != "Support" {
		t.Errorf("name = %q, want Support", got.Name)
	}

	cat.Name = "Help"
	if err := repo.Update(ctx, cat); err != nil {
		t.Fatalf("Update: %v", err)
	}

	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) == 0 {
		t.Error("List returned nothing")
	}

	// The count guards deletion of a category that still holds forums.
	n, err := repo.CountForumsByCategory(ctx, cat.ID)
	if err != nil {
		t.Fatalf("CountForumsByCategory: %v", err)
	}
	if n != 0 {
		t.Errorf("count = %d for an empty category, want 0", n)
	}

	if err := repo.Delete(ctx, cat.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := repo.GetByID(ctx, cat.ID); err == nil {
		t.Error("GetByID succeeded after Delete")
	}
}

// --- email confirmations ----------------------------------------------------

func TestEmailConfirmationRepo(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewEmailConfirmationRepo(db)
	u := newUser(t, db)

	hash := []byte("token-hash-1")
	if err := repo.Create(ctx, u.ID, hash, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("Create: %v", err)
	}

	latest, err := repo.GetLatestByUserID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetLatestByUserID: %v", err)
	}
	if latest.UserID != u.ID {
		t.Errorf("user = %d, want %d", latest.UserID, u.ID)
	}

	claimed, err := repo.ClaimByTokenHash(ctx, hash)
	if err != nil {
		t.Fatalf("ClaimByTokenHash: %v", err)
	}
	if claimed.UserID != u.ID {
		t.Errorf("claimed user = %d, want %d", claimed.UserID, u.ID)
	}

	// A token may only be claimed once. A spent token yields (nil, nil).
	again, err := repo.ClaimByTokenHash(ctx, hash)
	if err != nil {
		t.Fatalf("ClaimByTokenHash (second): %v", err)
	}
	if again != nil {
		t.Error("the same confirmation token was claimed twice")
	}

	if err := repo.DeleteByUserID(ctx, u.ID); err != nil {
		t.Fatalf("DeleteByUserID: %v", err)
	}
	left, err := repo.GetLatestByUserID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetLatestByUserID after delete: %v", err)
	}
	if left != nil {
		t.Error("confirmation still present after DeleteByUserID")
	}
}

// --- password resets --------------------------------------------------------

func TestPasswordResetRepo(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)

	repo := NewPasswordResetRepo(db)
	u := newUser(t, db)

	pr := &service.PasswordReset{
		UserID:    u.ID,
		TokenHash: "hash-1",
		ExpiresAt: time.Now().Add(time.Hour),
		// Create inserts CreatedAt verbatim; leaving it zero would place the row
		// in year 1 and hide it from the rate-limit window.
		CreatedAt: time.Now(),
	}
	if err := repo.Create(pr); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// The rate limiter counts recent requests per user.
	n, err := repo.CountRecentByUserID(u.ID, time.Hour)
	if err != nil {
		t.Fatalf("CountRecentByUserID: %v", err)
	}
	if n != 1 {
		t.Errorf("recent = %d, want 1", n)
	}

	claimed, err := repo.ClaimByTokenHash("hash-1")
	if err != nil {
		t.Fatalf("ClaimByTokenHash: %v", err)
	}
	if claimed.UserID != u.ID {
		t.Errorf("claimed user = %d, want %d", claimed.UserID, u.ID)
	}

	// A reset token is single-use; a spent token yields (nil, nil).
	againPR, err := repo.ClaimByTokenHash("hash-1")
	if err != nil {
		t.Fatalf("ClaimByTokenHash (second): %v", err)
	}
	if againPR != nil {
		t.Error("the same reset token was claimed twice")
	}
}
