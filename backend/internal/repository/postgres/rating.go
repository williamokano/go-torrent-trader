package postgres

import (
	"context"
	"database/sql"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// RatingRepo implements repository.RatingRepository using PostgreSQL.
type RatingRepo struct {
	db *sql.DB
}

// NewRatingRepo returns a new PostgreSQL-backed RatingRepository.
func NewRatingRepo(db *sql.DB) repository.RatingRepository {
	return &RatingRepo{db: db}
}

func (r *RatingRepo) Upsert(ctx context.Context, rating *model.Rating) error {
	query := `INSERT INTO torrent_ratings (torrent_id, user_id, rating)
		VALUES ($1, $2, $3)
		ON CONFLICT (torrent_id, user_id) DO UPDATE SET rating = EXCLUDED.rating, updated_at = NOW()
		RETURNING id, created_at, updated_at`

	return r.db.QueryRowContext(ctx, query,
		rating.TorrentID, rating.UserID, rating.Rating,
	).Scan(&rating.ID, &rating.CreatedAt, &rating.UpdatedAt)
}

func (r *RatingRepo) GetByTorrentAndUser(ctx context.Context, torrentID, userID int64) (*model.Rating, error) {
	query := `SELECT id, torrent_id, user_id, rating, created_at, updated_at
		FROM torrent_ratings
		WHERE torrent_id = $1 AND user_id = $2`

	var rt model.Rating
	err := r.db.QueryRowContext(ctx, query, torrentID, userID).Scan(
		&rt.ID, &rt.TorrentID, &rt.UserID, &rt.Rating, &rt.CreatedAt, &rt.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &rt, nil
}

func (r *RatingRepo) GetStatsByTorrent(ctx context.Context, torrentID int64) (float64, int, error) {
	query := `SELECT COALESCE(AVG(rating), 0), COUNT(*)
		FROM torrent_ratings
		WHERE torrent_id = $1`

	var avg float64
	var count int
	err := r.db.QueryRowContext(ctx, query, torrentID).Scan(&avg, &count)
	if err != nil {
		return 0, 0, err
	}
	return avg, count, nil
}
