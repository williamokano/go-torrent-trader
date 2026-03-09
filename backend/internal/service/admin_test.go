package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// mockAdminGroupRepo is a simple in-memory group repo for admin tests.
type mockAdminGroupRepo struct {
	groups []*model.Group
}

func newMockAdminGroupRepo() *mockAdminGroupRepo {
	return &mockAdminGroupRepo{
		groups: []*model.Group{
			{ID: 1, Name: "Administrator", Slug: "administrator", Level: 100, IsAdmin: true},
			{ID: 5, Name: "User", Slug: "user", Level: 20},
		},
	}
}

func (m *mockAdminGroupRepo) GetByID(_ context.Context, id int64) (*model.Group, error) {
	for _, g := range m.groups {
		if g.ID == id {
			return g, nil
		}
	}
	return nil, errors.New("group not found")
}

func (m *mockAdminGroupRepo) List(_ context.Context) ([]model.Group, error) {
	var result []model.Group
	for _, g := range m.groups {
		result = append(result, *g)
	}
	return result, nil
}

func TestAdminListUsers(t *testing.T) {
	userRepo := newMockUserRepo()
	groupRepo := newMockAdminGroupRepo()
	svc := NewAdminService(userRepo, groupRepo, event.NewInMemoryBus())

	// Create some users via auth
	authSvc := NewAuthService(userRepo, newTestSessionStore(), newTestPasswordResetStore(), &noopSender{}, "http://localhost:8080", event.NewInMemoryBus())
	_, _ = authSvc.Register(context.Background(), RegisterRequest{
		Username: "alice",
		Email:    "alice@example.com",
		Password: "password123",
	}, "127.0.0.1")
	_, _ = authSvc.Register(context.Background(), RegisterRequest{
		Username: "bob",
		Email:    "bob@example.com",
		Password: "password123",
	}, "127.0.0.1")

	views, total, err := svc.ListUsers(context.Background(), repository.ListUsersOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 2 {
		t.Errorf("expected 2 users, got %d", total)
	}
	if len(views) != 2 {
		t.Fatalf("expected 2 views, got %d", len(views))
	}
	if views[0].Username != "alice" {
		t.Errorf("expected alice, got %s", views[0].Username)
	}
}

func TestAdminUpdateUser_ChangeGroup(t *testing.T) {
	userRepo := newMockUserRepo()
	groupRepo := newMockAdminGroupRepo()
	svc := NewAdminService(userRepo, groupRepo, event.NewInMemoryBus())

	authSvc := NewAuthService(userRepo, newTestSessionStore(), newTestPasswordResetStore(), &noopSender{}, "http://localhost:8080", event.NewInMemoryBus())
	result, _ := authSvc.Register(context.Background(), RegisterRequest{
		Username: "changeme",
		Email:    "changeme@example.com",
		Password: "password123",
	}, "127.0.0.1")

	newGroupID := int64(1)
	view, err := svc.UpdateUser(context.Background(), 99, result.User.ID, AdminUpdateUserRequest{
		GroupID: &newGroupID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if view.GroupID != 1 {
		t.Errorf("expected group_id 1, got %d", view.GroupID)
	}
	if view.GroupName != "Administrator" {
		t.Errorf("expected Administrator, got %s", view.GroupName)
	}
}

func TestAdminUpdateUser_InvalidGroup(t *testing.T) {
	userRepo := newMockUserRepo()
	groupRepo := newMockAdminGroupRepo()
	svc := NewAdminService(userRepo, groupRepo, event.NewInMemoryBus())

	authSvc := NewAuthService(userRepo, newTestSessionStore(), newTestPasswordResetStore(), &noopSender{}, "http://localhost:8080", event.NewInMemoryBus())
	result, _ := authSvc.Register(context.Background(), RegisterRequest{
		Username: "invalidgrp",
		Email:    "invalidgrp@example.com",
		Password: "password123",
	}, "127.0.0.1")

	badGroupID := int64(999)
	_, err := svc.UpdateUser(context.Background(), 99, result.User.ID, AdminUpdateUserRequest{
		GroupID: &badGroupID,
	})
	if !errors.Is(err, ErrAdminGroupNotFound) {
		t.Errorf("expected ErrAdminGroupNotFound, got %v", err)
	}
}

func TestAdminUpdateUser_NotFound(t *testing.T) {
	userRepo := newMockUserRepo()
	groupRepo := newMockAdminGroupRepo()
	svc := NewAdminService(userRepo, groupRepo, event.NewInMemoryBus())

	_, err := svc.UpdateUser(context.Background(), 99, 999, AdminUpdateUserRequest{})
	if !errors.Is(err, ErrAdminUserNotFound) {
		t.Errorf("expected ErrAdminUserNotFound, got %v", err)
	}
}

func TestAdminUpdateUser_ToggleEnabled(t *testing.T) {
	userRepo := newMockUserRepo()
	groupRepo := newMockAdminGroupRepo()
	svc := NewAdminService(userRepo, groupRepo, event.NewInMemoryBus())

	authSvc := NewAuthService(userRepo, newTestSessionStore(), newTestPasswordResetStore(), &noopSender{}, "http://localhost:8080", event.NewInMemoryBus())
	result, _ := authSvc.Register(context.Background(), RegisterRequest{
		Username: "disableme",
		Email:    "disableme@example.com",
		Password: "password123",
	}, "127.0.0.1")

	disabled := false
	view, err := svc.UpdateUser(context.Background(), 99, result.User.ID, AdminUpdateUserRequest{
		Enabled: &disabled,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if view.Enabled {
		t.Error("expected user to be disabled")
	}
}

func TestAdminListGroups(t *testing.T) {
	userRepo := newMockUserRepo()
	groupRepo := newMockAdminGroupRepo()
	svc := NewAdminService(userRepo, groupRepo, event.NewInMemoryBus())

	groups, err := svc.ListGroups(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(groups) != 2 {
		t.Errorf("expected 2 groups, got %d", len(groups))
	}
}

func TestAdminListUsers_WithLastAccess(t *testing.T) {
	userRepo := newMockUserRepo()
	groupRepo := newMockAdminGroupRepo()
	svc := NewAdminService(userRepo, groupRepo, event.NewInMemoryBus())

	now := time.Now()
	userRepo.mu.Lock()
	userRepo.users = append(userRepo.users, &model.User{
		ID:         100,
		Username:   "active",
		Email:      "active@test.com",
		GroupID:    5,
		Enabled:    true,
		LastAccess: &now,
		CreatedAt:  now,
	})
	userRepo.mu.Unlock()

	views, _, err := svc.ListUsers(context.Background(), repository.ListUsersOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(views) != 1 {
		t.Fatalf("expected 1 view, got %d", len(views))
	}
	if views[0].LastAccess == nil {
		t.Error("expected LastAccess to be set")
	}
}
