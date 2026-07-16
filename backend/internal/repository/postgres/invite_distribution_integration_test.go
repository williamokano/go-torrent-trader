package postgres

import (
	"context"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/listener"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// TestInviteDistributionEndToEnd_GrantsOnceAndLogsActivity is the full-stack
// functional verification for BE-4.2: a real Postgres-backed repository,
// service, event bus, and activity-log listener wired together exactly like
// cmd/server/main.go does, run against a user sitting just inside a rule's
// thresholds. It proves the invite balance increments by exactly one and a
// human-readable activity log entry is written — the two outcomes the story's
// acceptance criteria call for ("grant +1" and "log distributions").
func TestInviteDistributionEndToEnd_GrantsOnceAndLogsActivity(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	// --- wire the real stack, mirroring cmd/server/main.go ---
	userRepo := NewUserRepo(db)
	groupRepo := NewGroupRepo(db)
	inviteDistRepo := NewInviteDistributionRepo(db)
	siteSettingsRepo := NewSiteSettingsRepo(db)
	activityLogRepo := NewActivityLogRepo(db)

	bus := event.NewInMemoryBus()
	settingsSvc := service.NewSiteSettingsService(siteSettingsRepo, bus)
	activityLogSvc := service.NewActivityLogService(activityLogRepo)
	listener.RegisterActivityLogListeners(bus, activityLogSvc, userRepo)
	inviteDistSvc := service.NewInviteDistributionService(inviteDistRepo, groupRepo, userRepo, settingsSvc, bus)

	// Enable the engine (site_settings.Set publishes events too, which is fine —
	// they're unrelated event types and won't add invite_auto_granted rows).
	if err := settingsSvc.Set(ctx, service.SettingInviteDistributionEnabled, "true", event.Actor{ID: 0, Username: "System"}); err != nil {
		t.Fatalf("enable invite distribution: %v", err)
	}

	// A user sitting just inside a "vip" rule: ratio 2.0 (>= min 1.0),
	// downloaded 2000 bytes (within [1000, 5000]), starting with 0 invites
	// against a ceiling of 2.
	vipGroup := groupIDBySlug(t, db, "vip")
	u := newUser(t, db)
	if _, err := db.ExecContext(ctx, `UPDATE users SET group_id=$1, uploaded=$2, downloaded=$3, invites=$4, can_invite=$5 WHERE id=$6`,
		vipGroup, int64(2000), int64(1000), 0, true, u.ID); err != nil {
		t.Fatalf("seed user near threshold: %v", err)
	}

	rule := &model.InviteDistributionRule{
		GroupID: vipGroup, MinRatio: 1.0, MinDownloadedBytes: 1000, MaxDownloadedBytes: 5000, MaxInvites: 2,
	}
	if err := inviteDistRepo.UpsertRule(ctx, rule); err != nil {
		t.Fatalf("upsert rule: %v", err)
	}

	summary, err := inviteDistSvc.Run(ctx, true)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if summary.Skipped || summary.Granted != 1 {
		t.Fatalf("expected exactly one grant, got %+v", summary)
	}

	// The invite count must increment exactly once.
	got, err := userRepo.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Invites != 1 {
		t.Fatalf("invites = %d, want 1 (exactly one grant)", got.Invites)
	}

	// Running again immediately (still force=true, bypassing the interval)
	// must not grant a second time in the same call — MaxInvites is 2, so this
	// specifically proves the ceiling (not the interval) is what would stop
	// runaway grants; here it still grants once more since 1 < 2.
	summary2, err := inviteDistSvc.Run(ctx, true)
	if err != nil {
		t.Fatalf("Run (second): %v", err)
	}
	if summary2.Granted != 1 {
		t.Fatalf("expected exactly one more grant (now at ceiling 2), got %+v", summary2)
	}
	got, err = userRepo.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Invites != 2 {
		t.Fatalf("invites = %d, want 2 (at ceiling)", got.Invites)
	}

	// A third run must grant nothing further: the user is now at the ceiling.
	summary3, err := inviteDistSvc.Run(ctx, true)
	if err != nil {
		t.Fatalf("Run (third): %v", err)
	}
	if summary3.Granted != 0 {
		t.Fatalf("expected no further grants at the ceiling, got %+v", summary3)
	}

	// A sensible, self-contained activity log entry must exist for each grant.
	logs, total, err := activityLogSvc.List(ctx, repository.ListActivityLogsOptions{Page: 1, PerPage: 10})
	if err != nil {
		t.Fatalf("list activity logs: %v", err)
	}
	if total != 2 {
		t.Fatalf("expected 2 invite_auto_granted log entries (one per grant), got %d", total)
	}
	for _, l := range logs {
		if l.EventType != "invite_auto_granted" {
			t.Errorf("unexpected event type in log: %s", l.EventType)
			continue
		}
		want := "System granted an invite to " + u.Username + " via auto-distribution"
		if l.Message != want {
			t.Errorf("activity log message = %q, want %q", l.Message, want)
		}
		if l.ActorID != nil {
			t.Errorf("expected no actor_id for a system-initiated grant, got %v", *l.ActorID)
		}
	}
}
