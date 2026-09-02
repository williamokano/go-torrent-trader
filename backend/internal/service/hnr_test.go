package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"

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
	svc := NewHnRService(repo, &fakeHnRGroupRepo{groups: hnrTestGroups()}, nil)
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
