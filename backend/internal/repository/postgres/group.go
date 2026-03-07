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
