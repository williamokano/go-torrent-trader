package postgres

import (
	"context"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

// Update writes a whole user row — test-only. Production writes go through
// UpdateFields, which touches only the columns a caller changed; fixtures
// here still find it convenient to set up a row in one call.
// awaiting_confirmation is clear-only, as in production.
func (r *UserRepo) Update(ctx context.Context, user *model.User) error {
	query := `UPDATE users SET
		username = $1, email = $2, password_hash = $3, password_scheme = $4,
		passkey = $5, group_id = $6, avatar = $7, title = $8, info = $9,
		enabled = $10, parked = $11, ip = $12, last_login = $13, last_access = $14,
		warned = $15, warn_until = $16, donor = $17, invited_by = $18,
		can_forum = $19, disabled_until = $20, activated_at = $21,
		awaiting_confirmation = awaiting_confirmation AND $22, updated_at = NOW()
	WHERE id = $23
	RETURNING updated_at`

	return r.db.QueryRowContext(ctx, query,
		user.Username, user.Email, user.PasswordHash, user.PasswordScheme,
		user.Passkey, user.GroupID, user.Avatar, user.Title, user.Info,
		user.Enabled, user.Parked, user.IP, user.LastLogin, user.LastAccess,
		user.Warned, user.WarnUntil, user.Donor, user.InvitedBy,
		user.CanForum, user.DisabledUntil, user.ActivatedAt, user.AwaitingConfirmation, user.ID,
	).Scan(&user.UpdatedAt)
}
