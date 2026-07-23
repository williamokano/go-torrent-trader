package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

// GroupRepo implements repository.GroupRepository and
// repository.GroupWriteRepository using PostgreSQL.
type GroupRepo struct {
	db *sql.DB
}

// NewGroupRepo returns a new PostgreSQL-backed group repository. It returns the
// concrete type so it can satisfy both the read (GroupRepository) and write
// (GroupWriteRepository) interfaces at the call site.
func NewGroupRepo(db *sql.DB) *GroupRepo {
	return &GroupRepo{db: db}
}

func (r *GroupRepo) List(ctx context.Context) ([]model.Group, error) {
	query := `SELECT id, name, slug, level, color, can_upload, can_download, can_invite,
		can_comment, can_forum, is_admin, is_moderator, is_immune, can_self_approve, created_at, updated_at
		FROM groups ORDER BY level ASC`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list groups: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var groups []model.Group
	for rows.Next() {
		var g model.Group
		if err := rows.Scan(
			&g.ID, &g.Name, &g.Slug, &g.Level, &g.Color,
			&g.CanUpload, &g.CanDownload, &g.CanInvite,
			&g.CanComment, &g.CanForum, &g.IsAdmin, &g.IsModerator, &g.IsImmune, &g.CanSelfApprove,
			&g.CreatedAt, &g.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan group: %w", err)
		}
		groups = append(groups, g)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate groups: %w", err)
	}

	return groups, nil
}

func (r *GroupRepo) GetByID(ctx context.Context, id int64) (*model.Group, error) {
	query := `SELECT id, name, slug, level, color, can_upload, can_download, can_invite,
		can_comment, can_forum, is_admin, is_moderator, is_immune, can_self_approve, created_at, updated_at
		FROM groups WHERE id = $1`

	var g model.Group
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&g.ID, &g.Name, &g.Slug, &g.Level, &g.Color,
		&g.CanUpload, &g.CanDownload, &g.CanInvite,
		&g.CanComment, &g.CanForum, &g.IsAdmin, &g.IsModerator, &g.IsImmune, &g.CanSelfApprove,
		&g.CreatedAt, &g.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get group by id: %w", err)
	}
	return &g, nil
}

func (r *GroupRepo) Create(ctx context.Context, g *model.Group) error {
	query := `INSERT INTO groups
		(name, slug, level, color, can_upload, can_download, can_invite, can_comment, can_forum, is_admin, is_moderator, is_immune)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id, created_at, updated_at`
	return r.db.QueryRowContext(ctx, query,
		g.Name, g.Slug, g.Level, g.Color,
		g.CanUpload, g.CanDownload, g.CanInvite, g.CanComment, g.CanForum,
		g.IsAdmin, g.IsModerator, g.IsImmune,
	).Scan(&g.ID, &g.CreatedAt, &g.UpdatedAt)
}

func (r *GroupRepo) Update(ctx context.Context, g *model.Group) error {
	query := `UPDATE groups SET
		name = $1, slug = $2, level = $3, color = $4,
		can_upload = $5, can_download = $6, can_invite = $7, can_comment = $8, can_forum = $9,
		is_admin = $10, is_moderator = $11, is_immune = $12, updated_at = NOW()
		WHERE id = $13
		RETURNING updated_at`
	err := r.db.QueryRowContext(ctx, query,
		g.Name, g.Slug, g.Level, g.Color,
		g.CanUpload, g.CanDownload, g.CanInvite, g.CanComment, g.CanForum,
		g.IsAdmin, g.IsModerator, g.IsImmune, g.ID,
	).Scan(&g.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return sql.ErrNoRows
	}
	if err != nil {
		return fmt.Errorf("update group: %w", err)
	}
	return nil
}

func (r *GroupRepo) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM groups WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete group: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete group rows affected: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *GroupRepo) CountMembers(ctx context.Context, groupID int64) (int, error) {
	var n int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM users WHERE group_id = $1`, groupID).Scan(&n); err != nil {
		return 0, fmt.Errorf("count group members: %w", err)
	}
	return n, nil
}
