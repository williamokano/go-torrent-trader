package model

// Permissions is a lightweight struct embedded in sessions for authorization checks.
type Permissions struct {
	GroupID     int64  `json:"group_id"`
	GroupName   string `json:"group_name"`
	Username    string `json:"username"`
	Level       int    `json:"level"`
	CanUpload   bool   `json:"can_upload"`
	CanDownload bool   `json:"can_download"`
	CanInvite   bool   `json:"can_invite"`
	CanComment  bool   `json:"can_comment"`
	CanForum    bool   `json:"can_forum"`
	IsAdmin     bool   `json:"is_admin"`
	IsModerator bool   `json:"is_moderator"`
	IsImmune    bool   `json:"is_immune"`
	// CanSelfApprove marks the Uploader class (BE-8.22c): may approve own uploads.
	CanSelfApprove bool `json:"can_self_approve"`
}

// IsStaff returns true if the user is an admin or moderator.
func (p Permissions) IsStaff() bool {
	return p.IsAdmin || p.IsModerator
}

// PermissionsFromGroup builds a Permissions struct from a Group.
func PermissionsFromGroup(g *Group) Permissions {
	return Permissions{
		GroupID:        g.ID,
		GroupName:      g.Name,
		Level:          g.Level,
		CanUpload:      g.CanUpload,
		CanDownload:    g.CanDownload,
		CanInvite:      g.CanInvite,
		CanComment:     g.CanComment,
		CanForum:       g.CanForum,
		IsAdmin:        g.IsAdmin,
		IsModerator:    g.IsModerator,
		IsImmune:       g.IsImmune,
		CanSelfApprove: g.CanSelfApprove,
		// CanFeed is deliberately absent. It would carry only the class half of
		// live feed access, so it would be wrong for any member whose own flag
		// was revoked, and stale the moment the class was edited — an inviting
		// but incorrect thing for a caller to branch on. FeedAccessService reads
		// the live rows instead.
	}
}
