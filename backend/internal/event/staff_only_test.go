package event

import (
	"sort"
	"testing"
)

func TestIsStaffOnly(t *testing.T) {
	staffOnly := []Type{
		BackupCreated, BackupDeleted, BackupDownloaded,
		CheatFlagged,
		EmailBanned, EmailUnbanned, IPBanned, IPUnbanned, UserQuickBanned,
		PasswordReset, PasskeyReset,
		TorrentReported, ReportResolved,
		WarningIssued, WarningLifted, UserWarned, UserUnwarned,
		RestrictionApplied, RestrictionLifted,
		InviteCreated, InviteRedeemed, InviteRevoked, InviteAutoGranted,
		"warning_escalation_ban", "warning_escalation_restrict",
	}
	for _, typ := range staffOnly {
		if !IsStaffOnly(typ) {
			t.Errorf("expected %s to be staff-only", typ)
		}
	}

	public := []Type{
		UserRegistered, UserBanned, UserUnbanned, UserGroupChanged, UserDeleted,
		TorrentUploaded, TorrentEdited, TorrentDeleted,
		CommentCreated, CommentDeleted, ReseedRequested,
		RegistrationModeChanged, ChatUserMuted, ChatUserUnmuted, NewsPublished,
		ForumTopicLocked, ForumCreated, ForumCategoryDeleted,
	}
	for _, typ := range public {
		if IsStaffOnly(typ) {
			t.Errorf("expected %s to be public", typ)
		}
	}
}

func TestStaffOnlyTypeStrings(t *testing.T) {
	strs := StaffOnlyTypeStrings()
	if len(strs) != len(staffOnlyTypes) {
		t.Fatalf("expected %d types, got %d", len(staffOnlyTypes), len(strs))
	}
	if !sort.StringsAreSorted(strs) {
		t.Errorf("expected sorted output, got %v", strs)
	}
	for _, s := range strs {
		if !IsStaffOnly(Type(s)) {
			t.Errorf("string %q does not round-trip through IsStaffOnly", s)
		}
	}
}
