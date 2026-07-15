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

// --- fakes ---

type fakePromotionRepo struct {
	rules     map[int64]model.PromotionRule
	metrics   []model.PromotionUserMetrics
	seedHours map[int64]int
	moves     map[int64]int64 // userID -> new groupID
	lastRun   *time.Time
	runs      [][2]int // {promoted, demoted}
}

func newFakePromotionRepo() *fakePromotionRepo {
	return &fakePromotionRepo{
		rules:     map[int64]model.PromotionRule{},
		seedHours: map[int64]int{},
		moves:     map[int64]int64{},
	}
}

func (f *fakePromotionRepo) ListRules(_ context.Context) ([]model.PromotionRule, error) {
	out := make([]model.PromotionRule, 0, len(f.rules))
	for _, r := range f.rules {
		out = append(out, r)
	}
	return out, nil
}

func (f *fakePromotionRepo) UpsertRule(_ context.Context, r *model.PromotionRule) error {
	f.rules[r.GroupID] = *r
	return nil
}

func (f *fakePromotionRepo) DeleteRule(_ context.Context, groupID int64) error {
	if _, ok := f.rules[groupID]; !ok {
		return sql.ErrNoRows
	}
	delete(f.rules, groupID)
	return nil
}

func (f *fakePromotionRepo) LadderMetrics(_ context.Context, groupIDs []int64) ([]model.PromotionUserMetrics, error) {
	inLadder := map[int64]bool{}
	for _, id := range groupIDs {
		inLadder[id] = true
	}
	var out []model.PromotionUserMetrics
	for _, m := range f.metrics {
		if inLadder[m.GroupID] {
			out = append(out, m)
		}
	}
	return out, nil
}

func (f *fakePromotionRepo) SeedHoursByUser(_ context.Context, _ time.Time, _ int) (map[int64]int, error) {
	return f.seedHours, nil
}

func (f *fakePromotionRepo) SetUserGroup(_ context.Context, userID, groupID int64) error {
	f.moves[userID] = groupID
	return nil
}

func (f *fakePromotionRepo) LastRunAt(_ context.Context) (time.Time, bool, error) {
	if f.lastRun == nil {
		return time.Time{}, false, nil
	}
	return *f.lastRun, true, nil
}

func (f *fakePromotionRepo) RecordRun(_ context.Context, promoted, demoted int) error {
	f.runs = append(f.runs, [2]int{promoted, demoted})
	return nil
}

type fakePromoGroupRepo struct{ groups []model.Group }

func (f *fakePromoGroupRepo) GetByID(_ context.Context, id int64) (*model.Group, error) {
	for i := range f.groups {
		if f.groups[i].ID == id {
			g := f.groups[i]
			return &g, nil
		}
	}
	return nil, sql.ErrNoRows
}

func (f *fakePromoGroupRepo) List(_ context.Context) ([]model.Group, error) {
	return f.groups, nil
}

// Ladder: User(id5,lvl20) -> PowerUser(id4,lvl40) -> Elite(id8,lvl60). IDs are
// deliberately out of level order to prove ordering is by level, not id.
func ladderGroups() []model.Group {
	return []model.Group{
		{ID: 5, Name: "User", Level: 20},
		{ID: 4, Name: "Power User", Level: 40},
		{ID: 8, Name: "Elite", Level: 60},
		{ID: 1, Name: "Administrator", Level: 100, IsAdmin: true},
	}
}

func newPromotionSvc(settings map[string]string, groups []model.Group, repo *fakePromotionRepo) *PromotionService {
	settingsRepo := newMockSiteSettingsRepo()
	for k, v := range settings {
		settingsRepo.settings[k] = &model.SiteSetting{Key: k, Value: v}
	}
	ss := NewSiteSettingsService(settingsRepo, event.NewInMemoryBus())
	return NewPromotionService(repo, &fakePromoGroupRepo{groups: groups}, ss)
}

func enabledSettings() map[string]string {
	return map[string]string{
		SettingPromotionEnabled:        "true",
		SettingPromotionIntervalDays:   "7",
		SettingPromotionSeedWindowDays: "30",
	}
}

func seedLadderRules(repo *fakePromotionRepo) {
	repo.rules[5] = model.PromotionRule{GroupID: 5} // floor, no constraints
	repo.rules[4] = model.PromotionRule{GroupID: 4, MinRatio: 1.0, MinUploaded: 100}
	repo.rules[8] = model.PromotionRule{GroupID: 8, MinRatio: 2.0, MinUploaded: 1000}
}

// --- tests ---

func TestPromotionRun_DisabledSkips(t *testing.T) {
	repo := newFakePromotionRepo()
	seedLadderRules(repo)
	svc := newPromotionSvc(map[string]string{SettingPromotionEnabled: "false"}, ladderGroups(), repo)

	sum, err := svc.Run(context.Background(), false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !sum.Skipped || sum.Reason != "disabled" {
		t.Errorf("expected skipped/disabled, got %+v", sum)
	}
	if len(repo.runs) != 0 {
		t.Error("disabled run must not record a run")
	}
}

func TestPromotionRun_IntervalNotElapsed(t *testing.T) {
	repo := newFakePromotionRepo()
	seedLadderRules(repo)
	recent := time.Now().Add(-24 * time.Hour) // 1 day ago, interval is 7
	repo.lastRun = &recent
	svc := newPromotionSvc(enabledSettings(), ladderGroups(), repo)

	sum, err := svc.Run(context.Background(), false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !sum.Skipped {
		t.Errorf("expected skipped due to interval, got %+v", sum)
	}
}

func TestPromotionRun_ForceBypassesInterval(t *testing.T) {
	repo := newFakePromotionRepo()
	seedLadderRules(repo)
	recent := time.Now().Add(-24 * time.Hour) // well within the 7-day interval
	repo.lastRun = &recent
	repo.metrics = []model.PromotionUserMetrics{
		{UserID: 100, GroupID: 5, Uploaded: 5000, Downloaded: 1000}, // qualifies for promotion
	}
	svc := newPromotionSvc(enabledSettings(), ladderGroups(), repo)

	// force=true must ignore the interval and actually evaluate.
	sum, err := svc.Run(context.Background(), true)
	if err != nil {
		t.Fatalf("Run(force): %v", err)
	}
	if sum.Skipped {
		t.Fatalf("forced run should not be skipped, got reason %q", sum.Reason)
	}
	if repo.moves[100] != 4 {
		t.Errorf("forced run did not promote user: move=%d, want 4", repo.moves[100])
	}
}

// A forced run is still a no-op when the engine is disabled — force bypasses the
// interval, not the master switch.
func TestPromotionRun_ForceStillRespectsDisabled(t *testing.T) {
	repo := newFakePromotionRepo()
	seedLadderRules(repo)
	svc := newPromotionSvc(map[string]string{SettingPromotionEnabled: "false"}, ladderGroups(), repo)

	sum, err := svc.Run(context.Background(), true)
	if err != nil {
		t.Fatalf("Run(force): %v", err)
	}
	if !sum.Skipped || sum.Reason != "disabled" {
		t.Errorf("forced run while disabled should skip/disabled, got %+v", sum)
	}
}

func TestPromotionRun_PromotesOneStepOnly(t *testing.T) {
	repo := newFakePromotionRepo()
	seedLadderRules(repo)
	// User in floor group 5, metrics good enough for Elite (id 8), but must
	// still move only one rung to Power User (id 4).
	repo.metrics = []model.PromotionUserMetrics{
		{UserID: 100, GroupID: 5, Uploaded: 5000, Downloaded: 1000}, // ratio 5.0
	}
	svc := newPromotionSvc(enabledSettings(), ladderGroups(), repo)

	sum, err := svc.Run(context.Background(), false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if sum.Promoted != 1 || sum.Demoted != 0 {
		t.Fatalf("promoted=%d demoted=%d, want 1/0", sum.Promoted, sum.Demoted)
	}
	if repo.moves[100] != 4 {
		t.Errorf("user moved to group %d, want 4 (one step up, not Elite)", repo.moves[100])
	}
	if len(repo.runs) != 1 || repo.runs[0] != [2]int{1, 0} {
		t.Errorf("run not recorded correctly: %v", repo.runs)
	}
}

func TestPromotionRun_DemotesWhenFailingCurrentRung(t *testing.T) {
	repo := newFakePromotionRepo()
	seedLadderRules(repo)
	// User in Power User (id 4) whose ratio dropped below Power User's 1.0.
	repo.metrics = []model.PromotionUserMetrics{
		{UserID: 101, GroupID: 4, Uploaded: 50, Downloaded: 1000}, // ratio 0.05
	}
	svc := newPromotionSvc(enabledSettings(), ladderGroups(), repo)

	sum, err := svc.Run(context.Background(), false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if sum.Demoted != 1 || repo.moves[101] != 5 {
		t.Errorf("expected demotion to group 5, got demoted=%d move=%d", sum.Demoted, repo.moves[101])
	}
}

func TestPromotionRun_FloorUserNeverDemoted(t *testing.T) {
	repo := newFakePromotionRepo()
	seedLadderRules(repo)
	// Terrible metrics but already at the floor rung (id 5) — must stay.
	repo.metrics = []model.PromotionUserMetrics{
		{UserID: 102, GroupID: 5, Uploaded: 0, Downloaded: 9999},
	}
	svc := newPromotionSvc(enabledSettings(), ladderGroups(), repo)

	sum, err := svc.Run(context.Background(), false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if sum.Promoted != 0 || sum.Demoted != 0 {
		t.Errorf("floor user should not move, got %+v", sum)
	}
	if _, moved := repo.moves[102]; moved {
		t.Error("floor user was moved")
	}
}

func TestPromotionRun_TopUserStaysWhenQualified(t *testing.T) {
	repo := newFakePromotionRepo()
	seedLadderRules(repo)
	// Top rung (Elite id 8), still meets Elite rule — no rung above, stays.
	repo.metrics = []model.PromotionUserMetrics{
		{UserID: 103, GroupID: 8, Uploaded: 100000, Downloaded: 1000}, // ratio 100
	}
	svc := newPromotionSvc(enabledSettings(), ladderGroups(), repo)

	sum, err := svc.Run(context.Background(), false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if sum.Promoted != 0 || sum.Demoted != 0 {
		t.Errorf("qualified top user should stay, got %+v", sum)
	}
}

func TestPromotionRun_SeedHoursGate(t *testing.T) {
	repo := newFakePromotionRepo()
	repo.rules[5] = model.PromotionRule{GroupID: 5}
	repo.rules[4] = model.PromotionRule{GroupID: 4, MinSeedHours: 10}
	// Two floor users; only the one with >=10 seed hours (from the seed map) is promoted.
	repo.metrics = []model.PromotionUserMetrics{
		{UserID: 200, GroupID: 5, Uploaded: 1, Downloaded: 1},
		{UserID: 201, GroupID: 5, Uploaded: 1, Downloaded: 1},
	}
	repo.seedHours = map[int64]int{200: 12, 201: 3}
	svc := newPromotionSvc(enabledSettings(), []model.Group{
		{ID: 5, Name: "User", Level: 20}, {ID: 4, Name: "Power User", Level: 40},
	}, repo)

	if _, err := svc.Run(context.Background(), false); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if repo.moves[200] != 4 {
		t.Errorf("user 200 (12h seed) should be promoted to 4, got %d", repo.moves[200])
	}
	if _, moved := repo.moves[201]; moved {
		t.Error("user 201 (3h seed) should not be promoted")
	}
}

func TestPromotionRun_LadderExcludesStaff(t *testing.T) {
	repo := newFakePromotionRepo()
	seedLadderRules(repo)
	repo.rules[1] = model.PromotionRule{GroupID: 1, MinRatio: 0} // rule wrongly on admin group
	// A user sitting in the admin group must never be considered by the engine.
	repo.metrics = []model.PromotionUserMetrics{
		{UserID: 300, GroupID: 1, Uploaded: 1, Downloaded: 9999},
	}
	svc := newPromotionSvc(enabledSettings(), ladderGroups(), repo)

	if _, err := svc.Run(context.Background(), false); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, moved := repo.moves[300]; moved {
		t.Error("user in staff group was moved by the engine")
	}
}

func TestPromotionUpsertRule_RejectsStaffAndInvalid(t *testing.T) {
	repo := newFakePromotionRepo()
	svc := newPromotionSvc(enabledSettings(), ladderGroups(), repo)
	ctx := context.Background()

	if _, err := svc.UpsertRule(ctx, 1, PromotionRuleInput{}); !errors.Is(err, ErrPromotionStaffGroup) {
		t.Errorf("staff group rule: err=%v, want ErrPromotionStaffGroup", err)
	}
	if _, err := svc.UpsertRule(ctx, 999, PromotionRuleInput{}); !errors.Is(err, ErrPromotionGroupNotFound) {
		t.Errorf("missing group: err=%v, want ErrPromotionGroupNotFound", err)
	}
	if _, err := svc.UpsertRule(ctx, 4, PromotionRuleInput{MinRatio: -1}); !errors.Is(err, ErrPromotionInvalidThreshold) {
		t.Errorf("negative threshold: err=%v, want ErrPromotionInvalidThreshold", err)
	}
	if _, err := svc.UpsertRule(ctx, 4, PromotionRuleInput{MinRatio: 1.5, MinUploaded: 100}); err != nil {
		t.Errorf("valid rule rejected: %v", err)
	}
	if repo.rules[4].MinRatio != 1.5 {
		t.Error("valid rule not persisted")
	}
}

func TestPromotionListRules_OrderedByLevel(t *testing.T) {
	repo := newFakePromotionRepo()
	seedLadderRules(repo)
	svc := newPromotionSvc(enabledSettings(), ladderGroups(), repo)

	views, err := svc.ListRules(context.Background())
	if err != nil {
		t.Fatalf("ListRules: %v", err)
	}
	if len(views) != 3 {
		t.Fatalf("got %d views, want 3", len(views))
	}
	if views[0].GroupID != 5 || views[1].GroupID != 4 || views[2].GroupID != 8 {
		t.Errorf("not ordered by level: %d, %d, %d", views[0].GroupID, views[1].GroupID, views[2].GroupID)
	}
}
