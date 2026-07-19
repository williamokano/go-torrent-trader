package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// NotificationDigestPreferenceRepo implements repository.NotificationDigestPreferenceRepository using PostgreSQL.
type NotificationDigestPreferenceRepo struct {
	db *sql.DB
}

// NewNotificationDigestPreferenceRepo creates a new NotificationDigestPreferenceRepo.
func NewNotificationDigestPreferenceRepo(db *sql.DB) repository.NotificationDigestPreferenceRepository {
	return &NotificationDigestPreferenceRepo{db: db}
}

func (r *NotificationDigestPreferenceRepo) GetFrequency(ctx context.Context, userID int64) (string, error) {
	var freq string
	err := r.db.QueryRowContext(ctx,
		`SELECT frequency FROM notification_digest_preferences WHERE user_id = $1`, userID,
	).Scan(&freq)
	if err == sql.ErrNoRows {
		// Default: off. No row means the user never opted in.
		return model.DigestOff, nil
	}
	return freq, err
}

func (r *NotificationDigestPreferenceRepo) SetFrequency(ctx context.Context, userID int64, frequency string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO notification_digest_preferences (user_id, frequency, updated_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (user_id) DO UPDATE SET frequency = EXCLUDED.frequency, updated_at = NOW()`,
		userID, frequency,
	)
	return err
}

func (r *NotificationDigestPreferenceRepo) ListDue(ctx context.Context, frequency string, sentBefore time.Time) ([]model.DigestRecipient, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT ndp.user_id, u.email, ndp.last_digest_sent_at
		FROM notification_digest_preferences ndp
		JOIN users u ON u.id = ndp.user_id
		WHERE ndp.frequency = $1
		  AND u.enabled = TRUE
		  AND (ndp.last_digest_sent_at IS NULL OR ndp.last_digest_sent_at < $2)`,
		frequency, sentBefore,
	)
	if err != nil {
		return nil, fmt.Errorf("list due digest recipients: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var recipients []model.DigestRecipient
	for rows.Next() {
		var rec model.DigestRecipient
		if err := rows.Scan(&rec.UserID, &rec.Email, &rec.LastDigestSentAt); err != nil {
			return nil, fmt.Errorf("scan digest recipient: %w", err)
		}
		recipients = append(recipients, rec)
	}
	return recipients, rows.Err()
}

func (r *NotificationDigestPreferenceRepo) MarkSent(ctx context.Context, userID int64, sentAt time.Time) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE notification_digest_preferences SET last_digest_sent_at = $2, updated_at = NOW() WHERE user_id = $1`,
		userID, sentAt,
	)
	return err
}
