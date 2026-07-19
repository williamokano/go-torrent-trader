package postgres

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

func TestNotificationRepoCountAndListUnreadSince(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewNotificationRepo(db)
	u := newUser(t, db)

	cutoff := time.Now()

	// Older than the cutoff: must be excluded even though unread.
	if err := repo.Create(ctx, &model.Notification{
		UserID: u.ID, Type: model.NotifSystem, Data: json.RawMessage(`{}`),
	}); err != nil {
		t.Fatalf("Create(old): %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`UPDATE notifications SET created_at = $1 WHERE user_id = $2`, cutoff.Add(-time.Hour), u.ID,
	); err != nil {
		t.Fatalf("backdating old notification: %v", err)
	}

	// After the cutoff and unread: must be counted/listed.
	newer := &model.Notification{UserID: u.ID, Type: model.NotifMention, Data: json.RawMessage(`{}`)}
	if err := repo.Create(ctx, newer); err != nil {
		t.Fatalf("Create(newer): %v", err)
	}

	// After the cutoff but already read: must be excluded.
	read := &model.Notification{UserID: u.ID, Type: model.NotifForumReply, Data: json.RawMessage(`{}`)}
	if err := repo.Create(ctx, read); err != nil {
		t.Fatalf("Create(read): %v", err)
	}
	if err := repo.MarkRead(ctx, u.ID, read.ID); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}

	count, err := repo.CountUnreadSince(ctx, u.ID, cutoff)
	if err != nil {
		t.Fatalf("CountUnreadSince: %v", err)
	}
	if count != 1 {
		t.Errorf("CountUnreadSince = %d, want 1", count)
	}

	list, err := repo.ListUnreadSince(ctx, u.ID, cutoff, 10)
	if err != nil {
		t.Fatalf("ListUnreadSince: %v", err)
	}
	if len(list) != 1 || list[0].ID != newer.ID {
		t.Errorf("ListUnreadSince = %+v, want just the newer unread notification", list)
	}

	// limit <= 0 falls back to a sane default instead of returning nothing.
	list, err = repo.ListUnreadSince(ctx, u.ID, cutoff, 0)
	if err != nil {
		t.Fatalf("ListUnreadSince(limit=0): %v", err)
	}
	if len(list) != 1 {
		t.Errorf("ListUnreadSince(limit=0) = %d items, want 1", len(list))
	}
}

func TestNotificationDigestPreferenceRepoLifecycle(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewNotificationDigestPreferenceRepo(db)
	u := newUser(t, db)

	// No row yet: defaults to off.
	freq, err := repo.GetFrequency(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetFrequency (default): %v", err)
	}
	if freq != model.DigestOff {
		t.Errorf("GetFrequency (default) = %q, want %q", freq, model.DigestOff)
	}

	// Set to daily, then confirm it reads back and the upsert doesn't duplicate the row.
	if err := repo.SetFrequency(ctx, u.ID, model.DigestDaily); err != nil {
		t.Fatalf("SetFrequency(daily): %v", err)
	}
	if err := repo.SetFrequency(ctx, u.ID, model.DigestWeekly); err != nil {
		t.Fatalf("SetFrequency(weekly): %v", err)
	}
	freq, err = repo.GetFrequency(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetFrequency: %v", err)
	}
	if freq != model.DigestWeekly {
		t.Errorf("GetFrequency = %q, want %q", freq, model.DigestWeekly)
	}
	var rowCount int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM notification_digest_preferences WHERE user_id = $1`, u.ID,
	).Scan(&rowCount); err != nil {
		t.Fatalf("counting rows: %v", err)
	}
	if rowCount != 1 {
		t.Errorf("expected the upsert to keep a single row, found %d", rowCount)
	}
}

func TestNotificationDigestPreferenceRepoListDue(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewNotificationDigestPreferenceRepo(db)
	now := time.Now()

	neverSent := newUser(t, db) // daily, never sent -> always due
	if err := repo.SetFrequency(ctx, neverSent.ID, model.DigestDaily); err != nil {
		t.Fatalf("SetFrequency(neverSent): %v", err)
	}

	recentlySent := newUser(t, db) // daily, sent an hour ago -> not due yet
	if err := repo.SetFrequency(ctx, recentlySent.ID, model.DigestDaily); err != nil {
		t.Fatalf("SetFrequency(recentlySent): %v", err)
	}
	if err := repo.MarkSent(ctx, recentlySent.ID, now.Add(-time.Hour)); err != nil {
		t.Fatalf("MarkSent(recentlySent): %v", err)
	}

	overdue := newUser(t, db) // daily, sent two days ago -> due
	if err := repo.SetFrequency(ctx, overdue.ID, model.DigestDaily); err != nil {
		t.Fatalf("SetFrequency(overdue): %v", err)
	}
	if err := repo.MarkSent(ctx, overdue.ID, now.Add(-48*time.Hour)); err != nil {
		t.Fatalf("MarkSent(overdue): %v", err)
	}

	weeklyUser := newUser(t, db) // weekly cadence -> must not show up in a "daily" query
	if err := repo.SetFrequency(ctx, weeklyUser.ID, model.DigestWeekly); err != nil {
		t.Fatalf("SetFrequency(weeklyUser): %v", err)
	}

	off := newUser(t, db) // explicit off -> never due
	if err := repo.SetFrequency(ctx, off.ID, model.DigestOff); err != nil {
		t.Fatalf("SetFrequency(off): %v", err)
	}

	// Cutoff mirrors NotificationDigestService: "due" means last sent before
	// (now - interval), or never sent.
	cutoff := now.Add(-24 * time.Hour)
	due, err := repo.ListDue(ctx, model.DigestDaily, cutoff)
	if err != nil {
		t.Fatalf("ListDue: %v", err)
	}

	gotIDs := map[int64]bool{}
	for _, r := range due {
		gotIDs[r.UserID] = true
		if r.UserID == overdue.ID && r.Email != overdue.Email {
			t.Errorf("ListDue email = %q, want %q", r.Email, overdue.Email)
		}
	}
	if !gotIDs[neverSent.ID] {
		t.Error("a user who was never sent a digest must be due")
	}
	if !gotIDs[overdue.ID] {
		t.Error("a user last sent well past the interval must be due")
	}
	if gotIDs[recentlySent.ID] {
		t.Error("a user sent within the interval must not be due yet")
	}
	if gotIDs[weeklyUser.ID] {
		t.Error("a weekly-cadence user must not appear in a daily ListDue query")
	}
	if gotIDs[off.ID] {
		t.Error("a user with digests off must never be due")
	}
}

func TestNotificationDigestPreferenceRepoListDueExcludesDisabledUsers(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewNotificationDigestPreferenceRepo(db)
	u := newUser(t, db)
	if err := repo.SetFrequency(ctx, u.ID, model.DigestDaily); err != nil {
		t.Fatalf("SetFrequency: %v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE users SET enabled = FALSE WHERE id = $1`, u.ID); err != nil {
		t.Fatalf("disabling user: %v", err)
	}

	due, err := repo.ListDue(ctx, model.DigestDaily, time.Now())
	if err != nil {
		t.Fatalf("ListDue: %v", err)
	}
	for _, r := range due {
		if r.UserID == u.ID {
			t.Error("a disabled user must not be returned as due for a digest")
		}
	}
}
