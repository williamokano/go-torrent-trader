package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// InviteRepo implements repository.InviteRepository using PostgreSQL.
type InviteRepo struct {
	db *sql.DB
}

// NewInviteRepo returns a new PostgreSQL-backed InviteRepository.
func NewInviteRepo(db *sql.DB) repository.InviteRepository {
	return &InviteRepo{db: db}
}

func scanInvite(row interface{ Scan(...any) error }) (*model.Invite, error) {
	var inv model.Invite
	err := row.Scan(
		&inv.ID, &inv.InviterID, &inv.Token,
		&inv.InviteeID, &inv.RedeemedAt, &inv.ExpiresAt, &inv.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	inv.Redeemed = inv.InviteeID != nil
	return &inv, nil
}

const inviteColumns = `id, inviter_id, token, used_by_id, used_at, expires_at, created_at`

func (r *InviteRepo) Create(ctx context.Context, invite *model.Invite) error {
	query := `INSERT INTO invites (inviter_id, token, expires_at)
		VALUES ($1, $2, $3)
		RETURNING id, created_at`
	err := r.db.QueryRowContext(ctx, query,
		invite.InviterID, invite.Token, invite.ExpiresAt,
	).Scan(&invite.ID, &invite.CreatedAt)
	if err != nil {
		return fmt.Errorf("create invite: %w", err)
	}
	return nil
}

func (r *InviteRepo) GetByToken(ctx context.Context, token string) (*model.Invite, error) {
	query := fmt.Sprintf("SELECT %s FROM invites WHERE token = $1", inviteColumns)
	return scanInvite(r.db.QueryRowContext(ctx, query, token))
}

func (r *InviteRepo) ListByInviter(ctx context.Context, inviterID int64, page, perPage int) ([]model.Invite, int64, error) {
	if page <= 0 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 25
	}
	if perPage > 100 {
		perPage = 100
	}

	var total int64
	countQuery := `SELECT COUNT(*) FROM invites WHERE inviter_id = $1`
	if err := r.db.QueryRowContext(ctx, countQuery, inviterID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count invites: %w", err)
	}

	offset := (page - 1) * perPage
	query := fmt.Sprintf(`SELECT %s FROM invites WHERE inviter_id = $1
		ORDER BY created_at DESC LIMIT $2 OFFSET $3`, inviteColumns)

	rows, err := r.db.QueryContext(ctx, query, inviterID, perPage, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list invites: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var invites []model.Invite
	for rows.Next() {
		var inv model.Invite
		if err := rows.Scan(
			&inv.ID, &inv.InviterID, &inv.Token,
			&inv.InviteeID, &inv.RedeemedAt, &inv.ExpiresAt, &inv.CreatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan invite: %w", err)
		}
		inv.Redeemed = inv.InviteeID != nil
		invites = append(invites, inv)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate invites: %w", err)
	}

	return invites, total, nil
}

func (r *InviteRepo) Redeem(ctx context.Context, token string, inviteeID int64) error {
	query := `UPDATE invites SET used_by_id = $1, used_at = $2
		WHERE token = $3 AND used_by_id IS NULL AND expires_at > NOW()`
	res, err := r.db.ExecContext(ctx, query, inviteeID, time.Now(), token)
	if err != nil {
		return fmt.Errorf("redeem invite: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("redeem invite rows affected: %w", err)
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *InviteRepo) CountPendingByInviter(ctx context.Context, inviterID int64) (int, error) {
	var count int
	query := `SELECT COUNT(*) FROM invites WHERE inviter_id = $1 AND used_by_id IS NULL AND expires_at > NOW()`
	if err := r.db.QueryRowContext(ctx, query, inviterID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count pending invites: %w", err)
	}
	return count, nil
}
