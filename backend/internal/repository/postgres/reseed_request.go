package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// ReseedRequestRepo implements repository.ReseedRequestRepository using PostgreSQL.
type ReseedRequestRepo struct {
	db *sql.DB
}

// NewReseedRequestRepo returns a new PostgreSQL-backed ReseedRequestRepository.
func NewReseedRequestRepo(db *sql.DB) repository.ReseedRequestRepository {
	return &ReseedRequestRepo{db: db}
}

func (r *ReseedRequestRepo) Create(ctx context.Context, req *model.ReseedRequest) error {
	query := `INSERT INTO reseed_requests (torrent_id, requester_id)
		VALUES ($1, $2)
		RETURNING id, created_at`
	err := r.db.QueryRowContext(ctx, query, req.TorrentID, req.RequesterID).Scan(&req.ID, &req.CreatedAt)
	if err != nil {
		return fmt.Errorf("create reseed request: %w", err)
	}
	return nil
}

func (r *ReseedRequestRepo) ExistsByTorrentAndUser(ctx context.Context, torrentID, userID int64) (bool, error) {
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM reseed_requests WHERE torrent_id = $1 AND requester_id = $2)`
	if err := r.db.QueryRowContext(ctx, query, torrentID, userID).Scan(&exists); err != nil {
		return false, fmt.Errorf("check existing reseed request: %w", err)
	}
	return exists, nil
}

func (r *ReseedRequestRepo) CountByTorrent(ctx context.Context, torrentID int64) (int, error) {
	var count int
	query := `SELECT COUNT(*) FROM reseed_requests WHERE torrent_id = $1`
	if err := r.db.QueryRowContext(ctx, query, torrentID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count reseed requests: %w", err)
	}
	return count, nil
}
