package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

var (
	ErrInvalidCredentials   = errors.New("invalid credentials")
	ErrUsernameTaken        = errors.New("username already taken")
	ErrEmailTaken           = errors.New("email already taken")
	ErrInvalidToken         = errors.New("invalid or expired token")
	ErrValidationFailed     = errors.New("validation failed")
	ErrResetRateLimitExceed = errors.New("too many password reset requests")
	ErrInvalidResetToken    = errors.New("invalid or expired reset token")
)

var (
	usernameRe = regexp.MustCompile(`^[a-zA-Z0-9_]{3,20}$`)
	emailRe    = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
)

const (
	adminGroupID   = 1
	defaultGroupID = 5 // "User" group from seed data
)

// AuthTokens holds the token pair returned after authentication.
type AuthTokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"` // seconds until access token expires
}

// RegisterRequest holds the input for user registration.
type RegisterRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

// LoginRequest holds the input for user login.
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// RefreshRequest holds the input for token refresh.
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// ForgotPasswordRequest holds the input for requesting a password reset.
type ForgotPasswordRequest struct {
	Email string `json:"email"`
}

// ResetPasswordRequest holds the input for resetting a password.
type ResetPasswordRequest struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

const (
	resetTokenRateLimit = 3               // max reset requests per email per hour
	resetTokenTTL       = 1 * time.Hour   // reset token expiry
)

// DefaultAccessTokenTTL is the default access token lifetime.
const DefaultAccessTokenTTL = 1 * time.Hour

// DefaultRefreshTokenTTL is the default refresh token lifetime.
const DefaultRefreshTokenTTL = 30 * 24 * time.Hour

// AuthService handles authentication business logic.
type AuthService struct {
	users           repository.UserRepository
	groups          repository.GroupRepository
	sessions        SessionStore
	passwordResets  PasswordResetStore
	email           EmailSender
	siteBaseURL     string
	accessTokenTTL  time.Duration
	refreshTokenTTL time.Duration
}

// NewAuthService creates a new AuthService with default token TTLs.
// groups may be nil; if so, sessions will have zero-value Permissions.
func NewAuthService(users repository.UserRepository, sessions SessionStore, passwordResets PasswordResetStore, email EmailSender, siteBaseURL string) *AuthService {
	return &AuthService{
		users:           users,
		sessions:        sessions,
		passwordResets:  passwordResets,
		email:           email,
		siteBaseURL:     siteBaseURL,
		accessTokenTTL:  DefaultAccessTokenTTL,
		refreshTokenTTL: DefaultRefreshTokenTTL,
	}
}

// NewAuthServiceWithTTL creates a new AuthService with custom token TTLs.
// groups may be nil; if so, sessions will have zero-value Permissions.
func NewAuthServiceWithTTL(users repository.UserRepository, sessions SessionStore, passwordResets PasswordResetStore, email EmailSender, siteBaseURL string, accessTTL, refreshTTL time.Duration, groups repository.GroupRepository) *AuthService {
	return &AuthService{
		users:           users,
		groups:          groups,
		sessions:        sessions,
		passwordResets:  passwordResets,
		email:           email,
		siteBaseURL:     siteBaseURL,
		accessTokenTTL:  accessTTL,
		refreshTokenTTL: refreshTTL,
	}
}

// SetPasswordResetStore sets the password reset store (useful for testing).
func (s *AuthService) SetPasswordResetStore(store PasswordResetStore) {
	s.passwordResets = store
}

// SetSiteBaseURL sets the site base URL used in password reset links (useful for testing).
func (s *AuthService) SetSiteBaseURL(url string) {
	s.siteBaseURL = url
}

// Register creates a new user account and returns auth tokens.
func (s *AuthService) Register(ctx context.Context, req RegisterRequest, ip string) (*model.User, *AuthTokens, error) {
	if err := validateRegistration(req); err != nil {
		return nil, nil, err
	}

	// Fast-path uniqueness checks for better UX error messages.
	// The DB unique constraint is the real safety net against races.
	if existing, err := s.users.GetByUsername(ctx, req.Username); err == nil && existing != nil {
		return nil, nil, ErrUsernameTaken
	}
	if existing, err := s.users.GetByEmail(ctx, req.Email); err == nil && existing != nil {
		return nil, nil, ErrEmailTaken
	}

	hash, err := HashPassword(req.Password)
	if err != nil {
		return nil, nil, fmt.Errorf("hash password: %w", err)
	}

	// Determine group: first user gets admin
	groupID := int64(defaultGroupID)
	isFirstUser, err := s.isFirstUser(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("check first user: %w", err)
	}
	if isFirstUser {
		groupID = adminGroupID
	}

	// Generate passkey for tracker authentication (32-char hex)
	passkeyFull, err := GenerateToken()
	if err != nil {
		return nil, nil, fmt.Errorf("generate passkey: %w", err)
	}
	passkey := passkeyFull[:32]

	user := &model.User{
		Username:       req.Username,
		Email:          req.Email,
		PasswordHash:   hash,
		PasswordScheme: "argon2id",
		Passkey:        &passkey,
		GroupID:        groupID,
		Enabled:        true,
		IP:             &ip,
	}

	if err := s.users.Create(ctx, user); err != nil {
		// Map DB unique constraint violations to domain errors.
		// This is the true race-condition safety net.
		errMsg := err.Error()
		if strings.Contains(errMsg, "unique") || strings.Contains(errMsg, "duplicate") {
			if strings.Contains(errMsg, "username") {
				return nil, nil, ErrUsernameTaken
			}
			if strings.Contains(errMsg, "email") {
				return nil, nil, ErrEmailTaken
			}
		}
		return nil, nil, fmt.Errorf("create user: %w", err)
	}

	tokens, err := s.createSession(ctx, user.ID, user.GroupID, ip)
	if err != nil {
		return nil, nil, fmt.Errorf("create session: %w", err)
	}

	return user, tokens, nil
}

// Login authenticates a user and returns auth tokens.
func (s *AuthService) Login(ctx context.Context, req LoginRequest, ip string) (*model.User, *AuthTokens, error) {
	user, err := s.users.GetByUsername(ctx, req.Username)
	if err != nil || user == nil {
		return nil, nil, ErrInvalidCredentials
	}

	if !user.Enabled {
		return nil, nil, ErrInvalidCredentials
	}

	match, err := VerifyPassword(req.Password, user.PasswordHash)
	if err != nil || !match {
		return nil, nil, ErrInvalidCredentials
	}

	tokens, err := s.createSession(ctx, user.ID, user.GroupID, ip)
	if err != nil {
		return nil, nil, fmt.Errorf("create session: %w", err)
	}

	// Update last login — non-fatal, log and continue
	now := time.Now()
	user.LastLogin = &now
	user.IP = &ip
	if err := s.users.Update(ctx, user); err != nil {
		slog.Error("failed to update last login", "user_id", user.ID, "error", err)
	}

	return user, tokens, nil
}

// Refresh issues a new token pair using a valid refresh token.
func (s *AuthService) Refresh(req RefreshRequest, ip string) (*AuthTokens, error) {
	sess := s.sessions.GetByRefreshToken(req.RefreshToken)
	if sess == nil {
		return nil, ErrInvalidToken
	}

	accessToken, err := GenerateToken()
	if err != nil {
		return nil, fmt.Errorf("generate access token: %w", err)
	}
	refreshToken, err := GenerateToken()
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}

	now := time.Now()
	newSession := &Session{
		UserID:           sess.UserID,
		GroupID:          sess.GroupID,
		Permissions:      sess.Permissions,
		AccessToken:      accessToken,
		RefreshToken:     refreshToken,
		DeviceName:       sess.DeviceName,
		IP:               ip,
		CreatedAt:        sess.CreatedAt,
		LastActive:       now,
		ExpiresAt:        now.Add(s.accessTokenTTL),
		RefreshExpiresAt: now.Add(s.refreshTokenTTL),
	}

	if err := s.sessions.Rotate(req.RefreshToken, newSession); err != nil {
		return nil, fmt.Errorf("rotate session: %w", err)
	}

	return &AuthTokens{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int64(s.accessTokenTTL.Seconds()),
	}, nil
}

// Logout invalidates the session for the given access token.
func (s *AuthService) Logout(accessToken string) {
	s.sessions.Delete(accessToken)
}

// GetCurrentUser returns the user by ID.
func (s *AuthService) GetCurrentUser(ctx context.Context, userID int64) (*model.User, error) {
	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	return user, nil
}

// Sessions returns the session store (used by the validator adapter).
func (s *AuthService) Sessions() SessionStore {
	return s.sessions
}

// ForgotPassword initiates a password reset for the given email.
// Always returns nil to prevent email enumeration — errors are logged, not returned to caller.
func (s *AuthService) ForgotPassword(ctx context.Context, req ForgotPasswordRequest) error {
	if req.Email == "" {
		return nil
	}

	user, err := s.users.GetByEmail(ctx, req.Email)
	if err != nil || user == nil {
		// Don't reveal whether the email exists
		return nil
	}

	// Rate limit: max 3 requests per email per hour
	recentCount, err := s.passwordResets.CountRecentByUserID(user.ID, 1*time.Hour)
	if err != nil {
		slog.Error("failed to count recent resets", "user_id", user.ID, "error", err)
		return nil
	}
	if recentCount >= resetTokenRateLimit {
		// Silently ignore — don't reveal rate limiting to the caller
		slog.Warn("password reset rate limit exceeded", "user_id", user.ID)
		return nil
	}

	rawToken, err := GenerateToken()
	if err != nil {
		slog.Error("failed to generate reset token", "error", err)
		return nil
	}

	tokenHash := hashToken(rawToken)
	now := time.Now()

	if err := s.passwordResets.Create(&PasswordReset{
		UserID:    user.ID,
		TokenHash: tokenHash,
		ExpiresAt: now.Add(resetTokenTTL),
		Used:      false,
		CreatedAt: now,
	}); err != nil {
		slog.Error("failed to store password reset token", "user_id", user.ID, "error", err)
		return nil
	}

	resetURL := fmt.Sprintf("%s/reset-password?token=%s", s.siteBaseURL, rawToken)
	slog.Info("password reset requested",
		"user_id", user.ID,
		"reset_url", resetURL,
	)

	htmlBody := fmt.Sprintf(
		`<h2>Password Reset</h2>
<p>You requested a password reset for your account.</p>
<p><a href="%s">Click here to reset your password</a></p>
<p>This link expires in 1 hour.</p>
<p>If you didn't request this, ignore this email.</p>`,
		resetURL,
	)

	if err := s.email.Send(ctx, req.Email, "Password Reset — TorrentTrader", htmlBody); err != nil {
		slog.Error("failed to send password reset email", "user_id", user.ID, "error", err)
		// Don't reveal email issues to caller
	}

	return nil
}

// ResetPassword validates a reset token and sets a new password.
func (s *AuthService) ResetPassword(ctx context.Context, req ResetPasswordRequest) error {
	if req.Token == "" || req.Password == "" {
		return ErrInvalidResetToken
	}

	if len(req.Password) < 8 {
		return fmt.Errorf("%w: password must be at least 8 characters", ErrValidationFailed)
	}
	if len(req.Password) > 1024 {
		return fmt.Errorf("%w: password must be at most 1024 characters", ErrValidationFailed)
	}

	tokenHash := hashToken(req.Token)

	// Atomically claim the token (mark as used) BEFORE changing the password.
	// This prevents TOCTOU race conditions and ensures a token can only be used once.
	pr, err := s.passwordResets.ClaimByTokenHash(tokenHash)
	if err != nil {
		return fmt.Errorf("claim reset token: %w", err)
	}
	if pr == nil {
		return ErrInvalidResetToken
	}

	user, err := s.users.GetByID(ctx, pr.UserID)
	if err != nil || user == nil {
		return ErrInvalidResetToken
	}

	newHash, err := HashPassword(req.Password)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	user.PasswordHash = newHash
	user.PasswordScheme = "argon2id"
	if err := s.users.Update(ctx, user); err != nil {
		return fmt.Errorf("update password: %w", err)
	}

	// Invalidate all sessions for this user
	s.sessions.DeleteByUserID(pr.UserID)

	slog.Info("password reset completed", "user_id", pr.UserID)
	return nil
}

// hashToken returns the hex-encoded SHA-256 hash of a token string.
func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

func (s *AuthService) createSession(ctx context.Context, userID, groupID int64, ip string) (*AuthTokens, error) {
	accessToken, err := GenerateToken()
	if err != nil {
		return nil, err
	}
	refreshToken, err := GenerateToken()
	if err != nil {
		return nil, err
	}

	// Load group permissions into the session
	var perms model.Permissions
	if s.groups != nil {
		group, err := s.groups.GetByID(ctx, groupID)
		if err != nil {
			return nil, fmt.Errorf("load group permissions: %w", err)
		}
		perms = model.PermissionsFromGroup(group)
	}

	now := time.Now()
	session := &Session{
		UserID:           userID,
		GroupID:          groupID,
		Permissions:      perms,
		AccessToken:      accessToken,
		RefreshToken:     refreshToken,
		IP:               ip,
		CreatedAt:        now,
		LastActive:       now,
		ExpiresAt:        now.Add(s.accessTokenTTL),
		RefreshExpiresAt: now.Add(s.refreshTokenTTL),
	}

	if err := s.sessions.Create(session); err != nil {
		return nil, fmt.Errorf("store session: %w", err)
	}

	return &AuthTokens{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int64(s.accessTokenTTL.Seconds()),
	}, nil
}

func (s *AuthService) isFirstUser(ctx context.Context) (bool, error) {
	count, err := s.users.Count(ctx)
	if err != nil {
		return false, fmt.Errorf("count users: %w", err)
	}
	return count == 0, nil
}

func validateRegistration(req RegisterRequest) error {
	if !usernameRe.MatchString(req.Username) {
		return fmt.Errorf("%w: username must be 3-20 alphanumeric characters or underscores", ErrValidationFailed)
	}
	if !emailRe.MatchString(req.Email) {
		return fmt.Errorf("%w: invalid email format", ErrValidationFailed)
	}
	if len(req.Password) < 8 {
		return fmt.Errorf("%w: password must be at least 8 characters", ErrValidationFailed)
	}
	if len(req.Password) > 1024 {
		return fmt.Errorf("%w: password must be at most 1024 characters", ErrValidationFailed)
	}
	return nil
}
