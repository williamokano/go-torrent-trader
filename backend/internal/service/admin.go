package service

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"math/big"
	"strings"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

var ErrAdminPasswordTooShort = fmt.Errorf("password must be at least 8 characters")

var (
	ErrAdminUserNotFound      = fmt.Errorf("user not found")
	ErrAdminGroupNotFound     = fmt.Errorf("group not found")
	ErrAdminInsufficientLevel = fmt.Errorf("insufficient group level to perform this action")
	ErrModNoteNotFound        = fmt.Errorf("mod note not found")
	ErrInvalidModNote         = fmt.Errorf("invalid mod note")
)

// AdminUserView is the user representation returned by admin endpoints.
type AdminUserView struct {
	ID         int64   `json:"id"`
	Username   string  `json:"username"`
	Email      string  `json:"email"`
	GroupID    int64   `json:"group_id"`
	GroupName  string  `json:"group_name"`
	Avatar     *string `json:"avatar"`
	Title      *string `json:"title"`
	Info       *string `json:"info"`
	Uploaded   int64   `json:"uploaded"`
	Downloaded int64   `json:"downloaded"`
	Enabled    bool    `json:"enabled"`
	Warned     bool    `json:"warned"`
	Donor      bool    `json:"donor"`
	Parked      bool    `json:"parked"`
	Passkey     *string `json:"passkey"`
	Invites     int     `json:"invites"`
	CanDownload bool    `json:"can_download"`
	CanUpload   bool    `json:"can_upload"`
	CanChat        bool    `json:"can_chat"`
	DisabledUntil  *string `json:"disabled_until"`
	CreatedAt      string  `json:"created_at"`
	LastAccess     *string `json:"last_access"`
}

// AdminUserDetailView extends AdminUserView with additional detail data.
type AdminUserDetailView struct {
	AdminUserView
	Ratio          float64                  `json:"ratio"`
	RecentUploads  []AdminTorrentSummary    `json:"recent_uploads"`
	WarningsCount  int                      `json:"warnings_count"`
	ModNotes       []AdminModNoteView       `json:"mod_notes"`
}

// AdminTorrentSummary is a lightweight torrent representation for admin views.
type AdminTorrentSummary struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	CreatedAt string `json:"created_at"`
}

// AdminModNoteView is the mod note representation returned by admin endpoints.
type AdminModNoteView struct {
	ID             int64  `json:"id"`
	UserID         int64  `json:"user_id"`
	AuthorID       int64  `json:"author_id"`
	AuthorUsername string `json:"author_username"`
	Note           string `json:"note"`
	CreatedAt      string `json:"created_at"`
}

// AdminUpdateUserRequest holds fields an admin can change on a user.
type AdminUpdateUserRequest struct {
	Username   *string `json:"username"`
	Email      *string `json:"email"`
	Avatar     *string `json:"avatar"`
	Title      *string `json:"title"`
	Info       *string `json:"info"`
	GroupID    *int64  `json:"group_id"`
	Uploaded   *int64  `json:"uploaded"`
	Downloaded *int64  `json:"downloaded"`
	Enabled    *bool   `json:"enabled"`
	Warned     *bool   `json:"warned"`
	Donor      *bool   `json:"donor"`
	Parked     *bool   `json:"parked"`
	Invites    *int    `json:"invites"`
}

// AdminService handles admin-only business logic.
type AdminService struct {
	users    repository.UserRepository
	groups   repository.GroupRepository
	sessions SessionStore
	email    EmailSender
	eventBus event.Bus
	modNotes repository.ModNoteRepository
	torrents repository.TorrentRepository
	warnings repository.WarningRepository
	messages repository.MessageRepository
	bans     *BanService
}

// NewAdminService creates a new AdminService.
func NewAdminService(users repository.UserRepository, groups repository.GroupRepository, bus event.Bus) *AdminService {
	return &AdminService{users: users, groups: groups, eventBus: bus}
}

// SetSessionStore sets the session store for session invalidation.
func (s *AdminService) SetSessionStore(sessions SessionStore) {
	s.sessions = sessions
}

// SetEmailSender sets the email sender for notifications.
func (s *AdminService) SetEmailSender(email EmailSender) {
	s.email = email
}

// SetModNoteRepo sets the mod note repository (setter to avoid changing all call sites).
func (s *AdminService) SetModNoteRepo(repo repository.ModNoteRepository) {
	s.modNotes = repo
}

// SetTorrentRepo sets the torrent repository for admin torrent operations.
func (s *AdminService) SetTorrentRepo(repo repository.TorrentRepository) {
	s.torrents = repo
}

// SetWarningRepo sets the warning repository for user detail views.
func (s *AdminService) SetWarningRepo(repo repository.WarningRepository) {
	s.warnings = repo
}

// SetMessageRepo sets the message repository for sending PMs.
func (s *AdminService) SetMessageRepo(repo repository.MessageRepository) {
	s.messages = repo
}

// SetBanService sets the ban service for IP/email bans.
func (s *AdminService) SetBanService(bans *BanService) {
	s.bans = bans
}

// ListUsers returns a paginated list of users with group names.
func (s *AdminService) ListUsers(ctx context.Context, opts repository.ListUsersOptions) ([]AdminUserView, int64, error) {
	users, total, err := s.users.List(ctx, opts)
	if err != nil {
		return nil, 0, fmt.Errorf("list users: %w", err)
	}

	// Collect unique group IDs and fetch group names
	groupIDs := make(map[int64]bool)
	for i := range users {
		groupIDs[users[i].GroupID] = true
	}
	groupNames := make(map[int64]string)
	for gid := range groupIDs {
		g, err := s.groups.GetByID(ctx, gid)
		if err == nil {
			groupNames[gid] = g.Name
		}
	}

	views := make([]AdminUserView, len(users))
	for i, u := range users {
		views[i] = s.userToView(&u, groupNames[u.GroupID])
	}

	return views, total, nil
}

// GetUserDetail returns a detailed admin view of a user.
func (s *AdminService) GetUserDetail(ctx context.Context, userID int64) (*AdminUserDetailView, error) {
	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return nil, ErrAdminUserNotFound
	}

	groupName := ""
	if g, err := s.groups.GetByID(ctx, user.GroupID); err == nil {
		groupName = g.Name
	}

	view := &AdminUserDetailView{
		AdminUserView: s.userToView(user, groupName),
	}

	// Compute ratio
	if user.Downloaded > 0 {
		view.Ratio = float64(user.Uploaded) / float64(user.Downloaded)
	} else if user.Uploaded > 0 {
		view.Ratio = -1 // infinite
	}

	// Recent uploads (admin view: include hidden/banned torrents)
	if s.torrents != nil {
		uid := userID
		uploads, _, err := s.torrents.List(ctx, repository.ListTorrentsOptions{
			UploaderID:    &uid,
			IncludeHidden: true,
			Page:          1,
			PerPage:       10,
			SortBy:        "created_at",
			SortOrder:     "desc",
		})
		if err != nil {
			slog.Error("admin: failed to fetch recent uploads", "user_id", userID, "error", err)
		} else {
			view.RecentUploads = make([]AdminTorrentSummary, len(uploads))
			for i, t := range uploads {
				view.RecentUploads[i] = AdminTorrentSummary{
					ID:        t.ID,
					Name:      t.Name,
					Size:      t.Size,
					CreatedAt: t.CreatedAt.Format("2006-01-02T15:04:05Z"),
				}
			}
		}
	}
	if view.RecentUploads == nil {
		view.RecentUploads = []AdminTorrentSummary{}
	}

	// Active warnings count
	if s.warnings != nil {
		count, err := s.warnings.CountActiveByUser(ctx, userID)
		if err != nil {
			slog.Error("admin: failed to fetch warnings count", "user_id", userID, "error", err)
		} else {
			view.WarningsCount = count
		}
	}

	// Mod notes
	if s.modNotes != nil {
		notes, err := s.modNotes.ListByUser(ctx, userID)
		if err != nil {
			slog.Error("admin: failed to fetch mod notes", "user_id", userID, "error", err)
		} else {
			view.ModNotes = make([]AdminModNoteView, len(notes))
			for i, n := range notes {
				view.ModNotes[i] = AdminModNoteView{
					ID:             n.ID,
					UserID:         n.UserID,
					AuthorID:       n.AuthorID,
					AuthorUsername: n.AuthorUsername,
					Note:           n.Note,
					CreatedAt:      n.CreatedAt.Format("2006-01-02T15:04:05Z"),
				}
			}
		}
	}
	if view.ModNotes == nil {
		view.ModNotes = []AdminModNoteView{}
	}

	return view, nil
}

// CreateModNote adds a private staff note to a user.
func (s *AdminService) CreateModNote(ctx context.Context, userID, authorID int64, noteText string) (*AdminModNoteView, error) {
	if noteText == "" {
		return nil, fmt.Errorf("%w: note cannot be empty", ErrInvalidModNote)
	}
	if len(noteText) > 10000 {
		return nil, fmt.Errorf("%w: note exceeds maximum length of 10,000 characters", ErrInvalidModNote)
	}

	// Verify user exists
	if _, err := s.users.GetByID(ctx, userID); err != nil {
		return nil, ErrAdminUserNotFound
	}

	if s.modNotes == nil {
		return nil, fmt.Errorf("mod notes not configured")
	}

	note := &model.ModNote{
		UserID:   userID,
		AuthorID: authorID,
		Note:     noteText,
	}
	if err := s.modNotes.Create(ctx, note); err != nil {
		return nil, fmt.Errorf("create mod note: %w", err)
	}

	// Get author username
	authorUsername := ""
	if author, err := s.users.GetByID(ctx, authorID); err == nil {
		authorUsername = author.Username
	}

	return &AdminModNoteView{
		ID:             note.ID,
		UserID:         note.UserID,
		AuthorID:       note.AuthorID,
		AuthorUsername: authorUsername,
		Note:           note.Note,
		CreatedAt:      note.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}, nil
}

var ErrModNoteDeleteForbidden = fmt.Errorf("not authorized to delete this note")

// DeleteModNote removes a mod note by ID. Only the author or an admin can delete a note.
func (s *AdminService) DeleteModNote(ctx context.Context, noteID, actorID int64, perms model.Permissions) error {
	if s.modNotes == nil {
		return fmt.Errorf("mod notes not configured")
	}

	note, err := s.modNotes.GetByID(ctx, noteID)
	if err != nil {
		return ErrModNoteNotFound
	}

	// Moderators can only delete their own notes; admins can delete anyone's.
	if note.AuthorID != actorID && !perms.IsAdmin {
		return ErrModNoteDeleteForbidden
	}

	if err := s.modNotes.Delete(ctx, noteID); err != nil {
		return ErrModNoteNotFound
	}
	return nil
}

// ListTorrents returns a paginated list of torrents for admin search.
func (s *AdminService) ListTorrents(ctx context.Context, opts repository.ListTorrentsOptions) ([]model.Torrent, int64, error) {
	if s.torrents == nil {
		return nil, 0, fmt.Errorf("torrent repo not configured")
	}
	if opts.Page <= 0 {
		opts.Page = 1
	}
	if opts.PerPage <= 0 {
		opts.PerPage = 25
	}
	if opts.PerPage > 100 {
		opts.PerPage = 100
	}
	// Admin view should see all torrents including hidden/banned.
	opts.IncludeHidden = true
	return s.torrents.List(ctx, opts)
}

// UpdateUser modifies admin-editable fields on a user. actorID is the admin performing the action.
func (s *AdminService) UpdateUser(ctx context.Context, actorID, userID int64, req AdminUpdateUserRequest) (*AdminUserView, error) {
	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return nil, ErrAdminUserNotFound
	}

	// Capture previous state for event detection
	oldEnabled := user.Enabled
	oldWarned := user.Warned
	oldGroupID := user.GroupID
	oldGroupName := ""
	if g, err := s.groups.GetByID(ctx, oldGroupID); err == nil {
		oldGroupName = g.Name
	}

	if req.Username != nil {
		user.Username = *req.Username
	}
	if req.Email != nil {
		user.Email = *req.Email
	}
	if req.Avatar != nil {
		user.Avatar = req.Avatar
	}
	if req.Title != nil {
		user.Title = req.Title
	}
	if req.Info != nil {
		user.Info = req.Info
	}
	if req.GroupID != nil {
		if _, err := s.groups.GetByID(ctx, *req.GroupID); err != nil {
			return nil, fmt.Errorf("%w: invalid group_id", ErrAdminGroupNotFound)
		}
		user.GroupID = *req.GroupID
	}
	if req.Uploaded != nil {
		user.Uploaded = *req.Uploaded
	}
	if req.Downloaded != nil {
		user.Downloaded = *req.Downloaded
	}
	if req.Enabled != nil {
		user.Enabled = *req.Enabled
	}
	if req.Warned != nil {
		user.Warned = *req.Warned
	}
	if req.Donor != nil {
		user.Donor = *req.Donor
	}
	if req.Parked != nil {
		user.Parked = *req.Parked
	}
	if req.Invites != nil {
		user.Invites = *req.Invites
	}

	if err := s.users.Update(ctx, user); err != nil {
		return nil, fmt.Errorf("update user: %w", err)
	}

	// Build actor for events
	actor := s.actorFromUserID(ctx, actorID)

	// Publish events for state changes
	if oldEnabled && !user.Enabled {
		s.eventBus.Publish(ctx, &event.UserBannedEvent{
			Base:     event.NewBase(event.UserBanned, actor),
			UserID:   user.ID,
			Username: user.Username,
		})
	}
	if !oldEnabled && user.Enabled {
		s.eventBus.Publish(ctx, &event.UserUnbannedEvent{
			Base:     event.NewBase(event.UserUnbanned, actor),
			UserID:   user.ID,
			Username: user.Username,
		})
	}
	if !oldWarned && user.Warned {
		s.eventBus.Publish(ctx, &event.UserWarnedEvent{
			Base:     event.NewBase(event.UserWarned, actor),
			UserID:   user.ID,
			Username: user.Username,
		})
	}
	if oldWarned && !user.Warned {
		s.eventBus.Publish(ctx, &event.UserUnwarnedEvent{
			Base:     event.NewBase(event.UserUnwarned, actor),
			UserID:   user.ID,
			Username: user.Username,
		})
	}
	if oldGroupID != user.GroupID {
		newGroupName := ""
		if g, err := s.groups.GetByID(ctx, user.GroupID); err == nil {
			newGroupName = g.Name
		}
		s.eventBus.Publish(ctx, &event.UserGroupChangedEvent{
			Base:         event.NewBase(event.UserGroupChanged, actor),
			UserID:       user.ID,
			Username:     user.Username,
			OldGroupName: oldGroupName,
			NewGroupName: newGroupName,
		})
	}

	groupName := ""
	if g, err := s.groups.GetByID(ctx, user.GroupID); err == nil {
		groupName = g.Name
	}

	view := s.userToView(user, groupName)
	return &view, nil
}

func (s *AdminService) userToView(u *model.User, groupName string) AdminUserView {
	view := AdminUserView{
		ID:         u.ID,
		Username:   u.Username,
		Email:      u.Email,
		GroupID:    u.GroupID,
		GroupName:  groupName,
		Uploaded:   u.Uploaded,
		Downloaded: u.Downloaded,
		Avatar:     u.Avatar,
		Title:      u.Title,
		Info:       u.Info,
		Enabled:    u.Enabled,
		Warned:     u.Warned,
		Donor:       u.Donor,
		Parked:      u.Parked,
		Passkey:     u.Passkey,
		Invites:     u.Invites,
		CanDownload: u.CanDownload,
		CanUpload:   u.CanUpload,
		CanChat:     u.CanChat,
		CreatedAt:   u.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
	if u.DisabledUntil != nil {
		du := u.DisabledUntil.Format("2006-01-02T15:04:05Z")
		view.DisabledUntil = &du
	}
	if u.LastAccess != nil {
		la := u.LastAccess.Format("2006-01-02T15:04:05Z")
		view.LastAccess = &la
	}
	return view
}

func (s *AdminService) actorFromUserID(ctx context.Context, userID int64) event.Actor {
	actor := event.Actor{ID: userID}
	if u, err := s.users.GetByID(ctx, userID); err == nil {
		actor.Username = u.Username
	}
	return actor
}

// ResetPassword resets the password for a user. If newPassword is empty, a random
// 16-char password is generated. Returns the (cleartext) password so the admin can
// share it manually if the notification email fails.
func (s *AdminService) ResetPassword(ctx context.Context, actorID, userID int64, newPassword string) (string, error) {
	actor, err := s.users.GetByID(ctx, actorID)
	if err != nil {
		return "", fmt.Errorf("load actor: %w", err)
	}

	target, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return "", ErrAdminUserNotFound
	}

	// Group-level check: actor must be in a higher-level group than the target
	if err := s.assertHigherLevel(ctx, actor, target); err != nil {
		return "", err
	}

	// Validate password length if admin-supplied
	if newPassword != "" && len(newPassword) < 8 {
		return "", ErrAdminPasswordTooShort
	}

	// Generate random password if not provided
	if newPassword == "" {
		generated, genErr := generateRandomPassword(16)
		if genErr != nil {
			return "", fmt.Errorf("generate password: %w", genErr)
		}
		newPassword = generated
	}

	hash, err := HashPassword(newPassword)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}

	target.PasswordHash = hash
	if err := s.users.Update(ctx, target); err != nil {
		return "", fmt.Errorf("update user: %w", err)
	}

	// Invalidate all sessions
	if s.sessions != nil {
		s.sessions.DeleteByUserID(userID)
	}

	// Send email notification (best-effort)
	if s.email != nil {
		body := fmt.Sprintf(
			"<p>Hello %s,</p><p>Your password has been reset by an administrator. Your new password is:</p><pre>%s</pre><p>Please log in and change it immediately.</p>",
			target.Username, newPassword,
		)
		if err := s.email.Send(ctx, target.Email, "Your password has been reset", body); err != nil {
			slog.Warn("failed to send password reset email", "user_id", target.ID, "email", target.Email, "error", err)
		}
	}

	// Publish event
	evtActor := event.Actor{ID: actorID, Username: actor.Username}
	s.eventBus.Publish(ctx, &event.PasswordResetEvent{
		Base:     event.NewBase(event.PasswordReset, evtActor),
		UserID:   target.ID,
		Username: target.Username,
	})

	return newPassword, nil
}

// ResetPasskey generates a new passkey for a user. Returns the new passkey.
func (s *AdminService) ResetPasskey(ctx context.Context, actorID, userID int64) (string, error) {
	actor, err := s.users.GetByID(ctx, actorID)
	if err != nil {
		return "", fmt.Errorf("load actor: %w", err)
	}

	target, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return "", ErrAdminUserNotFound
	}

	// Group-level check
	if err := s.assertHigherLevel(ctx, actor, target); err != nil {
		return "", err
	}

	token, err := GenerateToken()
	if err != nil {
		return "", fmt.Errorf("generate passkey: %w", err)
	}
	passkey := token[:32]
	target.Passkey = &passkey

	if err := s.users.Update(ctx, target); err != nil {
		return "", fmt.Errorf("update passkey: %w", err)
	}

	// NOTE: We intentionally do NOT invalidate web sessions here. The passkey is
	// used solely for tracker authentication (announce URLs in .torrent files), not
	// for web login. Resetting it should not log the user out of the website.

	// Send email notification (best-effort)
	if s.email != nil {
		body := fmt.Sprintf(
			"<p>Hello %s,</p><p>Your passkey has been reset by an administrator. All your existing .torrent files are now invalid and must be re-downloaded.</p><p>Your new passkey is:</p><pre>%s</pre>",
			target.Username, passkey,
		)
		if err := s.email.Send(ctx, target.Email, "Your passkey has been reset", body); err != nil {
			slog.Warn("failed to send passkey reset email", "user_id", target.ID, "email", target.Email, "error", err)
		}
	}

	// Publish event
	evtActor := event.Actor{ID: actorID, Username: actor.Username}
	s.eventBus.Publish(ctx, &event.PasskeyResetEvent{
		Base:     event.NewBase(event.PasskeyReset, evtActor),
		UserID:   target.ID,
		Username: target.Username,
	})

	return passkey, nil
}

// assertHigherLevel verifies the actor's group level is strictly higher than
// the target's. This prevents staff from resetting passwords of admins, etc.
func (s *AdminService) assertHigherLevel(ctx context.Context, actor, target *model.User) error {
	actorGroup, err := s.groups.GetByID(ctx, actor.GroupID)
	if err != nil {
		return fmt.Errorf("load actor group: %w", err)
	}
	targetGroup, err := s.groups.GetByID(ctx, target.GroupID)
	if err != nil {
		return fmt.Errorf("load target group: %w", err)
	}
	if actorGroup.Level <= targetGroup.Level && actor.ID != target.ID {
		return ErrAdminInsufficientLevel
	}
	return nil
}

// generateRandomPassword creates a random password of the given length using
// alphanumeric characters plus a small set of symbols.
func generateRandomPassword(length int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%&*"
	result := make([]byte, length)
	for i := range result {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		result[i] = charset[n.Int64()]
	}
	return string(result), nil
}

// QuickBanRequest holds the parameters for the quick ban action.
type QuickBanRequest struct {
	Reason       string `json:"reason"`
	BanIP        bool   `json:"ban_ip"`
	BanEmail     bool   `json:"ban_email"`
	DurationDays *int   `json:"duration_days"`
}

var ErrAdminBanReasonRequired = fmt.Errorf("ban reason is required")
var ErrCannotBanSelf = fmt.Errorf("cannot ban yourself")
var ErrInvalidBanDuration = fmt.Errorf("duration must be positive")
var ErrCommonEmailProvider = fmt.Errorf("cannot ban common email provider domain. Ban the specific email address instead")

// commonEmailProviders is a set of popular email domains that should never be
// domain-banned because it would block legitimate users at scale.
var commonEmailProviders = map[string]bool{
	"gmail.com":       true,
	"yahoo.com":       true,
	"outlook.com":     true,
	"hotmail.com":     true,
	"icloud.com":      true,
	"protonmail.com":  true,
	"aol.com":         true,
	"mail.com":        true,
	"zoho.com":        true,
	"yandex.com":      true,
}

// QuickBanResult holds detailed results of the quick ban operation.
type QuickBanResult struct {
	Banned       bool   `json:"banned"`
	IPBanned     bool   `json:"ip_banned"`
	EmailBanned  bool   `json:"email_banned"`
	EmailPattern string `json:"email_pattern,omitempty"`
	DurationDays *int   `json:"duration_days,omitempty"`
	Message      string `json:"message"`
}

// QuickBanUser performs a full ban in a single operation: disables user first
// (the critical operation), then sends PM, creates warning, optionally bans
// IP/email, and invalidates sessions.
func (s *AdminService) QuickBanUser(ctx context.Context, actorID, targetID int64, req QuickBanRequest) (*QuickBanResult, error) {
	if req.Reason == "" {
		return nil, ErrAdminBanReasonRequired
	}

	// Cannot ban yourself
	if actorID == targetID {
		return nil, ErrCannotBanSelf
	}

	// Validate duration if provided
	if req.DurationDays != nil && *req.DurationDays <= 0 {
		return nil, ErrInvalidBanDuration
	}

	actor, err := s.users.GetByID(ctx, actorID)
	if err != nil {
		return nil, fmt.Errorf("load actor: %w", err)
	}

	target, err := s.users.GetByID(ctx, targetID)
	if err != nil {
		return nil, ErrAdminUserNotFound
	}

	// Group-level check: actor must have higher group level than target
	if err := s.assertHigherLevel(ctx, actor, target); err != nil {
		return nil, err
	}

	// Check common email provider BEFORE making any changes
	if req.BanEmail {
		domain := splitEmail(target.Email)
		if domain != "" && commonEmailProviders[strings.ToLower(domain)] {
			return nil, ErrCommonEmailProvider
		}
	}

	result := &QuickBanResult{
		DurationDays: req.DurationDays,
	}

	// 1. Disable the user FIRST (the critical operation)
	target.Enabled = false
	if req.DurationDays != nil && *req.DurationDays > 0 {
		until := time.Now().Add(time.Duration(*req.DurationDays) * 24 * time.Hour)
		target.DisabledUntil = &until
	}

	if err := s.users.Update(ctx, target); err != nil {
		return nil, fmt.Errorf("disable user: %w", err)
	}
	result.Banned = true

	// 2. Send PM to user with ban reason (notification, not a prerequisite)
	if s.messages != nil {
		durationText := "permanent"
		if req.DurationDays != nil {
			durationText = fmt.Sprintf("%d days", *req.DurationDays)
		}
		body := fmt.Sprintf("Your account has been banned (%s).\n\nReason: %s", durationText, req.Reason)
		msg := &model.Message{
			SenderID:   actorID,
			ReceiverID: targetID,
			Subject:    "Account Banned",
			Body:       body,
		}
		if err := s.messages.Create(ctx, msg); err != nil {
			slog.Error("quick ban: failed to send ban PM", "user_id", targetID, "error", err)
		}
	}

	// 3. Create a warning record
	if s.warnings != nil {
		w := &model.Warning{
			UserID:   targetID,
			Type:     model.WarningTypeManual,
			Reason:   req.Reason,
			IssuedBy: &actorID,
			Status:   model.WarningStatusEscalated,
		}
		if err := s.warnings.Create(ctx, w); err != nil {
			slog.Error("quick ban: failed to create warning", "user_id", targetID, "error", err)
		}
	}

	// 4. Ban IP if requested
	if req.BanIP && s.bans != nil {
		ip := ""
		if target.IP != nil {
			ip = *target.IP
		}
		if ip != "" {
			reason := fmt.Sprintf("Quick ban of %s: %s", target.Username, req.Reason)
			if err := s.bans.BanIP(ctx, actorID, actor.Username, &model.BannedIP{
				IPRange: ip,
				Reason:  &reason,
			}); err != nil {
				slog.Error("quick ban: failed to ban IP", "ip", ip, "error", err)
			} else {
				result.IPBanned = true
			}
		}
		// If IP is nil, result.IPBanned stays false
	}

	// 5. Ban email domain if requested
	if req.BanEmail && s.bans != nil {
		domain := splitEmail(target.Email)
		if domain != "" {
			pattern := "*@" + domain
			reason := fmt.Sprintf("Quick ban of %s: %s", target.Username, req.Reason)
			if err := s.bans.BanEmail(ctx, actorID, actor.Username, &model.BannedEmail{
				Pattern: pattern,
				Reason:  &reason,
			}); err != nil {
				slog.Error("quick ban: failed to ban email domain", "pattern", pattern, "error", err)
			} else {
				result.EmailBanned = true
				result.EmailPattern = pattern
			}
		}
	}

	// 6. Invalidate all sessions
	if s.sessions != nil {
		s.sessions.DeleteByUserID(targetID)
	}

	// 7. Publish event
	evtActor := event.Actor{ID: actorID, Username: actor.Username}
	s.eventBus.Publish(ctx, &event.UserQuickBannedEvent{
		Base:         event.NewBase(event.UserQuickBanned, evtActor),
		UserID:       targetID,
		Username:     target.Username,
		Reason:       req.Reason,
		BanIP:        req.BanIP,
		BanEmail:     req.BanEmail,
		DurationDays: req.DurationDays,
	})

	result.Message = "User banned successfully"
	return result, nil
}

// splitEmail extracts the domain from an email address.
func splitEmail(email string) string {
	at := len(email) - 1
	for at >= 0 && email[at] != '@' {
		at--
	}
	if at < 0 || at == len(email)-1 {
		return ""
	}
	return email[at+1:]
}

// ReEnableExpiredBans re-enables users whose disabled_until has passed.
// Returns the number of users re-enabled.
func (s *AdminService) ReEnableExpiredBans(ctx context.Context) (int, error) {
	now := time.Now()
	disabled := false
	users, _, err := s.users.List(ctx, repository.ListUsersOptions{
		Enabled:             &disabled,
		DisabledUntilBefore: &now,
		PerPage:             1000,
		Page:                1,
	})
	if err != nil {
		return 0, fmt.Errorf("list expired temp bans: %w", err)
	}

	count := 0
	for i := range users {
		u := &users[i]
		u.Enabled = true
		u.DisabledUntil = nil
		if err := s.users.Update(ctx, u); err != nil {
			slog.Error("re-enable expired ban: failed to update user", "user_id", u.ID, "error", err)
			continue
		}
		count++

		// Publish unban event
		systemActor := event.Actor{ID: 0, Username: "System"}
		s.eventBus.Publish(ctx, &event.UserUnbannedEvent{
			Base:     event.NewBase(event.UserUnbanned, systemActor),
			UserID:   u.ID,
			Username: u.Username,
		})
	}

	return count, nil
}

// ListGroups returns all groups ordered by level.
func (s *AdminService) ListGroups(ctx context.Context) ([]model.Group, error) {
	return s.groups.List(ctx)
}
