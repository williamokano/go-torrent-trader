package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// userColumns lists every column in the users table in scan order.
const userColumns = `id, username, email, password_hash, password_scheme, passkey,
	group_id, uploaded, downloaded, avatar, title, info, enabled, parked,
	ip, last_login, last_access, invites, warned, warn_until, donor,
	created_at, updated_at`

// UserRepo implements repository.UserRepository using PostgreSQL.
type UserRepo struct {
	db *sql.DB
}

// NewUserRepo returns a new PostgreSQL-backed UserRepository.
func NewUserRepo(db *sql.DB) repository.UserRepository {
	return &UserRepo{db: db}
}

func scanUser(row interface{ Scan(...any) error }) (*model.User, error) {
	var u model.User
	err := row.Scan(
		&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.PasswordScheme, &u.Passkey,
		&u.GroupID, &u.Uploaded, &u.Downloaded, &u.Avatar, &u.Title, &u.Info,
		&u.Enabled, &u.Parked, &u.IP, &u.LastLogin, &u.LastAccess,
		&u.Invites, &u.Warned, &u.WarnUntil, &u.Donor,
		&u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *UserRepo) GetByID(ctx context.Context, id int64) (*model.User, error) {
	query := fmt.Sprintf("SELECT %s FROM users WHERE id = $1", userColumns)
	return scanUser(r.db.QueryRowContext(ctx, query, id))
}

func (r *UserRepo) GetByUsername(ctx context.Context, username string) (*model.User, error) {
	query := fmt.Sprintf("SELECT %s FROM users WHERE username = $1", userColumns)
	return scanUser(r.db.QueryRowContext(ctx, query, username))
}

func (r *UserRepo) GetByEmail(ctx context.Context, email string) (*model.User, error) {
	query := fmt.Sprintf("SELECT %s FROM users WHERE email = $1", userColumns)
	return scanUser(r.db.QueryRowContext(ctx, query, email))
}

func (r *UserRepo) GetByPasskey(ctx context.Context, passkey string) (*model.User, error) {
	query := fmt.Sprintf("SELECT %s FROM users WHERE passkey = $1", userColumns)
	return scanUser(r.db.QueryRowContext(ctx, query, passkey))
}

func (r *UserRepo) Count(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&count)
	return count, err
}

func (r *UserRepo) Create(ctx context.Context, user *model.User) error {
	query := `INSERT INTO users (
		username, email, password_hash, password_scheme, passkey,
		group_id, uploaded, downloaded, avatar, title, info, enabled, parked,
		ip, last_login, last_access, invites, warned, warn_until, donor
	) VALUES (
		$1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
		$11, $12, $13, $14, $15, $16, $17, $18, $19, $20
	) RETURNING id, created_at, updated_at`

	return r.db.QueryRowContext(ctx, query,
		user.Username, user.Email, user.PasswordHash, user.PasswordScheme, user.Passkey,
		user.GroupID, user.Uploaded, user.Downloaded, user.Avatar, user.Title,
		user.Info, user.Enabled, user.Parked, user.IP, user.LastLogin,
		user.LastAccess, user.Invites, user.Warned, user.WarnUntil, user.Donor,
	).Scan(&user.ID, &user.CreatedAt, &user.UpdatedAt)
}

func (r *UserRepo) Update(ctx context.Context, user *model.User) error {
	query := `UPDATE users SET
		username = $1, email = $2, password_hash = $3, password_scheme = $4,
		passkey = $5, group_id = $6, uploaded = $7, downloaded = $8,
		avatar = $9, title = $10, info = $11, enabled = $12, parked = $13,
		ip = $14, last_login = $15, last_access = $16, invites = $17,
		warned = $18, warn_until = $19, donor = $20, updated_at = NOW()
	WHERE id = $21
	RETURNING updated_at`

	return r.db.QueryRowContext(ctx, query,
		user.Username, user.Email, user.PasswordHash, user.PasswordScheme,
		user.Passkey, user.GroupID, user.Uploaded, user.Downloaded,
		user.Avatar, user.Title, user.Info, user.Enabled, user.Parked,
		user.IP, user.LastLogin, user.LastAccess, user.Invites,
		user.Warned, user.WarnUntil, user.Donor, user.ID,
	).Scan(&user.UpdatedAt)
}

func (r *UserRepo) List(ctx context.Context, opts repository.ListUsersOptions) ([]model.User, int64, error) {
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

	if opts.Search != "" {
		where += fmt.Sprintf(" AND (username ILIKE '%%' || $%d || '%%' OR email ILIKE '%%' || $%d || '%%')", argIdx, argIdx)
		args = append(args, opts.Search)
		argIdx++
	}
	if opts.GroupID != nil {
		where += fmt.Sprintf(" AND group_id = $%d", argIdx)
		args = append(args, *opts.GroupID)
		argIdx++
	}
	if opts.Enabled != nil {
		where += fmt.Sprintf(" AND enabled = $%d", argIdx)
		args = append(args, *opts.Enabled)
		argIdx++
	}

	// Count
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM users %s", where)
	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count users: %w", err)
	}

	// Sort
	sortCol := "created_at"
	switch opts.SortBy {
	case "username", "uploaded", "downloaded", "created_at":
		sortCol = opts.SortBy
	}
	sortDir := "DESC"
	if opts.SortOrder == "asc" {
		sortDir = "ASC"
	}

	offset := (page - 1) * perPage
	query := fmt.Sprintf("SELECT %s FROM users %s ORDER BY %s %s LIMIT $%d OFFSET $%d",
		userColumns, where, sortCol, sortDir, argIdx, argIdx+1)
	args = append(args, perPage, offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list users: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var users []model.User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, *u)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate users: %w", err)
	}

	return users, total, nil
}

// ListStaff returns users whose group has is_admin=true or is_moderator=true.
func (r *UserRepo) ListStaff(ctx context.Context) ([]model.User, error) {
	query := fmt.Sprintf(`SELECT %s FROM users
		WHERE group_id IN (SELECT id FROM groups WHERE is_admin = true OR is_moderator = true)
		ORDER BY group_id ASC, username ASC`, userColumns)

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list staff: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var users []model.User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, fmt.Errorf("scan staff user: %w", err)
		}
		users = append(users, *u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate staff: %w", err)
	}

	return users, nil
}

func (r *UserRepo) IncrementStats(ctx context.Context, id int64, uploadedDelta, downloadedDelta int64) error {
	query := `UPDATE users SET
		uploaded = GREATEST(0, uploaded + $1),
		downloaded = GREATEST(0, downloaded + $2),
		updated_at = NOW()
	WHERE id = $3`
	result, err := r.db.ExecContext(ctx, query, uploadedDelta, downloadedDelta, id)
	if err != nil {
		return fmt.Errorf("incrementing user stats: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
