package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// GroupRepo implements repository.GroupRepository using PostgreSQL.
type GroupRepo struct {
	db *sql.DB
}

// NewGroupRepo returns a new PostgreSQL-backed GroupRepository.
func NewGroupRepo(db *sql.DB) repository.GroupRepository {
	return &GroupRepo{db: db}
}

func (r *GroupRepo) List(ctx context.Context) ([]model.Group, error) {
	query := `SELECT id, name, slug, level, color, can_upload, can_download, can_invite,
		can_comment, can_forum, is_admin, is_moderator, is_immune, created_at, updated_at
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
			&g.CanComment, &g.CanForum, &g.IsAdmin, &g.IsModerator, &g.IsImmune,
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
		can_comment, can_forum, is_admin, is_moderator, is_immune, created_at, updated_at
		FROM groups WHERE id = $1`

	var g model.Group
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&g.ID, &g.Name, &g.Slug, &g.Level, &g.Color,
		&g.CanUpload, &g.CanDownload, &g.CanInvite,
		&g.CanComment, &g.CanForum, &g.IsAdmin, &g.IsModerator, &g.IsImmune,
		&g.CreatedAt, &g.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get group by id: %w", err)
	}
	return &g, nil
}
