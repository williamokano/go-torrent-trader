package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

type fakeHnRGroupRepo struct{ groups []model.Group }

func (f *fakeHnRGroupRepo) GetByID(_ context.Context, id int64) (*model.Group, error) {
	for i := range f.groups {
		if f.groups[i].ID == id {
			g := f.groups[i]
			return &g, nil
		}
	}
	return nil, sql.ErrNoRows
}

func (f *fakeHnRGroupRepo) List(_ context.Context) ([]model.Group, error) {
	return f.groups, nil
}

func hnrTestGroups() []model.Group {
	return []model.Group{
		{ID: 1, Name: "User", Level: 20},
		{ID: 2, Name: "VIP", Level: 60},
		{ID: 3, Name: "Moderator", Level: 80, IsModerator: true},
		{ID: 4, Name: "Administrator", Level: 100, IsAdmin: true},
	}
}

func setupHnRService() (*HnRService, *fakeHnRRepo) {
	repo := newFakeHnRRepo()
	// db is nil: these tests exercise rule CRUD and the evaluate/mark sweep,
	// neither of which touches RunDaemon's locking. RunDaemon itself is
	// validated separately against a live Postgres instance (advisory locks
	// have no meaningful fake).
	users := newMockUserRepoForRestrictions()
	bus := event.NewInMemoryBus()
	warnings := NewWarningService(newMockWarningRepo(), users, newMockMessageRepoForWarnings(), bus)
	restrictions := NewRestrictionService(newMockRestrictionRepo(), users, bus)
	svc := NewHnRService(nil, repo, &fakeHnRGroupRepo{groups: hnrTestGroups()}, users, warnings, restrictions, nil, bus)
	return svc, repo
}

func TestHnRService_UpsertRule_HappyPath(t *testing.T) {
	svc, _ := setupHnRService()

	rule, err := svc.UpsertRule(context.Background(), 1, HnRRuleInput{
		RequiredSeedHours: 240, RequiredRatio: 1.0, InactivityGraceHours: 48, MaxDaysToSatisfy: 30,
	})
	if err != nil {
		t.Fatalf("UpsertRule: %v", err)
	}
	if rule.RequiredSeedHours != 240 {
		t.Errorf("expected 240, got %d", rule.RequiredSeedHours)
	}
}

func TestHnRService_UpsertRule_RejectsStaffGroups(t *testing.T) {
	svc, _ := setupHnRService()

	for _, gid := range []int64{3, 4} { // moderator, admin
		_, err := svc.UpsertRule(context.Background(), gid, HnRRuleInput{RequiredSeedHours: 1})
		if !errors.Is(err, ErrHnRStaffGroup) {
			t.Errorf("group %d: got %v, want ErrHnRStaffGroup", gid, err)
		}
	}
}

func TestHnRService_UpsertRule_RejectsUnknownGroup(t *testing.T) {
	svc, _ := setupHnRService()
	_, err := svc.UpsertRule(context.Background(), 999, HnRRuleInput{RequiredSeedHours: 1})
	if !errors.Is(err, ErrHnRGroupNotFound) {
		t.Errorf("got %v, want ErrHnRGroupNotFound", err)
	}
}

func TestHnRService_UpsertRule_RejectsNegativeThresholds(t *testing.T) {
	svc, _ := setupHnRService()
	cases := []HnRRuleInput{
		{RequiredSeedHours: -1},
		{RequiredRatio: -0.5},
		{InactivityGraceHours: -1},
		{MaxDaysToSatisfy: -1},
	}
	for _, in := range cases {
		if _, err := svc.UpsertRule(context.Background(), 1, in); !errors.Is(err, ErrHnRInvalidThreshold) {
			t.Errorf("input %+v: got %v, want ErrHnRInvalidThreshold", in, err)
		}
	}
}

func TestHnRService_ListRules_OrderedByLevel(t *testing.T) {
	svc, repo := setupHnRService()
	// VIP (level 60) inserted before User (level 20), to prove ListRules
	// sorts by level rather than insertion order.
	if _, err := svc.UpsertRule(context.Background(), 2, HnRRuleInput{RequiredSeedHours: 1}); err != nil {
		t.Fatalf("upsert vip rule: %v", err)
	}
	if _, err := svc.UpsertRule(context.Background(), 1, HnRRuleInput{RequiredSeedHours: 1}); err != nil {
		t.Fatalf("upsert user rule: %v", err)
	}

	views, err := svc.ListRules(context.Background())
	if err != nil {
		t.Fatalf("ListRules: %v", err)
	}
	if len(views) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(views))
	}
	if views[0].GroupID != 1 || views[1].GroupID != 2 {
		t.Errorf("expected [User(1), VIP(2)] ordered by level, got [%d, %d]", views[0].GroupID, views[1].GroupID)
	}
	if len(repo.rules) != 2 {
		t.Fatalf("sanity check failed: %d rules stored", len(repo.rules))
	}
}

func TestHnRService_DeleteRule(t *testing.T) {
	svc, _ := setupHnRService()
	if _, err := svc.UpsertRule(context.Background(), 1, HnRRuleInput{RequiredSeedHours: 1}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := svc.DeleteRule(context.Background(), 1); err != nil {
		t.Fatalf("DeleteRule: %v", err)
	}
	if err := svc.DeleteRule(context.Background(), 1); !errors.Is(err, ErrHnRRuleNotFound) {
		t.Errorf("DeleteRule(missing) = %v, want ErrHnRRuleNotFound", err)
	}
}

// --- daemon sweep (evaluateAndMark, via RunDaemon with db=nil is not
// possible — RunDaemon requires a real db for locking, so these tests call
// runLocked-equivalent behavior through evaluateAndMark directly, which is
// exactly what runLocked calls once the lock is held) ---

func TestHnRService_EvaluateAndMark_BreachesOverdueRecord(t *testing.T) {
	svc, repo := setupHnRService()
	repo.setUserGroup(1, 10)
	repo.setTorrent(100, 1000, false)
	if err := repo.UpsertRule(context.Background(), &model.HnRRule{
		GroupID: 10, RequiredSeedHours: 100, RequiredRatio: 1.0,
		InactivityGraceHours: 1, MaxDaysToSatisfy: 30,
	}); err != nil {
		t.Fatalf("upsert rule: %v", err)
	}

	old := time.Now().Add(-2 * time.Hour)
	if _, err := repo.CreateIfNotExists(context.Background(), 1, 100, old); err != nil {
		t.Fatalf("create record: %v", err)
	}
	// Backdate last_seen_at so the 1-hour grace is exceeded.
	rec := repo.recordByUserTorrent(1, 100)
	rec.LastSeenAt = old

	counts, err := svc.evaluateAndMark(context.Background())
	if err != nil {
		t.Fatalf("evaluateAndMark: %v", err)
	}
	if counts.Scanned != 1 || counts.Breached != 1 {
		t.Fatalf("unexpected counts: %+v", counts)
	}
	if rec.State != model.HnRStateBreach {
		t.Errorf("expected state=hnr, got %s", rec.State)
	}
}

func TestHnRService_EvaluateAndMark_SatisfiesRecordMeetingPolicy(t *testing.T) {
	svc, repo := setupHnRService()
	repo.setUserGroup(1, 10)
	repo.setTorrent(100, 1000, false)
	if err := repo.UpsertRule(context.Background(), &model.HnRRule{
		GroupID: 10, RequiredSeedHours: 1, RequiredRatio: 1.0, InactivityGraceHours: 48,
	}); err != nil {
		t.Fatalf("upsert rule: %v", err)
	}
	if _, err := repo.CreateIfNotExists(context.Background(), 1, 100, time.Now()); err != nil {
		t.Fatalf("create record: %v", err)
	}
	rec := repo.recordByUserTorrent(1, 100)
	rec.SeededSeconds = 3600 // exactly the 1-hour requirement

	counts, err := svc.evaluateAndMark(context.Background())
	if err != nil {
		t.Fatalf("evaluateAndMark: %v", err)
	}
	if counts.Satisfied != 1 {
		t.Fatalf("unexpected counts: %+v", counts)
	}
	if rec.State != model.HnRStateSatisfied {
		t.Errorf("expected state=satisfied, got %s", rec.State)
	}
}

func TestHnRService_EvaluateAndMark_WaivesExemptTorrent(t *testing.T) {
	svc, repo := setupHnRService()
	repo.setUserGroup(1, 10)
	repo.setTorrent(100, 1000, true) // hnr_exempt=true
	if _, err := repo.CreateIfNotExists(context.Background(), 1, 100, time.Now()); err != nil {
		t.Fatalf("create record: %v", err)
	}
	// CreateIfNotExists itself refuses to create a record for an exempt
	// torrent, so force one into existence directly to prove evaluateAndMark
	// also waives a record that became exempt *after* it was created.
	if len(repo.records) == 0 {
		repo.records[1] = &model.HnRRecord{ID: 1, UserID: 1, TorrentID: 100, State: model.HnRStateActive, CompletedAt: time.Now(), LastSeenAt: time.Now()}
		repo.nextID = 2
	}

	counts, err := svc.evaluateAndMark(context.Background())
	if err != nil {
		t.Fatalf("evaluateAndMark: %v", err)
	}
	if counts.Scanned != 1 {
		t.Fatalf("unexpected counts: %+v", counts)
	}
	rec := repo.recordByUserTorrent(1, 100)
	if rec.State != model.HnRStateWaived {
		t.Errorf("expected state=waived for an exempt torrent, got %s", rec.State)
	}
}

func TestHnRService_EvaluateAndMark_WaivesRecordWithNoRuleForClass(t *testing.T) {
	svc, repo := setupHnRService()
	repo.setUserGroup(1, 10) // no rule registered for group 10 (e.g. VIP)
	repo.setTorrent(100, 1000, false)
	if _, err := repo.CreateIfNotExists(context.Background(), 1, 100, time.Now()); err != nil {
		t.Fatalf("create record: %v", err)
	}

	counts, err := svc.evaluateAndMark(context.Background())
	if err != nil {
		t.Fatalf("evaluateAndMark: %v", err)
	}
	if counts.Scanned != 1 {
		t.Fatalf("unexpected counts: %+v", counts)
	}
	rec := repo.recordByUserTorrent(1, 100)
	if rec.State != model.HnRStateWaived {
		t.Errorf("expected state=waived when the user's class has no rule, got %s", rec.State)
	}
}

func TestHnRService_EvaluateAndMark_LeavesRecordWithinGraceAlone(t *testing.T) {
	svc, repo := setupHnRService()
	repo.setUserGroup(1, 10)
	repo.setTorrent(100, 1000, false)
	if err := repo.UpsertRule(context.Background(), &model.HnRRule{
		GroupID: 10, RequiredSeedHours: 100, RequiredRatio: 1.0, InactivityGraceHours: 48,
	}); err != nil {
		t.Fatalf("upsert rule: %v", err)
	}
	if _, err := repo.CreateIfNotExists(context.Background(), 1, 100, time.Now()); err != nil {
		t.Fatalf("create record: %v", err)
	}

	counts, err := svc.evaluateAndMark(context.Background())
	if err != nil {
		t.Fatalf("evaluateAndMark: %v", err)
	}
	if counts.Breached != 0 || counts.Satisfied != 0 {
		t.Fatalf("expected no transitions for a record still within grace: %+v", counts)
	}
	rec := repo.recordByUserTorrent(1, 100)
	if rec.State != model.HnRStateActive {
		t.Errorf("expected state to remain active, got %s", rec.State)
	}
}

func TestHnRService_RunDaemon_UnavailableWithoutDB(t *testing.T) {
	svc, _ := setupHnRService()
	_, err := svc.RunDaemon(context.Background(), model.HnRRunTriggerManual, nil)
	if !errors.Is(err, ErrHnRDaemonUnavailable) {
		t.Fatalf("got %v, want ErrHnRDaemonUnavailable", err)
	}
}
