package service

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

var ErrAdminPasswordTooShort = fmt.Errorf("password must be at least 8 characters")

var (
	ErrAdminUserNotFound        = fmt.Errorf("user not found")
	ErrAdminGroupNotFound       = fmt.Errorf("group not found")
	ErrAdminInsufficientLevel   = fmt.Errorf("insufficient group level to perform this action")
	ErrModNoteNotFound          = fmt.Errorf("mod note not found")
	ErrInvalidModNote           = fmt.Errorf("invalid mod note")
	ErrAdminNegativeBonusPoints = fmt.Errorf("bonus points cannot be negative")
	// ErrAdminInvalidUsername guards the same charset registration enforces
	// (usernameRe, auth.go). Profile URLs are now built directly from the
	// username (/user/{username}), so a malformed value set here — a slash,
	// a space, anything encodeURIComponent would need to escape — would
	// silently break every link to this account sitewide the moment an admin
	// edits it, not just look wrong in a text field.
	ErrAdminInvalidUsername = fmt.Errorf("username must be 3-20 alphanumeric characters or underscores")
)

// AdminUserView is the user representation returned by admin endpoints.
type AdminUserView struct {
	ID            int64   `json:"id"`
	Username      string  `json:"username"`
	Email         string  `json:"email"`
	GroupID       int64   `json:"group_id"`
	GroupName     string  `json:"group_name"`
	Avatar        *string `json:"avatar"`
	Title         *string `json:"title"`
	Info          *string `json:"info"`
	Uploaded      int64   `json:"uploaded"`
	Downloaded    int64   `json:"downloaded"`
	Enabled       bool    `json:"enabled"`
	Warned        bool    `json:"warned"`
	Donor         bool    `json:"donor"`
	Parked        bool    `json:"parked"`
	Passkey       *string `json:"passkey"`
	Invites       int     `json:"invites"`
	BonusPoints   int64   `json:"bonus_points"`
	CanDownload   bool    `json:"can_download"`
	CanUpload     bool    `json:"can_upload"`
	CanChat       bool    `json:"can_chat"`
	CanForum      bool    `json:"can_forum"`
	CanInvite     bool    `json:"can_invite"`
	CanFeed       bool    `json:"can_feed"`
	DisabledUntil *string `json:"disabled_until"`
	CreatedAt     string  `json:"created_at"`
	LastAccess    *string `json:"last_access"`
}

// AdminUserDetailView extends AdminUserView with additional detail data.
type AdminUserDetailView struct {
	AdminUserView
	Ratio         float64               `json:"ratio"`
	RecentUploads []AdminTorrentSummary `json:"recent_uploads"`
	WarningsCount int                   `json:"warnings_count"`
	ModNotes      []AdminModNoteView    `json:"mod_notes"`
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
	// BonusPoints sets an absolute balance; the delta lands in the bonus
	// ledger as admin_adjust. Applied via the bonus repo, not the user Update.
	BonusPoints *int64 `json:"bonus_points"`
}

// adminUserWriteRepository is repository.UserRepository plus the
// admin-edit-only writes AdminService needs: an atomic absolute invite
// balance set, an atomic stats (uploaded/downloaded) set, and a full-row
// update — each committing its user_edit_history rows in the same
// transaction as the write it describes. (Named for what it does, not just
// invites, now that it also covers stats and the full-row update — it grew
// past its original invites-only scope over BE-8.17/BE-8.18.) Kept local to
// this file rather than added to repository.UserRepository, so the many
// existing mocks implementing UserRepository elsewhere in the test suite are
// untouched — the same approach service/restriction.go uses for
// privilegeFlagRepository.
type adminUserWriteRepository interface {
	repository.UserRepository
	// SetInvites atomically sets the invite balance and its audit entries in
	// one transaction.
	SetInvites(ctx context.Context, userID int64, invites int, entries []model.UserEditHistory) error
	// SetStats atomically sets uploaded/downloaded (independently, via nil to
	// leave a counter untouched) and their audit entries in one transaction —
	// bypassing UpdateWithHistory's full-row write, which no longer touches
	// either column, so a stale read elsewhere can never clobber stats
	// accrued concurrently by an announce.
	SetStats(ctx context.Context, userID int64, uploaded, downloaded *int64, entries []model.UserEditHistory) error
	// UpdateWithHistory is Update plus its audit entries, committed together.
	UpdateWithHistory(ctx context.Context, user *model.User, entries []model.UserEditHistory) error
}

// AdminService handles admin-only business logic.
type AdminService struct {
	users       adminUserWriteRepository
	groups      repository.GroupRepository
	groupWriter repository.GroupWriteRepository
	sessions    SessionStore
	email       EmailSender
	eventBus    event.Bus
	modNotes    repository.ModNoteRepository
	torrents    repository.TorrentRepository
	warnings    repository.WarningRepository
	messages    repository.MessageRepository
	bans        *BanService
	bonus       repository.BonusRepository
	editHistory repository.UserEditHistoryRepository
}

// NewAdminService creates a new AdminService.
func NewAdminService(users adminUserWriteRepository, groups repository.GroupRepository, bus event.Bus) *AdminService {
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

// SetGroupWriter sets the repository used for group create/update/delete.
func (s *AdminService) SetGroupWriter(w repository.GroupWriteRepository) {
	s.groupWriter = w
}

// SetBonusRepo sets the repository used for bonus point adjustments.
func (s *AdminService) SetBonusRepo(repo repository.BonusRepository) {
	s.bonus = repo
}

// SetEditHistoryRepo sets the repository used to read back the audit trail of
// admin edits to user profile fields (ListUserEditHistory). Writes no longer
// go through this repo's Record: UpdateWithHistory, SetStats, SetInvites, and
// BonusRepository.SetPoints each insert their own audit rows in the same
// transaction as the write they describe.
func (s *AdminService) SetEditHistoryRepo(repo repository.UserEditHistoryRepository) {
	s.editHistory = repo
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

	// Actor resolved up front: the username is snapshotted into every audit
	// row (so the trail survives the admin's deletion) and reused for events.
	actor := s.actorFromUserID(ctx, actorID)

	// Every field change is diffed into the edit-history audit trail: one row
	// per field with the old value, new value and acting admin. Each batch is
	// built here, then handed to the write that persists the fields it
	// describes (UpdateWithHistory/SetStats/SetInvites/BonusRepo.SetPoints),
	// which commits the write and the audit rows in one transaction — not
	// recorded as a separate best-effort step afterward.
	var audit []model.UserEditHistory
	record := func(field, oldValue, newValue string) {
		if oldValue == newValue {
			return
		}
		audit = append(audit, model.UserEditHistory{
			UserID:            user.ID,
			ChangedBy:         &actorID,
			ChangedByUsername: actor.Username,
			Field:             field,
			OldValue:          oldValue,
			NewValue:          newValue,
		})
	}

	if req.Username != nil {
		if !usernameRe.MatchString(*req.Username) {
			return nil, ErrAdminInvalidUsername
		}
		record("username", user.Username, *req.Username)
		user.Username = *req.Username
	}
	if req.Email != nil {
		record("email", user.Email, *req.Email)
		user.Email = *req.Email
	}
	if req.Avatar != nil {
		record("avatar", derefString(user.Avatar), *req.Avatar)
		user.Avatar = req.Avatar
	}
	if req.Title != nil {
		record("title", derefString(user.Title), *req.Title)
		user.Title = req.Title
	}
	if req.Info != nil {
		record("info", derefString(user.Info), *req.Info)
		user.Info = req.Info
	}
	if req.GroupID != nil {
		newGroup, err := s.groups.GetByID(ctx, *req.GroupID)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid group_id", ErrAdminGroupNotFound)
		}
		if user.GroupID != *req.GroupID {
			record("group", oldGroupName, newGroup.Name)
		}
		user.GroupID = *req.GroupID
	}
	if req.Enabled != nil {
		record("enabled", strconv.FormatBool(user.Enabled), strconv.FormatBool(*req.Enabled))
		user.Enabled = *req.Enabled
	}
	if req.Warned != nil {
		record("warned", strconv.FormatBool(user.Warned), strconv.FormatBool(*req.Warned))
		user.Warned = *req.Warned
	}
	if req.Donor != nil {
		record("donor", strconv.FormatBool(user.Donor), strconv.FormatBool(*req.Donor))
		user.Donor = *req.Donor
	}
	if req.Parked != nil {
		record("parked", strconv.FormatBool(user.Parked), strconv.FormatBool(*req.Parked))
		user.Parked = *req.Parked
	}

	// UpdateWithHistory commits the full-row write and its audit rows in one
	// transaction: a failed audit insert now rolls back the update, instead of
	// (as before this story) the update landing with the audit recorded
	// best-effort afterward.
	if err := s.users.UpdateWithHistory(ctx, user, audit); err != nil {
		return nil, fmt.Errorf("update user: %w", err)
	}
	audit = nil

	// Publish events for the state changes UpdateWithHistory just committed,
	// right away rather than after the independent stats/invites/bonus writes
	// below: those are separate transactions against unrelated columns, and a
	// later one failing must not leave an already-persisted ban/unban/warn/
	// unwarn/group-change silently un-evented (no ban PM, no downstream
	// listeners) just because this single request also touched, say, stats.
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

	// Uploaded/downloaded are set through the dedicated SetStats method, never
	// the full-row Update: Update no longer writes either column at all, so a
	// stale read elsewhere (this same handler, minted at request start) can
	// never clobber stats accrued concurrently by an announce — the same race
	// BE-8.16 closed for the privilege flags and BE-8.17 closed for invites,
	// now closed here for stats. uploaded/downloaded are independently
	// optional, so only the field(s) actually present in the request are sent
	// to SetStats; the other stays nil and SetStats leaves that column alone
	// via COALESCE rather than writing back a value read at request start.
	if req.Uploaded != nil || req.Downloaded != nil {
		if req.Uploaded != nil {
			record("uploaded", strconv.FormatInt(user.Uploaded, 10), strconv.FormatInt(*req.Uploaded, 10))
		}
		if req.Downloaded != nil {
			record("downloaded", strconv.FormatInt(user.Downloaded, 10), strconv.FormatInt(*req.Downloaded, 10))
		}
		if err := s.users.SetStats(ctx, user.ID, req.Uploaded, req.Downloaded, audit); err != nil {
			return nil, fmt.Errorf("set stats: %w", err)
		}
		if req.Uploaded != nil {
			user.Uploaded = *req.Uploaded
		}
		if req.Downloaded != nil {
			user.Downloaded = *req.Downloaded
		}
		audit = nil
	}

	// Invites are set through the dedicated SetInvites method, never the
	// full-row Update: two concurrent writers (this admin edit and an
	// auto-grant or invite creation elsewhere) could otherwise clobber each
	// other via a stale read, the same race BE-8.16 closed for the privilege
	// flags and this story closes for invites.
	if req.Invites != nil {
		oldInvites := user.Invites
		record("invites", strconv.Itoa(oldInvites), strconv.Itoa(*req.Invites))
		if err := s.users.SetInvites(ctx, user.ID, *req.Invites, audit); err != nil {
			return nil, fmt.Errorf("set invites: %w", err)
		}
		user.Invites = *req.Invites
		audit = nil
	}

	// Bonus points are set through the bonus repo, never the full-row Update:
	// the dedicated path takes a row lock and writes the admin_adjust ledger
	// entry, so it coexists with concurrent worker awards.
	if req.BonusPoints != nil {
		if *req.BonusPoints < 0 {
			return nil, ErrAdminNegativeBonusPoints
		}
		if s.bonus == nil {
			return nil, fmt.Errorf("bonus repository not configured")
		}
		oldBonus := user.BonusPoints
		record("bonus_points", strconv.FormatInt(oldBonus, 10), strconv.FormatInt(*req.BonusPoints, 10))
		if err := s.bonus.SetPoints(ctx, user.ID, *req.BonusPoints, actorID, audit); err != nil {
			return nil, fmt.Errorf("set bonus points: %w", err)
		}
		user.BonusPoints = *req.BonusPoints
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
		ID:          u.ID,
		Username:    u.Username,
		Email:       u.Email,
		GroupID:     u.GroupID,
		GroupName:   groupName,
		Uploaded:    u.Uploaded,
		Downloaded:  u.Downloaded,
		Avatar:      u.Avatar,
		Title:       u.Title,
		Info:        u.Info,
		Enabled:     u.Enabled,
		Warned:      u.Warned,
		Donor:       u.Donor,
		Parked:      u.Parked,
		Passkey:     u.Passkey,
		Invites:     u.Invites,
		BonusPoints: u.BonusPoints,
		CanDownload: u.CanDownload,
		CanUpload:   u.CanUpload,
		CanChat:     u.CanChat,
		CanForum:    u.CanForum,
		CanInvite:   u.CanInvite,
		CanFeed:     u.CanFeed,
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

// AdminUserEditHistoryView is the edit-history entry representation returned
// by admin endpoints.
type AdminUserEditHistoryView struct {
	ID                int64  `json:"id"`
	UserID            int64  `json:"user_id"`
	ChangedBy         *int64 `json:"changed_by"`
	ChangedByUsername string `json:"changed_by_username"`
	Field             string `json:"field"`
	OldValue          string `json:"old_value"`
	NewValue          string `json:"new_value"`
	CreatedAt         string `json:"created_at"`
}

// ListUserEditHistory returns the audit trail of admin edits for a user,
// newest first, plus the total entry count.
func (s *AdminService) ListUserEditHistory(ctx context.Context, userID int64, limit, offset int) ([]AdminUserEditHistoryView, int64, error) {
	if s.editHistory == nil {
		return nil, 0, fmt.Errorf("edit history repository not configured")
	}
	if _, err := s.users.GetByID(ctx, userID); err != nil {
		return nil, 0, ErrAdminUserNotFound
	}
	entries, total, err := s.editHistory.ListByUser(ctx, userID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list user edit history: %w", err)
	}
	views := make([]AdminUserEditHistoryView, 0, len(entries))
	for i := range entries {
		e := &entries[i]
		views = append(views, AdminUserEditHistoryView{
			ID:                e.ID,
			UserID:            e.UserID,
			ChangedBy:         e.ChangedBy,
			ChangedByUsername: e.ChangedByUsername,
			Field:             e.Field,
			OldValue:          e.OldValue,
			NewValue:          e.NewValue,
			CreatedAt:         e.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		})
	}
	return views, total, nil
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
	"gmail.com":      true,
	"yahoo.com":      true,
	"outlook.com":    true,
	"hotmail.com":    true,
	"icloud.com":     true,
	"protonmail.com": true,
	"aol.com":        true,
	"mail.com":       true,
	"zoho.com":       true,
	"yandex.com":     true,
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

// Group management errors. These are value errors (the caller sent something
// invalid or asked for a forbidden mutation), distinct from storage failures.
var (
	ErrGroupWritesUnavailable = fmt.Errorf("group management is not available")
	ErrGroupNotFound          = fmt.Errorf("group not found")
	ErrGroupNameRequired      = fmt.Errorf("group name is required")
	ErrGroupInvalidSlug       = fmt.Errorf("group slug must be lowercase letters, numbers, and hyphens")
	ErrGroupInvalidColor      = fmt.Errorf("group color must be a hex value like #55AA88")
	ErrGroupInvalidLevel      = fmt.Errorf("group level must be between 0 and 1000")
	ErrGroupNameTaken         = fmt.Errorf("a group with that name already exists")
	ErrGroupSlugTaken         = fmt.Errorf("a group with that slug already exists")
	ErrGroupProtected         = fmt.Errorf("this built-in group is required by registration and cannot be deleted or stripped of admin")
	ErrGroupHasMembers        = fmt.Errorf("group still has members; reassign them to another group before deleting")
)

var (
	groupSlugPattern  = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	groupColorPattern = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)
	groupSlugStripRe  = regexp.MustCompile(`[^a-z0-9]+`)
)

// GroupWriteRequest is the full representation an admin submits to create or
// update a group. Updates replace every field (the admin form always sends the
// whole object), so booleans left out default to false.
type GroupWriteRequest struct {
	Name        string  `json:"name"`
	Slug        string  `json:"slug"`
	Level       int     `json:"level"`
	Color       *string `json:"color"`
	CanUpload   bool    `json:"can_upload"`
	CanDownload bool    `json:"can_download"`
	CanInvite   bool    `json:"can_invite"`
	CanComment  bool    `json:"can_comment"`
	CanForum    bool    `json:"can_forum"`
	IsAdmin     bool    `json:"is_admin"`
	IsModerator bool    `json:"is_moderator"`
	IsImmune    bool    `json:"is_immune"`
	// CanFeed grants this class the live release feeds. An individual member can
	// still have it taken away by a restriction.
	//
	// A pointer, unlike its neighbours: those predate every client, but a client
	// written before can_feed existed omits the key, and a plain bool would read
	// that as false and revoke the feeds for the whole class on an unrelated
	// edit. Absent means "leave it as it was".
	CanFeed *bool `json:"can_feed"`
}

// slugifyGroupName derives a URL-safe slug from a group name, used when the
// admin leaves the slug blank.
func slugifyGroupName(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = groupSlugStripRe.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

// normalizeGroupInput validates and normalizes a write request into a Group.
// It does not set ID, CreatedAt, or UpdatedAt.
//
// current is the group being edited, or nil when creating. It supplies the value
// for any field the request may legitimately omit.
func normalizeGroupInput(req GroupWriteRequest, current *model.Group) (*model.Group, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, ErrGroupNameRequired
	}

	slug := strings.TrimSpace(req.Slug)
	if slug == "" {
		slug = slugifyGroupName(name)
	}
	if !groupSlugPattern.MatchString(slug) {
		return nil, ErrGroupInvalidSlug
	}

	if req.Level < 0 || req.Level > 1000 {
		return nil, ErrGroupInvalidLevel
	}

	var color *string
	if req.Color != nil {
		c := strings.TrimSpace(*req.Color)
		if c != "" {
			if !groupColorPattern.MatchString(c) {
				return nil, ErrGroupInvalidColor
			}
			color = &c
		}
	}

	// A new class gets the feeds unless it says otherwise, matching the column
	// default; an edit keeps whatever it had.
	canFeed := true
	if current != nil {
		canFeed = current.CanFeed
	}
	if req.CanFeed != nil {
		canFeed = *req.CanFeed
	}

	return &model.Group{
		Name:        name,
		Slug:        slug,
		Level:       req.Level,
		Color:       color,
		CanUpload:   req.CanUpload,
		CanDownload: req.CanDownload,
		CanInvite:   req.CanInvite,
		CanComment:  req.CanComment,
		CanForum:    req.CanForum,
		IsAdmin:     req.IsAdmin,
		IsModerator: req.IsModerator,
		IsImmune:    req.IsImmune,
		CanFeed:     canFeed,
	}, nil
}

// ensureGroupNameSlugFree rejects a name or slug already used by a different
// group. Groups are few, so scanning the full list is cheap and avoids extra
// repository methods; the admin-only, low-concurrency write path makes the
// check-then-write race a non-issue (the DB UNIQUE constraint is the backstop).
func (s *AdminService) ensureGroupNameSlugFree(ctx context.Context, name, slug string, excludeID int64) error {
	existing, err := s.groups.List(ctx)
	if err != nil {
		return fmt.Errorf("list groups: %w", err)
	}
	for i := range existing {
		g := &existing[i]
		if g.ID == excludeID {
			continue
		}
		if strings.EqualFold(g.Name, name) {
			return ErrGroupNameTaken
		}
		if g.Slug == slug {
			return ErrGroupSlugTaken
		}
	}
	return nil
}

// CreateGroup creates a new permission group.
func (s *AdminService) CreateGroup(ctx context.Context, req GroupWriteRequest) (*model.Group, error) {
	if s.groupWriter == nil {
		return nil, ErrGroupWritesUnavailable
	}
	group, err := normalizeGroupInput(req, nil)
	if err != nil {
		return nil, err
	}
	if err := s.ensureGroupNameSlugFree(ctx, group.Name, group.Slug, 0); err != nil {
		return nil, err
	}
	if err := s.groupWriter.Create(ctx, group); err != nil {
		return nil, fmt.Errorf("create group: %w", err)
	}
	return group, nil
}

// UpdateGroup replaces the fields of an existing group.
func (s *AdminService) UpdateGroup(ctx context.Context, id int64, req GroupWriteRequest) (*model.Group, error) {
	if s.groupWriter == nil {
		return nil, ErrGroupWritesUnavailable
	}

	existing, err := s.groups.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrGroupNotFound
		}
		return nil, fmt.Errorf("get group: %w", err)
	}

	group, err := normalizeGroupInput(req, existing)
	if err != nil {
		return nil, err
	}

	// Never let the built-in admin group lose its admin flag — that is how an
	// operator locks themselves (and everyone) out of the admin panel.
	if id == adminGroupID && !group.IsAdmin {
		return nil, ErrGroupProtected
	}

	if err := s.ensureGroupNameSlugFree(ctx, group.Name, group.Slug, id); err != nil {
		return nil, err
	}

	group.ID = id
	group.CreatedAt = existing.CreatedAt
	if err := s.groupWriter.Update(ctx, group); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrGroupNotFound
		}
		return nil, fmt.Errorf("update group: %w", err)
	}
	return group, nil
}

// DeleteGroup deletes a group, refusing to remove the registration-critical
// built-in groups or any group that still has members.
func (s *AdminService) DeleteGroup(ctx context.Context, id int64) error {
	if s.groupWriter == nil {
		return ErrGroupWritesUnavailable
	}
	if id == adminGroupID || id == defaultGroupID {
		return ErrGroupProtected
	}

	if _, err := s.groups.GetByID(ctx, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrGroupNotFound
		}
		return fmt.Errorf("get group: %w", err)
	}

	members, err := s.groupWriter.CountMembers(ctx, id)
	if err != nil {
		return fmt.Errorf("count group members: %w", err)
	}
	if members > 0 {
		return ErrGroupHasMembers
	}

	if err := s.groupWriter.Delete(ctx, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrGroupNotFound
		}
		return fmt.Errorf("delete group: %w", err)
	}
	return nil
}
