package service

import (
	"context"
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
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUsernameTaken      = errors.New("username already taken")
	ErrEmailTaken         = errors.New("email already taken")
	ErrInvalidToken       = errors.New("invalid or expired token")
	ErrValidationFailed   = errors.New("validation failed")
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

// AuthService handles authentication business logic.
type AuthService struct {
	users    repository.UserRepository
	sessions *SessionStore
}

// NewAuthService creates a new AuthService.
func NewAuthService(users repository.UserRepository, sessions *SessionStore) *AuthService {
	return &AuthService{
		users:    users,
		sessions: sessions,
	}
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

	tokens, err := s.createSession(user.ID, user.GroupID, ip)
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

	tokens, err := s.createSession(user.ID, user.GroupID, ip)
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
		AccessToken:      accessToken,
		RefreshToken:     refreshToken,
		DeviceName:       sess.DeviceName,
		IP:               ip,
		CreatedAt:        sess.CreatedAt,
		LastActive:       now,
		ExpiresAt:        now.Add(AccessTokenTTL),
		RefreshExpiresAt: now.Add(RefreshTokenTTL),
	}

	s.sessions.Rotate(req.RefreshToken, newSession)

	return &AuthTokens{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int64(AccessTokenTTL.Seconds()),
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
func (s *AuthService) Sessions() *SessionStore {
	return s.sessions
}

func (s *AuthService) createSession(userID, groupID int64, ip string) (*AuthTokens, error) {
	accessToken, err := GenerateToken()
	if err != nil {
		return nil, err
	}
	refreshToken, err := GenerateToken()
	if err != nil {
		return nil, err
	}

	now := time.Now()
	session := &Session{
		UserID:           userID,
		GroupID:          groupID,
		AccessToken:      accessToken,
		RefreshToken:     refreshToken,
		IP:               ip,
		CreatedAt:        now,
		LastActive:       now,
		ExpiresAt:        now.Add(AccessTokenTTL),
		RefreshExpiresAt: now.Add(RefreshTokenTTL),
	}

	s.sessions.Create(session)

	return &AuthTokens{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int64(AccessTokenTTL.Seconds()),
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
