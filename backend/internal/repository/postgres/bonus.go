package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

const bonusItemColumns = `id, slug, kind, name, description, price, reward_amount, enabled, sort, created_at, updated_at`

// BonusRepo implements repository.BonusRepository using PostgreSQL.
type BonusRepo struct {
	db *sql.DB
}

// NewBonusRepo returns a new PostgreSQL-backed BonusRepository.
func NewBonusRepo(db *sql.DB) *BonusRepo {
	return &BonusRepo{db: db}
}

func scanBonusItem(row interface{ Scan(...any) error }) (*model.BonusStoreItem, error) {
	var it model.BonusStoreItem
	err := row.Scan(
		&it.ID, &it.Slug, &it.Kind, &it.Name, &it.Description,
		&it.Price, &it.RewardAmount, &it.Enabled, &it.Sort,
		&it.CreatedAt, &it.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &it, nil
}

func (r *BonusRepo) ListEnabledItems(ctx context.Context) ([]model.BonusStoreItem, error) {
	query := fmt.Sprintf(`SELECT %s FROM bonus_store_items WHERE enabled = true ORDER BY sort, id`, bonusItemColumns)
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list bonus items: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var items []model.BonusStoreItem
	for rows.Next() {
		it, err := scanBonusItem(rows)
		if err != nil {
			return nil, fmt.Errorf("scan bonus item: %w", err)
		}
		items = append(items, *it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate bonus items: %w", err)
	}
	return items, nil
}

func (r *BonusRepo) GetItem(ctx context.Context, id int64) (*model.BonusStoreItem, error) {
	query := fmt.Sprintf(`SELECT %s FROM bonus_store_items WHERE id = $1`, bonusItemColumns)
	return scanBonusItem(r.db.QueryRowContext(ctx, query, id))
}

// AwardPoints applies one award cycle in a single transaction: an atomic
// balance increment plus a ledger row per user. Non-positive deltas are skipped.
func (r *BonusRepo) AwardPoints(ctx context.Context, awards map[int64]int64, reason string) error {
	if len(awards) == 0 {
		return nil
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin award tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for userID, delta := range awards {
		if delta <= 0 {
			continue
		}
		res, err := tx.ExecContext(ctx,
			`UPDATE users SET bonus_points = bonus_points + $1 WHERE id = $2`, delta, userID)
		if err != nil {
			return fmt.Errorf("award points to user %d: %w", userID, err)
		}
		// A vanished user (deleted between snapshot and award) is not an error;
		// just skip the ledger row too.
		if n, err := res.RowsAffected(); err != nil || n == 0 {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO bonus_transactions (user_id, delta, reason) VALUES ($1, $2, $3)`,
			userID, delta, reason); err != nil {
			return fmt.Errorf("ledger award for user %d: %w", userID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit award tx: %w", err)
	}
	return nil
}

// SetPoints sets an absolute balance under a row lock and records the delta as
// an admin_adjust ledger entry referencing the acting admin, plus the given
// user_edit_history entries — all three writes commit in one transaction, so
// a failed audit insert rolls back the balance change and ledger row instead
// of leaving a persisted change with no trail. No ledger row is written when
// the balance is unchanged; a nil or empty entries slice skips the audit
// insert but still sets the balance.
func (r *BonusRepo) SetPoints(ctx context.Context, userID, newBalance, actorID int64, entries []model.UserEditHistory) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin set-points tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var current int64
	if err := tx.QueryRowContext(ctx,
		`SELECT bonus_points FROM users WHERE id = $1 FOR UPDATE`, userID).Scan(&current); err != nil {
		return fmt.Errorf("lock user %d for set-points: %w", userID, err)
	}

	delta := newBalance - current
	if delta != 0 {
		if _, err := tx.ExecContext(ctx,
			`UPDATE users SET bonus_points = $1 WHERE id = $2`, newBalance, userID); err != nil {
			return fmt.Errorf("set points for user %d: %w", userID, err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO bonus_transactions (user_id, delta, reason, ref_id) VALUES ($1, $2, $3, $4)`,
			userID, delta, model.BonusReasonAdminAdjust, actorID); err != nil {
			return fmt.Errorf("ledger admin adjust for user %d: %w", userID, err)
		}
	}

	if err := insertEditHistoryTx(ctx, tx, entries); err != nil {
		return fmt.Errorf("set points for user %d: record: %w", userID, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit set-points tx: %w", err)
	}
	return nil
}

// PurchaseItem owns the whole purchase transaction: it loads the item, spends
// the points with a race-safe conditional decrement, applies the reward, and
// records the ledger row. Any failure rolls the whole purchase back.
func (r *BonusRepo) PurchaseItem(ctx context.Context, userID, itemID int64) (*model.BonusStoreItem, int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("begin purchase tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	query := fmt.Sprintf(`SELECT %s FROM bonus_store_items WHERE id = $1`, bonusItemColumns)
	item, err := scanBonusItem(tx.QueryRowContext(ctx, query, itemID))
	if err != nil {
		return nil, 0, err // sql.ErrNoRows surfaces as item-not-found in the service
	}
	if !item.Enabled {
		return nil, 0, repository.ErrBonusKindNotAvailable
	}

	// Race-safe spend: only succeeds when the balance covers the price.
	var newBalance int64
	err = tx.QueryRowContext(ctx,
		`UPDATE users SET bonus_points = bonus_points - $1 WHERE id = $2 AND bonus_points >= $1
		 RETURNING bonus_points`, item.Price, userID).Scan(&newBalance)
	if err == sql.ErrNoRows {
		return nil, 0, repository.ErrInsufficientBonusPoints
	}
	if err != nil {
		return nil, 0, fmt.Errorf("spend points: %w", err)
	}

	switch item.Kind {
	case model.BonusKindInvite:
		if _, err := tx.ExecContext(ctx,
			`UPDATE users SET invites = invites + $1 WHERE id = $2`, item.RewardAmount, userID); err != nil {
			return nil, 0, fmt.Errorf("grant invites: %w", err)
		}
	case model.BonusKindUploadCredit:
		if _, err := tx.ExecContext(ctx,
			`UPDATE users SET uploaded = uploaded + $1 WHERE id = $2`, item.RewardAmount, userID); err != nil {
			return nil, 0, fmt.Errorf("grant upload credit: %w", err)
		}
	default:
		// Catalogue-only kinds (freeleech_ticket, double_upload): effects are
		// not implemented yet, so the purchase is refused and rolled back.
		return nil, 0, repository.ErrBonusKindNotAvailable
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO bonus_transactions (user_id, delta, reason, ref_id) VALUES ($1, $2, $3, $4)`,
		userID, -item.Price, model.BonusReasonPurchase, item.ID); err != nil {
		return nil, 0, fmt.Errorf("ledger purchase: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, 0, fmt.Errorf("commit purchase tx: %w", err)
	}
	return item, newBalance, nil
}

// SeedingCounts returns the number of distinct torrents each user is currently
// seeding, from the live peers table.
func (r *BonusRepo) SeedingCounts(ctx context.Context) (map[int64]int64, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT user_id, COUNT(DISTINCT torrent_id) FROM peers WHERE seeder = true GROUP BY user_id`)
	if err != nil {
		return nil, fmt.Errorf("query seeding counts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	counts := make(map[int64]int64)
	for rows.Next() {
		var userID, n int64
		if err := rows.Scan(&userID, &n); err != nil {
			return nil, fmt.Errorf("scan seeding count: %w", err)
		}
		counts[userID] = n
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate seeding counts: %w", err)
	}
	return counts, nil
}

var _ repository.BonusRepository = (*BonusRepo)(nil)
