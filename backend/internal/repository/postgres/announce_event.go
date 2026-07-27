package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// AnnounceEventRepo implements repository.AnnounceEventRepository using PostgreSQL.
type AnnounceEventRepo struct {
	db *sql.DB
}

// NewAnnounceEventRepo returns a new PostgreSQL-backed AnnounceEventRepository.
func NewAnnounceEventRepo(db *sql.DB) repository.AnnounceEventRepository {
	return &AnnounceEventRepo{db: db}
}

func (r *AnnounceEventRepo) Create(ctx context.Context, e *model.AnnounceEvent) error {
	query := `INSERT INTO announce_events (
		user_id, torrent_id, peer_id, port, event,
		uploaded, downloaded, left_bytes,
		uploaded_delta, downloaded_delta, counted_downloaded_delta,
		seeder, announced_at
	) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	RETURNING id`
	return r.db.QueryRowContext(ctx, query,
		e.UserID, e.TorrentID, e.PeerID, e.Port, e.Event,
		e.Uploaded, e.Downloaded, e.LeftBytes,
		e.UploadedDelta, e.DownloadedDelta, e.CountedDownloadedDelta,
		e.Seeder, e.AnnouncedAt,
	).Scan(&e.ID)
}

func (r *AnnounceEventRepo) ListByUser(ctx context.Context, userID int64, page, perPage int) ([]repository.AnnounceEventWithTorrent, int64, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 25
	}
	if perPage > 100 {
		perPage = 100
	}
	offset := (page - 1) * perPage

	var total int64
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM announce_events WHERE user_id = $1`, userID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting announce events: %w", err)
	}

	// Past the end there is nothing to fetch, and this is the largest table on the
	// site: `?page=9999999` would otherwise make Postgres walk a billion index
	// entries to discard every one of them. The count is already in hand, so the
	// check is free.
	if int64(offset) >= total {
		return nil, total, nil
	}

	query := announceEventSelect + `
		WHERE ae.user_id = $1
		ORDER BY ae.announced_at DESC, ae.id DESC
		LIMIT $2 OFFSET $3`

	rows, err := r.db.QueryContext(ctx, query, userID, perPage, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("listing announce events: %w", err)
	}

	results, err := scanAnnounceEvents(rows)
	if err != nil {
		return nil, 0, err
	}
	return results, total, nil
}

func (r *AnnounceEventRepo) DeleteOlderThan(ctx context.Context, cutoff time.Time, limit int) (int64, error) {
	if limit < 1 {
		return 0, nil
	}

	// The subselect is what bounds the work: it walks idx_announce_events_announced_at
	// in order and stops at limit, so each statement deletes a predictable slice
	// regardless of how far behind retention has fallen.
	result, err := r.db.ExecContext(ctx,
		`DELETE FROM announce_events WHERE id IN (
			SELECT id FROM announce_events WHERE announced_at < $1 ORDER BY announced_at LIMIT $2
		)`, cutoff, limit)
	if err != nil {
		return 0, fmt.Errorf("deleting old announce events: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("checking rows affected: %w", err)
	}
	return n, nil
}

// announceEventSelect is the projection the listing reads. The torrent name is a
// LEFT JOIN because torrent_id is set to NULL when a torrent is deleted, and the
// log outliving the torrent is the point of an append-only log.
const announceEventSelect = `SELECT ae.id, ae.user_id, ae.torrent_id, ae.peer_id, ae.port, ae.event,
		ae.uploaded, ae.downloaded, ae.left_bytes,
		ae.uploaded_delta, ae.downloaded_delta, ae.counted_downloaded_delta,
		ae.seeder, ae.announced_at,
		COALESCE(t.name, 'Deleted Torrent') AS torrent_name
		FROM announce_events ae
		LEFT JOIN torrents t ON ae.torrent_id = t.id`

func scanAnnounceEvents(rows *sql.Rows) ([]repository.AnnounceEventWithTorrent, error) {
	defer func() { _ = rows.Close() }()

	var results []repository.AnnounceEventWithTorrent
	for rows.Next() {
		var item repository.AnnounceEventWithTorrent
		if err := rows.Scan(
			&item.ID, &item.UserID, &item.TorrentID, &item.PeerID, &item.Port, &item.Event,
			&item.Uploaded, &item.Downloaded, &item.LeftBytes,
			&item.UploadedDelta, &item.DownloadedDelta, &item.CountedDownloadedDelta,
			&item.Seeder, &item.AnnouncedAt, &item.TorrentName,
		); err != nil {
			return nil, fmt.Errorf("scanning announce event: %w", err)
		}
		results = append(results, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating announce events: %w", err)
	}
	return results, nil
}

// announceEventsTable is the one table this file rebuilds. Held as a constant
// because the REINDEX statements below cannot take it as a bind parameter —
// PostgreSQL has no placeholder for an identifier — so nothing derived from
// input may ever reach them.
const announceEventsTable = "announce_events"

// Reindex rebuilds the announce log's indexes with REINDEX ... CONCURRENTLY.
//
// CONCURRENTLY is what makes this runnable on a live tracker: a plain REINDEX
// holds the table against writers for the whole rebuild, which on the busiest
// table on the site would mean announces blocking for minutes. The concurrent
// form takes only brief locks at the start and the end.
//
// Two consequences follow from that and shape everything here:
//
//   - It cannot run inside a transaction, so every statement below is issued on
//     its own. That is also why a partial failure is possible and has to be
//     cleaned up rather than rolled back.
//   - A failed rebuild leaves an *invalid* index behind — "_ccnew" if it died
//     during the build, "_ccold" if it died just after the swap. Either is dead
//     weight the planner will not use, and the second is a full-size copy of a
//     live index. Nothing in PostgreSQL ever removes them, so a job that failed
//     every month would accumulate one every month. Clearing them is therefore
//     the first thing a run does, not an afterthought.
func (r *AnnounceEventRepo) Reindex(ctx context.Context) (repository.ReindexResult, error) {
	var result repository.ReindexResult

	dropped, err := r.dropInvalidIndexes(ctx)
	result.LeftoversDropped = dropped
	if err != nil {
		return result, err
	}

	if result.BytesBefore, err = r.indexesSize(ctx); err != nil {
		return result, err
	}

	// REINDEX TABLE rather than one statement per index: it covers the primary
	// key as well, and a new index added by a later migration is included without
	// anyone having to remember to add it here.
	if _, err := r.db.ExecContext(ctx,
		`REINDEX TABLE CONCURRENTLY `+quoteIdentifier(announceEventsTable)); err != nil {
		return result, fmt.Errorf("reindexing %s: %w", announceEventsTable, err)
	}

	if result.BytesAfter, err = r.indexesSize(ctx); err != nil {
		return result, err
	}
	return result, nil
}

func (r *AnnounceEventRepo) indexesSize(ctx context.Context) (int64, error) {
	var size int64
	if err := r.db.QueryRowContext(ctx,
		`SELECT pg_indexes_size($1::regclass)`, announceEventsTable).Scan(&size); err != nil {
		return 0, fmt.Errorf("measuring index size: %w", err)
	}
	return size, nil
}

// dropInvalidIndexes removes the wreckage of a previous failed REINDEX. It
// returns how many it dropped, even alongside an error, so the caller can report
// partial progress.
//
// A failed concurrent rebuild leaves one of two things, depending on where it
// died, and both have to go:
//
//   - "_ccnew", the half-built replacement, when it failed during the build.
//     Verified by cancelling a REINDEX TABLE CONCURRENTLY mid-flight.
//   - "_ccold", the original that it swapped out but could not drop, when it
//     failed in the narrow window after the swap. This one is the expensive
//     case: it is a full-size copy of a live index, not a partial stub, so it
//     roughly doubles pg_indexes_size and stays that way.
//
// Neither is ever cleaned up by PostgreSQL. Worse, a later rebuild does not fail
// on them — it emits "cannot reindex invalid index ... skipping" as a server
// notice, which the driver discards, so the job would log a success while the
// dead bytes sat there being counted in its own before/after numbers.
//
// Two predicates, and the important one is invalidity rather than the name: an
// invalid index cannot be used by the planner, so dropping it can never take
// away something that was doing work. The name match then keeps this to
// PostgreSQL's own generated names, so an invalid index someone is building by
// hand is left for them. It is a substring rather than a suffix match because
// PostgreSQL appends a counter ("_ccnew1") when the name is already taken.
func (r *AnnounceEventRepo) dropInvalidIndexes(ctx context.Context) (int, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT c.relname
		  FROM pg_class c
		  JOIN pg_index i ON i.indexrelid = c.oid
		  JOIN pg_class t ON t.oid = i.indrelid
		 WHERE t.relname = $1
		   AND NOT i.indisvalid
		   AND (c.relname LIKE '%\_ccnew%' OR c.relname LIKE '%\_ccold%')`, announceEventsTable)
	if err != nil {
		return 0, fmt.Errorf("finding invalid indexes: %w", err)
	}

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			_ = rows.Close()
			return 0, fmt.Errorf("scanning invalid index name: %w", err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, fmt.Errorf("iterating invalid indexes: %w", err)
	}
	if err := rows.Close(); err != nil {
		return 0, fmt.Errorf("closing invalid index rows: %w", err)
	}

	// CONCURRENTLY again, for the same reason as the rebuild: a plain DROP INDEX
	// takes an ACCESS EXCLUSIVE lock on announce_events, and the announce path
	// writes to it constantly.
	var n int
	for _, name := range names {
		if _, err := r.db.ExecContext(ctx,
			`DROP INDEX CONCURRENTLY IF EXISTS `+quoteIdentifier(name)); err != nil {
			return n, fmt.Errorf("dropping invalid index %q: %w", name, err)
		}
		n++
	}
	return n, nil
}

// quoteIdentifier renders a name safe to interpolate into DDL, which is the only
// way to name an index or a table in a statement that admits no bind parameters.
// The names reaching it come from pg_class rather than from a request, so this is
// defence in depth rather than the primary control — but an identifier built by
// concatenation is exactly the shape that stops being safe when someone reuses
// the helper, so it is written to be safe on its own terms.
func quoteIdentifier(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
