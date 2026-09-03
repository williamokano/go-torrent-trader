package service

import (
	"context"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

func TestHnRService_ListForUser_EmptyIsEmptySliceNotNil(t *testing.T) {
	svc, _ := setupHnRService()
	views, err := svc.ListForUser(context.Background(), 1)
	if err != nil {
		t.Fatalf("ListForUser: %v", err)
	}
	if views == nil {
		t.Fatal("expected an empty slice, got nil (would serialize as JSON null instead of [])")
	}
	if len(views) != 0 {
		t.Fatalf("expected no records, got %d", len(views))
	}
}

func TestHnRService_ListForUser_LiveEvaluatesAheadOfStoredState(t *testing.T) {
	svc, repo := setupHnRService()
	repo.setUserGroup(1, 10)
	repo.setTorrent(100, 1000, false)
	if err := repo.UpsertRule(context.Background(), &model.HnRRule{
		GroupID: 10, RequiredSeedHours: 100, RequiredRatio: 1.0, InactivityGraceHours: 1, MaxDaysToSatisfy: 30,
	}); err != nil {
		t.Fatalf("upsert rule: %v", err)
	}
	old := time.Now().Add(-2 * time.Hour)
	if _, err := repo.CreateIfNotExists(context.Background(), 1, 100, old); err != nil {
		t.Fatalf("create record: %v", err)
	}
	rec := repo.recordByUserTorrent(1, 100)
	rec.LastSeenAt = old // past the 1-hour grace, but the daemon hasn't run yet

	views, err := svc.ListForUser(context.Background(), 1)
	if err != nil {
		t.Fatalf("ListForUser: %v", err)
	}
	if len(views) != 1 {
		t.Fatalf("expected 1 record, got %d", len(views))
	}
	if views[0].State != model.HnRStateActive {
		t.Errorf("expected stored state still active (daemon hasn't run), got %s", views[0].State)
	}
	if views[0].DisplayStatus != string(HnRStatusBreach) {
		t.Errorf("expected display_status=breach evaluated live, got %s", views[0].DisplayStatus)
	}
}

func TestHnRService_ListForUser_LiveSeedingOverridesStaleBreach(t *testing.T) {
	svc, repo := setupHnRService()
	repo.setUserGroup(1, 10)
	repo.setTorrent(100, 1000, false)
	if err := repo.UpsertRule(context.Background(), &model.HnRRule{
		GroupID: 10, RequiredSeedHours: 100, RequiredRatio: 1.0, InactivityGraceHours: 1, MaxDaysToSatisfy: 30,
	}); err != nil {
		t.Fatalf("upsert rule: %v", err)
	}
	old := time.Now().Add(-2 * time.Hour)
	if _, err := repo.CreateIfNotExists(context.Background(), 1, 100, old); err != nil {
		t.Fatalf("create record: %v", err)
	}
	rec := repo.recordByUserTorrent(1, 100)
	rec.State = model.HnRStateBreach
	rec.LastSeenAt = old
	// The member just started their client again: peers proves it, even
	// though nothing has credited seed time yet.
	repo.setLiveSeeding(1, 100, true)

	views, err := svc.ListForUser(context.Background(), 1)
	if err != nil {
		t.Fatalf("ListForUser: %v", err)
	}
	if len(views) != 1 {
		t.Fatalf("expected 1 record, got %d", len(views))
	}
	if !views[0].CurrentlySeeding {
		t.Error("expected currently_seeding=true")
	}
	if views[0].DisplayStatus != string(HnRStatusMonitoring) {
		t.Errorf("expected an active seeding peer to override the stale breach to monitoring, got %s", views[0].DisplayStatus)
	}
}

func TestHnRService_ListForUser_ResolvedRecordsPassThroughStateDirectly(t *testing.T) {
	svc, repo := setupHnRService()
	repo.records[1] = &model.HnRRecord{
		ID: 1, UserID: 1, TorrentID: 100, State: model.HnRStateSatisfied,
		CompletedAt: time.Now(), LastSeenAt: time.Now(), ResolvedAt: ptrTime(time.Now()),
	}
	repo.records[2] = &model.HnRRecord{
		ID: 2, UserID: 1, TorrentID: 101, State: model.HnRStateCleared,
		CompletedAt: time.Now(), LastSeenAt: time.Now(), ResolvedAt: ptrTime(time.Now()),
	}
	repo.records[3] = &model.HnRRecord{
		ID: 3, UserID: 1, TorrentID: 102, State: model.HnRStateWaived,
		CompletedAt: time.Now(), LastSeenAt: time.Now(), ResolvedAt: ptrTime(time.Now()),
	}
	repo.nextID = 4

	views, err := svc.ListForUser(context.Background(), 1)
	if err != nil {
		t.Fatalf("ListForUser: %v", err)
	}
	if len(views) != 3 {
		t.Fatalf("expected 3 records, got %d", len(views))
	}
	byID := map[int64]HnRRecordView{}
	for _, v := range views {
		byID[v.ID] = v
	}
	if byID[1].DisplayStatus != model.HnRStateSatisfied {
		t.Errorf("satisfied record: got display_status=%s", byID[1].DisplayStatus)
	}
	if byID[2].DisplayStatus != model.HnRStateCleared {
		t.Errorf("cleared record: got display_status=%s", byID[2].DisplayStatus)
	}
	if byID[3].DisplayStatus != model.HnRStateWaived {
		t.Errorf("waived record: got display_status=%s", byID[3].DisplayStatus)
	}
}

func TestHnRService_ListForUser_NoRuleForClassDisplaysOpenRecordAsWaived(t *testing.T) {
	svc, repo := setupHnRService()
	repo.setUserGroup(1, 10) // no rule registered for group 10 (e.g. VIP)
	repo.setTorrent(100, 1000, false)
	if _, err := repo.CreateIfNotExists(context.Background(), 1, 100, time.Now()); err != nil {
		t.Fatalf("create record: %v", err)
	}

	views, err := svc.ListForUser(context.Background(), 1)
	if err != nil {
		t.Fatalf("ListForUser: %v", err)
	}
	if len(views) != 1 {
		t.Fatalf("expected 1 record, got %d", len(views))
	}
	if views[0].State != model.HnRStateActive {
		t.Errorf("expected stored state still active (daemon hasn't run), got %s", views[0].State)
	}
	if views[0].DisplayStatus != model.HnRStateWaived {
		t.Errorf("expected display_status=waived for a class with no rule, got %s", views[0].DisplayStatus)
	}
	if views[0].RequiredSeedHours != nil {
		t.Error("expected required_seed_hours to be nil when the class has no rule")
	}
}

func TestHnRService_ListForUser_IncludesThresholdsWhenRuleExists(t *testing.T) {
	svc, repo := setupHnRService()
	repo.setUserGroup(1, 10)
	repo.setTorrent(100, 1000, false)
	if err := repo.UpsertRule(context.Background(), &model.HnRRule{
		GroupID: 10, RequiredSeedHours: 240, RequiredRatio: 1.0, InactivityGraceHours: 48, MaxDaysToSatisfy: 30,
	}); err != nil {
		t.Fatalf("upsert rule: %v", err)
	}
	if _, err := repo.CreateIfNotExists(context.Background(), 1, 100, time.Now()); err != nil {
		t.Fatalf("create record: %v", err)
	}

	views, err := svc.ListForUser(context.Background(), 1)
	if err != nil {
		t.Fatalf("ListForUser: %v", err)
	}
	if len(views) != 1 {
		t.Fatalf("expected 1 record, got %d", len(views))
	}
	if views[0].RequiredSeedHours == nil || *views[0].RequiredSeedHours != 240 {
		t.Errorf("expected required_seed_hours=240, got %v", views[0].RequiredSeedHours)
	}
	if views[0].RequiredRatio == nil || *views[0].RequiredRatio != 1.0 {
		t.Errorf("expected required_ratio=1.0, got %v", views[0].RequiredRatio)
	}
	if views[0].DisplayStatus != string(HnRStatusMonitoring) {
		t.Errorf("expected display_status=monitoring for a fresh record within grace, got %s", views[0].DisplayStatus)
	}
}

func ptrTime(t time.Time) *time.Time { return &t }
