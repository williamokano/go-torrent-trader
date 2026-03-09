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

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
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
	ErrInviteRequired    = errors.New("invite code is required")
	ErrInvalidInviteCode = errors.New("invalid or expired invite code")
	ErrBannedEmail       = errors.New("email is banned")
	ErrBannedIP          = errors.New("IP address is banned")
	ErrEmailNotConfirmed       = errors.New("email not confirmed")
	ErrInvalidConfirmToken     = errors.New("invalid or expired confirmation token")
	ErrConfirmRateLimitExceed  = errors.New("too many confirmation email requests, please wait 5 minutes")
	ErrAccountAlreadyConfirmed = errors.New("account is already confirmed")
)

// BanChecker checks whether an email or IP is banned.
// When nil, ban checks are skipped (backward compatible).
type BanChecker interface {
	IsEmailBanned(ctx context.Context, email string) (bool, error)
	IsIPBanned(ctx context.Context, ip string) (bool, error)
}

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
	Username   string `json:"username"`
	Email      string `json:"email"`
	Password   string `json:"password"`
	InviteCode string `json:"invite_code"`
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
	resetTokenRateLimit      = 3               // max reset requests per email per hour
	resetTokenTTL            = 1 * time.Hour   // reset token expiry
	confirmTokenTTL          = 24 * time.Hour  // email confirmation token expiry
	confirmResendCooldown    = 5 * time.Minute // minimum wait between resends
)

// RegisterResult holds the result of a registration attempt.
type RegisterResult struct {
	User                      *model.User   `json:"user,omitempty"`
	Tokens                    *AuthTokens   `json:"tokens,omitempty"`
	EmailConfirmationRequired bool          `json:"email_confirmation_required,omitempty"`
	Message                   string        `json:"message,omitempty"`
}

// ResendConfirmationRequest holds input for resending a confirmation email.
type ResendConfirmationRequest struct {
	Email string `json:"email"`
}

// DefaultAccessTokenTTL is the default access token lifetime.
const DefaultAccessTokenTTL = 1 * time.Hour

// DefaultRefreshTokenTTL is the default refresh token lifetime.
const DefaultRefreshTokenTTL = 30 * 24 * time.Hour

// TaskEnqueuer abstracts background job enqueueing (e.g. asynq).
// When nil, emails are sent inline as a fallback.
type TaskEnqueuer interface {
	EnqueueSendEmail(ctx context.Context, to, subject, body string) error
}

// AuthService handles authentication business logic.
type AuthService struct {
	users              repository.UserRepository
	groups             repository.GroupRepository
	sessions           SessionStore
	passwordResets     PasswordResetStore
	emailConfirmations EmailConfirmationStore
	email              EmailSender
	taskEnqueuer       TaskEnqueuer
	siteName           string
	siteBaseURL        string
	accessTokenTTL     time.Duration
	refreshTokenTTL    time.Duration
	eventBus           event.Bus
	siteSettings       *SiteSettingsService
	inviteService      *InviteService
	banChecker         BanChecker
	requireEmailConfirm bool
}

// NewAuthService creates a new AuthService with default token TTLs.
// groups may be nil; if so, sessions will have zero-value Permissions.
func NewAuthService(users repository.UserRepository, sessions SessionStore, passwordResets PasswordResetStore, email EmailSender, siteBaseURL string, bus event.Bus) *AuthService {
	return &AuthService{
		users:           users,
		sessions:        sessions,
		passwordResets:  passwordResets,
		email:           email,
		siteBaseURL:     siteBaseURL,
		accessTokenTTL:  DefaultAccessTokenTTL,
		refreshTokenTTL: DefaultRefreshTokenTTL,
		eventBus:        bus,
	}
}

// NewAuthServiceWithTTL creates a new AuthService with custom token TTLs.
// groups may be nil; if so, sessions will have zero-value Permissions.
func NewAuthServiceWithTTL(users repository.UserRepository, sessions SessionStore, passwordResets PasswordResetStore, email EmailSender, siteBaseURL string, accessTTL, refreshTTL time.Duration, groups repository.GroupRepository, bus event.Bus) *AuthService {
	return &AuthService{
		users:           users,
		groups:          groups,
		sessions:        sessions,
		passwordResets:  passwordResets,
		email:           email,
		siteBaseURL:     siteBaseURL,
		accessTokenTTL:  accessTTL,
		refreshTokenTTL: refreshTTL,
		eventBus:        bus,
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

// SetSiteSettings sets the site settings service (used for registration mode checks).
func (s *AuthService) SetSiteSettings(svc *SiteSettingsService) {
	s.siteSettings = svc
}

// SetInviteService sets the invite service (used for invite code validation during registration).
func (s *AuthService) SetInviteService(svc *InviteService) {
	s.inviteService = svc
}

// SetBanChecker sets the ban checker used during registration and login.
// When nil, ban checks are skipped (backward compatible).
func (s *AuthService) SetBanChecker(checker BanChecker) {
	s.banChecker = checker
}

// SetTaskEnqueuer sets the background task enqueuer for async email sending.
// When nil, emails are sent inline as a fallback.
func (s *AuthService) SetTaskEnqueuer(enqueuer TaskEnqueuer) {
	s.taskEnqueuer = enqueuer
}

// SetEmailConfirmationStore sets the email confirmation store.
func (s *AuthService) SetEmailConfirmationStore(store EmailConfirmationStore) {
	s.emailConfirmations = store
}

// SetRequireEmailConfirm enables or disables email confirmation on registration.
func (s *AuthService) SetRequireEmailConfirm(require bool) {
	s.requireEmailConfirm = require
}

// SetSiteName sets the site name used in emails.
func (s *AuthService) SetSiteName(name string) {
	s.siteName = name
}

// Register creates a new user account and returns auth tokens (or confirmation required info).
func (s *AuthService) Register(ctx context.Context, req RegisterRequest, ip string) (*RegisterResult, error) {
	if err := validateRegistration(req); err != nil {
		return nil, err
	}

	// Check bans before proceeding
	if s.banChecker != nil {
		if banned, err := s.banChecker.IsEmailBanned(ctx, req.Email); err != nil {
			return nil, fmt.Errorf("check email ban: %w", err)
		} else if banned {
			return nil, ErrBannedEmail
		}
		if banned, err := s.banChecker.IsIPBanned(ctx, ip); err != nil {
			return nil, fmt.Errorf("check ip ban: %w", err)
		} else if banned {
			return nil, ErrBannedIP
		}
	}

	// Check registration mode and validate invite code
	var validatedInvite *model.Invite
	isFirstUser, err := s.isFirstUser(ctx)
	if err != nil {
		return nil, fmt.Errorf("check first user: %w", err)
	}

	// First user bypasses invite requirement (bootstrap)
	if !isFirstUser && s.siteSettings != nil {
		regMode := s.siteSettings.GetRegistrationMode(ctx)
		if regMode == RegistrationModeInviteOnly {
			if req.InviteCode == "" {
				return nil, ErrInviteRequired
			}
			if s.inviteService == nil {
				return nil, fmt.Errorf("invite service not configured")
			}
			invite, err := s.inviteService.ValidateInvite(ctx, req.InviteCode)
			if err != nil {
				return nil, ErrInvalidInviteCode
			}
			validatedInvite = invite
		}
	}

	// Fast-path uniqueness checks for better UX error messages.
	// The DB unique constraint is the real safety net against races.
	if existing, err := s.users.GetByUsername(ctx, req.Username); err == nil && existing != nil {
		return nil, ErrUsernameTaken
	}
	if existing, err := s.users.GetByEmail(ctx, req.Email); err == nil && existing != nil {
		return nil, ErrEmailTaken
	}

	hash, err := HashPassword(req.Password)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	// Determine group: first user gets admin
	groupID := int64(defaultGroupID)
	if isFirstUser {
		groupID = adminGroupID
	}

	// Generate passkey for tracker authentication (32-char hex)
	passkeyFull, err := GenerateToken()
	if err != nil {
		return nil, fmt.Errorf("generate passkey: %w", err)
	}
	passkey := passkeyFull[:32]

	// When email confirmation is required, create user with enabled=false
	// First user (admin bootstrap) always skips email confirmation
	needsConfirmation := s.requireEmailConfirm && !isFirstUser

	user := &model.User{
		Username:       req.Username,
		Email:          req.Email,
		PasswordHash:   hash,
		PasswordScheme: "argon2id",
		Passkey:        &passkey,
		GroupID:        groupID,
		Enabled:        !needsConfirmation,
		IP:             &ip,
	}

	// Link inviter to invitee
	if validatedInvite != nil {
		user.InvitedBy = &validatedInvite.InviterID
	}

	if err := s.users.Create(ctx, user); err != nil {
		// Map DB unique constraint violations to domain errors.
		// This is the true race-condition safety net.
		errMsg := err.Error()
		if strings.Contains(errMsg, "unique") || strings.Contains(errMsg, "duplicate") {
			if strings.Contains(errMsg, "username") {
				return nil, ErrUsernameTaken
			}
			if strings.Contains(errMsg, "email") {
				return nil, ErrEmailTaken
			}
		}
		return nil, fmt.Errorf("create user: %w", err)
	}

	// Redeem the invite now that the user is created
	if validatedInvite != nil && s.inviteService != nil {
		if _, err := s.inviteService.RedeemInvite(ctx, req.InviteCode, user.ID); err != nil {
			slog.Error("failed to redeem invite after registration", "user_id", user.ID, "invite_id", validatedInvite.ID, "error", err)
		}
	}

	s.eventBus.Publish(ctx, &event.UserRegisteredEvent{
		Base:   event.NewBase(event.UserRegistered, event.Actor{ID: user.ID, Username: user.Username}),
		UserID: user.ID,
	})

	// If email confirmation is required, send confirmation email instead of creating session
	if needsConfirmation {
		if err := s.sendConfirmationEmail(ctx, user); err != nil {
			slog.Error("failed to send confirmation email", "user_id", user.ID, "error", err)
		}
		return &RegisterResult{
			EmailConfirmationRequired: true,
			Message:                   "Please check your email to confirm your account",
		}, nil
	}

	tokens, err := s.createSession(ctx, user.ID, user.GroupID, ip)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	return &RegisterResult{
		User:   user,
		Tokens: tokens,
	}, nil
}

// Login authenticates a user and returns auth tokens.
func (s *AuthService) Login(ctx context.Context, req LoginRequest, ip string) (*model.User, *AuthTokens, error) {
	// Check IP ban before proceeding
	if s.banChecker != nil {
		if banned, err := s.banChecker.IsIPBanned(ctx, ip); err != nil {
			return nil, nil, fmt.Errorf("check ip ban: %w", err)
		} else if banned {
			return nil, nil, ErrBannedIP
		}
	}

	user, err := s.users.GetByUsername(ctx, req.Username)
	if err != nil || user == nil {
		return nil, nil, ErrInvalidCredentials
	}

	// Verify password BEFORE checking enabled status to prevent user enumeration.
	// An attacker should not learn account status without providing the correct password.
	match, err := VerifyPassword(req.Password, user.PasswordHash)
	if err != nil || !match {
		return nil, nil, ErrInvalidCredentials
	}

	if !user.Enabled {
		// Password is correct but account is disabled. Check if this is a pending
		// email confirmation (vs admin-disabled account).
		if s.emailConfirmations != nil {
			latest, ecErr := s.emailConfirmations.GetLatestByUserID(ctx, user.ID)
			if ecErr == nil && latest != nil && latest.ConfirmedAt == nil {
				return nil, nil, ErrEmailNotConfirmed
			}
		}
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

	s.eventBus.Publish(ctx, &event.UserLoginEvent{
		Base:   event.NewBase(event.UserLogin, event.Actor{ID: user.ID, Username: user.Username}),
		UserID: user.ID,
		IP:     ip,
	})

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

	if err := s.sendEmail(ctx, req.Email, "Password Reset — TorrentTrader", htmlBody); err != nil {
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

// ConfirmEmail validates a confirmation token and enables the user account.
// NOTE: If an admin disables a user during the confirmation window, confirming
// the token will re-enable them. Admins should delete pending confirmation
// tokens when manually disabling a user, or re-disable after the user confirms.
func (s *AuthService) ConfirmEmail(ctx context.Context, token string) error {
	if token == "" {
		return ErrInvalidConfirmToken
	}

	if s.emailConfirmations == nil {
		return ErrInvalidConfirmToken
	}

	tokenHash := hashTokenBytes(token)

	ec, err := s.emailConfirmations.ClaimByTokenHash(ctx, tokenHash)
	if err != nil {
		return fmt.Errorf("claim confirmation token: %w", err)
	}
	if ec == nil {
		return ErrInvalidConfirmToken
	}

	user, err := s.users.GetByID(ctx, ec.UserID)
	if err != nil || user == nil {
		return ErrInvalidConfirmToken
	}

	if user.Enabled {
		// Already confirmed, idempotent success
		return nil
	}

	user.Enabled = true
	if err := s.users.Update(ctx, user); err != nil {
		return fmt.Errorf("enable user: %w", err)
	}

	slog.Info("email confirmed", "user_id", ec.UserID)
	return nil
}

// ResendConfirmation sends a new confirmation email to a user who hasn't confirmed yet.
func (s *AuthService) ResendConfirmation(ctx context.Context, req ResendConfirmationRequest) error {
	if req.Email == "" {
		return nil
	}

	if s.emailConfirmations == nil {
		return nil
	}

	user, err := s.users.GetByEmail(ctx, req.Email)
	if err != nil || user == nil {
		// Don't reveal whether the email exists
		return nil
	}

	if user.Enabled {
		// Already confirmed — return success to prevent account status enumeration.
		slog.Info("resend confirmation skipped: account already confirmed", "user_id", user.ID)
		return nil
	}

	// Rate limit: check latest confirmation was > 5 minutes ago
	latest, err := s.emailConfirmations.GetLatestByUserID(ctx, user.ID)
	if err != nil {
		slog.Error("failed to get latest confirmation", "user_id", user.ID, "error", err)
		return nil
	}
	if latest != nil && time.Since(latest.CreatedAt) < confirmResendCooldown {
		return ErrConfirmRateLimitExceed
	}

	// Delete old tokens
	if err := s.emailConfirmations.DeleteByUserID(ctx, user.ID); err != nil {
		slog.Error("failed to delete old confirmations", "user_id", user.ID, "error", err)
		return nil
	}

	if err := s.sendConfirmationEmail(ctx, user); err != nil {
		slog.Error("failed to send confirmation email", "user_id", user.ID, "error", err)
	}

	return nil
}

// sendConfirmationEmail generates a token and sends a confirmation email.
func (s *AuthService) sendConfirmationEmail(ctx context.Context, user *model.User) error {
	if s.emailConfirmations == nil {
		return fmt.Errorf("email confirmation store not configured")
	}

	rawToken, err := GenerateToken()
	if err != nil {
		return fmt.Errorf("generate confirmation token: %w", err)
	}

	tokenHash := hashTokenBytes(rawToken)

	if err := s.emailConfirmations.Create(ctx, user.ID, tokenHash, time.Now().Add(confirmTokenTTL)); err != nil {
		return fmt.Errorf("store confirmation token: %w", err)
	}

	confirmURL := fmt.Sprintf("%s/confirm-email?token=%s", s.siteBaseURL, rawToken)

	siteName := s.siteName
	if siteName == "" {
		siteName = "TorrentTrader"
	}

	htmlBody := fmt.Sprintf(
		`<h2>Confirm your email address</h2>
<p>Welcome to %s! Please confirm your email by clicking the link below:</p>
<p><a href="%s">Confirm Email</a></p>
<p>This link expires in 24 hours.</p>
<p>If you didn't create this account, ignore this email.</p>`,
		siteName, confirmURL,
	)

	if err := s.sendEmail(ctx, user.Email, "Confirm your email address", htmlBody); err != nil {
		return fmt.Errorf("send confirmation email: %w", err)
	}

	slog.Info("confirmation email sent", "user_id", user.ID, "email", user.Email)
	return nil
}

// sendEmail sends an email via the background task enqueuer if available,
// falling back to inline sending via EmailSender.
func (s *AuthService) sendEmail(ctx context.Context, to, subject, htmlBody string) error {
	if s.taskEnqueuer != nil {
		return s.taskEnqueuer.EnqueueSendEmail(ctx, to, subject, htmlBody)
	}
	return s.email.Send(ctx, to, subject, htmlBody)
}

// hashTokenBytes returns the raw SHA-256 hash of a token string (as bytes for BYTEA storage).
func hashTokenBytes(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
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
