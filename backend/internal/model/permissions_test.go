package model_test

import (
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

func TestPermissions_IsStaff(t *testing.T) {
	tests := []struct {
		name     string
		perms    model.Permissions
		expected bool
	}{
		{"admin is staff", model.Permissions{IsAdmin: true}, true},
		{"moderator is staff", model.Permissions{IsModerator: true}, true},
		{"admin+moderator is staff", model.Permissions{IsAdmin: true, IsModerator: true}, true},
		{"regular user is not staff", model.Permissions{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.perms.IsStaff(); got != tt.expected {
				t.Errorf("IsStaff() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestPermissionsFromGroup(t *testing.T) {
	group := &model.Group{
		ID:          1,
		Name:        "Administrator",
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

	perms := model.PermissionsFromGroup(group)

	if perms.GroupID != 1 {
		t.Errorf("GroupID = %d, want 1", perms.GroupID)
	}
	if perms.GroupName != "Administrator" {
		t.Errorf("GroupName = %s, want Administrator", perms.GroupName)
	}
	if perms.Level != 100 {
		t.Errorf("Level = %d, want 100", perms.Level)
	}
	if !perms.IsAdmin {
		t.Error("IsAdmin should be true")
	}
	if perms.IsModerator {
		t.Error("IsModerator should be false")
	}
	if !perms.IsImmune {
		t.Error("IsImmune should be true")
	}
	if !perms.CanUpload || !perms.CanDownload || !perms.CanInvite || !perms.CanComment || !perms.CanForum {
		t.Error("all capabilities should be true for admin group")
	}
}

func TestPermissionsFromGroup_Validating(t *testing.T) {
	group := &model.Group{
		ID:          6,
		Name:        "Validating",
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

	perms := model.PermissionsFromGroup(group)

	if perms.CanUpload {
		t.Error("Validating should not be able to upload")
	}
	if !perms.CanDownload {
		t.Error("Validating should be able to download")
	}
	if perms.CanComment {
		t.Error("Validating should not be able to comment")
	}
	if perms.IsStaff() {
		t.Error("Validating should not be staff")
	}
}
