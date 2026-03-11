package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// NotificationPreferenceRepo implements repository.NotificationPreferenceRepository using PostgreSQL.
type NotificationPreferenceRepo struct {
	db *sql.DB
}

// NewNotificationPreferenceRepo creates a new NotificationPreferenceRepo.
func NewNotificationPreferenceRepo(db *sql.DB) repository.NotificationPreferenceRepository {
	return &NotificationPreferenceRepo{db: db}
}

func (r *NotificationPreferenceRepo) Get(ctx context.Context, userID int64, notifType string) (*model.NotificationPreference, error) {
	var p model.NotificationPreference
	err := r.db.QueryRowContext(ctx,
		`SELECT user_id, notification_type, enabled FROM notification_preferences WHERE user_id = $1 AND notification_type = $2`,
		userID, notifType,
	).Scan(&p.UserID, &p.NotificationType, &p.Enabled)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *NotificationPreferenceRepo) GetAll(ctx context.Context, userID int64) ([]model.NotificationPreference, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT user_id, notification_type, enabled FROM notification_preferences WHERE user_id = $1`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list preferences: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var prefs []model.NotificationPreference
	for rows.Next() {
		var p model.NotificationPreference
		if err := rows.Scan(&p.UserID, &p.NotificationType, &p.Enabled); err != nil {
			return nil, fmt.Errorf("scan preference: %w", err)
		}
		prefs = append(prefs, p)
	}
	return prefs, rows.Err()
}

func (r *NotificationPreferenceRepo) Set(ctx context.Context, userID int64, notifType string, enabled bool) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO notification_preferences (user_id, notification_type, enabled) VALUES ($1, $2, $3)
		ON CONFLICT (user_id, notification_type) DO UPDATE SET enabled = EXCLUDED.enabled`,
		userID, notifType, enabled,
	)
	return err
}

func (r *NotificationPreferenceRepo) IsEnabled(ctx context.Context, userID int64, notifType string) (bool, error) {
	var enabled bool
	err := r.db.QueryRowContext(ctx,
		`SELECT enabled FROM notification_preferences WHERE user_id = $1 AND notification_type = $2`,
		userID, notifType,
	).Scan(&enabled)
	if err == sql.ErrNoRows {
		// Default: enabled
		return true, nil
	}
	return enabled, err
}
