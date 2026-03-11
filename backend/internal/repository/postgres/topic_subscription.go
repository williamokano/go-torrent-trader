package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// TopicSubscriptionRepo implements repository.TopicSubscriptionRepository using PostgreSQL.
type TopicSubscriptionRepo struct {
	db *sql.DB
}

// NewTopicSubscriptionRepo creates a new TopicSubscriptionRepo.
func NewTopicSubscriptionRepo(db *sql.DB) repository.TopicSubscriptionRepository {
	return &TopicSubscriptionRepo{db: db}
}

func (r *TopicSubscriptionRepo) Subscribe(ctx context.Context, userID, topicID int64) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO topic_subscriptions (user_id, topic_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		userID, topicID,
	)
	return err
}

func (r *TopicSubscriptionRepo) Unsubscribe(ctx context.Context, userID, topicID int64) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM topic_subscriptions WHERE user_id = $1 AND topic_id = $2`,
		userID, topicID,
	)
	return err
}

func (r *TopicSubscriptionRepo) IsSubscribed(ctx context.Context, userID, topicID int64) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM topic_subscriptions WHERE user_id = $1 AND topic_id = $2)`,
		userID, topicID,
	).Scan(&exists)
	return exists, err
}

func (r *TopicSubscriptionRepo) ListSubscribers(ctx context.Context, topicID int64) ([]int64, error) {
	// Cap at 500 to avoid unbounded fan-out in the notification listener.
	rows, err := r.db.QueryContext(ctx,
		`SELECT user_id FROM topic_subscriptions WHERE topic_id = $1 LIMIT 500`, topicID,
	)
	if err != nil {
		return nil, fmt.Errorf("list subscribers: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var userIDs []int64
	for rows.Next() {
		var uid int64
		if err := rows.Scan(&uid); err != nil {
			return nil, fmt.Errorf("scan subscriber: %w", err)
		}
		userIDs = append(userIDs, uid)
	}
	return userIDs, rows.Err()
}
