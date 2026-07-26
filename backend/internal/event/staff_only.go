package event

import "sort"

// staffOnlyTypes classifies event types whose activity log entries must not be
// shown to regular members. Rationale per group:
//   - backups: infrastructure/opsec details (backup names, who downloads them)
//   - cheat flags: reveals anti-cheat detection rules to cheaters
//   - email/IP bans (incl. quick bans, which embed reason and +IP/+email scope):
//     publishing patterns and ranges aids ban evasion
//   - password/passkey resets: account-security events, social-engineering aid
//   - reports: protects reporter identity from retaliation
//   - warnings/restrictions: private moderation between staff and the user
//   - invites: who holds/issues invites invites begging and social engineering;
//     redeemed-event metadata also carries the invite token
//   - chat deletions: same reasoning as warnings — moderation between staff and
//     one member. Announcing "staff deleted 40 messages from X" to the whole site
//     amplifies whatever was removed, which is the opposite of what deleting it
//     was for. Note mutes and unmutes stay public deliberately: a mute is already
//     evident to everyone in the room when the person stops being able to post,
//     so hiding the log entry would conceal nothing.
var staffOnlyTypes = map[Type]struct{}{
	BackupCreated:      {},
	BackupDeleted:      {},
	BackupDownloaded:   {},
	CheatFlagged:       {},
	EmailBanned:        {},
	EmailUnbanned:      {},
	IPBanned:           {},
	IPUnbanned:         {},
	UserQuickBanned:    {},
	PasswordReset:      {},
	PasskeyReset:       {},
	TorrentReported:    {},
	ReportResolved:     {},
	WarningIssued:      {},
	WarningLifted:      {},
	UserWarned:         {},
	UserUnwarned:       {},
	RestrictionApplied: {},
	RestrictionLifted:  {},
	InviteCreated:      {},
	InviteRedeemed:     {},
	InviteRevoked:      {},
	InviteAutoGranted:  {},

	ChatMessageDeleted:      {},
	ChatUserMessagesDeleted: {},
	// Activity-log-only types written directly by the warning escalation
	// listener (no corresponding bus event): they expose warning counts and
	// thresholds. The escalation ban itself still surfaces publicly through
	// the user_banned event the listener publishes alongside.
	"warning_escalation_ban":      {},
	"warning_escalation_restrict": {},
}

// IsStaffOnly reports whether activity log entries for this event type are
// restricted to staff.
func IsStaffOnly(t Type) bool {
	_, ok := staffOnlyTypes[t]
	return ok
}

// StaffOnlyTypeStrings returns the staff-only event types as sorted strings,
// suitable for SQL exclusion filters.
func StaffOnlyTypeStrings() []string {
	out := make([]string, 0, len(staffOnlyTypes))
	for t := range staffOnlyTypes {
		out = append(out, string(t))
	}
	sort.Strings(out)
	return out
}
