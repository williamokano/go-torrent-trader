package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// RestrictionRepo implements repository.RestrictionRepository using PostgreSQL.
type RestrictionRepo struct {
	db *sql.DB
}

// NewRestrictionRepo returns a new PostgreSQL-backed RestrictionRepository.
func NewRestrictionRepo(db *sql.DB) repository.RestrictionRepository {
	return &RestrictionRepo{db: db}
}

func (r *RestrictionRepo) Create(ctx context.Context, restriction *model.Restriction) error {
	query := `INSERT INTO user_restrictions (user_id, restriction_type, reason, issued_by, expires_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at`

	return r.db.QueryRowContext(ctx, query,
		restriction.UserID, restriction.RestrictionType, restriction.Reason,
		restriction.IssuedBy, restriction.ExpiresAt,
	).Scan(&restriction.ID, &restriction.CreatedAt)
}

func (r *RestrictionRepo) GetByID(ctx context.Context, id int64) (*model.Restriction, error) {
	query := `SELECT ur.id, ur.user_id, ur.restriction_type, ur.reason, ur.issued_by,
		ur.expires_at, ur.lifted_at, ur.lifted_by, ur.created_at,
		COALESCE(iu.username, ''), COALESCE(lu.username, '')
		FROM user_restrictions ur
		LEFT JOIN users iu ON iu.id = ur.issued_by
		LEFT JOIN users lu ON lu.id = ur.lifted_by
		WHERE ur.id = $1`

	var restriction model.Restriction
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&restriction.ID, &restriction.UserID, &restriction.RestrictionType,
		&restriction.Reason, &restriction.IssuedBy, &restriction.ExpiresAt,
		&restriction.LiftedAt, &restriction.LiftedBy, &restriction.CreatedAt,
		&restriction.IssuedByUsername, &restriction.LiftedByUsername,
	)
	if err != nil {
		return nil, err
	}
	return &restriction, nil
}

func (r *RestrictionRepo) ListByUser(ctx context.Context, userID int64) ([]model.Restriction, error) {
	query := `SELECT ur.id, ur.user_id, ur.restriction_type, ur.reason, ur.issued_by,
		ur.expires_at, ur.lifted_at, ur.lifted_by, ur.created_at,
		COALESCE(iu.username, ''), COALESCE(lu.username, '')
		FROM user_restrictions ur
		LEFT JOIN users iu ON iu.id = ur.issued_by
		LEFT JOIN users lu ON lu.id = ur.lifted_by
		WHERE ur.user_id = $1
		ORDER BY ur.created_at DESC`

	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("list restrictions by user: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var restrictions []model.Restriction
	for rows.Next() {
		var restriction model.Restriction
		if err := rows.Scan(
			&restriction.ID, &restriction.UserID, &restriction.RestrictionType,
			&restriction.Reason, &restriction.IssuedBy, &restriction.ExpiresAt,
			&restriction.LiftedAt, &restriction.LiftedBy, &restriction.CreatedAt,
			&restriction.IssuedByUsername, &restriction.LiftedByUsername,
		); err != nil {
			return nil, fmt.Errorf("scan restriction: %w", err)
		}
		restrictions = append(restrictions, restriction)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate restrictions: %w", err)
	}
	return restrictions, nil
}

func (r *RestrictionRepo) ListActive(ctx context.Context) ([]model.Restriction, error) {
	query := `SELECT ur.id, ur.user_id, ur.restriction_type, ur.reason, ur.issued_by,
		ur.expires_at, ur.lifted_at, ur.lifted_by, ur.created_at,
		COALESCE(iu.username, ''), COALESCE(lu.username, '')
		FROM user_restrictions ur
		LEFT JOIN users iu ON iu.id = ur.issued_by
		LEFT JOIN users lu ON lu.id = ur.lifted_by
		WHERE ur.lifted_at IS NULL
		ORDER BY ur.created_at DESC`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list active restrictions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var restrictions []model.Restriction
	for rows.Next() {
		var restriction model.Restriction
		if err := rows.Scan(
			&restriction.ID, &restriction.UserID, &restriction.RestrictionType,
			&restriction.Reason, &restriction.IssuedBy, &restriction.ExpiresAt,
			&restriction.LiftedAt, &restriction.LiftedBy, &restriction.CreatedAt,
			&restriction.IssuedByUsername, &restriction.LiftedByUsername,
		); err != nil {
			return nil, fmt.Errorf("scan restriction: %w", err)
		}
		restrictions = append(restrictions, restriction)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate restrictions: %w", err)
	}
	return restrictions, nil
}

func (r *RestrictionRepo) Lift(ctx context.Context, id int64, liftedBy *int64) error {
	query := `UPDATE user_restrictions SET lifted_at = NOW(), lifted_by = $1 WHERE id = $2 AND lifted_at IS NULL`
	result, err := r.db.ExecContext(ctx, query, liftedBy, id)
	if err != nil {
		return fmt.Errorf("lift restriction: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// DeleteExpired lifts all restrictions past their expiry and returns them so the
// caller can update user flags.
func (r *RestrictionRepo) DeleteExpired(ctx context.Context) ([]model.Restriction, error) {
	query := `UPDATE user_restrictions
		SET lifted_at = NOW()
		WHERE lifted_at IS NULL AND expires_at IS NOT NULL AND expires_at <= NOW()
		RETURNING id, user_id, restriction_type, reason, issued_by, expires_at, lifted_at, lifted_by, created_at`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("delete expired restrictions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var restrictions []model.Restriction
	for rows.Next() {
		var restriction model.Restriction
		if err := rows.Scan(
			&restriction.ID, &restriction.UserID, &restriction.RestrictionType,
			&restriction.Reason, &restriction.IssuedBy, &restriction.ExpiresAt,
			&restriction.LiftedAt, &restriction.LiftedBy, &restriction.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan expired restriction: %w", err)
		}
		restrictions = append(restrictions, restriction)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate expired restrictions: %w", err)
	}
	return restrictions, nil
}
