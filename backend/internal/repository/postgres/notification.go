package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// NotificationRepo implements repository.NotificationRepository using PostgreSQL.
type NotificationRepo struct {
	db *sql.DB
}

// NewNotificationRepo creates a new NotificationRepo.
func NewNotificationRepo(db *sql.DB) repository.NotificationRepository {
	return &NotificationRepo{db: db}
}

func (r *NotificationRepo) Create(ctx context.Context, notif *model.Notification) error {
	return r.db.QueryRowContext(ctx,
		`INSERT INTO notifications (user_id, type, data) VALUES ($1, $2, $3) RETURNING id, created_at`,
		notif.UserID, notif.Type, notif.Data,
	).Scan(&notif.ID, &notif.CreatedAt)
}

func (r *NotificationRepo) GetByID(ctx context.Context, id int64) (*model.Notification, error) {
	var n model.Notification
	err := r.db.QueryRowContext(ctx,
		`SELECT id, user_id, type, data, is_read, created_at FROM notifications WHERE id = $1`, id,
	).Scan(&n.ID, &n.UserID, &n.Type, &n.Data, &n.Read, &n.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &n, nil
}

func (r *NotificationRepo) List(ctx context.Context, userID int64, opts repository.ListNotificationsOptions) ([]model.Notification, int64, error) {
	page := opts.Page
	perPage := opts.PerPage
	if page <= 0 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 25
	}
	if perPage > 100 {
		perPage = 100
	}

	// Count total
	countQuery := `SELECT COUNT(*) FROM notifications WHERE user_id = $1`
	args := []interface{}{userID}
	if opts.UnreadOnly {
		countQuery += ` AND is_read = FALSE`
	}
	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count notifications: %w", err)
	}

	// Fetch page
	offset := (page - 1) * perPage
	listQuery := `SELECT id, user_id, type, data, is_read, created_at FROM notifications WHERE user_id = $1`
	if opts.UnreadOnly {
		listQuery += ` AND is_read = FALSE`
	}
	listQuery += ` ORDER BY created_at DESC LIMIT $2 OFFSET $3`

	rows, err := r.db.QueryContext(ctx, listQuery, userID, perPage, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list notifications: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var notifications []model.Notification
	for rows.Next() {
		var n model.Notification
		if err := rows.Scan(&n.ID, &n.UserID, &n.Type, &n.Data, &n.Read, &n.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan notification: %w", err)
		}
		notifications = append(notifications, n)
	}
	return notifications, total, rows.Err()
}

func (r *NotificationRepo) MarkRead(ctx context.Context, userID, id int64) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE notifications SET is_read = TRUE WHERE id = $1 AND user_id = $2`, id, userID,
	)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *NotificationRepo) MarkAllRead(ctx context.Context, userID int64) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE notifications SET is_read = TRUE WHERE user_id = $1 AND is_read = FALSE`, userID,
	)
	return err
}

func (r *NotificationRepo) CountUnread(ctx context.Context, userID int64) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM notifications WHERE user_id = $1 AND is_read = FALSE`, userID,
	).Scan(&count)
	return count, err
}

func (r *NotificationRepo) DeleteOld(ctx context.Context, before time.Time) (int64, error) {
	result, err := r.db.ExecContext(ctx,
		`DELETE FROM notifications WHERE created_at < $1 AND is_read = TRUE`, before,
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
