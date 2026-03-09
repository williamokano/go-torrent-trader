package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
)

// mockEmailConfirmationStore is an in-memory EmailConfirmationStore for testing.
type mockEmailConfirmationStore struct {
	mu     sync.Mutex
	items  []*EmailConfirmation
	nextID int64
}

func newMockEmailConfirmationStore() *mockEmailConfirmationStore {
	return &mockEmailConfirmationStore{nextID: 1}
}

func (s *mockEmailConfirmationStore) Create(_ context.Context, userID int64, tokenHash []byte, expiresAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = append(s.items, &EmailConfirmation{
		ID:        s.nextID,
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now(),
	})
	s.nextID++
	return nil
}

func (s *mockEmailConfirmationStore) ClaimByTokenHash(_ context.Context, tokenHash []byte) (*EmailConfirmation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for _, ec := range s.items {
		if string(ec.TokenHash) == string(tokenHash) && ec.ConfirmedAt == nil && ec.ExpiresAt.After(now) {
			confirmedAt := now
			ec.ConfirmedAt = &confirmedAt
			return ec, nil
		}
	}
	return nil, nil
}

func (s *mockEmailConfirmationStore) GetLatestByUserID(_ context.Context, userID int64) (*EmailConfirmation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var latest *EmailConfirmation
	for _, ec := range s.items {
		if ec.UserID == userID {
			if latest == nil || ec.CreatedAt.After(latest.CreatedAt) {
				latest = ec
			}
		}
	}
	return latest, nil
}

func (s *mockEmailConfirmationStore) DeleteByUserID(_ context.Context, userID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var remaining []*EmailConfirmation
	for _, ec := range s.items {
		if ec.UserID != userID {
			remaining = append(remaining, ec)
		}
	}
	s.items = remaining
	return nil
}

func (s *mockEmailConfirmationStore) Items() []*EmailConfirmation {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.items
}

func newAuthServiceWithEmailConfirm() (*AuthService, *mockUserRepo, *mockEmailConfirmationStore, *noopSender) {
	repo := newMockUserRepo()
	sessions := newTestSessionStore()
	sender := &noopSender{}
	ecStore := newMockEmailConfirmationStore()
	svc := NewAuthService(repo, sessions, newTestPasswordResetStore(), sender, "http://localhost:5173", event.NewInMemoryBus())
	svc.SetEmailConfirmationStore(ecStore)
	svc.SetRequireEmailConfirm(true)
	svc.SetSiteName("TestTracker")
	return svc, repo, ecStore, sender
}

func TestRegister_WithEmailConfirm_RequiresConfirmation(t *testing.T) {
	svc, _, ecStore, sender := newAuthServiceWithEmailConfirm()

	// First user (admin) bypasses confirmation
	result, err := svc.Register(context.Background(), RegisterRequest{
		Username: "admin",
		Email:    "admin@example.com",
		Password: "password123",
	}, "127.0.0.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.EmailConfirmationRequired {
		t.Error("first user should not require email confirmation")
	}
	if result.User == nil || result.Tokens == nil {
		t.Error("first user should get user and tokens")
	}

	// Second user should require confirmation
	result, err = svc.Register(context.Background(), RegisterRequest{
		Username: "newuser",
		Email:    "new@example.com",
		Password: "password123",
	}, "127.0.0.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.EmailConfirmationRequired {
		t.Error("second user should require email confirmation")
	}
	if result.User != nil || result.Tokens != nil {
		t.Error("should not return user or tokens when confirmation required")
	}
	if result.Message == "" {
		t.Error("should return a message")
	}

	// Should have created a confirmation token
	items := ecStore.Items()
	if len(items) != 1 {
		t.Fatalf("expected 1 confirmation token, got %d", len(items))
	}

	// Should have sent an email
	if sender.SendCount != 1 {
		t.Errorf("expected 1 email sent, got %d", sender.SendCount)
	}
	if sender.LastTo != "new@example.com" {
		t.Errorf("expected email to new@example.com, got %s", sender.LastTo)
	}
}

func TestRegister_WithoutEmailConfirm_PreservesExistingBehavior(t *testing.T) {
	repo := newMockUserRepo()
	sessions := newTestSessionStore()
	svc := NewAuthService(repo, sessions, newTestPasswordResetStore(), &noopSender{}, "http://localhost:8080", event.NewInMemoryBus())
	// requireEmailConfirm defaults to false

	result, err := svc.Register(context.Background(), RegisterRequest{
		Username: "testuser",
		Email:    "test@example.com",
		Password: "password123",
	}, "127.0.0.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.EmailConfirmationRequired {
		t.Error("should not require email confirmation when disabled")
	}
	if result.User == nil || result.Tokens == nil {
		t.Error("should return user and tokens")
	}
}

func TestConfirmEmail_Success(t *testing.T) {
	svc, repo, _, _ := newAuthServiceWithEmailConfirm()

	// Register admin first
	_, _ = svc.Register(context.Background(), RegisterRequest{
		Username: "admin",
		Email:    "admin@example.com",
		Password: "password123",
	}, "127.0.0.1")

	// Register user that needs confirmation
	_, _ = svc.Register(context.Background(), RegisterRequest{
		Username: "newuser",
		Email:    "new@example.com",
		Password: "password123",
	}, "127.0.0.1")

	// User should be disabled
	user, _ := repo.GetByUsername(context.Background(), "newuser")
	if user.Enabled {
		t.Fatal("user should be disabled before confirmation")
	}

	// We need to get the token. Since we can't easily extract it from the email,
	// we'll generate a known token and create the confirmation manually.
	rawToken, _ := GenerateToken()
	tokenHash := hashTokenBytes(rawToken)
	ecStore := newMockEmailConfirmationStore()
	_ = ecStore.Create(context.Background(), user.ID, tokenHash, time.Now().Add(24*time.Hour))
	svc.SetEmailConfirmationStore(ecStore)

	err := svc.ConfirmEmail(context.Background(), rawToken)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// User should now be enabled
	updatedUser, _ := repo.GetByUsername(context.Background(), "newuser")
	if !updatedUser.Enabled {
		t.Error("user should be enabled after confirmation")
	}
}

func TestConfirmEmail_InvalidToken(t *testing.T) {
	svc, _, _, _ := newAuthServiceWithEmailConfirm()

	err := svc.ConfirmEmail(context.Background(), "bogustoken")
	if !errors.Is(err, ErrInvalidConfirmToken) {
		t.Errorf("expected ErrInvalidConfirmToken, got %v", err)
	}
}

func TestConfirmEmail_EmptyToken(t *testing.T) {
	svc, _, _, _ := newAuthServiceWithEmailConfirm()

	err := svc.ConfirmEmail(context.Background(), "")
	if !errors.Is(err, ErrInvalidConfirmToken) {
		t.Errorf("expected ErrInvalidConfirmToken, got %v", err)
	}
}

func TestConfirmEmail_ExpiredToken(t *testing.T) {
	svc, repo, _, _ := newAuthServiceWithEmailConfirm()

	// Register admin + user
	_, _ = svc.Register(context.Background(), RegisterRequest{
		Username: "admin",
		Email:    "admin@example.com",
		Password: "password123",
	}, "127.0.0.1")
	_, _ = svc.Register(context.Background(), RegisterRequest{
		Username: "expuser",
		Email:    "exp@example.com",
		Password: "password123",
	}, "127.0.0.1")

	user, _ := repo.GetByUsername(context.Background(), "expuser")
	rawToken, _ := GenerateToken()
	tokenHash := hashTokenBytes(rawToken)
	ecStore := newMockEmailConfirmationStore()
	// Create expired token
	_ = ecStore.Create(context.Background(), user.ID, tokenHash, time.Now().Add(-1*time.Hour))
	svc.SetEmailConfirmationStore(ecStore)

	err := svc.ConfirmEmail(context.Background(), rawToken)
	if !errors.Is(err, ErrInvalidConfirmToken) {
		t.Errorf("expected ErrInvalidConfirmToken for expired token, got %v", err)
	}
}

func TestLogin_EmailNotConfirmed(t *testing.T) {
	svc, _, _, _ := newAuthServiceWithEmailConfirm()

	// Register admin first
	_, _ = svc.Register(context.Background(), RegisterRequest{
		Username: "admin",
		Email:    "admin@example.com",
		Password: "password123",
	}, "127.0.0.1")

	// Register user that needs confirmation
	_, _ = svc.Register(context.Background(), RegisterRequest{
		Username: "unconfirmed",
		Email:    "unconf@example.com",
		Password: "password123",
	}, "127.0.0.1")

	// Try to login — should get email_not_confirmed error
	_, _, err := svc.Login(context.Background(), LoginRequest{
		Username: "unconfirmed",
		Password: "password123",
	}, "127.0.0.1")

	if !errors.Is(err, ErrEmailNotConfirmed) {
		t.Errorf("expected ErrEmailNotConfirmed, got %v", err)
	}
}

func TestResendConfirmation_Success(t *testing.T) {
	svc, _, _, sender := newAuthServiceWithEmailConfirm()

	// Register admin first
	_, _ = svc.Register(context.Background(), RegisterRequest{
		Username: "admin",
		Email:    "admin@example.com",
		Password: "password123",
	}, "127.0.0.1")

	// Register user that needs confirmation
	_, _ = svc.Register(context.Background(), RegisterRequest{
		Username: "resenduser",
		Email:    "resend@example.com",
		Password: "password123",
	}, "127.0.0.1")

	// Reset the mock email confirmation store with old timestamps to bypass rate limit
	ecStore := newMockEmailConfirmationStore()
	svc.SetEmailConfirmationStore(ecStore)

	sender.SendCount = 0
	err := svc.ResendConfirmation(context.Background(), ResendConfirmationRequest{
		Email: "resend@example.com",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sender.SendCount != 1 {
		t.Errorf("expected 1 email sent, got %d", sender.SendCount)
	}
}

func TestResendConfirmation_RateLimit(t *testing.T) {
	svc, _, ecStore, _ := newAuthServiceWithEmailConfirm()

	// Register admin first
	_, _ = svc.Register(context.Background(), RegisterRequest{
		Username: "admin",
		Email:    "admin@example.com",
		Password: "password123",
	}, "127.0.0.1")

	// Register user
	_, _ = svc.Register(context.Background(), RegisterRequest{
		Username: "rateuser",
		Email:    "rate@example.com",
		Password: "password123",
	}, "127.0.0.1")

	// ecStore already has a recently created token from registration
	_ = ecStore // tokens created < 5 min ago

	err := svc.ResendConfirmation(context.Background(), ResendConfirmationRequest{
		Email: "rate@example.com",
	})
	if !errors.Is(err, ErrConfirmRateLimitExceed) {
		t.Errorf("expected ErrConfirmRateLimitExceed, got %v", err)
	}
}

func TestResendConfirmation_AlreadyConfirmed(t *testing.T) {
	svc, _, _, sender := newAuthServiceWithEmailConfirm()

	// Admin is already enabled (confirmed)
	_, _ = svc.Register(context.Background(), RegisterRequest{
		Username: "admin",
		Email:    "admin@example.com",
		Password: "password123",
	}, "127.0.0.1")

	sender.SendCount = 0
	err := svc.ResendConfirmation(context.Background(), ResendConfirmationRequest{
		Email: "admin@example.com",
	})
	// Should return nil (anti-enumeration) instead of leaking "already confirmed"
	if err != nil {
		t.Errorf("expected nil (anti-enumeration), got %v", err)
	}
	if sender.SendCount != 0 {
		t.Errorf("expected no emails sent for already confirmed user, got %d", sender.SendCount)
	}
}

func TestConfirmEmail_DoubleConfirm(t *testing.T) {
	svc, repo, _, _ := newAuthServiceWithEmailConfirm()

	// Register admin + user
	_, _ = svc.Register(context.Background(), RegisterRequest{
		Username: "admin",
		Email:    "admin@example.com",
		Password: "password123",
	}, "127.0.0.1")
	_, _ = svc.Register(context.Background(), RegisterRequest{
		Username: "doubleuser",
		Email:    "double@example.com",
		Password: "password123",
	}, "127.0.0.1")

	user, _ := repo.GetByUsername(context.Background(), "doubleuser")
	rawToken, _ := GenerateToken()
	tokenHash := hashTokenBytes(rawToken)
	ecStore := newMockEmailConfirmationStore()
	_ = ecStore.Create(context.Background(), user.ID, tokenHash, time.Now().Add(24*time.Hour))
	svc.SetEmailConfirmationStore(ecStore)

	// First confirm should succeed
	err := svc.ConfirmEmail(context.Background(), rawToken)
	if err != nil {
		t.Fatalf("first confirm should succeed: %v", err)
	}

	// Second confirm with same token should return invalid token error
	// (token was already claimed by the first call)
	err = svc.ConfirmEmail(context.Background(), rawToken)
	if !errors.Is(err, ErrInvalidConfirmToken) {
		t.Errorf("expected ErrInvalidConfirmToken on double-confirm, got %v", err)
	}
}

func TestConfirmEmail_DoubleConfirm_DifferentTokens(t *testing.T) {
	svc, repo, _, _ := newAuthServiceWithEmailConfirm()

	_, _ = svc.Register(context.Background(), RegisterRequest{
		Username: "admin",
		Email:    "admin@example.com",
		Password: "password123",
	}, "127.0.0.1")
	_, _ = svc.Register(context.Background(), RegisterRequest{
		Username: "dbluser2",
		Email:    "dbl2@example.com",
		Password: "password123",
	}, "127.0.0.1")

	user, _ := repo.GetByUsername(context.Background(), "dbluser2")

	// Create first token and confirm
	rawToken1, _ := GenerateToken()
	tokenHash1 := hashTokenBytes(rawToken1)
	ecStore := newMockEmailConfirmationStore()
	_ = ecStore.Create(context.Background(), user.ID, tokenHash1, time.Now().Add(24*time.Hour))
	svc.SetEmailConfirmationStore(ecStore)

	err := svc.ConfirmEmail(context.Background(), rawToken1)
	if err != nil {
		t.Fatalf("first confirm should succeed: %v", err)
	}

	// Create second token and try to confirm — should fail since token was already claimed
	rawToken2, _ := GenerateToken()
	tokenHash2 := hashTokenBytes(rawToken2)
	_ = ecStore.Create(context.Background(), user.ID, tokenHash2, time.Now().Add(24*time.Hour))

	// User is now enabled, so ConfirmEmail should return nil (idempotent)
	err = svc.ConfirmEmail(context.Background(), rawToken2)
	if err != nil {
		t.Fatalf("confirming already-enabled user should succeed idempotently: %v", err)
	}
}

func TestResendConfirmation_NonexistentEmail(t *testing.T) {
	svc, _, _, sender := newAuthServiceWithEmailConfirm()

	sender.SendCount = 0
	err := svc.ResendConfirmation(context.Background(), ResendConfirmationRequest{
		Email: "nobody@example.com",
	})
	// Should not return error (don't reveal email existence)
	if err != nil {
		t.Fatalf("should not return error for nonexistent email: %v", err)
	}
	if sender.SendCount != 0 {
		t.Errorf("expected no emails sent, got %d", sender.SendCount)
	}
}
