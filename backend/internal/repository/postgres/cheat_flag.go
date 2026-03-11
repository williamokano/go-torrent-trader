package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// CheatFlagRepo implements repository.CheatFlagRepository.
type CheatFlagRepo struct {
	db *sql.DB
}

// NewCheatFlagRepo creates a new CheatFlagRepo.
func NewCheatFlagRepo(db *sql.DB) repository.CheatFlagRepository {
	return &CheatFlagRepo{db: db}
}

// Create inserts a new cheat flag.
func (r *CheatFlagRepo) Create(ctx context.Context, flag *model.CheatFlag) error {
	query := `INSERT INTO cheat_flags (user_id, torrent_id, flag_type, details)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at`
	return r.db.QueryRowContext(ctx, query,
		flag.UserID, flag.TorrentID, flag.FlagType, flag.Details,
	).Scan(&flag.ID, &flag.CreatedAt)
}

// GetByID returns a cheat flag by ID.
func (r *CheatFlagRepo) GetByID(ctx context.Context, id int64) (*model.CheatFlag, error) {
	query := `SELECT cf.id, cf.user_id, cf.torrent_id, cf.flag_type, cf.details,
		cf.dismissed, cf.dismissed_by, cf.dismissed_at, cf.created_at,
		u.username,
		COALESCE(t.name, ''),
		COALESCE(du.username, '')
		FROM cheat_flags cf
		JOIN users u ON u.id = cf.user_id
		LEFT JOIN torrents t ON t.id = cf.torrent_id
		LEFT JOIN users du ON du.id = cf.dismissed_by
		WHERE cf.id = $1`

	var f model.CheatFlag
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&f.ID, &f.UserID, &f.TorrentID, &f.FlagType, &f.Details,
		&f.Dismissed, &f.DismissedBy, &f.DismissedAt, &f.CreatedAt,
		&f.Username, &f.TorrentName, &f.DismisserName,
	)
	if err != nil {
		return nil, fmt.Errorf("get cheat flag: %w", err)
	}
	return &f, nil
}

// List returns cheat flags with pagination and filters.
func (r *CheatFlagRepo) List(ctx context.Context, opts repository.ListCheatFlagsOptions) ([]model.CheatFlag, int64, error) {
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
		where += fmt.Sprintf(" AND cf.user_id = $%d", argIdx)
		args = append(args, *opts.UserID)
		argIdx++
	}
	if opts.FlagType != nil {
		where += fmt.Sprintf(" AND cf.flag_type = $%d", argIdx)
		args = append(args, *opts.FlagType)
		argIdx++
	}
	if opts.Dismissed != nil {
		where += fmt.Sprintf(" AND cf.dismissed = $%d", argIdx)
		args = append(args, *opts.Dismissed)
		argIdx++
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM cheat_flags cf %s", where)
	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count cheat flags: %w", err)
	}

	offset := (page - 1) * perPage
	query := fmt.Sprintf(`SELECT cf.id, cf.user_id, cf.torrent_id, cf.flag_type, cf.details,
		cf.dismissed, cf.dismissed_by, cf.dismissed_at, cf.created_at,
		u.username,
		COALESCE(t.name, ''),
		COALESCE(du.username, '')
		FROM cheat_flags cf
		JOIN users u ON u.id = cf.user_id
		LEFT JOIN torrents t ON t.id = cf.torrent_id
		LEFT JOIN users du ON du.id = cf.dismissed_by
		%s
		ORDER BY cf.created_at DESC
		LIMIT $%d OFFSET $%d`, where, argIdx, argIdx+1)
	args = append(args, perPage, offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list cheat flags: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var flags []model.CheatFlag
	for rows.Next() {
		var f model.CheatFlag
		if err := rows.Scan(
			&f.ID, &f.UserID, &f.TorrentID, &f.FlagType, &f.Details,
			&f.Dismissed, &f.DismissedBy, &f.DismissedAt, &f.CreatedAt,
			&f.Username, &f.TorrentName, &f.DismisserName,
		); err != nil {
			return nil, 0, fmt.Errorf("scan cheat flag: %w", err)
		}
		flags = append(flags, f)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate cheat flags: %w", err)
	}

	return flags, total, nil
}

// Dismiss marks a cheat flag as dismissed.
func (r *CheatFlagRepo) Dismiss(ctx context.Context, id, dismissedBy int64) error {
	query := `UPDATE cheat_flags SET dismissed = TRUE, dismissed_by = $1, dismissed_at = NOW() WHERE id = $2 AND NOT dismissed`
	result, err := r.db.ExecContext(ctx, query, dismissedBy, id)
	if err != nil {
		return fmt.Errorf("dismiss cheat flag: %w", err)
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

// HasRecentUndismissed checks if an undismissed flag of the same type exists for this user
// and torrent within the cooldown window.
func (r *CheatFlagRepo) HasRecentUndismissed(ctx context.Context, userID int64, torrentID int64, flagType string, cooldownHours int) (bool, error) {
	query := `SELECT EXISTS(
		SELECT 1 FROM cheat_flags
		WHERE user_id = $1
		AND torrent_id = $2
		AND flag_type = $3
		AND NOT dismissed
		AND created_at > NOW() - make_interval(hours => $4)
	)`
	var exists bool
	err := r.db.QueryRowContext(ctx, query, userID, torrentID, flagType, cooldownHours).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check recent cheat flag: %w", err)
	}
	return exists, nil
}
