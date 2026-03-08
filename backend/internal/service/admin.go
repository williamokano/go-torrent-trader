package service

import (
	"context"
	"fmt"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

var (
	ErrAdminUserNotFound  = fmt.Errorf("user not found")
	ErrAdminGroupNotFound = fmt.Errorf("group not found")
)

// AdminUserView is the user representation returned by admin endpoints.
type AdminUserView struct {
	ID         int64   `json:"id"`
	Username   string  `json:"username"`
	Email      string  `json:"email"`
	GroupID    int64   `json:"group_id"`
	GroupName  string  `json:"group_name"`
	Uploaded   int64   `json:"uploaded"`
	Downloaded int64   `json:"downloaded"`
	Enabled    bool    `json:"enabled"`
	Warned     bool    `json:"warned"`
	CreatedAt  string  `json:"created_at"`
	LastAccess *string `json:"last_access"`
}

// AdminUpdateUserRequest holds fields an admin can change on a user.
type AdminUpdateUserRequest struct {
	GroupID *int64 `json:"group_id"`
	Enabled *bool  `json:"enabled"`
	Warned  *bool  `json:"warned"`
}

// AdminService handles admin-only business logic.
type AdminService struct {
	users  repository.UserRepository
	groups repository.GroupRepository
}

// NewAdminService creates a new AdminService.
func NewAdminService(users repository.UserRepository, groups repository.GroupRepository) *AdminService {
	return &AdminService{users: users, groups: groups}
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
		views[i] = AdminUserView{
			ID:         u.ID,
			Username:   u.Username,
			Email:      u.Email,
			GroupID:    u.GroupID,
			GroupName:  groupNames[u.GroupID],
			Uploaded:   u.Uploaded,
			Downloaded: u.Downloaded,
			Enabled:    u.Enabled,
			Warned:     u.Warned,
			CreatedAt:  u.CreatedAt.Format("2006-01-02T15:04:05Z"),
		}
		if u.LastAccess != nil {
			la := u.LastAccess.Format("2006-01-02T15:04:05Z")
			views[i].LastAccess = &la
		}
	}

	return views, total, nil
}

// UpdateUser modifies admin-editable fields on a user.
func (s *AdminService) UpdateUser(ctx context.Context, userID int64, req AdminUpdateUserRequest) (*AdminUserView, error) {
	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return nil, ErrAdminUserNotFound
	}

	if req.GroupID != nil {
		// Validate group exists
		if _, err := s.groups.GetByID(ctx, *req.GroupID); err != nil {
			return nil, fmt.Errorf("%w: invalid group_id", ErrAdminGroupNotFound)
		}
		user.GroupID = *req.GroupID
	}
	if req.Enabled != nil {
		user.Enabled = *req.Enabled
	}
	if req.Warned != nil {
		user.Warned = *req.Warned
	}

	if err := s.users.Update(ctx, user); err != nil {
		return nil, fmt.Errorf("update user: %w", err)
	}

	groupName := ""
	if g, err := s.groups.GetByID(ctx, user.GroupID); err == nil {
		groupName = g.Name
	}

	view := &AdminUserView{
		ID:         user.ID,
		Username:   user.Username,
		Email:      user.Email,
		GroupID:    user.GroupID,
		GroupName:  groupName,
		Uploaded:   user.Uploaded,
		Downloaded: user.Downloaded,
		Enabled:    user.Enabled,
		Warned:     user.Warned,
		CreatedAt:  user.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
	if user.LastAccess != nil {
		la := user.LastAccess.Format("2006-01-02T15:04:05Z")
		view.LastAccess = &la
	}

	return view, nil
}

// ListGroups returns all groups ordered by level.
func (s *AdminService) ListGroups(ctx context.Context) ([]model.Group, error) {
	return s.groups.List(ctx)
}
