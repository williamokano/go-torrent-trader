package service

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// memberMockGroupRepo includes admin, moderator, and user groups for member tests.
type memberMockGroupRepo struct {
	groups []*model.Group
}

func newMemberMockGroupRepo() *memberMockGroupRepo {
	return &memberMockGroupRepo{
		groups: []*model.Group{
			{ID: 1, Name: "Administrator", Slug: "administrator", Level: 100, IsAdmin: true},
			{ID: 2, Name: "Moderator", Slug: "moderator", Level: 80, IsModerator: true},
			{ID: 5, Name: "User", Slug: "user", Level: 20},
		},
	}
}

func (m *memberMockGroupRepo) GetByID(_ context.Context, id int64) (*model.Group, error) {
	for _, g := range m.groups {
		if g.ID == id {
			return g, nil
		}
	}
	return nil, errors.New("group not found")
}

func (m *memberMockGroupRepo) List(_ context.Context) ([]model.Group, error) {
	var result []model.Group
	for _, g := range m.groups {
		result = append(result, *g)
	}
	return result, nil
}

// staffAwareMockUserRepo is a mock that filters by staff groups for ListStaff.
type staffAwareMockUserRepo struct {
	mockUserRepo
	staffGroupIDs map[int64]bool
}

func newStaffAwareMockUserRepo() *staffAwareMockUserRepo {
	return &staffAwareMockUserRepo{
		mockUserRepo:  mockUserRepo{nextID: 1},
		staffGroupIDs: map[int64]bool{1: true, 2: true}, // admin, moderator
	}
}

func (m *staffAwareMockUserRepo) ListStaff(_ context.Context) ([]model.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []model.User
	for _, u := range m.users {
		if m.staffGroupIDs[u.GroupID] {
			result = append(result, *u)
		}
	}
	return result, nil
}

func TestMemberService_ListMembers(t *testing.T) {
	userRepo := newStaffAwareMockUserRepo()
	groupRepo := newMemberMockGroupRepo()
	svc := NewMemberService(&userRepo.mockUserRepo, groupRepo)

	// Create users via auth service
	authSvc := NewAuthService(&userRepo.mockUserRepo, newTestSessionStore(), newTestPasswordResetStore(), &noopSender{}, "http://localhost:8080", event.NewInMemoryBus())
	_, _, _ = authSvc.Register(context.Background(), RegisterRequest{
		Username: "alice",
		Email:    "alice@example.com",
		Password: "password123",
	}, "127.0.0.1")
	_, _, _ = authSvc.Register(context.Background(), RegisterRequest{
		Username: "bob",
		Email:    "bob@example.com",
		Password: "password123",
	}, "127.0.0.1")

	views, total, err := svc.ListMembers(context.Background(), repository.ListUsersOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 2 {
		t.Errorf("expected 2 total, got %d", total)
	}
	if len(views) != 2 {
		t.Fatalf("expected 2 views, got %d", len(views))
	}
	if views[0].Username != "alice" {
		t.Errorf("expected alice, got %s", views[0].Username)
	}
	if views[0].CreatedAt == "" {
		t.Error("expected CreatedAt to be set")
	}
}

func TestMemberService_ListMembers_WithStats(t *testing.T) {
	userRepo := newStaffAwareMockUserRepo()
	groupRepo := newMemberMockGroupRepo()
	svc := NewMemberService(&userRepo.mockUserRepo, groupRepo)

	now := time.Now()
	userRepo.mu.Lock()
	userRepo.users = append(userRepo.users, &model.User{
		ID:         100,
		Username:   "uploader",
		GroupID:    5,
		Uploaded:   2000,
		Downloaded: 1000,
		Enabled:    true,
		CreatedAt:  now,
	})
	userRepo.mu.Unlock()

	views, _, err := svc.ListMembers(context.Background(), repository.ListUsersOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(views) != 1 {
		t.Fatalf("expected 1 view, got %d", len(views))
	}
	if views[0].Ratio != 2.0 {
		t.Errorf("expected ratio 2.0, got %f", views[0].Ratio)
	}
	if views[0].GroupName != "User" {
		t.Errorf("expected group name User, got %s", views[0].GroupName)
	}
}

func TestMemberService_ListStaff(t *testing.T) {
	userRepo := newStaffAwareMockUserRepo()
	groupRepo := newMemberMockGroupRepo()
	svc := NewMemberService(userRepo, groupRepo)

	adminTitle := "Head Admin"

	userRepo.mu.Lock()
	userRepo.users = append(userRepo.users,
		&model.User{ID: 1, Username: "admin1", GroupID: 1, Title: &adminTitle, CreatedAt: time.Now()},
		&model.User{ID: 2, Username: "mod1", GroupID: 2, CreatedAt: time.Now()},
		&model.User{ID: 3, Username: "regularuser", GroupID: 5, CreatedAt: time.Now()},
	)
	userRepo.mu.Unlock()

	staff, err := svc.ListStaff(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(staff) != 2 {
		t.Fatalf("expected 2 staff, got %d", len(staff))
	}
	if staff[0].Username != "admin1" {
		t.Errorf("expected admin1, got %s", staff[0].Username)
	}
	if staff[0].GroupName != "Administrator" {
		t.Errorf("expected Administrator, got %s", staff[0].GroupName)
	}
	if staff[0].Title == nil || *staff[0].Title != "Head Admin" {
		t.Errorf("expected title 'Head Admin', got %v", staff[0].Title)
	}
	if staff[1].Username != "mod1" {
		t.Errorf("expected mod1, got %s", staff[1].Username)
	}
	if staff[1].GroupName != "Moderator" {
		t.Errorf("expected Moderator, got %s", staff[1].GroupName)
	}
}

func TestMemberService_ListStaff_Empty(t *testing.T) {
	userRepo := newStaffAwareMockUserRepo()
	groupRepo := newMemberMockGroupRepo()
	svc := NewMemberService(userRepo, groupRepo)

	staff, err := svc.ListStaff(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(staff) != 0 {
		t.Errorf("expected 0 staff, got %d", len(staff))
	}
}

func TestMemberRatio(t *testing.T) {
	tests := []struct {
		name       string
		uploaded   int64
		downloaded int64
		expected   float64
	}{
		{"both zero", 0, 0, 0},
		{"uploaded only", 1000, 0, math.Inf(1)},
		{"equal", 1000, 1000, 1.0},
		{"2:1", 2000, 1000, 2.0},
		{"1:2", 500, 1000, 0.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := memberRatio(tt.uploaded, tt.downloaded)
			if math.IsInf(tt.expected, 1) {
				if !math.IsInf(got, 1) {
					t.Errorf("expected +Inf, got %f", got)
				}
			} else if got != tt.expected {
				t.Errorf("expected %f, got %f", tt.expected, got)
			}
		})
	}
}
