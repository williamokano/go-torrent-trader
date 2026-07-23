package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// emptyMetadata is the JSONB default written when a torrent has no metadata
// values, so the NOT NULL metadata column never receives a NULL param.
var emptyMetadata = json.RawMessage("{}")

func metadataOrDefault(m json.RawMessage) json.RawMessage {
	if len(m) == 0 {
		return emptyMetadata
	}
	return m
}

const torrentColumns = `t.id, t.name, t.info_hash, t.size, t.description, t.nfo, t.category_id,
	c.name AS category_name, c.image_url AS category_image_url,
	t.uploader_id, t.anonymous, t.seeders, t.leechers, t.times_completed, t.comments_count,
	t.visible, t.banned, t.free, t.silver, t.file_count, t.files, t.metadata,
	CASE WHEN t.anonymous THEN 'Anonymous' ELSE COALESCE(u.username, 'Unknown') END AS uploader_name,
	CASE WHEN t.anonymous THEN false ELSE COALESCE(u.warned, false) END AS uploader_warned,
	t.moderation_status, t.assigned_moderator_id, COALESCE(am.username, '') AS assigned_moderator_name,
	t.approved_by, COALESCE(ap.username, '') AS approved_by_name, t.approved_at,
	t.created_at, t.updated_at`

// torrentJoins is the FROM/JOIN tail shared by every query that selects
// torrentColumns. It resolves the category, the uploader (u), the assigned
// moderator (am), and the approver (ap).
const torrentJoins = `FROM torrents t
	JOIN categories c ON t.category_id = c.id
	LEFT JOIN users u ON t.uploader_id = u.id
	LEFT JOIN users am ON t.assigned_moderator_id = am.id
	LEFT JOIN users ap ON t.approved_by = ap.id`

// TorrentRepo implements repository.TorrentRepository using PostgreSQL.
type TorrentRepo struct {
	db DBTX
}

// NewTorrentRepo returns a new PostgreSQL-backed TorrentRepository.
func NewTorrentRepo(db *sql.DB) *TorrentRepo {
	return &TorrentRepo{db: db}
}

// WithTx returns a copy of the repo bound to the given transaction.
func (r *TorrentRepo) WithTx(tx *sql.Tx) *TorrentRepo {
	return &TorrentRepo{db: tx}
}

func scanTorrent(row interface{ Scan(...any) error }) (*model.Torrent, error) {
	var t model.Torrent
	err := row.Scan(
		&t.ID, &t.Name, &t.InfoHash, &t.Size, &t.Description, &t.Nfo,
		&t.CategoryID, &t.CategoryName, &t.CategoryImageURL,
		&t.UploaderID, &t.Anonymous, &t.Seeders, &t.Leechers,
		&t.TimesCompleted, &t.CommentsCount, &t.Visible, &t.Banned,
		&t.Free, &t.Silver, &t.FileCount, &t.Files, &t.Metadata, &t.UploaderName,
		&t.UploaderWarned,
		&t.ModerationStatus, &t.AssignedModeratorID, &t.AssignedModeratorName,
		&t.ApprovedBy, &t.ApprovedByName, &t.ApprovedAt,
		&t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *TorrentRepo) GetByID(ctx context.Context, id int64) (*model.Torrent, error) {
	query := fmt.Sprintf("SELECT %s %s WHERE t.id = $1", torrentColumns, torrentJoins)
	return scanTorrent(r.db.QueryRowContext(ctx, query, id))
}

func (r *TorrentRepo) GetByInfoHash(ctx context.Context, infoHash []byte) (*model.Torrent, error) {
	query := fmt.Sprintf("SELECT %s %s WHERE t.info_hash = $1", torrentColumns, torrentJoins)
	return scanTorrent(r.db.QueryRowContext(ctx, query, infoHash))
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

	// Default: only show visible, non-banned, approved torrents (skip for admin
	// context). The approved-only filter keeps pending/rejected submissions out of
	// every public listing (BE-8.22).
	if !opts.IncludeHidden {
		conditions = append(conditions,
			"t.visible = true", "t.banned = false",
			"t.moderation_status = 'approved'")
	}

	if opts.CategoryID != nil {
		conditions = append(conditions, fmt.Sprintf("t.category_id = %s", nextArg()))
		args = append(args, *opts.CategoryID)
	}

	if opts.CreatedAfter != nil {
		conditions = append(conditions, fmt.Sprintf("t.created_at >= %s", nextArg()))
		args = append(args, *opts.CreatedAfter)
	}

	if opts.MaxSeeders != nil {
		conditions = append(conditions, fmt.Sprintf("t.seeders <= %s", nextArg()))
		args = append(args, *opts.MaxSeeders)
	}

	if opts.UploaderID != nil {
		conditions = append(conditions, fmt.Sprintf("t.uploader_id = %s", nextArg()))
		args = append(args, *opts.UploaderID)
	}

	if opts.ExcludeAnonymous {
		conditions = append(conditions, "t.anonymous = false")
	}

	// Metadata predicates (BE-3.13a). Equality filters are merged into a single
	// JSONB containment check so they can use the GIN index on torrents.metadata
	// (migration 066); numeric range filters compare the extracted value and are
	// guarded by jsonb_typeof so a non-numeric stored value can never abort the
	// ::numeric cast for the whole query.
	metaEq := make(map[string]any)
	for _, mf := range opts.MetadataFilters {
		switch mf.Op {
		case repository.MetaFilterGte, repository.MetaFilterLte:
			op := ">="
			if mf.Op == repository.MetaFilterLte {
				op = "<="
			}
			keyPh := nextArg()
			args = append(args, mf.Key)
			valPh := nextArg()
			args = append(args, mf.Value)
			// keyPh is referenced twice; Postgres allows reusing a placeholder.
			conditions = append(conditions, fmt.Sprintf(
				"jsonb_typeof(t.metadata -> %s) = 'number' AND (t.metadata ->> %s)::numeric %s %s",
				keyPh, keyPh, op, valPh,
			))
		default: // MetaFilterEq
			metaEq[mf.Key] = mf.Value
		}
	}
	if len(metaEq) > 0 {
		raw, mErr := json.Marshal(metaEq)
		if mErr != nil {
			return nil, 0, fmt.Errorf("encoding metadata filter: %w", mErr)
		}
		conditions = append(conditions, fmt.Sprintf("t.metadata @> %s::jsonb", nextArg()))
		args = append(args, string(raw))
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
			prefixQuery := BuildPrefixQuery(opts.Search)
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
		args = append(args, BuildPrefixQuery(opts.Search))
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
		"SELECT %s %s %s ORDER BY %s LIMIT %s OFFSET %s",
		torrentColumns, torrentJoins, where, orderClause, nextArg(), nextArg(),
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
		visible, banned, free, silver, file_count, files, metadata,
		moderation_status, approved_by, approved_at
	) VALUES (
		$1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
		$11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22
	) RETURNING id, created_at, updated_at`

	return r.db.QueryRowContext(ctx, query,
		torrent.Name, torrent.InfoHash, torrent.Size, torrent.Description,
		torrent.Nfo, torrent.CategoryID, torrent.UploaderID, torrent.Anonymous,
		torrent.Seeders, torrent.Leechers, torrent.TimesCompleted, torrent.CommentsCount,
		torrent.Visible, torrent.Banned, torrent.Free, torrent.Silver, torrent.FileCount,
		torrent.Files, metadataOrDefault(torrent.Metadata),
		moderationStatusOrDefault(torrent.ModerationStatus), torrent.ApprovedBy, torrent.ApprovedAt,
	).Scan(&torrent.ID, &torrent.CreatedAt, &torrent.UpdatedAt)
}

// moderationStatusOrDefault guards the NOT NULL moderation_status column so a
// zero-value model.Torrent (e.g. from a test that doesn't set it) still inserts a
// valid status instead of an empty string that would violate the CHECK constraint.
func moderationStatusOrDefault(s string) string {
	if s == "" {
		return model.ModerationPending
	}
	return s
}

func (r *TorrentRepo) Update(ctx context.Context, torrent *model.Torrent) error {
	query := `UPDATE torrents SET
		name = $1, info_hash = $2, size = $3, description = $4, nfo = $5,
		category_id = $6, uploader_id = $7, anonymous = $8, seeders = $9,
		leechers = $10, times_completed = $11, comments_count = $12,
		visible = $13, banned = $14, free = $15, silver = $16,
		file_count = $17, files = $18, metadata = $19, updated_at = NOW()
	WHERE id = $20
	RETURNING updated_at`

	return r.db.QueryRowContext(ctx, query,
		torrent.Name, torrent.InfoHash, torrent.Size, torrent.Description,
		torrent.Nfo, torrent.CategoryID, torrent.UploaderID, torrent.Anonymous,
		torrent.Seeders, torrent.Leechers, torrent.TimesCompleted, torrent.CommentsCount,
		torrent.Visible, torrent.Banned, torrent.Free, torrent.Silver,
		torrent.FileCount, torrent.Files, metadataOrDefault(torrent.Metadata), torrent.ID,
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

func (r *TorrentRepo) ListByUploader(ctx context.Context, uploaderID int64, limit int) ([]model.Torrent, error) {
	query := fmt.Sprintf("SELECT %s %s WHERE t.uploader_id = $1 AND t.visible = true AND t.banned = false AND t.moderation_status = 'approved' ORDER BY t.created_at DESC LIMIT $2", torrentColumns, torrentJoins)
	rows, err := r.db.QueryContext(ctx, query, uploaderID, limit)
	if err != nil {
		return nil, fmt.Errorf("listing torrents by uploader: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var torrents []model.Torrent
	for rows.Next() {
		t, err := scanTorrent(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning torrent: %w", err)
		}
		torrents = append(torrents, *t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating torrents: %w", err)
	}
	return torrents, nil
}

// ClaimModeration assigns (or reassigns) the torrent's moderator. Staff can steal a
// claim from a moderator who is taking too long, so this unconditionally overwrites.
func (r *TorrentRepo) ClaimModeration(ctx context.Context, torrentID, moderatorID int64) error {
	return r.execAffectingTorrent(ctx,
		`UPDATE torrents SET assigned_moderator_id = $1, updated_at = NOW() WHERE id = $2`,
		moderatorID, torrentID)
}

// UnclaimModeration clears the torrent's assigned moderator.
func (r *TorrentRepo) UnclaimModeration(ctx context.Context, torrentID int64) error {
	return r.execAffectingTorrent(ctx,
		`UPDATE torrents SET assigned_moderator_id = NULL, updated_at = NOW() WHERE id = $1`,
		torrentID)
}

// ApproveTorrent marks the torrent approved and records who approved it (their name
// surfaces via the ap join in torrentColumns). The assigned moderator is cleared —
// it's queue state that no longer applies once the item leaves the queue.
func (r *TorrentRepo) ApproveTorrent(ctx context.Context, torrentID, approverID int64) error {
	return r.execAffectingTorrent(ctx,
		`UPDATE torrents SET moderation_status = 'approved', approved_by = $1, approved_at = NOW(), assigned_moderator_id = NULL, updated_at = NOW() WHERE id = $2`,
		approverID, torrentID)
}

// RejectTorrent marks the torrent rejected, clearing any prior approval.
func (r *TorrentRepo) RejectTorrent(ctx context.Context, torrentID int64) error {
	return r.execAffectingTorrent(ctx,
		`UPDATE torrents SET moderation_status = 'rejected', approved_by = NULL, approved_at = NULL, updated_at = NOW() WHERE id = $1`,
		torrentID)
}

// execAffectingTorrent runs a single-row-affecting UPDATE, returning sql.ErrNoRows
// when the torrent id doesn't exist so the service maps it to a 404.
func (r *TorrentRepo) execAffectingTorrent(ctx context.Context, query string, args ...any) error {
	result, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update torrent moderation: %w", err)
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

// ListModerationQueue returns torrents in the requested moderation status, oldest
// first (FIFO), optionally scoped to a moderator's own claims or to unassigned
// items. Total is the count matching the same filters.
func (r *TorrentRepo) ListModerationQueue(ctx context.Context, opts repository.ModerationQueueOptions) (_ []model.Torrent, _ int64, err error) {
	status := opts.Status
	if status == "" {
		status = model.ModerationPending
	}

	var (
		conditions = []string{"t.moderation_status = $1"}
		args       = []any{status}
		argIdx     = 1
	)
	nextArg := func(v any) string {
		argIdx++
		args = append(args, v)
		return fmt.Sprintf("$%d", argIdx)
	}

	switch opts.Assigned {
	case repository.ModAssignedUnassigned:
		conditions = append(conditions, "t.assigned_moderator_id IS NULL")
	case repository.ModAssignedMine:
		conditions = append(conditions, "t.assigned_moderator_id = "+nextArg(opts.ModeratorID))
	}

	where := "WHERE " + strings.Join(conditions, " AND ")

	var total int64
	if err = r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM torrents t "+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting moderation queue: %w", err)
	}

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

	query := fmt.Sprintf("SELECT %s %s %s ORDER BY t.created_at ASC LIMIT %s OFFSET %s",
		torrentColumns, torrentJoins, where, nextArg(perPage), nextArg(offset))

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("querying moderation queue: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("closing moderation queue rows: %w", cerr)
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
		return nil, 0, fmt.Errorf("iterating moderation queue: %w", err)
	}
	return torrents, total, nil
}

// ListMissingRequiredMetadata returns torrents in a category whose metadata is
// missing at least one of requiredKeys — i.e. a field the category now marks
// required has no stored value. When uploaderID is non-nil the result is scoped
// to that uploader. Only the columns the metadata-issues report needs are
// populated on the returned torrents.
func (r *TorrentRepo) ListMissingRequiredMetadata(ctx context.Context, categoryID int64, requiredKeys []string, uploaderID *int64) ([]model.Torrent, error) {
	if len(requiredKeys) == 0 {
		return nil, nil
	}

	var (
		args   []any
		argIdx int
	)
	next := func(v any) string {
		argIdx++
		args = append(args, v)
		return fmt.Sprintf("$%d", argIdx)
	}

	conditions := []string{fmt.Sprintf("t.category_id = %s", next(categoryID))}

	// A torrent satisfies the schema when it has every required key; select the
	// ones missing at least one. jsonb_exists avoids the `?` operator so no
	// driver mistakes it for a bind placeholder.
	existsChecks := make([]string, len(requiredKeys))
	for i, key := range requiredKeys {
		existsChecks[i] = fmt.Sprintf("jsonb_exists(t.metadata, %s)", next(key))
	}
	conditions = append(conditions, fmt.Sprintf("NOT (%s)", strings.Join(existsChecks, " AND ")))

	if uploaderID != nil {
		conditions = append(conditions, fmt.Sprintf("t.uploader_id = %s", next(*uploaderID)))
	}

	query := fmt.Sprintf(`SELECT t.id, t.name, t.category_id, c.name, t.uploader_id, t.anonymous, t.metadata,
			CASE WHEN t.anonymous THEN 'Anonymous' ELSE COALESCE(u.username, 'Unknown') END
		FROM torrents t
		JOIN categories c ON t.category_id = c.id
		LEFT JOIN users u ON t.uploader_id = u.id
		WHERE %s
		ORDER BY t.id`, strings.Join(conditions, " AND "))

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing torrents missing required metadata: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var torrents []model.Torrent
	for rows.Next() {
		var t model.Torrent
		if err := rows.Scan(&t.ID, &t.Name, &t.CategoryID, &t.CategoryName,
			&t.UploaderID, &t.Anonymous, &t.Metadata, &t.UploaderName); err != nil {
			return nil, fmt.Errorf("scanning torrent: %w", err)
		}
		torrents = append(torrents, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating torrents: %w", err)
	}
	return torrents, nil
}
