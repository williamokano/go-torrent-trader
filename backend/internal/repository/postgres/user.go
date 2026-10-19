package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// userColumns lists every column in the users table in scan order.
// bonus_points is read here but deliberately NOT written by Create or
// UpdateFields: all writes to it go through BonusRepo's atomic statements, so
// no other write can clobber points awarded concurrently by the bonus worker.
const userColumns = `id, username, email, password_hash, password_scheme, passkey,
	group_id, uploaded, downloaded, avatar, title, info, enabled, parked,
	ip, last_login, last_access, invites, bonus_points, warned, warn_until, donor,
	invited_by, can_download, can_upload, can_chat, can_forum, can_invite, can_feed, disabled_until,
	activated_at, awaiting_confirmation, created_at, updated_at`

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
		&u.InvitedBy, &u.CanDownload, &u.CanUpload, &u.CanChat, &u.CanForum, &u.CanInvite, &u.CanFeed,
		&u.DisabledUntil, &u.ActivatedAt, &u.AwaitingConfirmation, &u.CreatedAt, &u.UpdatedAt,
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

func (r *UserRepo) ConfirmPendingEmail(ctx context.Context, id int64, at time.Time) (bool, error) {
	res, err := r.db.ExecContext(ctx, `UPDATE users SET
		enabled = true, awaiting_confirmation = false,
		activated_at = COALESCE(activated_at, $2), updated_at = NOW()
	WHERE id = $1 AND awaiting_confirmation`, id, at)
	if err != nil {
		return false, fmt.Errorf("confirm pending email: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("confirm pending email: rows affected: %w", err)
	}
	return n == 1, nil
}

func (r *UserRepo) Create(ctx context.Context, user *model.User) error {
	return insertUserRow(ctx, r.db, user)
}

// insertUserRow is the users INSERT, against the pool or an open transaction
// (InviteRepo.CreateUserRedeeming runs it alongside the invite redemption).
func insertUserRow(ctx context.Context, q DBTX, user *model.User) error {
	query := `INSERT INTO users (
		username, email, password_hash, password_scheme, passkey,
		group_id, uploaded, downloaded, avatar, title, info, enabled, parked,
		ip, last_login, last_access, invites, warned, warn_until, donor, invited_by,
		can_download, can_upload, can_chat, can_forum, can_invite, can_feed, disabled_until, activated_at,
		awaiting_confirmation
	) VALUES (
		$1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
		$11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21,
		$22, $23, $24, $25, $26, $27, $28, $29, $30
	) RETURNING id, created_at, updated_at`

	return q.QueryRowContext(ctx, query,
		user.Username, user.Email, user.PasswordHash, user.PasswordScheme, user.Passkey,
		user.GroupID, user.Uploaded, user.Downloaded, user.Avatar, user.Title,
		user.Info, user.Enabled, user.Parked, user.IP, user.LastLogin,
		user.LastAccess, user.Invites, user.Warned, user.WarnUntil, user.Donor,
		user.InvitedBy, user.CanDownload, user.CanUpload, user.CanChat, user.CanForum, user.CanInvite, user.CanFeed, user.DisabledUntil,
		user.ActivatedAt, user.AwaitingConfirmation,
	).Scan(&user.ID, &user.CreatedAt, &user.UpdatedAt)
}

// UpdateFields writes the columns the patch sets, and nothing else. Columns
// with their own atomic writers — the privilege flags (SetPrivilegeFlag),
// invites (AdjustInvites/SetInvites), stats (IncrementStats/SetStats) and
// bonus points (BonusRepo) — are not in UserPatch at all.
func (r *UserRepo) UpdateFields(ctx context.Context, id int64, p model.UserPatch) error {
	return updateUserFields(ctx, r.db, id, p)
}

// UpdateFieldsWithHistory is UpdateFields plus an audit trail: the write and
// the user_edit_history rows describing it are committed in one transaction,
// so a failed audit insert rolls back the update instead of leaving a
// persisted change with no trail. A nil or empty entries slice skips the
// audit insert but still performs the update.
func (r *UserRepo) UpdateFieldsWithHistory(ctx context.Context, id int64, p model.UserPatch, entries []model.UserEditHistory) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("update user with history: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := updateUserFields(ctx, tx, id, p); err != nil {
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

// LiftExpiredBans re-enables every account whose timed ban ended before now,
// writing what model.UserPatch.SetEnabled(true) writes — keep the two alike;
// checking that as each row is written: an account whose ban was made
// permanent (disabled_until cleared) after any earlier read is not touched.
// Lifting a ban also counts as activation, as enabling does everywhere.
func (r *UserRepo) LiftExpiredBans(ctx context.Context, now time.Time) ([]model.User, error) {
	rows, err := r.db.QueryContext(ctx, `
		UPDATE users SET
			enabled = true, awaiting_confirmation = false,
			activated_at = COALESCE(activated_at, $1),
			disabled_until = NULL, updated_at = NOW()
		WHERE enabled = false AND disabled_until IS NOT NULL AND disabled_until < $1
		RETURNING id, username`, now)
	if err != nil {
		return nil, fmt.Errorf("lift expired bans: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var lifted []model.User
	for rows.Next() {
		var u model.User
		if err := rows.Scan(&u.ID, &u.Username); err != nil {
			return nil, fmt.Errorf("lift expired bans: scan: %w", err)
		}
		lifted = append(lifted, u)
	}
	return lifted, rows.Err()
}

// setBuilder collects the SET clauses and arguments of an UPDATE.
type setBuilder struct {
	sets []string
	args []any
}

// setColumn adds "column = $n" when the patch field is set.
func setColumn[T any](b *setBuilder, column string, f model.Set[T]) {
	if !f.Valid {
		return
	}
	b.args = append(b.args, f.Value)
	b.sets = append(b.sets, fmt.Sprintf("%s = $%d", column, len(b.args)))
}

// updateUserFields builds the UPDATE for a patch. Column names come from the
// fixed list below, never from input, so the Sprintf is not an injection
// risk. It returns sql.ErrNoRows for an unknown id; an empty patch writes,
// and checks, nothing.
func updateUserFields(ctx context.Context, q DBTX, id int64, p model.UserPatch) error {
	sets, args := userPatchSets(p)
	if len(sets) == 0 {
		return nil
	}

	args = append(args, id)
	query := fmt.Sprintf("UPDATE users SET %s, updated_at = NOW() WHERE id = $%d",
		strings.Join(sets, ", "), len(args))
	res, err := q.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// userPatchSets turns a patch into SET clauses: one per field the patch
// sets. The column list must match model.UserPatch field for field; a test
// checks it does.
func userPatchSets(p model.UserPatch) ([]string, []any) {
	var b setBuilder
	setColumn(&b, "username", p.Username)
	setColumn(&b, "email", p.Email)
	setColumn(&b, "password_hash", p.PasswordHash)
	setColumn(&b, "password_scheme", p.PasswordScheme)
	setColumn(&b, "passkey", p.Passkey)
	setColumn(&b, "group_id", p.GroupID)
	setColumn(&b, "avatar", p.Avatar)
	setColumn(&b, "title", p.Title)
	setColumn(&b, "info", p.Info)
	setColumn(&b, "enabled", p.Enabled())
	setColumn(&b, "parked", p.Parked)
	setColumn(&b, "warned", p.Warned)
	setColumn(&b, "warn_until", p.WarnUntil)
	setColumn(&b, "donor", p.Donor)
	setColumn(&b, "ip", p.IP)
	setColumn(&b, "last_login", p.LastLogin)
	setColumn(&b, "disabled_until", p.DisabledUntil())
	if p.EndsConfirmation() {
		b.sets = append(b.sets, "awaiting_confirmation = false")
	}
	if p.ActivateAt.Valid {
		b.args = append(b.args, p.ActivateAt.Value)
		b.sets = append(b.sets, fmt.Sprintf("activated_at = COALESCE(activated_at, $%d)", len(b.args)))
	}
	return b.sets, b.args
}

// privilegeFlagColumn maps a validated restriction type to its users table
// column. The switch is closed over a fixed set of known columns — never
// built from caller input — so the fmt.Sprintf below is not a
// SQL-injection risk.
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
	case model.RestrictionTypeFeed:
		return "can_feed", true
	case model.RestrictionTypeForum:
		return "can_forum", true
	default:
		return "", false
	}
}

// SetPrivilegeFlag atomically sets a single privilege column for a user,
// outside UserPatch. This is the only path that may change
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
// adjusted (BonusRepo.AwardPoints). InviteDistributionService's auto-grant
// uses this instead of a read-modify-write, so it (and any patch, which has
// no invites field) can never clobber another write to this column. Spending
// an invite goes through InviteRepo.CreateInviteSpending instead.
func (r *UserRepo) AdjustInvites(ctx context.Context, userID int64, delta int64) error {
	// Defence in depth for a negative delta: one that would take the balance
	// below zero changes nothing (the schema refuses it too).
	res, err := r.db.ExecContext(ctx,
		`UPDATE users SET invites = invites + $1, updated_at = NOW() WHERE id = $2 AND invites + $1 >= 0`, delta, userID)
	if err != nil {
		return fmt.Errorf("adjust invites: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("adjust invites rows affected: %w", err)
	}
	if n == 0 {
		var exists bool
		if err := r.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE id = $1)`, userID).Scan(&exists); err != nil {
			return fmt.Errorf("adjust invites: check user: %w", err)
		}
		if exists {
			return repository.ErrInsufficientInvites
		}
		return sql.ErrNoRows
	}
	return nil
}

// SetInvites atomically sets a user's invite balance to an absolute value —
// UPDATE users SET invites = $1, outside UserPatch. This is
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

// SetStats atomically sets a user's uploaded/downloaded counters — outside
// UserPatch, which has no stats fields — for AdminService's admin edit
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

// CanFeed reports the user's own live-feed flag and their group, in one read.
//
// A narrow query rather than GetByID: this runs on every live-feed connect and
// on every feed listing, and it has no business loading a whole user row —
// including the password hash — to answer a boolean.
func (r *UserRepo) CanFeed(ctx context.Context, userID int64) (bool, int64, error) {
	var canFeed bool
	var groupID int64
	err := r.db.QueryRowContext(ctx,
		`SELECT can_feed, group_id FROM users WHERE id = $1`, userID,
	).Scan(&canFeed, &groupID)
	if err != nil {
		return false, 0, fmt.Errorf("read user feed access: %w", err)
	}
	return canFeed, groupID, nil
}
