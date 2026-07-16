package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

// queryGroupMetrics returns base per-user metrics (ratio inputs, tenure,
// uploaded torrent count) for every enabled user currently in one of the
// given groups. It is the one query for "per-user uploaded/downloaded/
// group/created_at" in the codebase — PromotionRepo.LadderMetrics and
// InviteDistributionRepo.GroupMetrics both call it rather than each keeping
// their own near-identical copy. Seeding hours (promotion-only) are fetched
// separately and merged in by that caller.
func queryGroupMetrics(ctx context.Context, db *sql.DB, groupIDs []int64) ([]model.PromotionUserMetrics, error) {
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

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query group metrics: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []model.PromotionUserMetrics
	for rows.Next() {
		var m model.PromotionUserMetrics
		if err := rows.Scan(&m.UserID, &m.GroupID, &m.Uploaded, &m.Downloaded, &m.CreatedAt, &m.Torrents); err != nil {
			return nil, fmt.Errorf("scan group metrics: %w", err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate group metrics: %w", err)
	}
	return out, nil
}
