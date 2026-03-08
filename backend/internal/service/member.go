package service

import (
	"context"
	"fmt"
	"math"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// MemberView is the public user representation returned by member list endpoints.
type MemberView struct {
	ID         int64   `json:"id"`
	Username   string  `json:"username"`
	GroupID    int64   `json:"group_id"`
	GroupName  string  `json:"group_name"`
	Uploaded   int64   `json:"uploaded"`
	Downloaded int64   `json:"downloaded"`
	Ratio      float64 `json:"ratio"`
	Donor      bool    `json:"donor"`
	CreatedAt  string  `json:"created_at"`
}

// StaffView is the user representation returned by the staff endpoint.
type StaffView struct {
	ID        int64   `json:"id"`
	Username  string  `json:"username"`
	GroupID   int64   `json:"group_id"`
	GroupName string  `json:"group_name"`
	Title     *string `json:"title"`
}

// MemberService handles member/staff list business logic.
type MemberService struct {
	users  repository.UserRepository
	groups repository.GroupRepository
}

// NewMemberService creates a new MemberService.
func NewMemberService(users repository.UserRepository, groups repository.GroupRepository) *MemberService {
	return &MemberService{users: users, groups: groups}
}

// ListMembers returns a paginated list of users with public profile data.
func (s *MemberService) ListMembers(ctx context.Context, opts repository.ListUsersOptions) ([]MemberView, int64, error) {
	users, total, err := s.users.List(ctx, opts)
	if err != nil {
		return nil, 0, fmt.Errorf("list members: %w", err)
	}

	groupNames := s.resolveGroupNames(ctx, users)

	views := make([]MemberView, len(users))
	for i, u := range users {
		views[i] = MemberView{
			ID:         u.ID,
			Username:   u.Username,
			GroupID:    u.GroupID,
			GroupName:  groupNames[u.GroupID],
			Uploaded:   u.Uploaded,
			Downloaded: u.Downloaded,
			Ratio:      memberRatio(u.Uploaded, u.Downloaded),
			Donor:      u.Donor,
			CreatedAt:  u.CreatedAt.Format("2006-01-02T15:04:05Z"),
		}
	}

	return views, total, nil
}

// ListStaff returns users who belong to admin or moderator groups.
func (s *MemberService) ListStaff(ctx context.Context) ([]StaffView, error) {
	users, err := s.users.ListStaff(ctx)
	if err != nil {
		return nil, fmt.Errorf("list staff: %w", err)
	}

	groupNames := s.resolveGroupNames(ctx, users)

	views := make([]StaffView, len(users))
	for i, u := range users {
		views[i] = StaffView{
			ID:        u.ID,
			Username:  u.Username,
			GroupID:   u.GroupID,
			GroupName: groupNames[u.GroupID],
			Title:     u.Title,
		}
	}

	return views, nil
}

// resolveGroupNames fetches group names for a list of users.
func (s *MemberService) resolveGroupNames(ctx context.Context, users []model.User) map[int64]string {
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
	return groupNames
}

// memberRatio computes the upload/download ratio.
func memberRatio(uploaded, downloaded int64) float64 {
	if downloaded == 0 {
		if uploaded == 0 {
			return 0
		}
		return math.Inf(1)
	}
	return float64(uploaded) / float64(downloaded)
}
