package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// userColumns lists every column in the users table in scan order.
// bonus_points is read here but deliberately NOT written by Create/Update:
// all writes to it go through BonusRepo's atomic statements, so the full-row
// Update can never clobber points awarded concurrently by the bonus worker.
const userColumns = `id, username, email, password_hash, password_scheme, passkey,
	group_id, uploaded, downloaded, avatar, title, info, enabled, parked,
	ip, last_login, last_access, invites, bonus_points, warned, warn_until, donor,
	invited_by, can_download, can_upload, can_chat, can_forum, can_invite, disabled_until,
	activated_at, created_at, updated_at`

// UserRepo implements repository.UserRepository using PostgreSQL.
type UserRepo struct {
	db *sql.DB
}

// NewUserRepo returns a new PostgreSQL-backed UserRepository. The concrete
// type is returned (rather than the repository.UserRepository interface) so
// callers that need SetPrivilegeFlag — a method deliberately kept off the
// widely-implemented UserRepository interface — can use it without a type
// assertion; it still satisfies repository.UserRepository wherever that
// narrower interface is expected.
func NewUserRepo(db *sql.DB) *UserRepo {
	return &UserRepo{db: db}
}

func scanUser(row interface{ Scan(...any) error }) (*model.User, error) {
	var u model.User
	err := row.Scan(
		&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.PasswordScheme, &u.Passkey,
		&u.GroupID, &u.Uploaded, &u.Downloaded, &u.Avatar, &u.Title, &u.Info,
		&u.Enabled, &u.Parked, &u.IP, &u.LastLogin, &u.LastAccess,
		&u.Invites, &u.BonusPoints, &u.Warned, &u.WarnUntil, &u.Donor,
		&u.InvitedBy, &u.CanDownload, &u.CanUpload, &u.CanChat, &u.CanForum, &u.CanInvite,
		&u.DisabledUntil, &u.ActivatedAt, &u.CreatedAt, &u.UpdatedAt,
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

// GetByUsernames resolves multiple usernames in one round trip via
// `= ANY($1)`. The Go []string is passed directly — the pgx v5 stdlib driver
// encodes it as a native Postgres array on its own; no pq.Array()-style
// wrapper (that's a lib/pq-only helper) is needed or correct here.
func (r *UserRepo) GetByUsernames(ctx context.Context, usernames []string) ([]model.User, error) {
	if len(usernames) == 0 {
		return nil, nil
	}

	query := fmt.Sprintf("SELECT %s FROM users WHERE username = ANY($1)", userColumns)
	rows, err := r.db.QueryContext(ctx, query, usernames)
	if err != nil {
		return nil, fmt.Errorf("get users by usernames: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var users []model.User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, *u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate users: %w", err)
	}
	return users, nil
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
		ip, last_login, last_access, invites, warned, warn_until, donor, invited_by,
		can_download, can_upload, can_chat, can_forum, can_invite, disabled_until, activated_at
	) VALUES (
		$1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
		$11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21,
		$22, $23, $24, $25, $26, $27, $28
	) RETURNING id, created_at, updated_at`

	return r.db.QueryRowContext(ctx, query,
		user.Username, user.Email, user.PasswordHash, user.PasswordScheme, user.Passkey,
		user.GroupID, user.Uploaded, user.Downloaded, user.Avatar, user.Title,
		user.Info, user.Enabled, user.Parked, user.IP, user.LastLogin,
		user.LastAccess, user.Invites, user.Warned, user.WarnUntil, user.Donor,
		user.InvitedBy, user.CanDownload, user.CanUpload, user.CanChat, user.CanForum, user.CanInvite, user.DisabledUntil,
		user.ActivatedAt,
	).Scan(&user.ID, &user.CreatedAt, &user.UpdatedAt)
}

// Update writes every user-editable column except can_download, can_upload,
// can_chat, can_invite, invites, uploaded, and downloaded. The four privilege
// flags are governed by RestrictionService; invites is governed by
// InviteService/InviteDistributionService's AdjustInvites and AdminService's
// SetInvites; uploaded/downloaded are governed by IncrementStats (announce
// accrual) and AdminService's SetStats. All seven are deliberately excluded
// here — mirroring how bonus_points is excluded — so a stale read-modify-write
// elsewhere (login's LastLogin write, an admin profile save) can never clobber
// a privilege flag changed concurrently by a restriction apply/lift, an invite
// balance changed concurrently by a grant/decrement, or stats accrued
// concurrently by an announce. Use SetPrivilegeFlag, AdjustInvites,
// SetInvites, or SetStats to change one of those columns.
func (r *UserRepo) Update(ctx context.Context, user *model.User) error {
	return updateUserRow(ctx, r.db, user)
}

// UpdateWithHistory is Update plus an audit trail: the full-row write and the
// user_edit_history rows describing it are committed in a single transaction,
// so a failed audit insert rolls back the update instead of leaving a
// persisted change with no trail. This is AdminService's path for the profile
// fields it diffs (username, email, group, enabled, warned, ...); a nil or
// empty entries slice skips the audit insert but still performs the update.
func (r *UserRepo) UpdateWithHistory(ctx context.Context, user *model.User, entries []model.UserEditHistory) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("update user with history: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := updateUserRow(ctx, tx, user); err != nil {
		return fmt.Errorf("update user with history: %w", err)
	}
	if err := insertEditHistoryTx(ctx, tx, entries); err != nil {
		return fmt.Errorf("update user with history: record: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("update user with history: commit: %w", err)
	}
	return nil
}

// updateUserRow runs the full-row UPDATE against either the pool or an
// open transaction (DBTX is satisfied by both *sql.DB and *sql.Tx), so
// Update and UpdateWithHistory share one query instead of drifting apart.
func updateUserRow(ctx context.Context, q DBTX, user *model.User) error {
	query := `UPDATE users SET
		username = $1, email = $2, password_hash = $3, password_scheme = $4,
		passkey = $5, group_id = $6, avatar = $7, title = $8, info = $9,
		enabled = $10, parked = $11, ip = $12, last_login = $13, last_access = $14,
		warned = $15, warn_until = $16, donor = $17, invited_by = $18,
		can_forum = $19, disabled_until = $20, activated_at = $21, updated_at = NOW()
	WHERE id = $22
	RETURNING updated_at`

	return q.QueryRowContext(ctx, query,
		user.Username, user.Email, user.PasswordHash, user.PasswordScheme,
		user.Passkey, user.GroupID, user.Avatar, user.Title, user.Info,
		user.Enabled, user.Parked, user.IP, user.LastLogin, user.LastAccess,
		user.Warned, user.WarnUntil, user.Donor, user.InvitedBy,
		user.CanForum, user.DisabledUntil, user.ActivatedAt, user.ID,
	).Scan(&user.UpdatedAt)
}

// privilegeFlagColumn maps a validated restriction type to its users table
// column. The switch is closed over four known columns — never built from
// caller input — so the fmt.Sprintf below is not a SQL-injection risk.
func privilegeFlagColumn(restrictionType string) (string, bool) {
	switch restrictionType {
	case model.RestrictionTypeDownload:
		return "can_download", true
	case model.RestrictionTypeUpload:
		return "can_upload", true
	case model.RestrictionTypeChat:
		return "can_chat", true
	case model.RestrictionTypeInvite:
		return "can_invite", true
	default:
		return "", false
	}
}

// SetPrivilegeFlag atomically sets a single privilege column for a user,
// bypassing Update's full-row write. This is the only path that may change
// can_download, can_upload, can_chat, or can_invite.
func (r *UserRepo) SetPrivilegeFlag(ctx context.Context, userID int64, restrictionType string, value bool) error {
	column, ok := privilegeFlagColumn(restrictionType)
	if !ok {
		return fmt.Errorf("unknown privilege flag: %s", restrictionType)
	}

	query := fmt.Sprintf(`UPDATE users SET %s = $1, updated_at = NOW() WHERE id = $2`, column)
	res, err := r.db.ExecContext(ctx, query, value, userID)
	if err != nil {
		return fmt.Errorf("set privilege flag: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("set privilege flag rows affected: %w", err)
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// AdjustInvites atomically applies a delta to a user's invite balance —
// UPDATE users SET invites = invites + $1 — mirroring how bonus_points is
// adjusted (BonusRepo.AwardPoints). InviteService.CreateInvite's decrement and
// InviteDistributionService's auto-grant both use this instead of a
// read-modify-write through Update, so those two paths (and any concurrent
// full-row Update, which no longer touches invites at all) can never clobber
// each other's write to this column.
func (r *UserRepo) AdjustInvites(ctx context.Context, userID int64, delta int64) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE users SET invites = invites + $1, updated_at = NOW() WHERE id = $2`, delta, userID)
	if err != nil {
		return fmt.Errorf("adjust invites: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("adjust invites rows affected: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// SetInvites atomically sets a user's invite balance to an absolute value —
// UPDATE users SET invites = $1 — bypassing Update's full-row write. This is
// AdminService's path for an admin's explicit "set invites to N" edit; unlike
// AdjustInvites it is not a delta, so two racing calls are a plain
// last-write-wins on the same column (the admin's intended target value),
// not a stale-read clobber of an unrelated concurrent change. The invites
// write and its audit entries commit in one transaction — its only caller
// (AdminService) always wants both or neither. A nil or empty entries slice
// skips the audit insert but still sets the balance.
func (r *UserRepo) SetInvites(ctx context.Context, userID int64, invites int, entries []model.UserEditHistory) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("set invites: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx,
		`UPDATE users SET invites = $1, updated_at = NOW() WHERE id = $2`, invites, userID)
	if err != nil {
		return fmt.Errorf("set invites: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("set invites rows affected: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}

	if err := insertEditHistoryTx(ctx, tx, entries); err != nil {
		return fmt.Errorf("set invites: record: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("set invites: commit: %w", err)
	}
	return nil
}

// SetStats atomically sets a user's uploaded/downloaded counters — bypassing
// Update's (now stats-free) full-row write — for AdminService's admin edit
// path only; announce accrual continues to go through IncrementStats. Each
// of uploaded and downloaded is independently optional (COALESCE keeps the
// current DB value when nil): an admin edit that only touches one of the two
// must not overwrite the other with a value read at request start, which
// would reopen exactly the clobber window this method exists to close if the
// admin edits, say, uploaded while an announce concurrently accrues
// downloaded. The write and its audit entries commit in one transaction.
func (r *UserRepo) SetStats(ctx context.Context, userID int64, uploaded, downloaded *int64, entries []model.UserEditHistory) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("set stats: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx,
		`UPDATE users SET
			uploaded = COALESCE($1, uploaded),
			downloaded = COALESCE($2, downloaded),
			updated_at = NOW()
		WHERE id = $3`, uploaded, downloaded, userID)
	if err != nil {
		return fmt.Errorf("set stats: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("set stats rows affected: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}

	if err := insertEditHistoryTx(ctx, tx, entries); err != nil {
		return fmt.Errorf("set stats: record: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("set stats: commit: %w", err)
	}
	return nil
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
	if opts.DisabledUntilBefore != nil {
		where += fmt.Sprintf(" AND disabled_until IS NOT NULL AND disabled_until < $%d", argIdx)
		args = append(args, *opts.DisabledUntilBefore)
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

func (r *UserRepo) UpdateLastAccess(ctx context.Context, id int64) error {
	query := `UPDATE users SET last_access = NOW(), updated_at = NOW() WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("updating last access: %w", err)
	}
	return nil
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
