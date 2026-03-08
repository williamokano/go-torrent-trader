package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// ActivityLogRepo implements repository.ActivityLogRepository using PostgreSQL.
type ActivityLogRepo struct {
	db *sql.DB
}

// NewActivityLogRepo returns a new PostgreSQL-backed ActivityLogRepository.
func NewActivityLogRepo(db *sql.DB) repository.ActivityLogRepository {
	return &ActivityLogRepo{db: db}
}

func (r *ActivityLogRepo) Create(ctx context.Context, log *model.ActivityLog) error {
	query := `INSERT INTO activity_logs (event_type, actor_id, message, metadata)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at`
	err := r.db.QueryRowContext(ctx, query,
		log.EventType, log.ActorID, log.Message, log.Metadata,
	).Scan(&log.ID, &log.CreatedAt)
	if err != nil {
		return fmt.Errorf("create activity log: %w", err)
	}
	return nil
}

func (r *ActivityLogRepo) List(ctx context.Context, opts repository.ListActivityLogsOptions) ([]model.ActivityLog, int64, error) {
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

	if opts.EventType != nil {
		where += fmt.Sprintf(" AND al.event_type = $%d", argIdx)
		args = append(args, *opts.EventType)
		argIdx++
	}
	if opts.ActorID != nil {
		where += fmt.Sprintf(" AND al.actor_id = $%d", argIdx)
		args = append(args, *opts.ActorID)
		argIdx++
	}

	// Count
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM activity_logs al %s", where)
	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count activity logs: %w", err)
	}

	// Fetch with actor username
	offset := (page - 1) * perPage
	query := fmt.Sprintf(`SELECT al.id, al.event_type, al.actor_id, al.message, al.metadata, al.created_at
		FROM activity_logs al
		%s ORDER BY al.created_at DESC LIMIT $%d OFFSET $%d`,
		where, argIdx, argIdx+1)
	args = append(args, perPage, offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list activity logs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var logs []model.ActivityLog
	for rows.Next() {
		var l model.ActivityLog
		if err := rows.Scan(&l.ID, &l.EventType, &l.ActorID, &l.Message, &l.Metadata, &l.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan activity log: %w", err)
		}
		logs = append(logs, l)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate activity logs: %w", err)
	}

	return logs, total, nil
}
