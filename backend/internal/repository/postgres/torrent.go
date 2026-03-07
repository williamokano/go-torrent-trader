package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

const torrentColumns = `t.id, t.name, t.info_hash, t.size, t.description, t.nfo, t.category_id,
	c.name AS category_name,
	t.uploader_id, t.anonymous, t.seeders, t.leechers, t.times_completed, t.comments_count,
	t.visible, t.banned, t.free, t.silver, t.file_count, t.created_at, t.updated_at`

// TorrentRepo implements repository.TorrentRepository using PostgreSQL.
type TorrentRepo struct {
	db *sql.DB
}

// NewTorrentRepo returns a new PostgreSQL-backed TorrentRepository.
func NewTorrentRepo(db *sql.DB) repository.TorrentRepository {
	return &TorrentRepo{db: db}
}

func scanTorrent(row interface{ Scan(...any) error }) (*model.Torrent, error) {
	var t model.Torrent
	err := row.Scan(
		&t.ID, &t.Name, &t.InfoHash, &t.Size, &t.Description, &t.Nfo,
		&t.CategoryID, &t.CategoryName,
		&t.UploaderID, &t.Anonymous, &t.Seeders, &t.Leechers,
		&t.TimesCompleted, &t.CommentsCount, &t.Visible, &t.Banned,
		&t.Free, &t.Silver, &t.FileCount, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *TorrentRepo) GetByID(ctx context.Context, id int64) (*model.Torrent, error) {
	query := fmt.Sprintf("SELECT %s FROM torrents t JOIN categories c ON t.category_id = c.id WHERE t.id = $1", torrentColumns)
	return scanTorrent(r.db.QueryRowContext(ctx, query, id))
}

func (r *TorrentRepo) GetByInfoHash(ctx context.Context, infoHash []byte) (*model.Torrent, error) {
	query := fmt.Sprintf("SELECT %s FROM torrents t JOIN categories c ON t.category_id = c.id WHERE t.info_hash = $1", torrentColumns)
	return scanTorrent(r.db.QueryRowContext(ctx, query, infoHash))
}

// allowedSortColumns restricts the columns that can be used for ORDER BY.
// buildPrefixQuery converts user input into a tsquery with prefix matching.
// "frie beyond" → "frie:* & beyond:*"
// Special characters are stripped to prevent tsquery syntax errors.
func buildPrefixQuery(search string) string {
	words := strings.Fields(search)
	var parts []string
	for _, w := range words {
		// Strip any tsquery operators to prevent injection
		cleaned := strings.Map(func(r rune) rune {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
				return r
			}
			return -1
		}, w)
		if cleaned != "" {
			parts = append(parts, cleaned+":*")
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " & ")
}

var allowedSortColumns = map[string]string{
	"name":       "t.name",
	"created_at": "t.created_at",
	"size":       "t.size",
	"seeders":    "t.seeders",
	"leechers":   "t.leechers",
}

func (r *TorrentRepo) List(ctx context.Context, opts repository.ListTorrentsOptions) (_ []model.Torrent, _ int64, err error) {
	var (
		conditions []string
		args       []any
		argIdx     int
	)

	nextArg := func() string {
		argIdx++
		return fmt.Sprintf("$%d", argIdx)
	}

	// Default: only show visible, non-banned torrents.
	conditions = append(conditions, "t.visible = true", "t.banned = false")

	if opts.CategoryID != nil {
		conditions = append(conditions, fmt.Sprintf("t.category_id = %s", nextArg()))
		args = append(args, *opts.CategoryID)
	}

	// useFullText tracks whether to apply ts_rank ordering.
	var useFullText bool

	if len(opts.Search) >= 2 {
		if len(opts.Search) < 3 {
			// Short queries (2 chars): fall back to ILIKE since tsvector
			// requires meaningful lexemes that short strings rarely produce.
			escaped := strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(opts.Search)
			conditions = append(conditions, fmt.Sprintf("t.name ILIKE %s", nextArg()))
			args = append(args, "%"+escaped+"%")
		} else {
			// 3+ chars: use PostgreSQL full-text search with prefix matching.
			// Convert "frie beyond" → "frie:* & beyond:*" for prefix search.
			prefixQuery := buildPrefixQuery(opts.Search)
			if prefixQuery != "" {
				conditions = append(conditions, fmt.Sprintf("t.search_vector @@ to_tsquery('english', %s)", nextArg()))
				args = append(args, prefixQuery)
				useFullText = true
			} else {
				// All special characters — fall back to ILIKE
				escaped := strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(opts.Search)
				conditions = append(conditions, fmt.Sprintf("t.name ILIKE %s", nextArg()))
				args = append(args, "%"+escaped+"%")
			}
		}
	}

	where := ""
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}

	// Count total matching rows.
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM torrents t %s", where)
	var total int64
	if err = r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting torrents: %w", err)
	}

	// Determine sort column and order (safe against SQL injection).
	sortCol := "t.created_at"
	if col, ok := allowedSortColumns[opts.SortBy]; ok {
		sortCol = col
	}

	sortOrder := "DESC"
	if strings.EqualFold(opts.SortOrder, "asc") {
		sortOrder = "ASC"
	}

	// When full-text search is active and no explicit sort was requested,
	// prepend ts_rank so the most relevant results appear first.
	orderClause := fmt.Sprintf("%s %s", sortCol, sortOrder)
	if useFullText && opts.SortBy == "" {
		orderClause = fmt.Sprintf("ts_rank(t.search_vector, to_tsquery('english', %s)) DESC, %s", nextArg(), orderClause)
		args = append(args, buildPrefixQuery(opts.Search))
	}

	// Pagination defaults.
	page := opts.Page
	if page < 1 {
		page = 1
	}
	perPage := opts.PerPage
	if perPage < 1 {
		perPage = 25
	}
	if perPage > 100 {
		perPage = 100
	}
	offset := (page - 1) * perPage

	selectQuery := fmt.Sprintf(
		"SELECT %s FROM torrents t JOIN categories c ON t.category_id = c.id %s ORDER BY %s LIMIT %s OFFSET %s",
		torrentColumns, where, orderClause, nextArg(), nextArg(),
	)
	args = append(args, perPage, offset)

	rows, err := r.db.QueryContext(ctx, selectQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("querying torrents: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("closing torrent rows: %w", cerr)
		}
	}()

	var torrents []model.Torrent
	for rows.Next() {
		t, err := scanTorrent(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scanning torrent: %w", err)
		}
		torrents = append(torrents, *t)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating torrents: %w", err)
	}

	return torrents, total, nil
}

func (r *TorrentRepo) Create(ctx context.Context, torrent *model.Torrent) error {
	query := `INSERT INTO torrents (
		name, info_hash, size, description, nfo, category_id, uploader_id,
		anonymous, seeders, leechers, times_completed, comments_count,
		visible, banned, free, silver, file_count
	) VALUES (
		$1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
		$11, $12, $13, $14, $15, $16, $17
	) RETURNING id, created_at, updated_at`

	return r.db.QueryRowContext(ctx, query,
		torrent.Name, torrent.InfoHash, torrent.Size, torrent.Description,
		torrent.Nfo, torrent.CategoryID, torrent.UploaderID, torrent.Anonymous,
		torrent.Seeders, torrent.Leechers, torrent.TimesCompleted, torrent.CommentsCount,
		torrent.Visible, torrent.Banned, torrent.Free, torrent.Silver, torrent.FileCount,
	).Scan(&torrent.ID, &torrent.CreatedAt, &torrent.UpdatedAt)
}

func (r *TorrentRepo) Update(ctx context.Context, torrent *model.Torrent) error {
	query := `UPDATE torrents SET
		name = $1, info_hash = $2, size = $3, description = $4, nfo = $5,
		category_id = $6, uploader_id = $7, anonymous = $8, seeders = $9,
		leechers = $10, times_completed = $11, comments_count = $12,
		visible = $13, banned = $14, free = $15, silver = $16,
		file_count = $17, updated_at = NOW()
	WHERE id = $18
	RETURNING updated_at`

	return r.db.QueryRowContext(ctx, query,
		torrent.Name, torrent.InfoHash, torrent.Size, torrent.Description,
		torrent.Nfo, torrent.CategoryID, torrent.UploaderID, torrent.Anonymous,
		torrent.Seeders, torrent.Leechers, torrent.TimesCompleted, torrent.CommentsCount,
		torrent.Visible, torrent.Banned, torrent.Free, torrent.Silver,
		torrent.FileCount, torrent.ID,
	).Scan(&torrent.UpdatedAt)
}

func (r *TorrentRepo) Delete(ctx context.Context, id int64) error {
	query := `DELETE FROM torrents WHERE id = $1`
	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("deleting torrent: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *TorrentRepo) IncrementSeeders(ctx context.Context, id int64, delta int) error {
	query := `UPDATE torrents SET seeders = GREATEST(0, seeders + $1), updated_at = NOW() WHERE id = $2`
	result, err := r.db.ExecContext(ctx, query, delta, id)
	if err != nil {
		return fmt.Errorf("incrementing seeders: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *TorrentRepo) IncrementTimesCompleted(ctx context.Context, id int64) error {
	query := `UPDATE torrents SET times_completed = times_completed + 1, updated_at = NOW() WHERE id = $1`
	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("incrementing times_completed: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *TorrentRepo) IncrementLeechers(ctx context.Context, id int64, delta int) error {
	query := `UPDATE torrents SET leechers = GREATEST(0, leechers + $1), updated_at = NOW() WHERE id = $2`
	result, err := r.db.ExecContext(ctx, query, delta, id)
	if err != nil {
		return fmt.Errorf("incrementing leechers: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
