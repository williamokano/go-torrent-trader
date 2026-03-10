package service

import (
	"context"
	"fmt"
	"math"
	"net/url"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

var (
	ErrUserNotFound       = fmt.Errorf("user not found")
	ErrIncorrectPassword  = fmt.Errorf("incorrect password")
)

// UpdateProfileRequest holds the input for updating a user's profile.
type UpdateProfileRequest struct {
	Avatar *string `json:"avatar"`
	Title  *string `json:"title"`
	Info   *string `json:"info"`
}

// ChangePasswordRequest holds the input for changing a user's password.
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// RecentUpload is a summary of a recently uploaded torrent.
type RecentUpload struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

// PublicProfile is the profile data visible to any authenticated user.
type PublicProfile struct {
	ID              int64          `json:"id"`
	Username        string         `json:"username"`
	GroupID         int64          `json:"group_id"`
	GroupName       string         `json:"group_name"`
	Avatar          *string        `json:"avatar"`
	Title           *string        `json:"title"`
	Info            *string        `json:"info"`
	Uploaded        int64          `json:"uploaded"`
	Downloaded      int64          `json:"downloaded"`
	Ratio           float64        `json:"ratio"`
	SeedingCount    int            `json:"seeding_count"`
	LeechingCount   int            `json:"leeching_count"`
	Donor           bool           `json:"donor"`
	Warned          bool           `json:"warned"`
	CreatedAt       string         `json:"created_at"`
	InvitedByID     *int64         `json:"invited_by_id,omitempty"`
	InvitedByName   *string        `json:"invited_by_name,omitempty"`
	RecentUploads   []RecentUpload `json:"recent_uploads"`
}

// OwnerProfile extends PublicProfile with fields only visible to the profile owner.
type OwnerProfile struct {
	PublicProfile
	Email       string            `json:"email"`
	Passkey     string            `json:"passkey"`
	Invites     int               `json:"invites"`
	LastLogin   *string           `json:"last_login"`
	Permissions *model.Permissions `json:"permissions,omitempty"`
	CanDownload bool              `json:"can_download"`
	CanUpload   bool              `json:"can_upload"`
	CanChat     bool              `json:"can_chat"`
}

// UserService handles user profile business logic.
type UserService struct {
	users    repository.UserRepository
	sessions SessionStore
	groups   repository.GroupRepository
	peers    repository.PeerRepository
	torrents repository.TorrentRepository
}

// NewUserService creates a new UserService.
func NewUserService(users repository.UserRepository, sessions SessionStore, groups repository.GroupRepository, peers repository.PeerRepository, torrents repository.TorrentRepository) *UserService {
	return &UserService{users: users, sessions: sessions, groups: groups, peers: peers, torrents: torrents}
}

// GetProfile returns a user's profile. If viewerID matches the profile user ID,
// private fields are included.
func (s *UserService) GetProfile(ctx context.Context, userID, viewerID int64) (interface{}, error) {
	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return nil, ErrUserNotFound
	}

	pub := s.buildPublicProfile(ctx, user)

	if viewerID == userID {
		return buildOwnerProfile(user, pub), nil
	}

	return pub, nil
}

// GetFullProfile returns the owner profile for the given user (used by /auth/me).
func (s *UserService) GetFullProfile(ctx context.Context, userID int64) (*OwnerProfile, error) {
	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return nil, ErrUserNotFound
	}

	pub := s.buildPublicProfile(ctx, user)
	op := buildOwnerProfile(user, pub)

	if s.groups != nil {
		group, err := s.groups.GetByID(ctx, user.GroupID)
		if err == nil {
			perms := model.PermissionsFromGroup(group)
			op.Permissions = &perms
		}
	}

	return op, nil
}

// UpdateProfile updates the user's avatar, title, and info fields.
func (s *UserService) UpdateProfile(ctx context.Context, userID int64, req UpdateProfileRequest) (*OwnerProfile, error) {
	if err := validateProfileUpdate(req); err != nil {
		return nil, err
	}

	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return nil, ErrUserNotFound
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

	if err := s.users.Update(ctx, user); err != nil {
		return nil, fmt.Errorf("update profile: %w", err)
	}

	pub := s.buildPublicProfile(ctx, user)
	return buildOwnerProfile(user, pub), nil
}

// ChangePassword verifies the current password, hashes the new one, persists it,
// and invalidates all sessions except the current one.
func (s *UserService) ChangePassword(ctx context.Context, userID int64, currentAccessToken string, req ChangePasswordRequest) error {
	if err := validateChangePassword(req); err != nil {
		return err
	}

	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return ErrUserNotFound
	}

	match, err := VerifyPassword(req.CurrentPassword, user.PasswordHash)
	if err != nil || !match {
		return ErrIncorrectPassword
	}

	newHash, err := HashPassword(req.NewPassword)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	user.PasswordHash = newHash
	user.PasswordScheme = "argon2id"

	if err := s.users.Update(ctx, user); err != nil {
		return fmt.Errorf("update password: %w", err)
	}

	// Invalidate all sessions except the current one
	s.sessions.DeleteByUserIDExcept(userID, currentAccessToken)

	return nil
}

// RegeneratePasskey generates a new 32-char hex passkey for the user.
func (s *UserService) RegeneratePasskey(ctx context.Context, userID int64) (string, error) {
	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return "", ErrUserNotFound
	}

	token, err := GenerateToken()
	if err != nil {
		return "", fmt.Errorf("generate passkey: %w", err)
	}
	passkey := token[:32]
	user.Passkey = &passkey

	if err := s.users.Update(ctx, user); err != nil {
		return "", fmt.Errorf("update passkey: %w", err)
	}

	return passkey, nil
}

func (s *UserService) buildPublicProfile(ctx context.Context, u *model.User) PublicProfile {
	pub := PublicProfile{
		ID:         u.ID,
		Username:   u.Username,
		GroupID:    u.GroupID,
		Avatar:     u.Avatar,
		Title:      u.Title,
		Info:       u.Info,
		Uploaded:   u.Uploaded,
		Downloaded: u.Downloaded,
		Ratio:      calculateRatio(u.Uploaded, u.Downloaded),
		Donor:      u.Donor,
		Warned:     u.Warned,
		CreatedAt:  u.CreatedAt.Format("2006-01-02T15:04:05Z"),
		InvitedByID: u.InvitedBy,
	}
	if u.InvitedBy != nil {
		if inviter, err := s.users.GetByID(ctx, *u.InvitedBy); err == nil {
			pub.InvitedByName = &inviter.Username
		}
	}

	if s.groups != nil {
		if group, err := s.groups.GetByID(ctx, u.GroupID); err == nil {
			pub.GroupName = group.Name
		}
	}

	if s.peers != nil {
		seeding, leeching, err := s.peers.CountByUser(ctx, u.ID)
		if err == nil {
			pub.SeedingCount = seeding
			pub.LeechingCount = leeching
		}
	}

	if s.torrents != nil {
		if uploads, err := s.torrents.ListByUploader(ctx, u.ID, 5); err == nil {
			recentUploads := make([]RecentUpload, len(uploads))
			for i, t := range uploads {
				recentUploads[i] = RecentUpload{
					ID:        t.ID,
					Name:      t.Name,
					CreatedAt: t.CreatedAt.Format("2006-01-02T15:04:05Z"),
				}
			}
			pub.RecentUploads = recentUploads
		}
	}

	return pub
}

func buildOwnerProfile(u *model.User, pub PublicProfile) *OwnerProfile {
	op := &OwnerProfile{
		PublicProfile: pub,
		Email:         u.Email,
		Passkey:       derefString(u.Passkey),
		Invites:       u.Invites,
		CanDownload:   u.CanDownload,
		CanUpload:     u.CanUpload,
		CanChat:       u.CanChat,
	}

	if u.LastLogin != nil {
		ll := u.LastLogin.Format("2006-01-02T15:04:05Z")
		op.LastLogin = &ll
	}

	return op
}

func calculateRatio(uploaded, downloaded int64) float64 {
	if downloaded == 0 {
		if uploaded == 0 {
			return 0
		}
		return math.Inf(1)
	}
	return float64(uploaded) / float64(downloaded)
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func validateProfileUpdate(req UpdateProfileRequest) error {
	if req.Avatar != nil && *req.Avatar != "" {
		u, err := url.Parse(*req.Avatar)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return fmt.Errorf("%w: avatar must be a valid HTTP or HTTPS URL", ErrValidationFailed)
		}
	}

	if req.Title != nil && len(*req.Title) > 100 {
		return fmt.Errorf("%w: title must be at most 100 characters", ErrValidationFailed)
	}

	if req.Info != nil && len(*req.Info) > 5000 {
		return fmt.Errorf("%w: info must be at most 5000 characters", ErrValidationFailed)
	}

	return nil
}

func validateChangePassword(req ChangePasswordRequest) error {
	if req.CurrentPassword == "" {
		return fmt.Errorf("%w: current password is required", ErrValidationFailed)
	}
	if len(req.NewPassword) < 8 {
		return fmt.Errorf("%w: new password must be at least 8 characters", ErrValidationFailed)
	}
	if len(req.NewPassword) > 1024 {
		return fmt.Errorf("%w: new password must be at most 1024 characters", ErrValidationFailed)
	}
	return nil
}
