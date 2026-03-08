package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// BanRepo implements repository.BanRepository using PostgreSQL.
type BanRepo struct {
	db *sql.DB
}

// NewBanRepo returns a new PostgreSQL-backed BanRepository.
func NewBanRepo(db *sql.DB) repository.BanRepository {
	return &BanRepo{db: db}
}

func (r *BanRepo) CreateEmailBan(ctx context.Context, ban *model.BannedEmail) error {
	query := `INSERT INTO banned_emails (pattern, reason, created_by)
		VALUES ($1, $2, $3)
		RETURNING id, created_at`
	err := r.db.QueryRowContext(ctx, query, ban.Pattern, ban.Reason, ban.CreatedBy).
		Scan(&ban.ID, &ban.CreatedAt)
	if err != nil {
		return fmt.Errorf("create email ban: %w", err)
	}
	return nil
}

func (r *BanRepo) DeleteEmailBan(ctx context.Context, id int64) error {
	query := `DELETE FROM banned_emails WHERE id = $1`
	res, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("delete email ban: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete email ban rows affected: %w", err)
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *BanRepo) ListEmailBans(ctx context.Context) ([]model.BannedEmail, error) {
	query := `SELECT id, pattern, reason, created_by, created_at FROM banned_emails ORDER BY created_at DESC`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list email bans: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var bans []model.BannedEmail
	for rows.Next() {
		var b model.BannedEmail
		if err := rows.Scan(&b.ID, &b.Pattern, &b.Reason, &b.CreatedBy, &b.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan email ban: %w", err)
		}
		bans = append(bans, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate email bans: %w", err)
	}
	return bans, nil
}

func (r *BanRepo) IsEmailBanned(ctx context.Context, email string) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM banned_emails WHERE $1 LIKE pattern)`
	var exists bool
	if err := r.db.QueryRowContext(ctx, query, email).Scan(&exists); err != nil {
		return false, fmt.Errorf("check email ban: %w", err)
	}
	return exists, nil
}

func (r *BanRepo) CreateIPBan(ctx context.Context, ban *model.BannedIP) error {
	query := `INSERT INTO banned_ips (ip_range, reason, created_by)
		VALUES ($1, $2, $3)
		RETURNING id, created_at`
	err := r.db.QueryRowContext(ctx, query, ban.IPRange, ban.Reason, ban.CreatedBy).
		Scan(&ban.ID, &ban.CreatedAt)
	if err != nil {
		return fmt.Errorf("create ip ban: %w", err)
	}
	return nil
}

func (r *BanRepo) DeleteIPBan(ctx context.Context, id int64) error {
	query := `DELETE FROM banned_ips WHERE id = $1`
	res, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("delete ip ban: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete ip ban rows affected: %w", err)
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *BanRepo) ListIPBans(ctx context.Context) ([]model.BannedIP, error) {
	query := `SELECT id, ip_range, reason, created_by, created_at FROM banned_ips ORDER BY created_at DESC`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list ip bans: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var bans []model.BannedIP
	for rows.Next() {
		var b model.BannedIP
		if err := rows.Scan(&b.ID, &b.IPRange, &b.Reason, &b.CreatedBy, &b.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan ip ban: %w", err)
		}
		bans = append(bans, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ip bans: %w", err)
	}
	return bans, nil
}

func (r *BanRepo) IsIPBanned(ctx context.Context, ip string) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM banned_ips WHERE $1::inet <<= ip_range)`
	var exists bool
	if err := r.db.QueryRowContext(ctx, query, ip).Scan(&exists); err != nil {
		return false, fmt.Errorf("check ip ban: %w", err)
	}
	return exists, nil
}
