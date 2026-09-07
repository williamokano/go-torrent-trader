package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

// --- automatic exemption rules --------------------------------------------------

const hnrExemptRuleColumns = `id, criterion, threshold, enabled, created_at, updated_at`

func scanHnRExemptRule(row interface{ Scan(...any) error }) (*model.HnRExemptRule, error) {
	var rule model.HnRExemptRule
	if err := row.Scan(
		&rule.ID, &rule.Criterion, &rule.Threshold, &rule.Enabled,
		&rule.CreatedAt, &rule.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &rule, nil
}

func (r *HnRRepo) ListExemptRules(ctx context.Context) ([]model.HnRExemptRule, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+hnrExemptRuleColumns+` FROM hnr_exempt_rules ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list hnr exempt rules: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []model.HnRExemptRule
	for rows.Next() {
		rule, err := scanHnRExemptRule(rows)
		if err != nil {
			return nil, fmt.Errorf("scan hnr exempt rule: %w", err)
		}
		out = append(out, *rule)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate hnr exempt rules: %w", err)
	}
	return out, nil
}

func (r *HnRRepo) GetExemptRule(ctx context.Context, id int64) (*model.HnRExemptRule, error) {
	return scanHnRExemptRule(r.db.QueryRowContext(ctx,
		`SELECT `+hnrExemptRuleColumns+` FROM hnr_exempt_rules WHERE id = $1`, id))
}

func (r *HnRRepo) CreateExemptRule(ctx context.Context, rule *model.HnRExemptRule) error {
	return r.db.QueryRowContext(ctx,
		`INSERT INTO hnr_exempt_rules (criterion, threshold, enabled)
		 VALUES ($1, $2, $3)
		 RETURNING id, created_at, updated_at`,
		rule.Criterion, rule.Threshold, rule.Enabled,
	).Scan(&rule.ID, &rule.CreatedAt, &rule.UpdatedAt)
}

func (r *HnRRepo) UpdateExemptRule(ctx context.Context, rule *model.HnRExemptRule) error {
	err := r.db.QueryRowContext(ctx,
		`UPDATE hnr_exempt_rules
		 SET criterion = $2, threshold = $3, enabled = $4, updated_at = NOW()
		 WHERE id = $1
		 RETURNING created_at, updated_at`,
		rule.ID, rule.Criterion, rule.Threshold, rule.Enabled,
	).Scan(&rule.CreatedAt, &rule.UpdatedAt)
	return err
}

func (r *HnRRepo) DeleteExemptRule(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM hnr_exempt_rules WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete hnr exempt rule: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete hnr exempt rule rows affected: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// hnrExemptMatchClause is the SQL predicate that decides whether a torrent
// matches any enabled rule. It is shared by both halves of ApplyExemptRules so
// "matches" and "no longer matches" can never diverge. `t` is the torrents
// alias in the surrounding statement.
const hnrExemptMatchClause = `EXISTS (
	SELECT 1 FROM hnr_exempt_rules er
	WHERE er.enabled = true
	  AND (
		(er.criterion = 'min_seeders'    AND t.seeders >= er.threshold) OR
		(er.criterion = 'max_size_bytes' AND t.size     <= er.threshold)
	  )
)`

// ApplyExemptRules is the daemon's automatic-exemption pass. In two statements,
// run in one REPEATABLE READ transaction so both see the same snapshot of
// torrents.seeders / .size — otherwise a concurrent announce between them could
// let one torrent be counted by both the flag and the release statement:
//
//   - flag every torrent that matches an enabled rule, has not been touched by
//     staff (hnr_exempt_source IS NULL), and is not already exempt — setting
//     hnr_exempt = true, source = 'auto';
//   - clear every torrent the auto pass had flagged (source = 'auto') that no
//     longer matches any enabled rule — back to hnr_exempt = false, source NULL.
//
// Manual exemptions (source = 'manual') are never touched in either direction,
// which is the whole reason the provenance column exists. Disabling every rule
// releases all auto exemptions on the next run.
func (r *HnRRepo) ApplyExemptRules(ctx context.Context) (exempted, released int, err error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return 0, 0, fmt.Errorf("apply hnr exempt rules (begin): %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	exemptRes, err := tx.ExecContext(ctx, `
		UPDATE torrents t SET hnr_exempt = true, hnr_exempt_source = 'auto', updated_at = NOW()
		WHERE t.hnr_exempt = false
		  AND t.hnr_exempt_source IS NULL
		  AND `+hnrExemptMatchClause)
	if err != nil {
		return 0, 0, fmt.Errorf("apply hnr exempt rules (flag): %w", err)
	}
	exemptN, err := exemptRes.RowsAffected()
	if err != nil {
		return 0, 0, fmt.Errorf("apply hnr exempt rules (flag rows): %w", err)
	}

	releaseRes, err := tx.ExecContext(ctx, `
		UPDATE torrents t SET hnr_exempt = false, hnr_exempt_source = NULL, updated_at = NOW()
		WHERE t.hnr_exempt_source = 'auto'
		  AND NOT `+hnrExemptMatchClause)
	if err != nil {
		return int(exemptN), 0, fmt.Errorf("apply hnr exempt rules (release): %w", err)
	}
	releaseN, err := releaseRes.RowsAffected()
	if err != nil {
		return int(exemptN), 0, fmt.Errorf("apply hnr exempt rules (release rows): %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, 0, fmt.Errorf("apply hnr exempt rules (commit): %w", err)
	}
	return int(exemptN), int(releaseN), nil
}
