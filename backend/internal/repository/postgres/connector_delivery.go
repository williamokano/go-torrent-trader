package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

const connectorDeliveryColumns = `id, instance_id, event_key, event_type, payload, status,
	attempts, last_error, next_attempt_at, created_at, updated_at`

// ConnectorDeliveryRepo implements repository.ConnectorDeliveryRepository using
// PostgreSQL.
type ConnectorDeliveryRepo struct {
	db *sql.DB
}

// NewConnectorDeliveryRepo returns a new PostgreSQL-backed
// ConnectorDeliveryRepository.
func NewConnectorDeliveryRepo(db *sql.DB) repository.ConnectorDeliveryRepository {
	return &ConnectorDeliveryRepo{db: db}
}

// InsertPending relies on the (instance_id, event_key) unique index for dedupe.
// ON CONFLICT DO NOTHING makes a duplicate dispatch a silent no-op even when two
// dispatchers race, which a check-then-insert could not.
func (r *ConnectorDeliveryRepo) InsertPending(ctx context.Context, d *model.ConnectorDelivery) (bool, error) {
	query := `INSERT INTO connector_deliveries (instance_id, event_key, event_type, payload, status, next_attempt_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (instance_id, event_key) DO NOTHING
		RETURNING id, created_at, updated_at`

	status := d.Status
	if status == "" {
		status = model.DeliveryPending
	}

	err := r.db.QueryRowContext(ctx, query,
		d.InstanceID, d.EventKey, d.EventType, jsonbOrEmpty(d.Payload), status, d.NextAttemptAt,
	).Scan(&d.ID, &d.CreatedAt, &d.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		// The conflict target matched: this event was already recorded for this
		// instance, so there is nothing new to deliver.
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("insert connector delivery: %w", err)
	}
	d.Status = status
	return true, nil
}

func (r *ConnectorDeliveryRepo) ListDue(ctx context.Context, instanceID int64, now time.Time, limit int) ([]model.ConnectorDelivery, error) {
	if limit <= 0 {
		limit = 100
	}
	query := fmt.Sprintf(`SELECT %s FROM connector_deliveries
		WHERE instance_id = $1 AND status = $2
		  AND (next_attempt_at IS NULL OR next_attempt_at <= $3)
		ORDER BY id
		LIMIT $4`, connectorDeliveryColumns)

	rows, err := r.db.QueryContext(ctx, query, instanceID, model.DeliveryPending, now, limit)
	if err != nil {
		return nil, fmt.Errorf("list due connector deliveries: %w", err)
	}
	return scanConnectorDeliveries(rows)
}

// ClaimForDelivery re-checks the due predicate inside the UPDATE, so the claim
// is atomic: exactly one of two racing drains sees a row affected.
func (r *ConnectorDeliveryRepo) ClaimForDelivery(ctx context.Context, id int64, leaseUntil, now time.Time) (bool, error) {
	result, err := r.db.ExecContext(ctx,
		`UPDATE connector_deliveries
		 SET next_attempt_at = $1, updated_at = NOW()
		 WHERE id = $2 AND status = $3
		   AND (next_attempt_at IS NULL OR next_attempt_at <= $4)`,
		leaseUntil, id, model.DeliveryPending, now)
	if err != nil {
		return false, fmt.Errorf("claim connector delivery: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("checking rows affected: %w", err)
	}
	return n > 0, nil
}

// CountSentSince reports how much of the per-minute budget has been spent.
//
// It counts 'sent' rows only. Coalesced rows are deliberately excluded: a batch
// of N of them was represented by a single "+N more" message on the wire, so
// counting each row would charge N for one message and stall the instance for
// minutes after any burst — the opposite of what coalescing is for. The cost is
// that the summary itself goes uncharged, i.e. an under-count of at most one
// message per drain, which errs towards still announcing rather than wedging.
func (r *ConnectorDeliveryRepo) CountSentSince(ctx context.Context, instanceID int64, since time.Time) (int64, error) {
	var count int64
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM connector_deliveries
		 WHERE instance_id = $1 AND status = $2 AND updated_at >= $3`,
		instanceID, model.DeliverySent, since,
	).Scan(&count); err != nil {
		return 0, fmt.Errorf("count sent connector deliveries: %w", err)
	}
	return count, nil
}

func (r *ConnectorDeliveryRepo) MarkSent(ctx context.Context, id int64, status string) error {
	// last_error is left in place on purpose: on a row that took several
	// attempts it is the only record of why, and the status already says the
	// delivery ultimately succeeded. The status predicate matters too — a slow
	// drain finishing after another one dead-lettered its leased row must not
	// flip 'failed' back to 'sent'.
	result, err := r.db.ExecContext(ctx,
		`UPDATE connector_deliveries
		 SET status = $1, next_attempt_at = NULL, updated_at = NOW()
		 WHERE id = $2 AND status = $3`, status, id, model.DeliveryPending)
	if err != nil {
		return fmt.Errorf("mark connector delivery %s: %w", status, err)
	}
	return requireOneRow(result)
}

func (r *ConnectorDeliveryRepo) MarkFailedAttempt(ctx context.Context, id int64, attempts int, lastError string, nextAttemptAt *time.Time) error {
	// No next attempt means the row is dead-lettered: it stays visible in the
	// admin delivery log as 'failed' rather than being retried forever.
	result, err := r.db.ExecContext(ctx,
		`UPDATE connector_deliveries
		 SET attempts = $1,
		     last_error = $2,
		     next_attempt_at = $3,
		     status = CASE WHEN $3::timestamptz IS NULL THEN $4 ELSE $5 END,
		     updated_at = NOW()
		 WHERE id = $6`,
		attempts, lastError, nextAttemptAt, model.DeliveryFailed, model.DeliveryPending, id)
	if err != nil {
		return fmt.Errorf("mark connector delivery attempt: %w", err)
	}
	return requireOneRow(result)
}

func (r *ConnectorDeliveryRepo) ListByInstance(ctx context.Context, instanceID int64, page, perPage int) ([]model.ConnectorDelivery, int64, error) {
	if page <= 0 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 25
	}
	if perPage > 100 {
		perPage = 100
	}

	var total int64
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM connector_deliveries WHERE instance_id = $1`, instanceID,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count connector deliveries: %w", err)
	}

	query := fmt.Sprintf(`SELECT %s FROM connector_deliveries
		WHERE instance_id = $1
		ORDER BY created_at DESC, id DESC
		LIMIT $2 OFFSET $3`, connectorDeliveryColumns)

	rows, err := r.db.QueryContext(ctx, query, instanceID, perPage, (page-1)*perPage)
	if err != nil {
		return nil, 0, fmt.Errorf("list connector deliveries: %w", err)
	}
	deliveries, err := scanConnectorDeliveries(rows)
	if err != nil {
		return nil, 0, err
	}
	return deliveries, total, nil
}

func (r *ConnectorDeliveryRepo) LatestStatusByInstance(ctx context.Context) (map[int64]model.ConnectorDelivery, error) {
	query := fmt.Sprintf(`SELECT DISTINCT ON (instance_id) %s
		FROM connector_deliveries
		ORDER BY instance_id, created_at DESC, id DESC`, connectorDeliveryColumns)

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list latest connector deliveries: %w", err)
	}
	deliveries, err := scanConnectorDeliveries(rows)
	if err != nil {
		return nil, err
	}
	latest := make(map[int64]model.ConnectorDelivery, len(deliveries))
	for _, d := range deliveries {
		latest[d.InstanceID] = d
	}
	return latest, nil
}

func (r *ConnectorDeliveryRepo) InstancesWithDue(ctx context.Context, now time.Time) ([]int64, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT DISTINCT instance_id FROM connector_deliveries
		 WHERE status = $1 AND (next_attempt_at IS NULL OR next_attempt_at <= $2)`,
		model.DeliveryPending, now)
	if err != nil {
		return nil, fmt.Errorf("list instances with due deliveries: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan instance id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate instances with due deliveries: %w", err)
	}
	return ids, nil
}

// DeleteOld prunes the delivery log. Pending rows are deliberately spared: they
// are queued work, and retention must never be the reason an announcement went
// missing. Anything pending long enough to be caught here has already been
// dead-lettered to 'failed' by the drain handler.
func (r *ConnectorDeliveryRepo) DeleteOld(ctx context.Context, cutoff time.Time) (int64, error) {
	result, err := r.db.ExecContext(ctx,
		`DELETE FROM connector_deliveries WHERE created_at < $1 AND status <> $2`,
		cutoff, model.DeliveryPending)
	if err != nil {
		return 0, fmt.Errorf("delete old connector deliveries: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("checking rows affected: %w", err)
	}
	return n, nil
}

func scanConnectorDeliveries(rows *sql.Rows) ([]model.ConnectorDelivery, error) {
	defer func() { _ = rows.Close() }()

	var deliveries []model.ConnectorDelivery
	for rows.Next() {
		var d model.ConnectorDelivery
		if err := rows.Scan(&d.ID, &d.InstanceID, &d.EventKey, &d.EventType, &d.Payload,
			&d.Status, &d.Attempts, &d.LastError, &d.NextAttemptAt, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan connector delivery: %w", err)
		}
		deliveries = append(deliveries, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate connector deliveries: %w", err)
	}
	return deliveries, nil
}

// requireOneRow turns "updated nothing" into sql.ErrNoRows, so a delivery row
// deleted out from under the worker surfaces instead of looking like success.
func requireOneRow(result sql.Result) error {
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
