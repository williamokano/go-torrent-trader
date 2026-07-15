package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// PromotionRepo implements repository.PromotionRepository using PostgreSQL.
type PromotionRepo struct {
	db *sql.DB
}

// NewPromotionRepo returns a new PostgreSQL-backed PromotionRepository.
func NewPromotionRepo(db *sql.DB) *PromotionRepo {
	return &PromotionRepo{db: db}
}

func (r *PromotionRepo) ListRules(ctx context.Context) ([]model.PromotionRule, error) {
	query := `SELECT group_id, min_ratio, min_uploaded, min_age_days, min_torrents, min_seed_hours, created_at, updated_at
		FROM promotion_rules ORDER BY group_id`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list promotion rules: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var rules []model.PromotionRule
	for rows.Next() {
		var rule model.PromotionRule
		if err := rows.Scan(
			&rule.GroupID, &rule.MinRatio, &rule.MinUploaded, &rule.MinAgeDays,
			&rule.MinTorrents, &rule.MinSeedHours, &rule.CreatedAt, &rule.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan promotion rule: %w", err)
		}
		rules = append(rules, rule)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate promotion rules: %w", err)
	}
	return rules, nil
}

func (r *PromotionRepo) UpsertRule(ctx context.Context, rule *model.PromotionRule) error {
	query := `INSERT INTO promotion_rules
		(group_id, min_ratio, min_uploaded, min_age_days, min_torrents, min_seed_hours)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (group_id) DO UPDATE SET
			min_ratio = EXCLUDED.min_ratio,
			min_uploaded = EXCLUDED.min_uploaded,
			min_age_days = EXCLUDED.min_age_days,
			min_torrents = EXCLUDED.min_torrents,
			min_seed_hours = EXCLUDED.min_seed_hours,
			updated_at = NOW()
		RETURNING created_at, updated_at`
	return r.db.QueryRowContext(ctx, query,
		rule.GroupID, rule.MinRatio, rule.MinUploaded, rule.MinAgeDays,
		rule.MinTorrents, rule.MinSeedHours,
	).Scan(&rule.CreatedAt, &rule.UpdatedAt)
}

func (r *PromotionRepo) DeleteRule(ctx context.Context, groupID int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM promotion_rules WHERE group_id = $1`, groupID)
	if err != nil {
		return fmt.Errorf("delete promotion rule: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete promotion rule rows affected: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// LadderMetrics returns base metrics (ratio inputs, tenure, uploaded torrent
// count) for every enabled user currently in one of the ladder groups. Seeding
// hours are fetched separately via SeedHoursByUser and merged by the caller.
func (r *PromotionRepo) LadderMetrics(ctx context.Context, groupIDs []int64) ([]model.PromotionUserMetrics, error) {
	if len(groupIDs) == 0 {
		return nil, nil
	}

	placeholders := make([]string, len(groupIDs))
	args := make([]any, len(groupIDs))
	for i, id := range groupIDs {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}

	query := fmt.Sprintf(`SELECT u.id, u.group_id, u.uploaded, u.downloaded, u.created_at,
		COALESCE(tc.cnt, 0) AS torrent_count
		FROM users u
		LEFT JOIN (SELECT uploader_id, COUNT(*) AS cnt FROM torrents GROUP BY uploader_id) tc
			ON tc.uploader_id = u.id
		WHERE u.enabled = true AND u.group_id IN (%s)`, strings.Join(placeholders, ", "))

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query ladder metrics: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []model.PromotionUserMetrics
	for rows.Next() {
		var m model.PromotionUserMetrics
		if err := rows.Scan(&m.UserID, &m.GroupID, &m.Uploaded, &m.Downloaded, &m.CreatedAt, &m.Torrents); err != nil {
			return nil, fmt.Errorf("scan ladder metrics: %w", err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ladder metrics: %w", err)
	}
	return out, nil
}

// SeedHoursByUser estimates seeding hours per user within the window [since, now].
// For each peer (user, torrent, peer_id) it sums the gaps between consecutive
// announces during which the peer was seeding, capping each gap at capSeconds so
// an offline stretch between two seeding announces is not counted as continuous
// seed time. The result is keyed by user id and only includes users with seed
// time; callers default the rest to zero.
func (r *PromotionRepo) SeedHoursByUser(ctx context.Context, since time.Time, capSeconds int) (map[int64]int, error) {
	query := `
		SELECT user_id, COALESCE(SUM(seed_gap), 0) AS seed_seconds
		FROM (
			SELECT user_id,
				LEAST(
					EXTRACT(EPOCH FROM (announced_at - LAG(announced_at) OVER w)),
					$2::double precision
				) AS seed_gap,
				LAG(seeder) OVER w AS prev_seeder
			FROM announce_events
			WHERE announced_at >= $1
			WINDOW w AS (PARTITION BY user_id, torrent_id, peer_id ORDER BY announced_at)
		) gaps
		WHERE prev_seeder = true AND seed_gap > 0
		GROUP BY user_id`

	rows, err := r.db.QueryContext(ctx, query, since, capSeconds)
	if err != nil {
		return nil, fmt.Errorf("query seed hours: %w", err)
	}
	defer func() { _ = rows.Close() }()

	result := make(map[int64]int)
	for rows.Next() {
		var userID int64
		var seedSeconds float64
		if err := rows.Scan(&userID, &seedSeconds); err != nil {
			return nil, fmt.Errorf("scan seed hours: %w", err)
		}
		result[userID] = int(seedSeconds / 3600)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate seed hours: %w", err)
	}
	return result, nil
}

func (r *PromotionRepo) SetUserGroup(ctx context.Context, userID, groupID int64) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE users SET group_id = $1, updated_at = NOW() WHERE id = $2`, groupID, userID)
	if err != nil {
		return fmt.Errorf("set user group: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("set user group rows affected: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *PromotionRepo) LastRunAt(ctx context.Context) (time.Time, bool, error) {
	var ranAt time.Time
	err := r.db.QueryRowContext(ctx, `SELECT ran_at FROM promotion_runs ORDER BY ran_at DESC LIMIT 1`).Scan(&ranAt)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, fmt.Errorf("last promotion run: %w", err)
	}
	return ranAt, true, nil
}

func (r *PromotionRepo) RecordRun(ctx context.Context, promoted, demoted int) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO promotion_runs (promoted, demoted) VALUES ($1, $2)`, promoted, demoted)
	if err != nil {
		return fmt.Errorf("record promotion run: %w", err)
	}
	return nil
}

var _ repository.PromotionRepository = (*PromotionRepo)(nil)
