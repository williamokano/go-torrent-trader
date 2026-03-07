package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

// mockUserRepo is an in-memory user repository for testing.
type mockUserRepo struct {
	mu    sync.Mutex
	users []*model.User
	nextID int64
}

func newMockUserRepo() *mockUserRepo {
	return &mockUserRepo{nextID: 1}
}

func (m *mockUserRepo) GetByID(_ context.Context, id int64) (*model.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.users {
		if u.ID == id {
			return u, nil
		}
	}
	return nil, errors.New("not found")
}

func (m *mockUserRepo) GetByUsername(_ context.Context, username string) (*model.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.users {
		if u.Username == username {
			return u, nil
		}
	}
	return nil, errors.New("not found")
}

func (m *mockUserRepo) GetByEmail(_ context.Context, email string) (*model.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.users {
		if u.Email == email {
			return u, nil
		}
	}
	return nil, errors.New("not found")
}

func (m *mockUserRepo) GetByPasskey(_ context.Context, _ string) (*model.User, error) {
	return nil, errors.New("not found")
}

func (m *mockUserRepo) Count(_ context.Context) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return int64(len(m.users)), nil
}

func (m *mockUserRepo) Create(_ context.Context, user *model.User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	user.ID = m.nextID
	m.nextID++
	m.users = append(m.users, user)
	return nil
}

func (m *mockUserRepo) Update(_ context.Context, user *model.User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, u := range m.users {
		if u.ID == user.ID {
			m.users[i] = user
			return nil
		}
	}
	return errors.New("not found")
}

func (m *mockUserRepo) IncrementStats(_ context.Context, id int64, uploadedDelta, downloadedDelta int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.users {
		if u.ID == id {
			u.Uploaded += uploadedDelta
			u.Downloaded += downloadedDelta
			return nil
		}
	}
	return errors.New("not found")
}

func TestRegister_Success(t *testing.T) {
	repo := newMockUserRepo()
	sessions := NewMemorySessionStore()
	svc := NewAuthService(repo, sessions, NewMemoryPasswordResetStore(), &NoopSender{}, "http://localhost:8080")

	user, tokens, err := svc.Register(context.Background(), RegisterRequest{
		Username: "testuser",
		Email:    "test@example.com",
		Password: "password123",
	}, "127.0.0.1")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.Username != "testuser" {
		t.Errorf("expected username testuser, got %s", user.Username)
	}
	if user.PasswordScheme != "argon2id" {
		t.Errorf("expected argon2id scheme, got %s", user.PasswordScheme)
	}
	if tokens.AccessToken == "" || tokens.RefreshToken == "" {
		t.Error("expected non-empty tokens")
	}
	if tokens.ExpiresIn != int64(DefaultAccessTokenTTL.Seconds()) {
		t.Errorf("expected expires_in=%d, got %d", int64(DefaultAccessTokenTTL.Seconds()), tokens.ExpiresIn)
	}
}

func TestRegister_FirstUserGetsAdmin(t *testing.T) {
	repo := newMockUserRepo()
	sessions := NewMemorySessionStore()
	svc := NewAuthService(repo, sessions, NewMemoryPasswordResetStore(), &NoopSender{}, "http://localhost:8080")

	user, _, err := svc.Register(context.Background(), RegisterRequest{
		Username: "admin",
		Email:    "admin@example.com",
		Password: "password123",
	}, "127.0.0.1")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.GroupID != 1 {
		t.Errorf("first user should get admin group (1), got %d", user.GroupID)
	}
}

func TestRegister_SecondUserGetsDefaultGroup(t *testing.T) {
	repo := newMockUserRepo()
	sessions := NewMemorySessionStore()
	svc := NewAuthService(repo, sessions, NewMemoryPasswordResetStore(), &NoopSender{}, "http://localhost:8080")

	// Register first user
	_, _, _ = svc.Register(context.Background(), RegisterRequest{
		Username: "admin",
		Email:    "admin@example.com",
		Password: "password123",
	}, "127.0.0.1")

	// Register second user
	user, _, err := svc.Register(context.Background(), RegisterRequest{
		Username: "normaluser",
		Email:    "normal@example.com",
		Password: "password123",
	}, "127.0.0.1")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.GroupID != 5 {
		t.Errorf("second user should get default group (5), got %d", user.GroupID)
	}
}

func TestRegister_DuplicateUsername(t *testing.T) {
	repo := newMockUserRepo()
	sessions := NewMemorySessionStore()
	svc := NewAuthService(repo, sessions, NewMemoryPasswordResetStore(), &NoopSender{}, "http://localhost:8080")

	_, _, _ = svc.Register(context.Background(), RegisterRequest{
		Username: "dupe",
		Email:    "dupe1@example.com",
		Password: "password123",
	}, "127.0.0.1")

	_, _, err := svc.Register(context.Background(), RegisterRequest{
		Username: "dupe",
		Email:    "dupe2@example.com",
		Password: "password123",
	}, "127.0.0.1")

	if !errors.Is(err, ErrUsernameTaken) {
		t.Errorf("expected ErrUsernameTaken, got %v", err)
	}
}

func TestRegister_DuplicateEmail(t *testing.T) {
	repo := newMockUserRepo()
	sessions := NewMemorySessionStore()
	svc := NewAuthService(repo, sessions, NewMemoryPasswordResetStore(), &NoopSender{}, "http://localhost:8080")

	_, _, _ = svc.Register(context.Background(), RegisterRequest{
		Username: "user1",
		Email:    "same@example.com",
		Password: "password123",
	}, "127.0.0.1")

	_, _, err := svc.Register(context.Background(), RegisterRequest{
		Username: "user2",
		Email:    "same@example.com",
		Password: "password123",
	}, "127.0.0.1")

	if !errors.Is(err, ErrEmailTaken) {
		t.Errorf("expected ErrEmailTaken, got %v", err)
	}
}

func TestRegister_ValidationErrors(t *testing.T) {
	repo := newMockUserRepo()
	sessions := NewMemorySessionStore()
	svc := NewAuthService(repo, sessions, NewMemoryPasswordResetStore(), &NoopSender{}, "http://localhost:8080")

	tests := []struct {
		name string
		req  RegisterRequest
	}{
		{"short username", RegisterRequest{Username: "ab", Email: "a@b.com", Password: "password123"}},
		{"long username", RegisterRequest{Username: "abcdefghijklmnopqrstu", Email: "a@b.com", Password: "password123"}},
		{"invalid chars", RegisterRequest{Username: "test user", Email: "a@b.com", Password: "password123"}},
		{"bad email", RegisterRequest{Username: "testuser", Email: "not-email", Password: "password123"}},
		{"short password", RegisterRequest{Username: "testuser", Email: "a@b.com", Password: "short"}},
		{"long password", RegisterRequest{Username: "testuser", Email: "a@b.com", Password: string(make([]byte, 1025))}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := svc.Register(context.Background(), tt.req, "127.0.0.1")
			if !errors.Is(err, ErrValidationFailed) {
				t.Errorf("expected ErrValidationFailed, got %v", err)
			}
		})
	}
}

func TestLogin_Success(t *testing.T) {
	repo := newMockUserRepo()
	sessions := NewMemorySessionStore()
	svc := NewAuthService(repo, sessions, NewMemoryPasswordResetStore(), &NoopSender{}, "http://localhost:8080")

	_, _, _ = svc.Register(context.Background(), RegisterRequest{
		Username: "loginuser",
		Email:    "login@example.com",
		Password: "password123",
	}, "127.0.0.1")

	user, tokens, err := svc.Login(context.Background(), LoginRequest{
		Username: "loginuser",
		Password: "password123",
	}, "127.0.0.1")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.Username != "loginuser" {
		t.Errorf("expected username loginuser, got %s", user.Username)
	}
	if tokens.AccessToken == "" {
		t.Error("expected non-empty access token")
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	repo := newMockUserRepo()
	sessions := NewMemorySessionStore()
	svc := NewAuthService(repo, sessions, NewMemoryPasswordResetStore(), &NoopSender{}, "http://localhost:8080")

	_, _, _ = svc.Register(context.Background(), RegisterRequest{
		Username: "loginuser",
		Email:    "login@example.com",
		Password: "password123",
	}, "127.0.0.1")

	_, _, err := svc.Login(context.Background(), LoginRequest{
		Username: "loginuser",
		Password: "wrongpassword",
	}, "127.0.0.1")

	if !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestLogin_NonexistentUser(t *testing.T) {
	repo := newMockUserRepo()
	sessions := NewMemorySessionStore()
	svc := NewAuthService(repo, sessions, NewMemoryPasswordResetStore(), &NoopSender{}, "http://localhost:8080")

	_, _, err := svc.Login(context.Background(), LoginRequest{
		Username: "ghost",
		Password: "password123",
	}, "127.0.0.1")

	if !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestRefresh_Success(t *testing.T) {
	repo := newMockUserRepo()
	sessions := NewMemorySessionStore()
	svc := NewAuthService(repo, sessions, NewMemoryPasswordResetStore(), &NoopSender{}, "http://localhost:8080")

	_, tokens, _ := svc.Register(context.Background(), RegisterRequest{
		Username: "refreshuser",
		Email:    "refresh@example.com",
		Password: "password123",
	}, "127.0.0.1")

	newTokens, err := svc.Refresh(RefreshRequest{
		RefreshToken: tokens.RefreshToken,
	}, "127.0.0.1")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if newTokens.AccessToken == tokens.AccessToken {
		t.Error("new access token should differ from old one")
	}
	if newTokens.RefreshToken == tokens.RefreshToken {
		t.Error("new refresh token should differ from old one")
	}

	// Old tokens should be invalid
	if sessions.GetByAccessToken(tokens.AccessToken) != nil {
		t.Error("old access token should be invalidated")
	}
}

func TestRefresh_InvalidToken(t *testing.T) {
	repo := newMockUserRepo()
	sessions := NewMemorySessionStore()
	svc := NewAuthService(repo, sessions, NewMemoryPasswordResetStore(), &NoopSender{}, "http://localhost:8080")

	_, err := svc.Refresh(RefreshRequest{
		RefreshToken: "bogus",
	}, "127.0.0.1")

	if !errors.Is(err, ErrInvalidToken) {
		t.Errorf("expected ErrInvalidToken, got %v", err)
	}
}

func TestLogout(t *testing.T) {
	repo := newMockUserRepo()
	sessions := NewMemorySessionStore()
	svc := NewAuthService(repo, sessions, NewMemoryPasswordResetStore(), &NoopSender{}, "http://localhost:8080")

	_, tokens, _ := svc.Register(context.Background(), RegisterRequest{
		Username: "logoutuser",
		Email:    "logout@example.com",
		Password: "password123",
	}, "127.0.0.1")

	svc.Logout(tokens.AccessToken)

	if sessions.GetByAccessToken(tokens.AccessToken) != nil {
		t.Error("session should be invalidated after logout")
	}
}

func TestGetCurrentUser(t *testing.T) {
	repo := newMockUserRepo()
	sessions := NewMemorySessionStore()
	svc := NewAuthService(repo, sessions, NewMemoryPasswordResetStore(), &NoopSender{}, "http://localhost:8080")

	registered, _, _ := svc.Register(context.Background(), RegisterRequest{
		Username: "meuser",
		Email:    "me@example.com",
		Password: "password123",
	}, "127.0.0.1")

	user, err := svc.GetCurrentUser(context.Background(), registered.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.Username != "meuser" {
		t.Errorf("expected meuser, got %s", user.Username)
	}
}

func TestForgotPassword_GeneratesToken(t *testing.T) {
	repo := newMockUserRepo()
	sessions := NewMemorySessionStore()
	svc := NewAuthService(repo, sessions, NewMemoryPasswordResetStore(), &NoopSender{}, "http://localhost:8080")
	store := NewMemoryPasswordResetStore()
	svc.SetPasswordResetStore(store)

	// Register a user
	_, _, _ = svc.Register(context.Background(), RegisterRequest{
		Username: "resetuser",
		Email:    "reset@example.com",
		Password: "password123",
	}, "127.0.0.1")

	err := svc.ForgotPassword(context.Background(), ForgotPasswordRequest{
		Email: "reset@example.com",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify a reset token was created
	resets := store.Resets()
	if len(resets) != 1 {
		t.Fatalf("expected 1 reset token, got %d", len(resets))
	}
	if resets[0].Used {
		t.Error("reset token should not be marked as used")
	}
	if resets[0].TokenHash == "" {
		t.Error("reset token hash should not be empty")
	}
}

func TestForgotPassword_NonexistentEmail_NoError(t *testing.T) {
	repo := newMockUserRepo()
	sessions := NewMemorySessionStore()
	svc := NewAuthService(repo, sessions, NewMemoryPasswordResetStore(), &NoopSender{}, "http://localhost:8080")
	store := NewMemoryPasswordResetStore()
	svc.SetPasswordResetStore(store)

	err := svc.ForgotPassword(context.Background(), ForgotPasswordRequest{
		Email: "nonexistent@example.com",
	})
	if err != nil {
		t.Fatalf("should not return error for nonexistent email: %v", err)
	}

	// No token should be created
	resets := store.Resets()
	if len(resets) != 0 {
		t.Errorf("expected 0 reset tokens, got %d", len(resets))
	}
}

func TestForgotPassword_RateLimit(t *testing.T) {
	repo := newMockUserRepo()
	sessions := NewMemorySessionStore()
	svc := NewAuthService(repo, sessions, NewMemoryPasswordResetStore(), &NoopSender{}, "http://localhost:8080")
	store := NewMemoryPasswordResetStore()
	svc.SetPasswordResetStore(store)

	_, _, _ = svc.Register(context.Background(), RegisterRequest{
		Username: "ratelimit",
		Email:    "ratelimit@example.com",
		Password: "password123",
	}, "127.0.0.1")

	// Send 3 requests (the limit)
	for i := 0; i < 3; i++ {
		_ = svc.ForgotPassword(context.Background(), ForgotPasswordRequest{
			Email: "ratelimit@example.com",
		})
	}

	resets := store.Resets()
	if len(resets) != 3 {
		t.Fatalf("expected 3 reset tokens, got %d", len(resets))
	}

	// 4th request should be silently ignored
	_ = svc.ForgotPassword(context.Background(), ForgotPasswordRequest{
		Email: "ratelimit@example.com",
	})

	resets = store.Resets()
	if len(resets) != 3 {
		t.Errorf("expected still 3 reset tokens after rate limit, got %d", len(resets))
	}
}

func TestResetPassword_Success(t *testing.T) {
	repo := newMockUserRepo()
	sessions := NewMemorySessionStore()
	svc := NewAuthService(repo, sessions, NewMemoryPasswordResetStore(), &NoopSender{}, "http://localhost:8080")
	store := NewMemoryPasswordResetStore()
	svc.SetPasswordResetStore(store)

	// Register and login to create a session
	_, tokens, _ := svc.Register(context.Background(), RegisterRequest{
		Username: "resetpw",
		Email:    "resetpw@example.com",
		Password: "oldpassword1",
	}, "127.0.0.1")

	// Verify the session exists
	if sessions.GetByAccessToken(tokens.AccessToken) == nil {
		t.Fatal("session should exist before reset")
	}

	// Request forgot password
	_ = svc.ForgotPassword(context.Background(), ForgotPasswordRequest{
		Email: "resetpw@example.com",
	})

	// Get the raw token by working backwards from the stored hash
	// We need to capture the token from the service — let's generate one manually
	rawToken, _ := GenerateToken()
	tokenHash := hashTokenForTest(rawToken)
	now := time.Now()
	// Clear the store and add our known token
	store.ClearResets()
	_ = store.Create(&PasswordReset{
		UserID:    1, // first user
		TokenHash: tokenHash,
		ExpiresAt: now.Add(1 * time.Hour),
		Used:      false,
		CreatedAt: now,
	})

	err := svc.ResetPassword(context.Background(), ResetPasswordRequest{
		Token:    rawToken,
		Password: "newpassword1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Token should be marked used
	resets := store.Resets()
	if !resets[0].Used {
		t.Error("reset token should be marked as used")
	}

	// Old session should be invalidated
	if sessions.GetByAccessToken(tokens.AccessToken) != nil {
		t.Error("old session should be invalidated after password reset")
	}

	// Should be able to login with new password
	_, _, err = svc.Login(context.Background(), LoginRequest{
		Username: "resetpw",
		Password: "newpassword1",
	}, "127.0.0.1")
	if err != nil {
		t.Fatalf("should be able to login with new password: %v", err)
	}

	// Old password should not work
	_, _, err = svc.Login(context.Background(), LoginRequest{
		Username: "resetpw",
		Password: "oldpassword1",
	}, "127.0.0.1")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Error("old password should not work after reset")
	}
}

func TestResetPassword_InvalidToken(t *testing.T) {
	repo := newMockUserRepo()
	sessions := NewMemorySessionStore()
	svc := NewAuthService(repo, sessions, NewMemoryPasswordResetStore(), &NoopSender{}, "http://localhost:8080")

	err := svc.ResetPassword(context.Background(), ResetPasswordRequest{
		Token:    "bogustoken",
		Password: "newpassword1",
	})
	if !errors.Is(err, ErrInvalidResetToken) {
		t.Errorf("expected ErrInvalidResetToken, got %v", err)
	}
}

func TestResetPassword_ExpiredToken(t *testing.T) {
	repo := newMockUserRepo()
	sessions := NewMemorySessionStore()
	svc := NewAuthService(repo, sessions, NewMemoryPasswordResetStore(), &NoopSender{}, "http://localhost:8080")
	store := NewMemoryPasswordResetStore()
	svc.SetPasswordResetStore(store)

	_, _, _ = svc.Register(context.Background(), RegisterRequest{
		Username: "expired",
		Email:    "expired@example.com",
		Password: "password123",
	}, "127.0.0.1")

	rawToken, _ := GenerateToken()
	tokenHash := hashTokenForTest(rawToken)
	_ = store.Create(&PasswordReset{
		UserID:    1,
		TokenHash: tokenHash,
		ExpiresAt: time.Now().Add(-1 * time.Hour), // expired
		Used:      false,
		CreatedAt: time.Now().Add(-2 * time.Hour),
	})

	err := svc.ResetPassword(context.Background(), ResetPasswordRequest{
		Token:    rawToken,
		Password: "newpassword1",
	})
	if !errors.Is(err, ErrInvalidResetToken) {
		t.Errorf("expected ErrInvalidResetToken for expired token, got %v", err)
	}
}

func TestResetPassword_UsedToken(t *testing.T) {
	repo := newMockUserRepo()
	sessions := NewMemorySessionStore()
	svc := NewAuthService(repo, sessions, NewMemoryPasswordResetStore(), &NoopSender{}, "http://localhost:8080")
	store := NewMemoryPasswordResetStore()
	svc.SetPasswordResetStore(store)

	_, _, _ = svc.Register(context.Background(), RegisterRequest{
		Username: "usedtoken",
		Email:    "usedtoken@example.com",
		Password: "password123",
	}, "127.0.0.1")

	rawToken, _ := GenerateToken()
	tokenHash := hashTokenForTest(rawToken)
	_ = store.Create(&PasswordReset{
		UserID:    1,
		TokenHash: tokenHash,
		ExpiresAt: time.Now().Add(1 * time.Hour),
		Used:      true, // already used
		CreatedAt: time.Now(),
	})

	err := svc.ResetPassword(context.Background(), ResetPasswordRequest{
		Token:    rawToken,
		Password: "newpassword1",
	})
	if !errors.Is(err, ErrInvalidResetToken) {
		t.Errorf("expected ErrInvalidResetToken for used token, got %v", err)
	}
}

func TestResetPassword_WeakPassword(t *testing.T) {
	repo := newMockUserRepo()
	sessions := NewMemorySessionStore()
	svc := NewAuthService(repo, sessions, NewMemoryPasswordResetStore(), &NoopSender{}, "http://localhost:8080")

	err := svc.ResetPassword(context.Background(), ResetPasswordRequest{
		Token:    "sometoken",
		Password: "short",
	})
	if !errors.Is(err, ErrValidationFailed) {
		t.Errorf("expected ErrValidationFailed for short password, got %v", err)
	}
}

// hashTokenForTest wraps the package-private hashToken for test readability.
func hashTokenForTest(token string) string {
	return hashToken(token)
}

func TestLogin_DisabledUser(t *testing.T) {
	repo := newMockUserRepo()
	sessions := NewMemorySessionStore()
	svc := NewAuthService(repo, sessions, NewMemoryPasswordResetStore(), &NoopSender{}, "http://localhost:8080")

	_, _, _ = svc.Register(context.Background(), RegisterRequest{
		Username: "disabled",
		Email:    "disabled@example.com",
		Password: "password123",
	}, "127.0.0.1")

	// Disable the user
	repo.mu.Lock()
	for _, u := range repo.users {
		if u.Username == "disabled" {
			u.Enabled = false
		}
	}
	repo.mu.Unlock()

	_, _, err := svc.Login(context.Background(), LoginRequest{
		Username: "disabled",
		Password: "password123",
	}, "127.0.0.1")

	if !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("expected ErrInvalidCredentials for disabled user, got %v", err)
	}
}
