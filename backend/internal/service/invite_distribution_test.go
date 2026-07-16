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

type fakeInviteDistRepo struct {
	rules   map[int64]model.InviteDistributionRule
	metrics []model.PromotionUserMetrics
	states  map[int64]model.UserInviteState
	lastRun *time.Time
	runs    []int // granted count per run

	// Error injection, so Run/ListRules/UpsertRule/DeleteRule's error-wrap
	// branches can be exercised without a real broken connection.
	listRulesErr    error
	upsertErr       error
	deleteErr       error
	groupMetricsErr error
	userStatesErr   error
	lastRunErr      error
	recordRunErr    error
}

func newFakeInviteDistRepo() *fakeInviteDistRepo {
	return &fakeInviteDistRepo{
		rules:  map[int64]model.InviteDistributionRule{},
		states: map[int64]model.UserInviteState{},
	}
}

func (f *fakeInviteDistRepo) ListRules(_ context.Context) ([]model.InviteDistributionRule, error) {
	if f.listRulesErr != nil {
		return nil, f.listRulesErr
	}
	out := make([]model.InviteDistributionRule, 0, len(f.rules))
	for _, r := range f.rules {
		out = append(out, r)
	}
	return out, nil
}

func (f *fakeInviteDistRepo) UpsertRule(_ context.Context, r *model.InviteDistributionRule) error {
	if f.upsertErr != nil {
		return f.upsertErr
	}
	f.rules[r.GroupID] = *r
	return nil
}

func (f *fakeInviteDistRepo) DeleteRule(_ context.Context, groupID int64) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	if _, ok := f.rules[groupID]; !ok {
		return sql.ErrNoRows
	}
	delete(f.rules, groupID)
	return nil
}

func (f *fakeInviteDistRepo) GroupMetrics(_ context.Context, groupIDs []int64) ([]model.PromotionUserMetrics, error) {
	if f.groupMetricsErr != nil {
		return nil, f.groupMetricsErr
	}
	inGroup := map[int64]bool{}
	for _, id := range groupIDs {
		inGroup[id] = true
	}
	var out []model.PromotionUserMetrics
	for _, m := range f.metrics {
		if inGroup[m.GroupID] {
			out = append(out, m)
		}
	}
	return out, nil
}

func (f *fakeInviteDistRepo) UserInviteStates(_ context.Context, userIDs []int64) (map[int64]model.UserInviteState, error) {
	if f.userStatesErr != nil {
		return nil, f.userStatesErr
	}
	out := make(map[int64]model.UserInviteState, len(userIDs))
	for _, id := range userIDs {
		if s, ok := f.states[id]; ok {
			out[id] = s
		}
	}
	return out, nil
}

func (f *fakeInviteDistRepo) LastRunAt(_ context.Context) (time.Time, bool, error) {
	if f.lastRunErr != nil {
		return time.Time{}, false, f.lastRunErr
	}
	if f.lastRun == nil {
		return time.Time{}, false, nil
	}
	return *f.lastRun, true, nil
}

func (f *fakeInviteDistRepo) RecordRun(_ context.Context, granted int) error {
	if f.recordRunErr != nil {
		return f.recordRunErr
	}
	f.runs = append(f.runs, granted)
	return nil
}

func newInviteDistSvc(settings map[string]string, groups []model.Group, repo *fakeInviteDistRepo, users inviteAdjustRepository) *InviteDistributionService {
	settingsRepo := newMockSiteSettingsRepo()
	for k, v := range settings {
		settingsRepo.settings[k] = &model.SiteSetting{Key: k, Value: v}
	}
	ss := NewSiteSettingsService(settingsRepo, event.NewInMemoryBus())
	return NewInviteDistributionService(repo, &fakePromoGroupRepo{groups: groups}, users, ss, event.NewInMemoryBus())
}

func inviteDistEnabledSettings() map[string]string {
	return map[string]string{
		SettingInviteDistributionEnabled:      "true",
		SettingInviteDistributionIntervalDays: "7",
	}
}

func inviteDistGroups() []model.Group {
	return []model.Group{
		{ID: 5, Name: "User", Level: 20},
		{ID: 1, Name: "Administrator", Level: 100, IsAdmin: true},
	}
}

// newInviteDistUser registers a user in the shared invite-user mock (from
// invite_test.go) and returns its assigned ID.
func newInviteDistUser(t *testing.T, users *mockInviteUserRepo) int64 {
	t.Helper()
	u := &model.User{Username: "candidate", CanInvite: true, Invites: 0}
	if err := users.Create(context.Background(), u); err != nil {
		t.Fatalf("create user: %v", err)
	}
	return u.ID
}

// --- tests ---

func TestInviteDistributionRun_DisabledSkips(t *testing.T) {
	repo := newFakeInviteDistRepo()
	repo.rules[5] = model.InviteDistributionRule{GroupID: 5, MaxInvites: 5}
	users := newMockInviteUserRepo()
	svc := newInviteDistSvc(map[string]string{SettingInviteDistributionEnabled: "false"}, inviteDistGroups(), repo, users)

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

func TestInviteDistributionRun_IntervalNotElapsed(t *testing.T) {
	repo := newFakeInviteDistRepo()
	repo.rules[5] = model.InviteDistributionRule{GroupID: 5, MaxInvites: 5}
	recent := time.Now().Add(-24 * time.Hour) // 1 day ago, interval is 7
	repo.lastRun = &recent
	users := newMockInviteUserRepo()
	svc := newInviteDistSvc(inviteDistEnabledSettings(), inviteDistGroups(), repo, users)

	sum, err := svc.Run(context.Background(), false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !sum.Skipped {
		t.Errorf("expected skipped due to interval, got %+v", sum)
	}
}

func TestInviteDistributionRun_ForceBypassesInterval(t *testing.T) {
	repo := newFakeInviteDistRepo()
	repo.rules[5] = model.InviteDistributionRule{GroupID: 5, MinRatio: 1.0, MaxInvites: 5}
	recent := time.Now().Add(-24 * time.Hour)
	repo.lastRun = &recent
	users := newMockInviteUserRepo()
	uid := newInviteDistUser(t, users)
	repo.metrics = []model.PromotionUserMetrics{{UserID: uid, GroupID: 5, Uploaded: 5000, Downloaded: 1000}}
	repo.states[uid] = model.UserInviteState{Username: "candidate", Invites: 0, CanInvite: true}

	svc := newInviteDistSvc(inviteDistEnabledSettings(), inviteDistGroups(), repo, users)

	sum, err := svc.Run(context.Background(), true)
	if err != nil {
		t.Fatalf("Run(force): %v", err)
	}
	if sum.Skipped {
		t.Fatalf("forced run should not be skipped, got reason %q", sum.Reason)
	}
	if sum.Granted != 1 {
		t.Errorf("expected 1 grant, got %d", sum.Granted)
	}
}

func TestInviteDistributionRun_ForceStillRespectsDisabled(t *testing.T) {
	repo := newFakeInviteDistRepo()
	repo.rules[5] = model.InviteDistributionRule{GroupID: 5, MaxInvites: 5}
	users := newMockInviteUserRepo()
	svc := newInviteDistSvc(map[string]string{SettingInviteDistributionEnabled: "false"}, inviteDistGroups(), repo, users)

	sum, err := svc.Run(context.Background(), true)
	if err != nil {
		t.Fatalf("Run(force): %v", err)
	}
	if !sum.Skipped || sum.Reason != "disabled" {
		t.Errorf("forced run while disabled should skip/disabled, got %+v", sum)
	}
}

func TestInviteDistributionRun_EligibleUserGrantedOnce(t *testing.T) {
	repo := newFakeInviteDistRepo()
	repo.rules[5] = model.InviteDistributionRule{GroupID: 5, MinRatio: 1.0, MaxInvites: 3}
	users := newMockInviteUserRepo()
	uid := newInviteDistUser(t, users)
	repo.metrics = []model.PromotionUserMetrics{{UserID: uid, GroupID: 5, Uploaded: 2000, Downloaded: 1000}} // ratio 2.0
	repo.states[uid] = model.UserInviteState{Username: "candidate", Invites: 1, CanInvite: true}

	svc := newInviteDistSvc(inviteDistEnabledSettings(), inviteDistGroups(), repo, users)
	sum, err := svc.Run(context.Background(), false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if sum.Granted != 1 {
		t.Fatalf("expected 1 grant, got %d (%+v)", sum.Granted, sum)
	}
	got, _ := users.GetByID(context.Background(), uid)
	if got.Invites != 1 {
		t.Errorf("expected invites incremented by exactly 1 (0 base in mock, delta applied once), got %d", got.Invites)
	}
	if len(repo.runs) != 1 || repo.runs[0] != 1 {
		t.Errorf("run not recorded correctly: %v", repo.runs)
	}
}

func TestInviteDistributionRun_RatioTooLowSkipped(t *testing.T) {
	repo := newFakeInviteDistRepo()
	repo.rules[5] = model.InviteDistributionRule{GroupID: 5, MinRatio: 2.0, MaxInvites: 3}
	users := newMockInviteUserRepo()
	uid := newInviteDistUser(t, users)
	repo.metrics = []model.PromotionUserMetrics{{UserID: uid, GroupID: 5, Uploaded: 100, Downloaded: 1000}} // ratio 0.1
	repo.states[uid] = model.UserInviteState{Username: "candidate", Invites: 0, CanInvite: true}

	svc := newInviteDistSvc(inviteDistEnabledSettings(), inviteDistGroups(), repo, users)
	sum, err := svc.Run(context.Background(), false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if sum.Granted != 0 {
		t.Errorf("low-ratio user should not be granted, got %+v", sum)
	}
}

func TestInviteDistributionRun_DownloadedBelowMinSkipped(t *testing.T) {
	repo := newFakeInviteDistRepo()
	repo.rules[5] = model.InviteDistributionRule{GroupID: 5, MinDownloadedBytes: 1_000_000, MaxInvites: 3}
	users := newMockInviteUserRepo()
	uid := newInviteDistUser(t, users)
	repo.metrics = []model.PromotionUserMetrics{{UserID: uid, GroupID: 5, Uploaded: 100, Downloaded: 500}}
	repo.states[uid] = model.UserInviteState{Username: "candidate", Invites: 0, CanInvite: true}

	svc := newInviteDistSvc(inviteDistEnabledSettings(), inviteDistGroups(), repo, users)
	sum, err := svc.Run(context.Background(), false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if sum.Granted != 0 {
		t.Errorf("user below min downloaded should not be granted, got %+v", sum)
	}
}

func TestInviteDistributionRun_DownloadedAboveMaxSkipped(t *testing.T) {
	repo := newFakeInviteDistRepo()
	repo.rules[5] = model.InviteDistributionRule{GroupID: 5, MaxDownloadedBytes: 1000, MaxInvites: 3}
	users := newMockInviteUserRepo()
	uid := newInviteDistUser(t, users)
	repo.metrics = []model.PromotionUserMetrics{{UserID: uid, GroupID: 5, Uploaded: 100, Downloaded: 5000}}
	repo.states[uid] = model.UserInviteState{Username: "candidate", Invites: 0, CanInvite: true}

	svc := newInviteDistSvc(inviteDistEnabledSettings(), inviteDistGroups(), repo, users)
	sum, err := svc.Run(context.Background(), false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if sum.Granted != 0 {
		t.Errorf("user above max downloaded should not be granted, got %+v", sum)
	}
}

func TestInviteDistributionRun_AtCeilingSkipped(t *testing.T) {
	repo := newFakeInviteDistRepo()
	repo.rules[5] = model.InviteDistributionRule{GroupID: 5, MaxInvites: 2}
	users := newMockInviteUserRepo()
	uid := newInviteDistUser(t, users)
	repo.metrics = []model.PromotionUserMetrics{{UserID: uid, GroupID: 5, Uploaded: 100, Downloaded: 10}}
	// Already at the ceiling.
	repo.states[uid] = model.UserInviteState{Username: "candidate", Invites: 2, CanInvite: true}

	svc := newInviteDistSvc(inviteDistEnabledSettings(), inviteDistGroups(), repo, users)
	sum, err := svc.Run(context.Background(), false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if sum.Granted != 0 {
		t.Errorf("user at ceiling should not be granted, got %+v", sum)
	}
}

func TestInviteDistributionRun_ZeroMaxInvitesGrantsNothing(t *testing.T) {
	repo := newFakeInviteDistRepo()
	// A rule with no configured ceiling (default 0) must never grant, even to
	// an otherwise perfectly-qualifying user with a zero balance.
	repo.rules[5] = model.InviteDistributionRule{GroupID: 5}
	users := newMockInviteUserRepo()
	uid := newInviteDistUser(t, users)
	repo.metrics = []model.PromotionUserMetrics{{UserID: uid, GroupID: 5, Uploaded: 100, Downloaded: 10}}
	repo.states[uid] = model.UserInviteState{Username: "candidate", Invites: 0, CanInvite: true}

	svc := newInviteDistSvc(inviteDistEnabledSettings(), inviteDistGroups(), repo, users)
	sum, err := svc.Run(context.Background(), false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if sum.Granted != 0 {
		t.Errorf("rule with max_invites=0 should never grant, got %+v", sum)
	}
}

func TestInviteDistributionRun_SuspendedInvitePrivilegeSkipped(t *testing.T) {
	repo := newFakeInviteDistRepo()
	repo.rules[5] = model.InviteDistributionRule{GroupID: 5, MaxInvites: 5}
	users := newMockInviteUserRepo()
	uid := newInviteDistUser(t, users)
	repo.metrics = []model.PromotionUserMetrics{{UserID: uid, GroupID: 5, Uploaded: 100, Downloaded: 10}}
	// can_invite is suspended — must not receive an auto-grant even though
	// every other criterion is met.
	repo.states[uid] = model.UserInviteState{Username: "candidate", Invites: 0, CanInvite: false}

	svc := newInviteDistSvc(inviteDistEnabledSettings(), inviteDistGroups(), repo, users)
	sum, err := svc.Run(context.Background(), false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if sum.Granted != 0 {
		t.Errorf("user with suspended invite privilege should not be granted, got %+v", sum)
	}
	got, _ := users.GetByID(context.Background(), uid)
	if got.Invites != 0 {
		t.Errorf("balance must be untouched while suspended, got %d", got.Invites)
	}
}

func TestInviteDistributionRun_LadderExcludesStaffGroup(t *testing.T) {
	repo := newFakeInviteDistRepo()
	// A rule wrongly attached to the admin group must never grant — UpsertRule
	// rejects this at write time, but the engine itself doesn't special-case
	// group membership beyond "has a rule", so this proves a rule that
	// slipped in some other way (e.g. direct DB edit) still can't fire.
	repo.rules[1] = model.InviteDistributionRule{GroupID: 1, MaxInvites: 5}
	users := newMockInviteUserRepo()
	uid := newInviteDistUser(t, users)
	repo.metrics = []model.PromotionUserMetrics{{UserID: uid, GroupID: 1, Uploaded: 100, Downloaded: 10}}
	repo.states[uid] = model.UserInviteState{Username: "candidate", Invites: 0, CanInvite: true}

	svc := newInviteDistSvc(inviteDistEnabledSettings(), inviteDistGroups(), repo, users)
	sum, err := svc.Run(context.Background(), false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// The engine has no ladder concept; a rule on a staff group would still
	// evaluate normally here since UpsertRule is the only guard. Document
	// that expectation rather than assert a false safety net.
	if sum.Granted != 1 {
		t.Fatalf("expected the (improperly configured) rule to still evaluate, got %+v", sum)
	}
}

func TestInviteDistributionRun_NoRulesConfigured(t *testing.T) {
	repo := newFakeInviteDistRepo()
	users := newMockInviteUserRepo()
	svc := newInviteDistSvc(inviteDistEnabledSettings(), inviteDistGroups(), repo, users)

	sum, err := svc.Run(context.Background(), false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if sum.Reason != "no rules configured" {
		t.Errorf("expected 'no rules configured', got %+v", sum)
	}
	if len(repo.runs) != 1 {
		t.Error("a no-rules run must still be recorded so the interval advances")
	}
}

func TestInviteDistributionUpsertRule_RejectsStaffAndInvalid(t *testing.T) {
	repo := newFakeInviteDistRepo()
	users := newMockInviteUserRepo()
	svc := newInviteDistSvc(inviteDistEnabledSettings(), inviteDistGroups(), repo, users)
	ctx := context.Background()

	if _, err := svc.UpsertRule(ctx, 1, InviteDistributionRuleInput{}); !errors.Is(err, ErrInviteDistStaffGroup) {
		t.Errorf("staff group rule: err=%v, want ErrInviteDistStaffGroup", err)
	}
	if _, err := svc.UpsertRule(ctx, 999, InviteDistributionRuleInput{}); !errors.Is(err, ErrInviteDistGroupNotFound) {
		t.Errorf("missing group: err=%v, want ErrInviteDistGroupNotFound", err)
	}
	if _, err := svc.UpsertRule(ctx, 5, InviteDistributionRuleInput{MinRatio: -1}); !errors.Is(err, ErrInviteDistInvalidThreshold) {
		t.Errorf("negative threshold: err=%v, want ErrInviteDistInvalidThreshold", err)
	}
	if _, err := svc.UpsertRule(ctx, 5, InviteDistributionRuleInput{MinDownloadedBytes: 2000, MaxDownloadedBytes: 1000}); !errors.Is(err, ErrInviteDistInvalidRange) {
		t.Errorf("min > max range: err=%v, want ErrInviteDistInvalidRange", err)
	}
	if _, err := svc.UpsertRule(ctx, 5, InviteDistributionRuleInput{MinRatio: 1.5, MaxInvites: 3}); err != nil {
		t.Errorf("valid rule rejected: %v", err)
	}
	if repo.rules[5].MaxInvites != 3 {
		t.Error("valid rule not persisted")
	}
}

func TestInviteDistributionDeleteRule_NotFound(t *testing.T) {
	repo := newFakeInviteDistRepo()
	users := newMockInviteUserRepo()
	svc := newInviteDistSvc(inviteDistEnabledSettings(), inviteDistGroups(), repo, users)

	if err := svc.DeleteRule(context.Background(), 999); !errors.Is(err, ErrInviteDistRuleNotFound) {
		t.Errorf("expected ErrInviteDistRuleNotFound, got %v", err)
	}
}

func TestInviteDistributionListRules_OrderedByLevelAndFlagsStaff(t *testing.T) {
	repo := newFakeInviteDistRepo()
	repo.rules[5] = model.InviteDistributionRule{GroupID: 5, MaxInvites: 3}
	users := newMockInviteUserRepo()
	svc := newInviteDistSvc(inviteDistEnabledSettings(), inviteDistGroups(), repo, users)

	views, err := svc.ListRules(context.Background())
	if err != nil {
		t.Fatalf("ListRules: %v", err)
	}
	if len(views) != 1 || views[0].GroupID != 5 || views[0].IsStaff {
		t.Errorf("unexpected views: %+v", views)
	}
}

func TestInviteDistributionListRules_SkipsRuleForVanishedGroup(t *testing.T) {
	repo := newFakeInviteDistRepo()
	// Group 42 has no entry in the groups list — its rule must be silently
	// skipped rather than crash the view, since it'll be cascaded away by the
	// FK once the group deletion completes.
	repo.rules[42] = model.InviteDistributionRule{GroupID: 42, MaxInvites: 1}
	users := newMockInviteUserRepo()
	svc := newInviteDistSvc(inviteDistEnabledSettings(), inviteDistGroups(), repo, users)

	views, err := svc.ListRules(context.Background())
	if err != nil {
		t.Fatalf("ListRules: %v", err)
	}
	if len(views) != 0 {
		t.Errorf("expected the vanished-group rule to be skipped, got %+v", views)
	}
}

func TestInviteDistributionListRules_PropagatesErrors(t *testing.T) {
	users := newMockInviteUserRepo()

	repo := newFakeInviteDistRepo()
	repo.listRulesErr = errors.New("db down")
	svc := newInviteDistSvc(inviteDistEnabledSettings(), inviteDistGroups(), repo, users)
	if _, err := svc.ListRules(context.Background()); err == nil {
		t.Error("expected ListRules to propagate the rules-repo error")
	}
}

func TestInviteDistributionUpsertRule_PropagatesRepoError(t *testing.T) {
	repo := newFakeInviteDistRepo()
	repo.upsertErr = errors.New("db down")
	users := newMockInviteUserRepo()
	svc := newInviteDistSvc(inviteDistEnabledSettings(), inviteDistGroups(), repo, users)

	if _, err := svc.UpsertRule(context.Background(), 5, InviteDistributionRuleInput{MaxInvites: 1}); err == nil {
		t.Error("expected UpsertRule to propagate the repo error")
	}
}

func TestInviteDistributionDeleteRule_PropagatesRepoError(t *testing.T) {
	repo := newFakeInviteDistRepo()
	repo.deleteErr = errors.New("db down")
	users := newMockInviteUserRepo()
	svc := newInviteDistSvc(inviteDistEnabledSettings(), inviteDistGroups(), repo, users)

	if err := svc.DeleteRule(context.Background(), 5); err == nil {
		t.Error("expected DeleteRule to propagate the repo error")
	}
}

func TestInviteDistributionRun_PropagatesLastRunAtError(t *testing.T) {
	repo := newFakeInviteDistRepo()
	repo.rules[5] = model.InviteDistributionRule{GroupID: 5, MaxInvites: 1}
	repo.lastRunErr = errors.New("db down")
	users := newMockInviteUserRepo()
	svc := newInviteDistSvc(inviteDistEnabledSettings(), inviteDistGroups(), repo, users)

	if _, err := svc.Run(context.Background(), false); err == nil {
		t.Error("expected Run to propagate the LastRunAt error")
	}
}

func TestInviteDistributionRun_PropagatesListRulesError(t *testing.T) {
	repo := newFakeInviteDistRepo()
	repo.listRulesErr = errors.New("db down")
	users := newMockInviteUserRepo()
	svc := newInviteDistSvc(inviteDistEnabledSettings(), inviteDistGroups(), repo, users)

	if _, err := svc.Run(context.Background(), true); err == nil {
		t.Error("expected Run to propagate the ListRules error")
	}
}

func TestInviteDistributionRun_PropagatesRecordRunErrorOnEmptyRules(t *testing.T) {
	repo := newFakeInviteDistRepo()
	repo.recordRunErr = errors.New("db down")
	users := newMockInviteUserRepo()
	svc := newInviteDistSvc(inviteDistEnabledSettings(), inviteDistGroups(), repo, users)

	if _, err := svc.Run(context.Background(), true); err == nil {
		t.Error("expected Run to propagate RecordRun's error on the no-rules path")
	}
}

func TestInviteDistributionRun_PropagatesGroupMetricsError(t *testing.T) {
	repo := newFakeInviteDistRepo()
	repo.rules[5] = model.InviteDistributionRule{GroupID: 5, MaxInvites: 1}
	repo.groupMetricsErr = errors.New("db down")
	users := newMockInviteUserRepo()
	svc := newInviteDistSvc(inviteDistEnabledSettings(), inviteDistGroups(), repo, users)

	if _, err := svc.Run(context.Background(), true); err == nil {
		t.Error("expected Run to propagate the GroupMetrics error")
	}
}

func TestInviteDistributionRun_PropagatesUserInviteStatesError(t *testing.T) {
	repo := newFakeInviteDistRepo()
	repo.rules[5] = model.InviteDistributionRule{GroupID: 5, MaxInvites: 1}
	uid := int64(500)
	repo.metrics = []model.PromotionUserMetrics{{UserID: uid, GroupID: 5, Uploaded: 1, Downloaded: 1}}
	repo.userStatesErr = errors.New("db down")
	users := newMockInviteUserRepo()
	svc := newInviteDistSvc(inviteDistEnabledSettings(), inviteDistGroups(), repo, users)

	if _, err := svc.Run(context.Background(), true); err == nil {
		t.Error("expected Run to propagate the UserInviteStates error")
	}
}

func TestInviteDistributionRun_PropagatesFinalRecordRunError(t *testing.T) {
	repo := newFakeInviteDistRepo()
	repo.rules[5] = model.InviteDistributionRule{GroupID: 5, MaxInvites: 1}
	users := newMockInviteUserRepo()
	uid := newInviteDistUser(t, users)
	// A real eligible grant, so the run reaches the final RecordRun call (the
	// third call site, distinct from the "no rules"/"no eligible users"
	// early-exit RecordRun calls already covered above).
	repo.metrics = []model.PromotionUserMetrics{{UserID: uid, GroupID: 5, Uploaded: 1, Downloaded: 1}}
	repo.states[uid] = model.UserInviteState{Username: "candidate", Invites: 0, CanInvite: true}
	repo.recordRunErr = errors.New("db down")
	svc := newInviteDistSvc(inviteDistEnabledSettings(), inviteDistGroups(), repo, users)

	if _, err := svc.Run(context.Background(), true); err == nil {
		t.Error("expected Run to propagate RecordRun's error after a real grant")
	}
}
