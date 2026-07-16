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

// InviteDistributionRepo implements repository.InviteDistributionRepository
// using PostgreSQL.
type InviteDistributionRepo struct {
	db *sql.DB
}

// NewInviteDistributionRepo returns a new PostgreSQL-backed
// InviteDistributionRepository.
func NewInviteDistributionRepo(db *sql.DB) *InviteDistributionRepo {
	return &InviteDistributionRepo{db: db}
}

func (r *InviteDistributionRepo) ListRules(ctx context.Context) ([]model.InviteDistributionRule, error) {
	query := `SELECT group_id, min_ratio, min_downloaded_bytes, max_downloaded_bytes, max_invites, created_at, updated_at
		FROM invite_distribution_rules ORDER BY group_id`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list invite distribution rules: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var rules []model.InviteDistributionRule
	for rows.Next() {
		var rule model.InviteDistributionRule
		if err := rows.Scan(
			&rule.GroupID, &rule.MinRatio, &rule.MinDownloadedBytes, &rule.MaxDownloadedBytes,
			&rule.MaxInvites, &rule.CreatedAt, &rule.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan invite distribution rule: %w", err)
		}
		rules = append(rules, rule)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate invite distribution rules: %w", err)
	}
	return rules, nil
}

func (r *InviteDistributionRepo) UpsertRule(ctx context.Context, rule *model.InviteDistributionRule) error {
	query := `INSERT INTO invite_distribution_rules
		(group_id, min_ratio, min_downloaded_bytes, max_downloaded_bytes, max_invites)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (group_id) DO UPDATE SET
			min_ratio = EXCLUDED.min_ratio,
			min_downloaded_bytes = EXCLUDED.min_downloaded_bytes,
			max_downloaded_bytes = EXCLUDED.max_downloaded_bytes,
			max_invites = EXCLUDED.max_invites,
			updated_at = NOW()
		RETURNING created_at, updated_at`
	return r.db.QueryRowContext(ctx, query,
		rule.GroupID, rule.MinRatio, rule.MinDownloadedBytes, rule.MaxDownloadedBytes, rule.MaxInvites,
	).Scan(&rule.CreatedAt, &rule.UpdatedAt)
}

func (r *InviteDistributionRepo) DeleteRule(ctx context.Context, groupID int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM invite_distribution_rules WHERE group_id = $1`, groupID)
	if err != nil {
		return fmt.Errorf("delete invite distribution rule: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete invite distribution rule rows affected: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// GroupMetrics reuses the shared per-user metrics query — see
// queryGroupMetrics in user_metrics.go — rather than duplicating
// PromotionRepo.LadderMetrics' SQL.
func (r *InviteDistributionRepo) GroupMetrics(ctx context.Context, groupIDs []int64) ([]model.PromotionUserMetrics, error) {
	return queryGroupMetrics(ctx, r.db, groupIDs)
}

// UserInviteStates bulk-fetches the current invite balance, username, and
// can_invite flag for a set of users, keyed by user id. A single query rather
// than one GetByID per candidate, since the engine calls this once per run
// over every eligible user.
func (r *InviteDistributionRepo) UserInviteStates(ctx context.Context, userIDs []int64) (map[int64]model.UserInviteState, error) {
	result := make(map[int64]model.UserInviteState, len(userIDs))
	if len(userIDs) == 0 {
		return result, nil
	}

	placeholders := make([]string, len(userIDs))
	args := make([]any, len(userIDs))
	for i, id := range userIDs {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}

	query := fmt.Sprintf(`SELECT id, username, invites, can_invite FROM users WHERE id IN (%s)`,
		strings.Join(placeholders, ", "))
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query user invite states: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var id int64
		var state model.UserInviteState
		if err := rows.Scan(&id, &state.Username, &state.Invites, &state.CanInvite); err != nil {
			return nil, fmt.Errorf("scan user invite state: %w", err)
		}
		result[id] = state
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate user invite states: %w", err)
	}
	return result, nil
}

func (r *InviteDistributionRepo) LastRunAt(ctx context.Context) (time.Time, bool, error) {
	var ranAt time.Time
	err := r.db.QueryRowContext(ctx, `SELECT ran_at FROM invite_distribution_runs ORDER BY ran_at DESC LIMIT 1`).Scan(&ranAt)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, fmt.Errorf("last invite distribution run: %w", err)
	}
	return ranAt, true, nil
}

func (r *InviteDistributionRepo) RecordRun(ctx context.Context, granted int) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO invite_distribution_runs (granted) VALUES ($1)`, granted)
	if err != nil {
		return fmt.Errorf("record invite distribution run: %w", err)
	}
	return nil
}

var _ repository.InviteDistributionRepository = (*InviteDistributionRepo)(nil)
