package service

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"sync"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

// mockBanRepo is an in-memory BanRepository for testing.
type mockBanRepo struct {
	mu          sync.Mutex
	emailBans   []model.BannedEmail
	ipBans      []model.BannedIP
	nextEmailID int64
	nextIPID    int64
}

func newMockBanRepo() *mockBanRepo {
	return &mockBanRepo{nextEmailID: 1, nextIPID: 1}
}

func (m *mockBanRepo) CreateEmailBan(_ context.Context, ban *model.BannedEmail) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, b := range m.emailBans {
		if b.Pattern == ban.Pattern {
			return fmt.Errorf("duplicate key: pattern already exists")
		}
	}
	ban.ID = m.nextEmailID
	m.nextEmailID++
	m.emailBans = append(m.emailBans, *ban)
	return nil
}

func (m *mockBanRepo) DeleteEmailBan(_ context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, b := range m.emailBans {
		if b.ID == id {
			m.emailBans = append(m.emailBans[:i], m.emailBans[i+1:]...)
			return nil
		}
	}
	return sql.ErrNoRows
}

func (m *mockBanRepo) ListEmailBans(_ context.Context) ([]model.BannedEmail, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]model.BannedEmail, len(m.emailBans))
	copy(result, m.emailBans)
	return result, nil
}

func (m *mockBanRepo) IsEmailBanned(_ context.Context, email string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, b := range m.emailBans {
		// Simple pattern matching: support exact match and suffix match with %
		if matchesLikePattern(email, b.Pattern) {
			return true, nil
		}
	}
	return false, nil
}

func (m *mockBanRepo) CreateIPBan(_ context.Context, ban *model.BannedIP) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	ban.ID = m.nextIPID
	m.nextIPID++
	m.ipBans = append(m.ipBans, *ban)
	return nil
}

func (m *mockBanRepo) DeleteIPBan(_ context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, b := range m.ipBans {
		if b.ID == id {
			m.ipBans = append(m.ipBans[:i], m.ipBans[i+1:]...)
			return nil
		}
	}
	return sql.ErrNoRows
}

func (m *mockBanRepo) ListIPBans(_ context.Context) ([]model.BannedIP, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]model.BannedIP, len(m.ipBans))
	copy(result, m.ipBans)
	return result, nil
}

func (m *mockBanRepo) IsIPBanned(_ context.Context, ip string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false, nil
	}
	for _, b := range m.ipBans {
		_, cidr, err := net.ParseCIDR(b.IPRange)
		if err != nil {
			// Try as single IP
			if net.ParseIP(b.IPRange) != nil && b.IPRange == ip {
				return true, nil
			}
			continue
		}
		if cidr.Contains(parsedIP) {
			return true, nil
		}
	}
	return false, nil
}

// matchesLikePattern does a simple SQL LIKE-style match for testing.
// Supports: exact match, %suffix, prefix%, %contains%.
func matchesLikePattern(value, pattern string) bool {
	if pattern == value {
		return true
	}
	if len(pattern) >= 2 && pattern[0] == '%' && pattern[len(pattern)-1] == '%' {
		return contains(value, pattern[1:len(pattern)-1])
	}
	if len(pattern) >= 1 && pattern[0] == '%' {
		return hasSuffix(value, pattern[1:])
	}
	if len(pattern) >= 1 && pattern[len(pattern)-1] == '%' {
		return hasPrefix(value, pattern[:len(pattern)-1])
	}
	return false
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr) >= 0
}

func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func searchString(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func TestBanEmail_Success(t *testing.T) {
	repo := newMockBanRepo()
	svc := NewBanService(repo, event.NewInMemoryBus())

	ban := &model.BannedEmail{Pattern: "%@mailinator.com"}
	err := svc.BanEmail(context.Background(), 1, "admin", ban)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ban.ID == 0 {
		t.Error("expected ban ID to be set")
	}
}

func TestUnbanEmail_Success(t *testing.T) {
	repo := newMockBanRepo()
	svc := NewBanService(repo, event.NewInMemoryBus())

	ban := &model.BannedEmail{Pattern: "%@mailinator.com"}
	_ = svc.BanEmail(context.Background(), 1, "admin", ban)

	err := svc.UnbanEmail(context.Background(), 1, "admin", ban.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	bans, _ := svc.ListEmailBans(context.Background())
	if len(bans) != 0 {
		t.Errorf("expected 0 email bans, got %d", len(bans))
	}
}

func TestUnbanEmail_NotFound(t *testing.T) {
	repo := newMockBanRepo()
	svc := NewBanService(repo, event.NewInMemoryBus())

	err := svc.UnbanEmail(context.Background(), 1, "admin", 999)
	if err == nil {
		t.Error("expected error for nonexistent ban")
	}
}

func TestCheckEmail_Banned(t *testing.T) {
	repo := newMockBanRepo()
	svc := NewBanService(repo, event.NewInMemoryBus())

	ban := &model.BannedEmail{Pattern: "%@mailinator.com"}
	_ = svc.BanEmail(context.Background(), 1, "admin", ban)

	banned, err := svc.CheckEmail(context.Background(), "spammer@mailinator.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !banned {
		t.Error("expected email to be banned")
	}
}

func TestCheckEmail_NotBanned(t *testing.T) {
	repo := newMockBanRepo()
	svc := NewBanService(repo, event.NewInMemoryBus())

	ban := &model.BannedEmail{Pattern: "%@mailinator.com"}
	_ = svc.BanEmail(context.Background(), 1, "admin", ban)

	banned, err := svc.CheckEmail(context.Background(), "user@gmail.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if banned {
		t.Error("expected email NOT to be banned")
	}
}

func TestBanIP_Success(t *testing.T) {
	repo := newMockBanRepo()
	svc := NewBanService(repo, event.NewInMemoryBus())

	ban := &model.BannedIP{IPRange: "10.0.0.0/8"}
	err := svc.BanIP(context.Background(), 1, "admin", ban)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ban.ID == 0 {
		t.Error("expected ban ID to be set")
	}
}

func TestUnbanIP_Success(t *testing.T) {
	repo := newMockBanRepo()
	svc := NewBanService(repo, event.NewInMemoryBus())

	ban := &model.BannedIP{IPRange: "10.0.0.0/8"}
	_ = svc.BanIP(context.Background(), 1, "admin", ban)

	err := svc.UnbanIP(context.Background(), 1, "admin", ban.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	bans, _ := svc.ListIPBans(context.Background())
	if len(bans) != 0 {
		t.Errorf("expected 0 IP bans, got %d", len(bans))
	}
}

func TestCheckIP_Banned(t *testing.T) {
	repo := newMockBanRepo()
	svc := NewBanService(repo, event.NewInMemoryBus())

	ban := &model.BannedIP{IPRange: "10.0.0.0/8"}
	_ = svc.BanIP(context.Background(), 1, "admin", ban)

	banned, err := svc.CheckIP(context.Background(), "10.1.2.3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !banned {
		t.Error("expected IP to be banned")
	}
}

func TestCheckIP_NotBanned(t *testing.T) {
	repo := newMockBanRepo()
	svc := NewBanService(repo, event.NewInMemoryBus())

	ban := &model.BannedIP{IPRange: "10.0.0.0/8"}
	_ = svc.BanIP(context.Background(), 1, "admin", ban)

	banned, err := svc.CheckIP(context.Background(), "192.168.1.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if banned {
		t.Error("expected IP NOT to be banned")
	}
}

func TestListEmailBans(t *testing.T) {
	repo := newMockBanRepo()
	svc := NewBanService(repo, event.NewInMemoryBus())

	_ = svc.BanEmail(context.Background(), 1, "admin", &model.BannedEmail{Pattern: "%@spam.com"})
	_ = svc.BanEmail(context.Background(), 1, "admin", &model.BannedEmail{Pattern: "bad@example.com"})

	bans, err := svc.ListEmailBans(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(bans) != 2 {
		t.Errorf("expected 2 email bans, got %d", len(bans))
	}
}

func TestListIPBans(t *testing.T) {
	repo := newMockBanRepo()
	svc := NewBanService(repo, event.NewInMemoryBus())

	_ = svc.BanIP(context.Background(), 1, "admin", &model.BannedIP{IPRange: "10.0.0.0/8"})
	_ = svc.BanIP(context.Background(), 1, "admin", &model.BannedIP{IPRange: "172.16.0.0/12"})

	bans, err := svc.ListIPBans(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(bans) != 2 {
		t.Errorf("expected 2 IP bans, got %d", len(bans))
	}
}

// TestAuthService_BannedEmailRejectsRegistration tests that registration is
// rejected when the email matches a banned pattern.
func TestAuthService_BannedEmailRejectsRegistration(t *testing.T) {
	repo := newMockUserRepo()
	sessions := newTestSessionStore()
	bus := event.NewInMemoryBus()
	svc := NewAuthService(repo, sessions, newTestPasswordResetStore(), &noopSender{}, "http://localhost:8080", bus)

	banRepo := newMockBanRepo()
	banSvc := NewBanService(banRepo, bus)
	_ = banSvc.BanEmail(context.Background(), 1, "admin", &model.BannedEmail{Pattern: "%@mailinator.com"})
	svc.SetBanChecker(banSvc)

	_, _, err := svc.Register(context.Background(), RegisterRequest{
		Username: "spammer",
		Email:    "spammer@mailinator.com",
		Password: "password123",
	}, "127.0.0.1")

	if err == nil {
		t.Fatal("expected error for banned email")
	}
	if err.Error() != ErrBannedEmail.Error() && !contains(err.Error(), "banned") {
		t.Errorf("expected banned email error, got: %v", err)
	}
}

// TestAuthService_BannedIPRejectsRegistration tests that registration is
// rejected when the IP is banned.
func TestAuthService_BannedIPRejectsRegistration(t *testing.T) {
	repo := newMockUserRepo()
	sessions := newTestSessionStore()
	bus := event.NewInMemoryBus()
	svc := NewAuthService(repo, sessions, newTestPasswordResetStore(), &noopSender{}, "http://localhost:8080", bus)

	banRepo := newMockBanRepo()
	banSvc := NewBanService(banRepo, bus)
	_ = banSvc.BanIP(context.Background(), 1, "admin", &model.BannedIP{IPRange: "10.0.0.0/8"})
	svc.SetBanChecker(banSvc)

	_, _, err := svc.Register(context.Background(), RegisterRequest{
		Username: "badactor",
		Email:    "badactor@example.com",
		Password: "password123",
	}, "10.1.2.3")

	if err == nil {
		t.Fatal("expected error for banned IP")
	}
	if err.Error() != ErrBannedIP.Error() && !contains(err.Error(), "banned") {
		t.Errorf("expected banned IP error, got: %v", err)
	}
}

// TestAuthService_BannedIPRejectsLogin tests that login is rejected when the IP is banned.
func TestAuthService_BannedIPRejectsLogin(t *testing.T) {
	repo := newMockUserRepo()
	sessions := newTestSessionStore()
	bus := event.NewInMemoryBus()
	svc := NewAuthService(repo, sessions, newTestPasswordResetStore(), &noopSender{}, "http://localhost:8080", bus)

	// Register a user first (no ban yet)
	_, _, _ = svc.Register(context.Background(), RegisterRequest{
		Username: "normaluser",
		Email:    "normal@example.com",
		Password: "password123",
	}, "127.0.0.1")

	// Now set up the ban checker
	banRepo := newMockBanRepo()
	banSvc := NewBanService(banRepo, bus)
	_ = banSvc.BanIP(context.Background(), 1, "admin", &model.BannedIP{IPRange: "10.0.0.0/8"})
	svc.SetBanChecker(banSvc)

	_, _, err := svc.Login(context.Background(), LoginRequest{
		Username: "normaluser",
		Password: "password123",
	}, "10.1.2.3")

	if err == nil {
		t.Fatal("expected error for banned IP login")
	}
	if err.Error() != ErrBannedIP.Error() && !contains(err.Error(), "banned") {
		t.Errorf("expected banned IP error, got: %v", err)
	}
}

// TestAuthService_NoBanChecker_AllowsRegistration verifies backward compatibility
// when no BanChecker is set.
func TestAuthService_NoBanChecker_AllowsRegistration(t *testing.T) {
	repo := newMockUserRepo()
	sessions := newTestSessionStore()
	svc := NewAuthService(repo, sessions, newTestPasswordResetStore(), &noopSender{}, "http://localhost:8080", event.NewInMemoryBus())

	user, tokens, err := svc.Register(context.Background(), RegisterRequest{
		Username: "testuser",
		Email:    "test@mailinator.com",
		Password: "password123",
	}, "10.1.2.3")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user == nil || tokens == nil {
		t.Fatal("expected user and tokens")
	}
}
