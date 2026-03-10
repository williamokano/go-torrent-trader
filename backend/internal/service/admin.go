package service

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"math/big"

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
	Parked     bool    `json:"parked"`
	Passkey    *string `json:"passkey"`
	Invites    int     `json:"invites"`
	CreatedAt  string  `json:"created_at"`
	LastAccess *string `json:"last_access"`
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

	// Recent uploads
	if s.torrents != nil {
		uploads, err := s.torrents.ListByUploader(ctx, userID, 10)
		if err == nil {
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
		if err == nil {
			view.WarningsCount = count
		}
	}

	// Mod notes
	if s.modNotes != nil {
		notes, err := s.modNotes.ListByUser(ctx, userID)
		if err == nil {
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

// DeleteModNote removes a mod note by ID.
func (s *AdminService) DeleteModNote(ctx context.Context, noteID int64) error {
	if s.modNotes == nil {
		return fmt.Errorf("mod notes not configured")
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
		Donor:      u.Donor,
		Parked:     u.Parked,
		Passkey:    u.Passkey,
		Invites:    u.Invites,
		CreatedAt:  u.CreatedAt.Format("2006-01-02T15:04:05Z"),
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

// ListGroups returns all groups ordered by level.
func (s *AdminService) ListGroups(ctx context.Context) ([]model.Group, error) {
	return s.groups.List(ctx)
}
