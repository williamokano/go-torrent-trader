package service

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

)

func TestGetProfile_PublicView(t *testing.T) {
	repo := newMockUserRepo()
	sessions := newTestSessionStore()
	svc := NewUserService(repo, sessions)

	// Create a user via auth service to get proper hashing etc.
	authSvc := NewAuthService(repo, sessions, newTestPasswordResetStore(), &noopSender{}, "http://localhost:8080")
	user, _, _ := authSvc.Register(context.Background(), RegisterRequest{
		Username: "profileuser",
		Email:    "profile@example.com",
		Password: "password123",
	}, "127.0.0.1")

	// View as different user
	profile, err := svc.GetProfile(context.Background(), user.ID, 999)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pub, ok := profile.(PublicProfile)
	if !ok {
		t.Fatal("expected PublicProfile for non-owner view")
	}
	if pub.Username != "profileuser" {
		t.Errorf("expected username profileuser, got %s", pub.Username)
	}
	if pub.ID != user.ID {
		t.Errorf("expected id %d, got %d", user.ID, pub.ID)
	}
}

func TestGetProfile_OwnerView(t *testing.T) {
	repo := newMockUserRepo()
	sessions := newTestSessionStore()
	svc := NewUserService(repo, sessions)

	authSvc := NewAuthService(repo, sessions, newTestPasswordResetStore(), &noopSender{}, "http://localhost:8080")
	user, _, _ := authSvc.Register(context.Background(), RegisterRequest{
		Username: "owneruser",
		Email:    "owner@example.com",
		Password: "password123",
	}, "127.0.0.1")

	profile, err := svc.GetProfile(context.Background(), user.ID, user.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	op, ok := profile.(*OwnerProfile)
	if !ok {
		t.Fatal("expected OwnerProfile for owner view")
	}
	if op.Email != "owner@example.com" {
		t.Errorf("expected email owner@example.com, got %s", op.Email)
	}
	// Owner should see full passkey
	if op.Passkey != *user.Passkey {
		t.Errorf("expected full passkey %q, got %q", *user.Passkey, op.Passkey)
	}
}

func TestGetProfile_NotFound(t *testing.T) {
	repo := newMockUserRepo()
	sessions := newTestSessionStore()
	svc := NewUserService(repo, sessions)

	_, err := svc.GetProfile(context.Background(), 999, 1)
	if !errors.Is(err, ErrUserNotFound) {
		t.Errorf("expected ErrUserNotFound, got %v", err)
	}
}

func TestGetFullProfile(t *testing.T) {
	repo := newMockUserRepo()
	sessions := newTestSessionStore()
	svc := NewUserService(repo, sessions)

	authSvc := NewAuthService(repo, sessions, newTestPasswordResetStore(), &noopSender{}, "http://localhost:8080")
	user, _, _ := authSvc.Register(context.Background(), RegisterRequest{
		Username: "fulluser",
		Email:    "full@example.com",
		Password: "password123",
	}, "127.0.0.1")

	profile, err := svc.GetFullProfile(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if profile.Email != "full@example.com" {
		t.Errorf("expected email full@example.com, got %s", profile.Email)
	}
	if profile.Username != "fulluser" {
		t.Errorf("expected username fulluser, got %s", profile.Username)
	}
}

func TestUpdateProfile_Success(t *testing.T) {
	repo := newMockUserRepo()
	sessions := newTestSessionStore()
	svc := NewUserService(repo, sessions)

	authSvc := NewAuthService(repo, sessions, newTestPasswordResetStore(), &noopSender{}, "http://localhost:8080")
	user, _, _ := authSvc.Register(context.Background(), RegisterRequest{
		Username: "updateuser",
		Email:    "update@example.com",
		Password: "password123",
	}, "127.0.0.1")

	avatar := "https://example.com/avatar.png"
	title := "My Title"
	info := "My bio text"

	profile, err := svc.UpdateProfile(context.Background(), user.ID, UpdateProfileRequest{
		Avatar: &avatar,
		Title:  &title,
		Info:   &info,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if profile.Avatar == nil || *profile.Avatar != avatar {
		t.Errorf("expected avatar %s, got %v", avatar, profile.Avatar)
	}
	if profile.Title == nil || *profile.Title != title {
		t.Errorf("expected title %s, got %v", title, profile.Title)
	}
	if profile.Info == nil || *profile.Info != info {
		t.Errorf("expected info %s, got %v", info, profile.Info)
	}
}

func TestUpdateProfile_PartialUpdate(t *testing.T) {
	repo := newMockUserRepo()
	sessions := newTestSessionStore()
	svc := NewUserService(repo, sessions)

	authSvc := NewAuthService(repo, sessions, newTestPasswordResetStore(), &noopSender{}, "http://localhost:8080")
	user, _, _ := authSvc.Register(context.Background(), RegisterRequest{
		Username: "partial",
		Email:    "partial@example.com",
		Password: "password123",
	}, "127.0.0.1")

	// Set initial title
	title := "Initial"
	_, _ = svc.UpdateProfile(context.Background(), user.ID, UpdateProfileRequest{Title: &title})

	// Only update info, title should remain
	info := "New bio"
	profile, err := svc.UpdateProfile(context.Background(), user.ID, UpdateProfileRequest{Info: &info})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if profile.Title == nil || *profile.Title != "Initial" {
		t.Errorf("expected title to remain 'Initial', got %v", profile.Title)
	}
	if profile.Info == nil || *profile.Info != "New bio" {
		t.Errorf("expected info 'New bio', got %v", profile.Info)
	}
}

func TestUpdateProfile_InvalidAvatarURL(t *testing.T) {
	repo := newMockUserRepo()
	sessions := newTestSessionStore()
	svc := NewUserService(repo, sessions)

	authSvc := NewAuthService(repo, sessions, newTestPasswordResetStore(), &noopSender{}, "http://localhost:8080")
	user, _, _ := authSvc.Register(context.Background(), RegisterRequest{
		Username: "badavatar",
		Email:    "badavatar@example.com",
		Password: "password123",
	}, "127.0.0.1")

	badURL := "not-a-url"
	_, err := svc.UpdateProfile(context.Background(), user.ID, UpdateProfileRequest{Avatar: &badURL})
	if !errors.Is(err, ErrValidationFailed) {
		t.Errorf("expected ErrValidationFailed, got %v", err)
	}
}

func TestUpdateProfile_TitleTooLong(t *testing.T) {
	repo := newMockUserRepo()
	sessions := newTestSessionStore()
	svc := NewUserService(repo, sessions)

	authSvc := NewAuthService(repo, sessions, newTestPasswordResetStore(), &noopSender{}, "http://localhost:8080")
	user, _, _ := authSvc.Register(context.Background(), RegisterRequest{
		Username: "longtitle",
		Email:    "longtitle@example.com",
		Password: "password123",
	}, "127.0.0.1")

	longTitle := string(make([]byte, 101))
	_, err := svc.UpdateProfile(context.Background(), user.ID, UpdateProfileRequest{Title: &longTitle})
	if !errors.Is(err, ErrValidationFailed) {
		t.Errorf("expected ErrValidationFailed, got %v", err)
	}
}

func TestUpdateProfile_InfoTooLong(t *testing.T) {
	repo := newMockUserRepo()
	sessions := newTestSessionStore()
	svc := NewUserService(repo, sessions)

	authSvc := NewAuthService(repo, sessions, newTestPasswordResetStore(), &noopSender{}, "http://localhost:8080")
	user, _, _ := authSvc.Register(context.Background(), RegisterRequest{
		Username: "longinfo",
		Email:    "longinfo@example.com",
		Password: "password123",
	}, "127.0.0.1")

	longInfo := string(make([]byte, 5001))
	_, err := svc.UpdateProfile(context.Background(), user.ID, UpdateProfileRequest{Info: &longInfo})
	if !errors.Is(err, ErrValidationFailed) {
		t.Errorf("expected ErrValidationFailed, got %v", err)
	}
}

func TestChangePassword_Success(t *testing.T) {
	repo := newMockUserRepo()
	sessions := newTestSessionStore()
	svc := NewUserService(repo, sessions)

	authSvc := NewAuthService(repo, sessions, newTestPasswordResetStore(), &noopSender{}, "http://localhost:8080")
	user, tokens, _ := authSvc.Register(context.Background(), RegisterRequest{
		Username: "changepw",
		Email:    "changepw@example.com",
		Password: "oldpassword1",
	}, "127.0.0.1")

	// Create a second session
	_, tokens2, _ := authSvc.Login(context.Background(), LoginRequest{
		Username: "changepw",
		Password: "oldpassword1",
	}, "127.0.0.1")

	err := svc.ChangePassword(context.Background(), user.ID, tokens.AccessToken, ChangePasswordRequest{
		CurrentPassword: "oldpassword1",
		NewPassword:     "newpassword1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Current session should still work
	if sessions.GetByAccessToken(tokens.AccessToken) == nil {
		t.Error("current session should be kept")
	}

	// Other session should be invalidated
	if sessions.GetByAccessToken(tokens2.AccessToken) != nil {
		t.Error("other session should be invalidated")
	}

	// New password should work
	_, _, err = authSvc.Login(context.Background(), LoginRequest{
		Username: "changepw",
		Password: "newpassword1",
	}, "127.0.0.1")
	if err != nil {
		t.Fatalf("should be able to login with new password: %v", err)
	}

	// Old password should fail
	_, _, err = authSvc.Login(context.Background(), LoginRequest{
		Username: "changepw",
		Password: "oldpassword1",
	}, "127.0.0.1")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Error("old password should not work")
	}
}

func TestChangePassword_WrongCurrentPassword(t *testing.T) {
	repo := newMockUserRepo()
	sessions := newTestSessionStore()
	svc := NewUserService(repo, sessions)

	authSvc := NewAuthService(repo, sessions, newTestPasswordResetStore(), &noopSender{}, "http://localhost:8080")
	user, tokens, _ := authSvc.Register(context.Background(), RegisterRequest{
		Username: "wrongcurr",
		Email:    "wrongcurr@example.com",
		Password: "password123",
	}, "127.0.0.1")

	err := svc.ChangePassword(context.Background(), user.ID, tokens.AccessToken, ChangePasswordRequest{
		CurrentPassword: "wrongpassword",
		NewPassword:     "newpassword1",
	})
	if !errors.Is(err, ErrIncorrectPassword) {
		t.Errorf("expected ErrIncorrectPassword, got %v", err)
	}
}

func TestChangePassword_WeakNewPassword(t *testing.T) {
	repo := newMockUserRepo()
	sessions := newTestSessionStore()
	svc := NewUserService(repo, sessions)

	authSvc := NewAuthService(repo, sessions, newTestPasswordResetStore(), &noopSender{}, "http://localhost:8080")
	user, tokens, _ := authSvc.Register(context.Background(), RegisterRequest{
		Username: "weaknew",
		Email:    "weaknew@example.com",
		Password: "password123",
	}, "127.0.0.1")

	err := svc.ChangePassword(context.Background(), user.ID, tokens.AccessToken, ChangePasswordRequest{
		CurrentPassword: "password123",
		NewPassword:     "short",
	})
	if !errors.Is(err, ErrValidationFailed) {
		t.Errorf("expected ErrValidationFailed, got %v", err)
	}
}

func TestRegeneratePasskey_Success(t *testing.T) {
	repo := newMockUserRepo()
	sessions := newTestSessionStore()
	svc := NewUserService(repo, sessions)

	authSvc := NewAuthService(repo, sessions, newTestPasswordResetStore(), &noopSender{}, "http://localhost:8080")
	user, _, _ := authSvc.Register(context.Background(), RegisterRequest{
		Username: "passkey",
		Email:    "passkey@example.com",
		Password: "password123",
	}, "127.0.0.1")

	oldPasskey := *user.Passkey

	newPasskey, err := svc.RegeneratePasskey(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(newPasskey) != 32 {
		t.Errorf("expected 32-char passkey, got %d chars", len(newPasskey))
	}
	if newPasskey == oldPasskey {
		t.Error("new passkey should differ from old one")
	}

	// Verify it was persisted
	updated, _ := repo.GetByID(context.Background(), user.ID)
	if updated.Passkey == nil || *updated.Passkey != newPasskey {
		t.Error("passkey should be updated in repository")
	}
}

func TestRegeneratePasskey_UserNotFound(t *testing.T) {
	repo := newMockUserRepo()
	sessions := newTestSessionStore()
	svc := NewUserService(repo, sessions)

	_, err := svc.RegeneratePasskey(context.Background(), 999)
	if !errors.Is(err, ErrUserNotFound) {
		t.Errorf("expected ErrUserNotFound, got %v", err)
	}
}

func TestCalculateRatio(t *testing.T) {
	tests := []struct {
		name       string
		uploaded   int64
		downloaded int64
		expected   float64
	}{
		{"both zero", 0, 0, 0},
		{"uploaded only", 1000, 0, math.Inf(1)},
		{"equal", 1000, 1000, 1.0},
		{"more uploaded", 2000, 1000, 2.0},
		{"less uploaded", 500, 1000, 0.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateRatio(tt.uploaded, tt.downloaded)
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

func TestDerefString(t *testing.T) {
	if got := derefString(nil); got != "" {
		t.Errorf("expected empty for nil, got %q", got)
	}
	s := "hello"
	if got := derefString(&s); got != "hello" {
		t.Errorf("expected %q, got %q", "hello", got)
	}
}

func TestDeleteByUserIDExcept(t *testing.T) {
	sessions := newTestSessionStore()

	future := time.Now().Add(1 * time.Hour)

	// Create two sessions for user 1
	_ = sessions.Create(&Session{UserID: 1, AccessToken: "keep", RefreshToken: "r1", ExpiresAt: future, RefreshExpiresAt: future})
	_ = sessions.Create(&Session{UserID: 1, AccessToken: "delete", RefreshToken: "r2", ExpiresAt: future, RefreshExpiresAt: future})
	// Create a session for user 2
	_ = sessions.Create(&Session{UserID: 2, AccessToken: "other", RefreshToken: "r3", ExpiresAt: future, RefreshExpiresAt: future})

	sessions.DeleteByUserIDExcept(1, "keep")

	if sessions.GetByAccessToken("keep") == nil {
		t.Error("kept session should still exist")
	}
	if sessions.GetByAccessToken("delete") != nil {
		t.Error("other user 1 session should be deleted")
	}
	if sessions.GetByAccessToken("other") == nil {
		t.Error("user 2 session should not be affected")
	}
}