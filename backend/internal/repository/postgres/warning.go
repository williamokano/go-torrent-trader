package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// WarningRepo implements repository.WarningRepository using PostgreSQL.
type WarningRepo struct {
	db *sql.DB
}

// NewWarningRepo returns a new PostgreSQL-backed WarningRepository.
func NewWarningRepo(db *sql.DB) repository.WarningRepository {
	return &WarningRepo{db: db}
}

func (r *WarningRepo) Create(ctx context.Context, w *model.Warning) error {
	query := `INSERT INTO warnings (user_id, type, reason, issued_by, status, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at, updated_at`
	err := r.db.QueryRowContext(ctx, query,
		w.UserID, w.Type, w.Reason, w.IssuedBy, w.Status, w.ExpiresAt,
	).Scan(&w.ID, &w.CreatedAt, &w.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create warning: %w", err)
	}
	return nil
}

func (r *WarningRepo) GetByID(ctx context.Context, id int64) (*model.Warning, error) {
	query := `SELECT w.id, w.user_id, w.type, w.reason, w.issued_by, w.status,
		w.lifted_at, w.lifted_by, w.lifted_reason, w.expires_at, w.created_at, w.updated_at,
		COALESCE(u.username, '') AS username,
		ib.username AS issued_by_name,
		lb.username AS lifted_by_name
		FROM warnings w
		LEFT JOIN users u ON u.id = w.user_id
		LEFT JOIN users ib ON ib.id = w.issued_by
		LEFT JOIN users lb ON lb.id = w.lifted_by
		WHERE w.id = $1`
	return scanWarning(r.db.QueryRowContext(ctx, query, id))
}

func (r *WarningRepo) ListByUser(ctx context.Context, userID int64, includeInactive bool) ([]model.Warning, error) {
	where := "WHERE w.user_id = $1"
	if !includeInactive {
		where += " AND w.status = 'active'"
	}
	query := fmt.Sprintf(`SELECT w.id, w.user_id, w.type, w.reason, w.issued_by, w.status,
		w.lifted_at, w.lifted_by, w.lifted_reason, w.expires_at, w.created_at, w.updated_at,
		COALESCE(u.username, '') AS username,
		ib.username AS issued_by_name,
		lb.username AS lifted_by_name
		FROM warnings w
		LEFT JOIN users u ON u.id = w.user_id
		LEFT JOIN users ib ON ib.id = w.issued_by
		LEFT JOIN users lb ON lb.id = w.lifted_by
		%s ORDER BY w.created_at DESC`, where)

	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("list warnings by user: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanWarnings(rows)
}

func (r *WarningRepo) ListAll(ctx context.Context, opts repository.ListWarningsOptions) ([]model.Warning, int64, error) {
	page := opts.Page
	if page <= 0 {
		page = 1
	}
	perPage := opts.PerPage
	if perPage <= 0 {
		perPage = 25
	}
	if perPage > 100 {
		perPage = 100
	}

	where := "WHERE 1=1"
	args := []interface{}{}
	argIdx := 1

	if opts.UserID != nil {
		where += fmt.Sprintf(" AND w.user_id = $%d", argIdx)
		args = append(args, *opts.UserID)
		argIdx++
	}
	if opts.Status != nil && *opts.Status != "all" {
		where += fmt.Sprintf(" AND w.status = $%d", argIdx)
		args = append(args, *opts.Status)
		argIdx++
	}
	if opts.Search != "" {
		where += fmt.Sprintf(" AND u.username ILIKE $%d", argIdx)
		args = append(args, "%"+opts.Search+"%")
		argIdx++
	}

	// Count
	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM warnings w
		LEFT JOIN users u ON u.id = w.user_id %s`, where)
	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count warnings: %w", err)
	}

	// Fetch
	offset := (page - 1) * perPage
	query := fmt.Sprintf(`SELECT w.id, w.user_id, w.type, w.reason, w.issued_by, w.status,
		w.lifted_at, w.lifted_by, w.lifted_reason, w.expires_at, w.created_at, w.updated_at,
		COALESCE(u.username, '') AS username,
		ib.username AS issued_by_name,
		lb.username AS lifted_by_name
		FROM warnings w
		LEFT JOIN users u ON u.id = w.user_id
		LEFT JOIN users ib ON ib.id = w.issued_by
		LEFT JOIN users lb ON lb.id = w.lifted_by
		%s ORDER BY w.created_at DESC LIMIT $%d OFFSET $%d`,
		where, argIdx, argIdx+1)
	args = append(args, perPage, offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list warnings: %w", err)
	}
	defer func() { _ = rows.Close() }()

	warnings, err := scanWarnings(rows)
	if err != nil {
		return nil, 0, err
	}

	return warnings, total, nil
}

func (r *WarningRepo) Update(ctx context.Context, w *model.Warning) error {
	query := `UPDATE warnings SET
		status = $1, lifted_at = $2, lifted_by = $3, lifted_reason = $4,
		expires_at = $5, updated_at = NOW()
		WHERE id = $6`
	res, err := r.db.ExecContext(ctx, query,
		w.Status, w.LiftedAt, w.LiftedBy, w.LiftedReason,
		w.ExpiresAt, w.ID,
	)
	if err != nil {
		return fmt.Errorf("update warning: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("update warning rows affected: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *WarningRepo) CountActiveByUser(ctx context.Context, userID int64) (int, error) {
	var count int
	query := `SELECT COUNT(*) FROM warnings WHERE user_id = $1 AND status = 'active'`
	if err := r.db.QueryRowContext(ctx, query, userID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count active warnings: %w", err)
	}
	return count, nil
}

func (r *WarningRepo) GetActiveRatioWarning(ctx context.Context, userID int64) (*model.Warning, error) {
	query := `SELECT w.id, w.user_id, w.type, w.reason, w.issued_by, w.status,
		w.lifted_at, w.lifted_by, w.lifted_reason, w.expires_at, w.created_at, w.updated_at,
		COALESCE(u.username, '') AS username,
		ib.username AS issued_by_name,
		lb.username AS lifted_by_name
		FROM warnings w
		LEFT JOIN users u ON u.id = w.user_id
		LEFT JOIN users ib ON ib.id = w.issued_by
		LEFT JOIN users lb ON lb.id = w.lifted_by
		WHERE w.user_id = $1 AND w.status = 'active' AND w.type = 'ratio_soft'
		ORDER BY w.created_at DESC LIMIT 1`
	w, err := scanWarning(r.db.QueryRowContext(ctx, query, userID))
	if err != nil {
		return nil, err
	}
	return w, nil
}

func (r *WarningRepo) GetUsersWithLowRatio(ctx context.Context, threshold float64, minDownloaded int64) ([]model.User, error) {
	// Find enabled users whose ratio is below threshold and who have downloaded enough.
	// ratio = uploaded / downloaded (avoid division by zero).
	query := `SELECT id, username, email, password_hash, password_scheme, passkey,
		group_id, uploaded, downloaded, avatar, title, info,
		enabled, parked, ip, last_login, last_access, invites,
		warned, warn_until, donor, invited_by, created_at, updated_at
		FROM users
		WHERE enabled = true
		  AND downloaded > $1
		  AND (uploaded::float / downloaded::float) < $2`
	rows, err := r.db.QueryContext(ctx, query, minDownloaded, threshold)
	if err != nil {
		return nil, fmt.Errorf("get users with low ratio: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var users []model.User
	for rows.Next() {
		var u model.User
		if err := rows.Scan(
			&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.PasswordScheme, &u.Passkey,
			&u.GroupID, &u.Uploaded, &u.Downloaded, &u.Avatar, &u.Title, &u.Info,
			&u.Enabled, &u.Parked, &u.IP, &u.LastLogin, &u.LastAccess, &u.Invites,
			&u.Warned, &u.WarnUntil, &u.Donor, &u.InvitedBy, &u.CreatedAt, &u.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate users: %w", err)
	}
	return users, nil
}

func scanWarning(row interface{ Scan(...any) error }) (*model.Warning, error) {
	var w model.Warning
	err := row.Scan(
		&w.ID, &w.UserID, &w.Type, &w.Reason, &w.IssuedBy, &w.Status,
		&w.LiftedAt, &w.LiftedBy, &w.LiftedReason, &w.ExpiresAt,
		&w.CreatedAt, &w.UpdatedAt,
		&w.Username, &w.IssuedByName, &w.LiftedByName,
	)
	if err != nil {
		return nil, err
	}
	return &w, nil
}

func scanWarnings(rows *sql.Rows) ([]model.Warning, error) {
	var warnings []model.Warning
	for rows.Next() {
		var w model.Warning
		if err := rows.Scan(
			&w.ID, &w.UserID, &w.Type, &w.Reason, &w.IssuedBy, &w.Status,
			&w.LiftedAt, &w.LiftedBy, &w.LiftedReason, &w.ExpiresAt,
			&w.CreatedAt, &w.UpdatedAt,
			&w.Username, &w.IssuedByName, &w.LiftedByName,
		); err != nil {
			return nil, fmt.Errorf("scan warning: %w", err)
		}
		warnings = append(warnings, w)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate warnings: %w", err)
	}
	return warnings, nil
}

// Ensure compile-time interface satisfaction.
var _ repository.WarningRepository = (*WarningRepo)(nil)
