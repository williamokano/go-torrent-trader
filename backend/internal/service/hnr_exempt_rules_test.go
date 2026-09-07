package service

import (
	"context"
	"errors"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

func setupHnRServiceWithSettings(s *SiteSettingsService) (*HnRService, *fakeHnRRepo) {
	repo := newFakeHnRRepo()
	users := newMockUserRepoForRestrictions()
	bus := event.NewInMemoryBus()
	warnings := NewWarningService(newMockWarningRepo(), users, newMockMessageRepoForWarnings(), bus)
	restrictions := NewRestrictionService(newMockRestrictionRepo(), users, bus)
	svc := NewHnRService(nil, repo, &fakeHnRGroupRepo{groups: hnrTestGroups()}, users, warnings, restrictions, s, bus)
	return svc, repo
}

func TestHnRService_ExemptRule_Validation(t *testing.T) {
	svc, _ := setupHnRService()
	ctx := context.Background()

	for _, in := range []HnRExemptRuleInput{
		{Criterion: "nonsense", Threshold: 1, Enabled: true},
		{Criterion: model.HnRExemptCriterionMinSeeders, Threshold: -1, Enabled: true},
		// min_seeders 0 would match every torrent (seeders >= 0) and turn HnR
		// off site-wide — rejected.
		{Criterion: model.HnRExemptCriterionMinSeeders, Threshold: 0, Enabled: true},
	} {
		if _, err := svc.CreateExemptRule(ctx, in); !errors.Is(err, ErrHnRInvalidExemptRule) {
			t.Errorf("Create %+v: got %v, want ErrHnRInvalidExemptRule", in, err)
		}
	}

	// max_size_bytes 0 is fine — no torrent is 0 bytes, so it simply matches
	// nothing.
	if _, err := svc.CreateExemptRule(ctx, HnRExemptRuleInput{
		Criterion: model.HnRExemptCriterionMaxSize, Threshold: 0, Enabled: true,
	}); err != nil {
		t.Errorf("max_size_bytes threshold 0 should be accepted, got %v", err)
	}

	if _, err := svc.UpdateExemptRule(ctx, 999, HnRExemptRuleInput{
		Criterion: model.HnRExemptCriterionMaxSize, Threshold: 1, Enabled: true,
	}); !errors.Is(err, ErrHnRExemptRuleNotFound) {
		t.Errorf("Update missing: got %v, want ErrHnRExemptRuleNotFound", err)
	}
	if err := svc.DeleteExemptRule(ctx, 999); !errors.Is(err, ErrHnRExemptRuleNotFound) {
		t.Errorf("Delete missing: got %v, want ErrHnRExemptRuleNotFound", err)
	}
}

func TestHnRService_ApplyExemptRules_GatedOnMasterSwitch(t *testing.T) {
	ctx := context.Background()
	minSeeders := HnRExemptRuleInput{Criterion: model.HnRExemptCriterionMinSeeders, Threshold: 50, Enabled: true}

	// HnR off → the pass is a no-op even with a matching rule.
	svcOff, repoOff := setupHnRServiceWithSettings(settingsWith(map[string]string{SettingHnREnabled: "false"}))
	repoOff.torrentSize[1] = 1024
	repoOff.torrentSeeders[1] = 100
	if _, err := svcOff.CreateExemptRule(ctx, minSeeders); err != nil {
		t.Fatal(err)
	}
	if ex, rel, err := svcOff.applyExemptRules(ctx, 0); err != nil || ex != 0 || rel != 0 {
		t.Fatalf("HnR off: expected no-op, got exempted=%d released=%d", ex, rel)
	}
	if repoOff.torrentExempt[1] {
		t.Error("HnR off: torrent 1 should not have been auto-exempted")
	}

	// HnR on → the pass flags the matching torrent as auto-exempt.
	svcOn, repoOn := setupHnRServiceWithSettings(settingsWith(map[string]string{SettingHnREnabled: "true"}))
	repoOn.torrentSize[1] = 1024
	repoOn.torrentSeeders[1] = 100
	if _, err := svcOn.CreateExemptRule(ctx, minSeeders); err != nil {
		t.Fatal(err)
	}
	if ex, rel, err := svcOn.applyExemptRules(ctx, 0); err != nil || ex != 1 || rel != 0 {
		t.Fatalf("HnR on: expected exempted=1 released=0, got %d/%d", ex, rel)
	}
	if !repoOn.torrentExempt[1] || repoOn.torrentExemptSource[1] != model.HnRExemptSourceAuto {
		t.Errorf("HnR on: torrent 1 exempt=%v source=%q, want true/auto",
			repoOn.torrentExempt[1], repoOn.torrentExemptSource[1])
	}
}
