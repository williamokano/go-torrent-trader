package testutil

import "github.com/williamokano/go-torrent-trader/backend/internal/model"

// AdminPermissions returns Permissions matching the seeded Administrator group.
func AdminPermissions() model.Permissions {
	return model.Permissions{
		GroupID:     1,
		GroupName:   "Administrator",
		Level:       100,
		CanUpload:   true,
		CanDownload: true,
		CanInvite:   true,
		CanComment:  true,
		CanForum:    true,
		IsAdmin:     true,
		IsModerator: false,
		IsImmune:    true,
	}
}

// ModeratorPermissions returns Permissions matching the seeded Moderator group.
func ModeratorPermissions() model.Permissions {
	return model.Permissions{
		GroupID:     2,
		GroupName:   "Moderator",
		Level:       80,
		CanUpload:   true,
		CanDownload: true,
		CanInvite:   true,
		CanComment:  true,
		CanForum:    true,
		IsAdmin:     false,
		IsModerator: true,
		IsImmune:    true,
	}
}

// UserPermissions returns Permissions matching the seeded User group.
func UserPermissions() model.Permissions {
	return model.Permissions{
		GroupID:     5,
		GroupName:   "User",
		Level:       20,
		CanUpload:   true,
		CanDownload: true,
		CanInvite:   false,
		CanComment:  true,
		CanForum:    true,
		IsAdmin:     false,
		IsModerator: false,
		IsImmune:    false,
	}
}

// ValidatingPermissions returns Permissions matching the seeded Validating group.
func ValidatingPermissions() model.Permissions {
	return model.Permissions{
		GroupID:     6,
		GroupName:   "Validating",
		Level:       10,
		CanUpload:   false,
		CanDownload: true,
		CanInvite:   false,
		CanComment:  false,
		CanForum:    false,
		IsAdmin:     false,
		IsModerator: false,
		IsImmune:    false,
	}
}
