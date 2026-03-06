package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

const reportColumns = `id, reporter_id, torrent_id, reason, resolved, resolved_by, resolved_at, created_at`

// ReportRepo implements repository.ReportRepository using PostgreSQL.
type ReportRepo struct {
	db *sql.DB
}

// NewReportRepo returns a new PostgreSQL-backed ReportRepository.
func NewReportRepo(db *sql.DB) repository.ReportRepository {
	return &ReportRepo{db: db}
}

func scanReport(row interface{ Scan(...any) error }) (*model.Report, error) {
	var r model.Report
	err := row.Scan(
		&r.ID, &r.ReporterID, &r.TorrentID, &r.Reason,
		&r.Resolved, &r.ResolvedBy, &r.ResolvedAt, &r.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (r *ReportRepo) Create(ctx context.Context, report *model.Report) error {
	query := `INSERT INTO reports (reporter_id, torrent_id, reason)
		VALUES ($1, $2, $3)
		RETURNING id, created_at`
	err := r.db.QueryRowContext(ctx, query,
		report.ReporterID, report.TorrentID, report.Reason,
	).Scan(&report.ID, &report.CreatedAt)
	if err != nil {
		return fmt.Errorf("create report: %w", err)
	}
	return nil
}

func (r *ReportRepo) GetByID(ctx context.Context, id int64) (*model.Report, error) {
	query := fmt.Sprintf("SELECT %s FROM reports WHERE id = $1", reportColumns)
	return scanReport(r.db.QueryRowContext(ctx, query, id))
}

func (r *ReportRepo) ExistsByReporterAndTorrent(ctx context.Context, reporterID int64, torrentID *int64) (bool, error) {
	var exists bool
	var query string
	var args []interface{}
	if torrentID != nil {
		query = `SELECT EXISTS(SELECT 1 FROM reports WHERE reporter_id = $1 AND torrent_id = $2)`
		args = []interface{}{reporterID, *torrentID}
	} else {
		query = `SELECT EXISTS(SELECT 1 FROM reports WHERE reporter_id = $1 AND torrent_id IS NULL)`
		args = []interface{}{reporterID}
	}
	if err := r.db.QueryRowContext(ctx, query, args...).Scan(&exists); err != nil {
		return false, fmt.Errorf("check existing report: %w", err)
	}
	return exists, nil
}

func (r *ReportRepo) List(ctx context.Context, opts repository.ListReportsOptions) ([]model.Report, int64, error) {
	// Defaults
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

	if opts.Status != nil {
		switch *opts.Status {
		case "resolved":
			where += fmt.Sprintf(" AND resolved = $%d", argIdx)
			args = append(args, true)
			argIdx++
		case "pending":
			where += fmt.Sprintf(" AND resolved = $%d", argIdx)
			args = append(args, false)
			argIdx++
		}
	}

	// Count
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM reports %s", where)
	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count reports: %w", err)
	}

	// Fetch
	offset := (page - 1) * perPage
	query := fmt.Sprintf("SELECT %s FROM reports %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d",
		reportColumns, where, argIdx, argIdx+1)
	args = append(args, perPage, offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list reports: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			_ = closeErr
		}
	}()

	var reports []model.Report
	for rows.Next() {
		var rpt model.Report
		if err := rows.Scan(
			&rpt.ID, &rpt.ReporterID, &rpt.TorrentID, &rpt.Reason,
			&rpt.Resolved, &rpt.ResolvedBy, &rpt.ResolvedAt, &rpt.CreatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan report: %w", err)
		}
		reports = append(reports, rpt)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate reports: %w", err)
	}

	return reports, total, nil
}

func (r *ReportRepo) Resolve(ctx context.Context, id, resolvedByUserID int64) error {
	query := `UPDATE reports SET resolved = true, resolved_by = $1, resolved_at = NOW() WHERE id = $2`
	res, err := r.db.ExecContext(ctx, query, resolvedByUserID, id)
	if err != nil {
		return fmt.Errorf("resolve report: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("resolve report rows affected: %w", err)
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}
