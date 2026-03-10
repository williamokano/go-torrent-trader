package service

import (
	"context"
	"errors"
	"fmt"
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

// mockModNoteRepo is an in-memory mod note repo for tests.
type mockModNoteRepo struct {
	notes  []*model.ModNote
	nextID int64
}

func newMockModNoteRepo() *mockModNoteRepo {
	return &mockModNoteRepo{nextID: 1}
}

func (m *mockModNoteRepo) Create(_ context.Context, note *model.ModNote) error {
	note.ID = m.nextID
	note.CreatedAt = time.Now()
	m.nextID++
	m.notes = append(m.notes, note)
	return nil
}

func (m *mockModNoteRepo) GetByID(_ context.Context, id int64) (*model.ModNote, error) {
	for _, n := range m.notes {
		if n.ID == id {
			return n, nil
		}
	}
	return nil, errors.New("not found")
}

func (m *mockModNoteRepo) ListByUser(_ context.Context, userID int64) ([]model.ModNote, error) {
	var result []model.ModNote
	for _, n := range m.notes {
		if n.UserID == userID {
			result = append(result, *n)
		}
	}
	return result, nil
}

func (m *mockModNoteRepo) Delete(_ context.Context, id int64) error {
	for i, n := range m.notes {
		if n.ID == id {
			m.notes = append(m.notes[:i], m.notes[i+1:]...)
			return nil
		}
	}
	return errors.New("not found")
}

func newAdminServiceWithDeps(t *testing.T) (*AdminService, *mockUserRepo, *mockAdminGroupRepo, *memorySessionStore, *noopSender) {
	t.Helper()
	userRepo := newMockUserRepo()
	groupRepo := newMockAdminGroupRepo()
	sessions := newTestSessionStore()
	emailSender := &noopSender{}
	bus := event.NewInMemoryBus()

	svc := NewAdminService(userRepo, groupRepo, bus)
	svc.SetSessionStore(sessions)
	svc.SetEmailSender(emailSender)

	return svc, userRepo, groupRepo, sessions, emailSender
}

func createTestUserForAdmin(t *testing.T, userRepo *mockUserRepo, groupID int64) *model.User {
	t.Helper()
	user := &model.User{
		Username:     fmt.Sprintf("user%d", time.Now().UnixNano()),
		Email:        fmt.Sprintf("user%d@test.com", time.Now().UnixNano()),
		PasswordHash: "$argon2id$v=19$m=65536,t=1,p=4$fake$fakehash",
		GroupID:      groupID,
		Enabled:      true,
		CreatedAt:    time.Now(),
	}
	if err := userRepo.Create(context.Background(), user); err != nil {
		t.Fatalf("create test user: %v", err)
	}
	return user
}

func TestAdminGetUserDetail(t *testing.T) {
	userRepo := newMockUserRepo()
	groupRepo := newMockAdminGroupRepo()
	modNoteRepo := newMockModNoteRepo()
	svc := NewAdminService(userRepo, groupRepo, event.NewInMemoryBus())
	svc.SetModNoteRepo(modNoteRepo)

	authSvc := NewAuthService(userRepo, newTestSessionStore(), newTestPasswordResetStore(), &noopSender{}, "http://localhost:8080", event.NewInMemoryBus())
	result, _ := authSvc.Register(context.Background(), RegisterRequest{
		Username: "detailuser",
		Email:    "detail@example.com",
		Password: "password123",
	}, "127.0.0.1")

	detail, err := svc.GetUserDetail(context.Background(), result.User.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if detail.Username != "detailuser" {
		t.Errorf("expected detailuser, got %s", detail.Username)
	}
	if len(detail.ModNotes) != 0 {
		t.Errorf("expected 0 mod notes, got %d", len(detail.ModNotes))
	}
	if len(detail.RecentUploads) != 0 {
		t.Errorf("expected 0 recent uploads, got %d", len(detail.RecentUploads))
	}
}

func TestAdminGetUserDetail_NotFound(t *testing.T) {
	userRepo := newMockUserRepo()
	groupRepo := newMockAdminGroupRepo()
	svc := NewAdminService(userRepo, groupRepo, event.NewInMemoryBus())

	_, err := svc.GetUserDetail(context.Background(), 999)
	if !errors.Is(err, ErrAdminUserNotFound) {
		t.Errorf("expected ErrAdminUserNotFound, got %v", err)
	}
}

func TestAdminResetPassword_AutoGenerate(t *testing.T) {
	svc, userRepo, _, sessions, emailSender := newAdminServiceWithDeps(t)

	// Create admin (group 1, level 100) and target user (group 5, level 20)
	admin := createTestUserForAdmin(t, userRepo, 1)
	target := createTestUserForAdmin(t, userRepo, 5)

	// Create a session for the target so we can verify it's deleted
	_ = sessions.Create(&Session{
		UserID:           target.ID,
		AccessToken:      "target-token",
		RefreshToken:     "target-refresh",
		ExpiresAt:        time.Now().Add(time.Hour),
		RefreshExpiresAt: time.Now().Add(24 * time.Hour),
	})

	newPass, err := svc.ResetPassword(context.Background(), admin.ID, target.ID, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(newPass) != 16 {
		t.Errorf("expected 16-char auto-generated password, got %d chars", len(newPass))
	}

	// Verify session was invalidated
	if sessions.GetByAccessToken("target-token") != nil {
		t.Error("expected target session to be deleted")
	}

	// Verify email was sent
	if emailSender.SendCount != 1 {
		t.Errorf("expected 1 email sent, got %d", emailSender.SendCount)
	}
	if emailSender.LastTo != target.Email {
		t.Errorf("expected email to %s, got %s", target.Email, emailSender.LastTo)
	}

	// Verify password actually works
	ok, err := VerifyPassword(newPass, target.PasswordHash)
	if err != nil {
		t.Fatalf("verify password: %v", err)
	}
	if !ok {
		t.Error("new password does not verify")
	}
}

func TestAdminResetPassword_WithPassword(t *testing.T) {
	svc, userRepo, _, _, _ := newAdminServiceWithDeps(t)

	admin := createTestUserForAdmin(t, userRepo, 1)
	target := createTestUserForAdmin(t, userRepo, 5)

	newPass, err := svc.ResetPassword(context.Background(), admin.ID, target.ID, "MyNewPassword123!")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if newPass != "MyNewPassword123!" {
		t.Errorf("expected provided password, got %s", newPass)
	}

	ok, err := VerifyPassword("MyNewPassword123!", target.PasswordHash)
	if err != nil {
		t.Fatalf("verify password: %v", err)
	}
	if !ok {
		t.Error("provided password does not verify")
	}
}

func TestAdminResetPassword_InsufficientLevel(t *testing.T) {
	svc, userRepo, _, _, _ := newAdminServiceWithDeps(t)

	// Both users in the same group (level 20)
	user1 := createTestUserForAdmin(t, userRepo, 5)
	user2 := createTestUserForAdmin(t, userRepo, 5)

	_, err := svc.ResetPassword(context.Background(), user1.ID, user2.ID, "newpass")
	if !errors.Is(err, ErrAdminInsufficientLevel) {
		t.Errorf("expected ErrAdminInsufficientLevel, got %v", err)
	}
}

func TestAdminResetPassword_UserNotFound(t *testing.T) {
	svc, userRepo, _, _, _ := newAdminServiceWithDeps(t)

	admin := createTestUserForAdmin(t, userRepo, 1)

	_, err := svc.ResetPassword(context.Background(), admin.ID, 9999, "newpass")
	if !errors.Is(err, ErrAdminUserNotFound) {
		t.Errorf("expected ErrAdminUserNotFound, got %v", err)
	}
}

func TestAdminResetPasskey(t *testing.T) {
	svc, userRepo, _, _, emailSender := newAdminServiceWithDeps(t)

	admin := createTestUserForAdmin(t, userRepo, 1)
	target := createTestUserForAdmin(t, userRepo, 5)

	newPasskey, err := svc.ResetPasskey(context.Background(), admin.ID, target.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(newPasskey) != 32 {
		t.Errorf("expected 32-char passkey, got %d chars", len(newPasskey))
	}

	// Verify passkey was updated on the user
	updated, _ := userRepo.GetByID(context.Background(), target.ID)
	if updated.Passkey == nil || *updated.Passkey != newPasskey {
		t.Error("passkey not updated on user")
	}

	// Verify email sent
	if emailSender.SendCount != 1 {
		t.Errorf("expected 1 email sent, got %d", emailSender.SendCount)
	}
}

func TestAdminResetPasskey_InsufficientLevel(t *testing.T) {
	svc, userRepo, _, _, _ := newAdminServiceWithDeps(t)

	user1 := createTestUserForAdmin(t, userRepo, 5)
	user2 := createTestUserForAdmin(t, userRepo, 5)

	_, err := svc.ResetPasskey(context.Background(), user1.ID, user2.ID)
	if !errors.Is(err, ErrAdminInsufficientLevel) {
		t.Errorf("expected ErrAdminInsufficientLevel, got %v", err)
	}
}

func TestAdminResetPassword_SameLevelAdmins(t *testing.T) {
	svc, userRepo, groupRepo, _, _ := newAdminServiceWithDeps(t)

	// Add a second admin group at the same level (100) as Administrator
	groupRepo.groups = append(groupRepo.groups, &model.Group{
		ID: 2, Name: "SysOp", Slug: "sysop", Level: 100, IsAdmin: true,
	})

	// Create two admins in different groups but same level
	admin1 := createTestUserForAdmin(t, userRepo, 1) // Administrator, level 100
	admin2 := createTestUserForAdmin(t, userRepo, 2) // SysOp, level 100

	// admin1 trying to reset admin2's password should fail (equal level)
	_, err := svc.ResetPassword(context.Background(), admin1.ID, admin2.ID, "newpass123")
	if !errors.Is(err, ErrAdminInsufficientLevel) {
		t.Errorf("expected ErrAdminInsufficientLevel when resetting same-level admin, got %v", err)
	}

	// admin2 trying to reset admin1's password should also fail
	_, err = svc.ResetPasskey(context.Background(), admin2.ID, admin1.ID)
	if !errors.Is(err, ErrAdminInsufficientLevel) {
		t.Errorf("expected ErrAdminInsufficientLevel when resetting same-level admin passkey, got %v", err)
	}
}

func TestAdminResetPassword_TooShort(t *testing.T) {
	svc, userRepo, _, _, _ := newAdminServiceWithDeps(t)

	admin := createTestUserForAdmin(t, userRepo, 1)
	target := createTestUserForAdmin(t, userRepo, 5)

	_, err := svc.ResetPassword(context.Background(), admin.ID, target.ID, "short")
	if !errors.Is(err, ErrAdminPasswordTooShort) {
		t.Errorf("expected ErrAdminPasswordTooShort, got %v", err)
	}
}

func TestAdminCreateModNote(t *testing.T) {
	userRepo := newMockUserRepo()
	groupRepo := newMockAdminGroupRepo()
	modNoteRepo := newMockModNoteRepo()
	svc := NewAdminService(userRepo, groupRepo, event.NewInMemoryBus())
	svc.SetModNoteRepo(modNoteRepo)

	authSvc := NewAuthService(userRepo, newTestSessionStore(), newTestPasswordResetStore(), &noopSender{}, "http://localhost:8080", event.NewInMemoryBus())
	result, _ := authSvc.Register(context.Background(), RegisterRequest{
		Username: "noteuser",
		Email:    "note@example.com",
		Password: "password123",
	}, "127.0.0.1")

	note, err := svc.CreateModNote(context.Background(), result.User.ID, result.User.ID, "This is a test note")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if note.Note != "This is a test note" {
		t.Errorf("expected note text, got %s", note.Note)
	}
	if note.ID == 0 {
		t.Error("expected non-zero note ID")
	}
}

func TestAdminCreateModNote_EmptyNote(t *testing.T) {
	userRepo := newMockUserRepo()
	groupRepo := newMockAdminGroupRepo()
	modNoteRepo := newMockModNoteRepo()
	svc := NewAdminService(userRepo, groupRepo, event.NewInMemoryBus())
	svc.SetModNoteRepo(modNoteRepo)

	authSvc := NewAuthService(userRepo, newTestSessionStore(), newTestPasswordResetStore(), &noopSender{}, "http://localhost:8080", event.NewInMemoryBus())
	result, _ := authSvc.Register(context.Background(), RegisterRequest{
		Username: "emptynote",
		Email:    "emptynote@example.com",
		Password: "password123",
	}, "127.0.0.1")

	_, err := svc.CreateModNote(context.Background(), result.User.ID, result.User.ID, "")
	if !errors.Is(err, ErrInvalidModNote) {
		t.Errorf("expected ErrInvalidModNote, got %v", err)
	}
}

func TestAdminDeleteModNote(t *testing.T) {
	userRepo := newMockUserRepo()
	groupRepo := newMockAdminGroupRepo()
	modNoteRepo := newMockModNoteRepo()
	svc := NewAdminService(userRepo, groupRepo, event.NewInMemoryBus())
	svc.SetModNoteRepo(modNoteRepo)

	authSvc := NewAuthService(userRepo, newTestSessionStore(), newTestPasswordResetStore(), &noopSender{}, "http://localhost:8080", event.NewInMemoryBus())
	result, _ := authSvc.Register(context.Background(), RegisterRequest{
		Username: "delnote",
		Email:    "delnote@example.com",
		Password: "password123",
	}, "127.0.0.1")

	adminPerms := model.Permissions{IsAdmin: true}
	note, _ := svc.CreateModNote(context.Background(), result.User.ID, result.User.ID, "delete me")

	// Author can delete their own note
	err := svc.DeleteModNote(context.Background(), note.ID, result.User.ID, model.Permissions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Deleting again should fail
	err = svc.DeleteModNote(context.Background(), note.ID, result.User.ID, adminPerms)
	if !errors.Is(err, ErrModNoteNotFound) {
		t.Errorf("expected ErrModNoteNotFound, got %v", err)
	}
}

func TestAdminDeleteModNote_ForbiddenForNonAuthor(t *testing.T) {
	userRepo := newMockUserRepo()
	groupRepo := newMockAdminGroupRepo()
	modNoteRepo := newMockModNoteRepo()
	svc := NewAdminService(userRepo, groupRepo, event.NewInMemoryBus())
	svc.SetModNoteRepo(modNoteRepo)

	authSvc := NewAuthService(userRepo, newTestSessionStore(), newTestPasswordResetStore(), &noopSender{}, "http://localhost:8080", event.NewInMemoryBus())
	author, _ := authSvc.Register(context.Background(), RegisterRequest{
		Username: "noteauthor",
		Email:    "noteauthor@example.com",
		Password: "password123",
	}, "127.0.0.1")

	note, _ := svc.CreateModNote(context.Background(), author.User.ID, author.User.ID, "private note")

	// Non-author moderator (not admin) should be forbidden
	otherModID := int64(9999)
	modPerms := model.Permissions{IsModerator: true}
	err := svc.DeleteModNote(context.Background(), note.ID, otherModID, modPerms)
	if !errors.Is(err, ErrModNoteDeleteForbidden) {
		t.Errorf("expected ErrModNoteDeleteForbidden, got %v", err)
	}

	// Admin can delete anyone's note
	adminPerms := model.Permissions{IsAdmin: true}
	err = svc.DeleteModNote(context.Background(), note.ID, otherModID, adminPerms)
	if err != nil {
		t.Fatalf("expected admin to delete note, got %v", err)
	}
}

func TestAdminCreateModNote_UserNotFound(t *testing.T) {
	userRepo := newMockUserRepo()
	groupRepo := newMockAdminGroupRepo()
	modNoteRepo := newMockModNoteRepo()
	svc := NewAdminService(userRepo, groupRepo, event.NewInMemoryBus())
	svc.SetModNoteRepo(modNoteRepo)

	_, err := svc.CreateModNote(context.Background(), 999, 1, "test")
	if !errors.Is(err, ErrAdminUserNotFound) {
		t.Errorf("expected ErrAdminUserNotFound, got %v", err)
	}
}

func TestAdminCreateModNote_TooLong(t *testing.T) {
	userRepo := newMockUserRepo()
	groupRepo := newMockAdminGroupRepo()
	modNoteRepo := newMockModNoteRepo()
	svc := NewAdminService(userRepo, groupRepo, event.NewInMemoryBus())
	svc.SetModNoteRepo(modNoteRepo)

	authSvc := NewAuthService(userRepo, newTestSessionStore(), newTestPasswordResetStore(), &noopSender{}, "http://localhost:8080", event.NewInMemoryBus())
	result, _ := authSvc.Register(context.Background(), RegisterRequest{
		Username: "longnote",
		Email:    "longnote@example.com",
		Password: "password123",
	}, "127.0.0.1")

	// Create a string longer than 10000 chars
	buf := make([]byte, 10001)
	for i := range buf {
		buf[i] = 'a'
	}

	_, err := svc.CreateModNote(context.Background(), result.User.ID, result.User.ID, string(buf))
	if !errors.Is(err, ErrInvalidModNote) {
		t.Errorf("expected ErrInvalidModNote, got %v", err)
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
