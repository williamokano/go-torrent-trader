package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// historyRecorder is implemented by fakeEditHistoryRepo (admin_edit_history_test.go)
// so mockUserRepo and fakeBonusRepo can push audit entries directly into it —
// mirroring how, in production, UserRepo.UpdateWithHistory/SetStats/SetInvites
// and BonusRepo.SetPoints each insert into the same user_edit_history table
// within their own transaction, independent of UserEditHistoryRepo.Record.
type historyRecorder interface {
	push(entries []model.UserEditHistory)
}

// mockUserRepo is an in-memory user repository for testing.
type mockUserRepo struct {
	mu          sync.Mutex
	users       []*model.User
	nextID      int64
	historySink historyRecorder // optional; entries are discarded if nil
	// updateErr/statsErr/invitesErr independently fail UpdateWithHistory/
	// SetStats/SetInvites without mutating state or pushing history —
	// simulating that one method's write+audit transaction failing. Kept as
	// three separate fields (not one shared error) so a test can prove a
	// later stage failing does not touch what an earlier stage already
	// committed, matching how these are genuinely independent transactions
	// against real Postgres.
	updateErr  error
	statsErr   error
	invitesErr error
}

func newMockUserRepo() *mockUserRepo {
	return &mockUserRepo{nextID: 1}
}

func (m *mockUserRepo) push(entries []model.UserEditHistory) {
	if m.historySink != nil {
		m.historySink.push(entries)
	}
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

func (m *mockUserRepo) GetByUsernames(_ context.Context, usernames []string) ([]model.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	want := make(map[string]bool, len(usernames))
	for _, name := range usernames {
		want[name] = true
	}
	var found []model.User
	for _, u := range m.users {
		if want[u.Username] {
			found = append(found, *u)
		}
	}
	return found, nil
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

func (m *mockUserRepo) List(_ context.Context, opts repository.ListUsersOptions) ([]model.User, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []model.User
	for _, u := range m.users {
		if opts.Enabled != nil && u.Enabled != *opts.Enabled {
			continue
		}
		if opts.DisabledUntilBefore != nil {
			if u.DisabledUntil == nil || !u.DisabledUntil.Before(*opts.DisabledUntilBefore) {
				continue
			}
		}
		result = append(result, *u)
	}
	return result, int64(len(result)), nil
}

func (m *mockUserRepo) UpdateLastAccess(_ context.Context, _ int64) error { return nil }

func (m *mockUserRepo) SetInvites(_ context.Context, userID int64, invites int, entries []model.UserEditHistory) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.invitesErr != nil {
		return m.invitesErr
	}
	for _, u := range m.users {
		if u.ID == userID {
			u.Invites = invites
			m.push(entries)
			return nil
		}
	}
	return errors.New("not found")
}

func (m *mockUserRepo) SetStats(_ context.Context, userID int64, uploaded, downloaded *int64, entries []model.UserEditHistory) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.statsErr != nil {
		return m.statsErr
	}
	for _, u := range m.users {
		if u.ID == userID {
			if uploaded != nil {
				u.Uploaded = *uploaded
			}
			if downloaded != nil {
				u.Downloaded = *downloaded
			}
			m.push(entries)
			return nil
		}
	}
	return errors.New("not found")
}

func (m *mockUserRepo) UpdateWithHistory(_ context.Context, user *model.User, entries []model.UserEditHistory) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.updateErr != nil {
		return m.updateErr
	}
	for i, u := range m.users {
		if u.ID == user.ID {
			m.users[i] = user
			m.push(entries)
			return nil
		}
	}
	return errors.New("not found")
}

func (m *mockUserRepo) ListStaff(_ context.Context) ([]model.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	// In the real implementation, this filters by group is_admin/is_moderator.
	// For tests, we return all users (tests control what's seeded).
	var result []model.User
	for _, u := range m.users {
		result = append(result, *u)
	}
	return result, nil
}

func TestRegister_Success(t *testing.T) {
	repo := newMockUserRepo()
	sessions := newTestSessionStore()
	svc := NewAuthService(repo, sessions, newTestPasswordResetStore(), &noopSender{}, "http://localhost:8080", event.NewInMemoryBus())

	result, err := svc.Register(context.Background(), RegisterRequest{
		Username: "testuser",
		Email:    "test@example.com",
		Password: "password123",
	}, "127.0.0.1")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.User.Username != "testuser" {
		t.Errorf("expected username testuser, got %s", result.User.Username)
	}
	if result.User.PasswordScheme != "argon2id" {
		t.Errorf("expected argon2id scheme, got %s", result.User.PasswordScheme)
	}
	if result.Tokens.AccessToken == "" || result.Tokens.RefreshToken == "" {
		t.Error("expected non-empty tokens")
	}
	if result.Tokens.ExpiresIn != int64(DefaultAccessTokenTTL.Seconds()) {
		t.Errorf("expected expires_in=%d, got %d", int64(DefaultAccessTokenTTL.Seconds()), result.Tokens.ExpiresIn)
	}
}

// Create writes the struct rather than relying on column defaults, so a
// privilege left unset at registration is inserted as false. A new member would
// silently have no live feeds.
func TestRegister_GrantsFeedAccess(t *testing.T) {
	repo := newMockUserRepo()
	sessions := newTestSessionStore()
	svc := NewAuthService(repo, sessions, newTestPasswordResetStore(), &noopSender{}, "http://localhost:8080", event.NewInMemoryBus())

	result, err := svc.Register(context.Background(), RegisterRequest{
		Username: "testuser",
		Email:    "test@example.com",
		Password: "password123",
	}, "127.0.0.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.User.CanFeed {
		t.Error("a new member must be able to watch the live feeds")
	}
}

// TestRegister_SetsActivatedAtWhenNoConfirmationRequired is part of the
// regression coverage for BE-8.19: when no email confirmation is required,
// the account is usable the instant it's created, so ActivatedAt must be
// stamped at registration — not left to a login or some later request that
// may never come (e.g. the account is banned before either happens).
func TestRegister_SetsActivatedAtWhenNoConfirmationRequired(t *testing.T) {
	repo := newMockUserRepo()
	sessions := newTestSessionStore()
	svc := NewAuthService(repo, sessions, newTestPasswordResetStore(), &noopSender{}, "http://localhost:8080", event.NewInMemoryBus())

	result, err := svc.Register(context.Background(), RegisterRequest{
		Username: "activateduser",
		Email:    "activated@example.com",
		Password: "password123",
	}, "127.0.0.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.User.ActivatedAt == nil {
		t.Error("ActivatedAt must be set at registration when no email confirmation is required")
	}
}

func TestRegister_FirstUserGetsAdmin(t *testing.T) {
	repo := newMockUserRepo()
	sessions := newTestSessionStore()
	svc := NewAuthService(repo, sessions, newTestPasswordResetStore(), &noopSender{}, "http://localhost:8080", event.NewInMemoryBus())

	result, err := svc.Register(context.Background(), RegisterRequest{
		Username: "admin",
		Email:    "admin@example.com",
		Password: "password123",
	}, "127.0.0.1")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.User.GroupID != 1 {
		t.Errorf("first user should get admin group (1), got %d", result.User.GroupID)
	}
}

func TestRegister_SecondUserGetsDefaultGroup(t *testing.T) {
	repo := newMockUserRepo()
	sessions := newTestSessionStore()
	svc := NewAuthService(repo, sessions, newTestPasswordResetStore(), &noopSender{}, "http://localhost:8080", event.NewInMemoryBus())

	// Register first user
	_, _ = svc.Register(context.Background(), RegisterRequest{
		Username: "admin",
		Email:    "admin@example.com",
		Password: "password123",
	}, "127.0.0.1")

	// Register second user
	result, err := svc.Register(context.Background(), RegisterRequest{
		Username: "normaluser",
		Email:    "normal@example.com",
		Password: "password123",
	}, "127.0.0.1")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.User.GroupID != 5 {
		t.Errorf("second user should get default group (5), got %d", result.User.GroupID)
	}
}

func TestRegister_DuplicateUsername(t *testing.T) {
	repo := newMockUserRepo()
	sessions := newTestSessionStore()
	svc := NewAuthService(repo, sessions, newTestPasswordResetStore(), &noopSender{}, "http://localhost:8080", event.NewInMemoryBus())

	_, _ = svc.Register(context.Background(), RegisterRequest{
		Username: "dupe",
		Email:    "dupe1@example.com",
		Password: "password123",
	}, "127.0.0.1")

	_, err := svc.Register(context.Background(), RegisterRequest{
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
	sessions := newTestSessionStore()
	svc := NewAuthService(repo, sessions, newTestPasswordResetStore(), &noopSender{}, "http://localhost:8080", event.NewInMemoryBus())

	_, _ = svc.Register(context.Background(), RegisterRequest{
		Username: "user1",
		Email:    "same@example.com",
		Password: "password123",
	}, "127.0.0.1")

	_, err := svc.Register(context.Background(), RegisterRequest{
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
	sessions := newTestSessionStore()
	svc := NewAuthService(repo, sessions, newTestPasswordResetStore(), &noopSender{}, "http://localhost:8080", event.NewInMemoryBus())

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
			_, err := svc.Register(context.Background(), tt.req, "127.0.0.1")
			if !errors.Is(err, ErrValidationFailed) {
				t.Errorf("expected ErrValidationFailed, got %v", err)
			}
		})
	}
}

func TestLogin_Success(t *testing.T) {
	repo := newMockUserRepo()
	sessions := newTestSessionStore()
	svc := NewAuthService(repo, sessions, newTestPasswordResetStore(), &noopSender{}, "http://localhost:8080", event.NewInMemoryBus())

	_, _ = svc.Register(context.Background(), RegisterRequest{
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

// TestLogin_SetsActivatedAtOnFirstLogin is part of the regression coverage
// for BE-8.19. A user created directly with ActivatedAt unset (as an
// email-confirmation-required registration would leave it) must have
// ActivatedAt stamped on their very first successful login — not on some
// later request. A user banned immediately after this login must not read
// as "never activated" and be swept up by the cleanup worker.
func TestLogin_SetsActivatedAtOnFirstLogin(t *testing.T) {
	repo := newMockUserRepo()
	sessions := newTestSessionStore()
	svc := NewAuthService(repo, sessions, newTestPasswordResetStore(), &noopSender{}, "http://localhost:8080", event.NewInMemoryBus())

	hash, err := HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	seeded := &model.User{
		Username:       "neveractivated",
		Email:          "neveractivated@example.com",
		PasswordHash:   hash,
		PasswordScheme: "argon2id",
		GroupID:        1,
		Enabled:        true,
		// ActivatedAt deliberately left nil, mirroring a user who confirmed
		// via a path that predates ActivatedAt tracking, or any other route
		// to enabled=true that isn't Register/ConfirmEmail.
	}
	if err := repo.Create(context.Background(), seeded); err != nil {
		t.Fatalf("seeding user: %v", err)
	}

	user, _, err := svc.Login(context.Background(), LoginRequest{
		Username: "neveractivated",
		Password: "password123",
	}, "127.0.0.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.ActivatedAt == nil {
		t.Error("ActivatedAt must be set on first successful login")
	}
}

// TestLogin_PreservesEarlierActivatedAt proves a second login does not push
// ActivatedAt forward — it records when the account was first activated, not
// when it was last logged into (that's LastLogin's job).
func TestLogin_PreservesEarlierActivatedAt(t *testing.T) {
	repo := newMockUserRepo()
	sessions := newTestSessionStore()
	svc := NewAuthService(repo, sessions, newTestPasswordResetStore(), &noopSender{}, "http://localhost:8080", event.NewInMemoryBus())

	hash, err := HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	firstActivation := time.Now().Add(-72 * time.Hour)
	seeded := &model.User{
		Username:       "repeatlogin",
		Email:          "repeatlogin@example.com",
		PasswordHash:   hash,
		PasswordScheme: "argon2id",
		GroupID:        1,
		Enabled:        true,
		ActivatedAt:    &firstActivation,
	}
	if err := repo.Create(context.Background(), seeded); err != nil {
		t.Fatalf("seeding user: %v", err)
	}

	user, _, err := svc.Login(context.Background(), LoginRequest{
		Username: "repeatlogin",
		Password: "password123",
	}, "127.0.0.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.ActivatedAt == nil || !user.ActivatedAt.Equal(firstActivation) {
		t.Errorf("ActivatedAt = %v, want unchanged from the seeded %v", user.ActivatedAt, firstActivation)
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	repo := newMockUserRepo()
	sessions := newTestSessionStore()
	svc := NewAuthService(repo, sessions, newTestPasswordResetStore(), &noopSender{}, "http://localhost:8080", event.NewInMemoryBus())

	_, _ = svc.Register(context.Background(), RegisterRequest{
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
	sessions := newTestSessionStore()
	svc := NewAuthService(repo, sessions, newTestPasswordResetStore(), &noopSender{}, "http://localhost:8080", event.NewInMemoryBus())

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
	sessions := newTestSessionStore()
	svc := NewAuthService(repo, sessions, newTestPasswordResetStore(), &noopSender{}, "http://localhost:8080", event.NewInMemoryBus())

	result, _ := svc.Register(context.Background(), RegisterRequest{
		Username: "refreshuser",
		Email:    "refresh@example.com",
		Password: "password123",
	}, "127.0.0.1")

	tokens := result.Tokens
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
	sessions := newTestSessionStore()
	svc := NewAuthService(repo, sessions, newTestPasswordResetStore(), &noopSender{}, "http://localhost:8080", event.NewInMemoryBus())

	_, err := svc.Refresh(RefreshRequest{
		RefreshToken: "bogus",
	}, "127.0.0.1")

	if !errors.Is(err, ErrInvalidToken) {
		t.Errorf("expected ErrInvalidToken, got %v", err)
	}
}

func TestLogout(t *testing.T) {
	repo := newMockUserRepo()
	sessions := newTestSessionStore()
	svc := NewAuthService(repo, sessions, newTestPasswordResetStore(), &noopSender{}, "http://localhost:8080", event.NewInMemoryBus())

	result, _ := svc.Register(context.Background(), RegisterRequest{
		Username: "logoutuser",
		Email:    "logout@example.com",
		Password: "password123",
	}, "127.0.0.1")

	svc.Logout(result.Tokens.AccessToken)

	if sessions.GetByAccessToken(result.Tokens.AccessToken) != nil {
		t.Error("session should be invalidated after logout")
	}
}

func TestGetCurrentUser(t *testing.T) {
	repo := newMockUserRepo()
	sessions := newTestSessionStore()
	svc := NewAuthService(repo, sessions, newTestPasswordResetStore(), &noopSender{}, "http://localhost:8080", event.NewInMemoryBus())

	result, _ := svc.Register(context.Background(), RegisterRequest{
		Username: "meuser",
		Email:    "me@example.com",
		Password: "password123",
	}, "127.0.0.1")

	user, err := svc.GetCurrentUser(context.Background(), result.User.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.Username != "meuser" {
		t.Errorf("expected meuser, got %s", user.Username)
	}
}

func TestForgotPassword_GeneratesToken(t *testing.T) {
	repo := newMockUserRepo()
	sessions := newTestSessionStore()
	svc := NewAuthService(repo, sessions, newTestPasswordResetStore(), &noopSender{}, "http://localhost:8080", event.NewInMemoryBus())
	store := newTestPasswordResetStore()
	svc.SetPasswordResetStore(store)

	// Register a user
	_, _ = svc.Register(context.Background(), RegisterRequest{
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
	sessions := newTestSessionStore()
	svc := NewAuthService(repo, sessions, newTestPasswordResetStore(), &noopSender{}, "http://localhost:8080", event.NewInMemoryBus())
	store := newTestPasswordResetStore()
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
	sessions := newTestSessionStore()
	svc := NewAuthService(repo, sessions, newTestPasswordResetStore(), &noopSender{}, "http://localhost:8080", event.NewInMemoryBus())
	store := newTestPasswordResetStore()
	svc.SetPasswordResetStore(store)

	_, _ = svc.Register(context.Background(), RegisterRequest{
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
	sessions := newTestSessionStore()
	svc := NewAuthService(repo, sessions, newTestPasswordResetStore(), &noopSender{}, "http://localhost:8080", event.NewInMemoryBus())
	store := newTestPasswordResetStore()
	svc.SetPasswordResetStore(store)

	// Register and login to create a session
	result, _ := svc.Register(context.Background(), RegisterRequest{
		Username: "resetpw",
		Email:    "resetpw@example.com",
		Password: "oldpassword1",
	}, "127.0.0.1")

	tokens := result.Tokens
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
	sessions := newTestSessionStore()
	svc := NewAuthService(repo, sessions, newTestPasswordResetStore(), &noopSender{}, "http://localhost:8080", event.NewInMemoryBus())

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
	sessions := newTestSessionStore()
	svc := NewAuthService(repo, sessions, newTestPasswordResetStore(), &noopSender{}, "http://localhost:8080", event.NewInMemoryBus())
	store := newTestPasswordResetStore()
	svc.SetPasswordResetStore(store)

	_, _ = svc.Register(context.Background(), RegisterRequest{
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
	sessions := newTestSessionStore()
	svc := NewAuthService(repo, sessions, newTestPasswordResetStore(), &noopSender{}, "http://localhost:8080", event.NewInMemoryBus())
	store := newTestPasswordResetStore()
	svc.SetPasswordResetStore(store)

	_, _ = svc.Register(context.Background(), RegisterRequest{
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
	sessions := newTestSessionStore()
	svc := NewAuthService(repo, sessions, newTestPasswordResetStore(), &noopSender{}, "http://localhost:8080", event.NewInMemoryBus())

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
	sessions := newTestSessionStore()
	svc := NewAuthService(repo, sessions, newTestPasswordResetStore(), &noopSender{}, "http://localhost:8080", event.NewInMemoryBus())

	_, _ = svc.Register(context.Background(), RegisterRequest{
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
