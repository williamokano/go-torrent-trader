package service

import (
	"context"
	"errors"
	"sync"
	"testing"

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

func TestRegister_Success(t *testing.T) {
	repo := newMockUserRepo()
	sessions := NewSessionStore()
	svc := NewAuthService(repo, sessions)

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
	if tokens.ExpiresIn != int64(AccessTokenTTL.Seconds()) {
		t.Errorf("expected expires_in=%d, got %d", int64(AccessTokenTTL.Seconds()), tokens.ExpiresIn)
	}
}

func TestRegister_FirstUserGetsAdmin(t *testing.T) {
	repo := newMockUserRepo()
	sessions := NewSessionStore()
	svc := NewAuthService(repo, sessions)

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
	sessions := NewSessionStore()
	svc := NewAuthService(repo, sessions)

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
	sessions := NewSessionStore()
	svc := NewAuthService(repo, sessions)

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
	sessions := NewSessionStore()
	svc := NewAuthService(repo, sessions)

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
	sessions := NewSessionStore()
	svc := NewAuthService(repo, sessions)

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
	sessions := NewSessionStore()
	svc := NewAuthService(repo, sessions)

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
	sessions := NewSessionStore()
	svc := NewAuthService(repo, sessions)

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
	sessions := NewSessionStore()
	svc := NewAuthService(repo, sessions)

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
	sessions := NewSessionStore()
	svc := NewAuthService(repo, sessions)

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
	sessions := NewSessionStore()
	svc := NewAuthService(repo, sessions)

	_, err := svc.Refresh(RefreshRequest{
		RefreshToken: "bogus",
	}, "127.0.0.1")

	if !errors.Is(err, ErrInvalidToken) {
		t.Errorf("expected ErrInvalidToken, got %v", err)
	}
}

func TestLogout(t *testing.T) {
	repo := newMockUserRepo()
	sessions := NewSessionStore()
	svc := NewAuthService(repo, sessions)

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
	sessions := NewSessionStore()
	svc := NewAuthService(repo, sessions)

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

func TestLogin_DisabledUser(t *testing.T) {
	repo := newMockUserRepo()
	sessions := NewSessionStore()
	svc := NewAuthService(repo, sessions)

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
