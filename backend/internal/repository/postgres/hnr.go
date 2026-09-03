package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// HnRRepo implements repository.HnRRepository using PostgreSQL.
type HnRRepo struct {
	db *sql.DB
}

// NewHnRRepo returns a new PostgreSQL-backed HnRRepository.
func NewHnRRepo(db *sql.DB) *HnRRepo {
	return &HnRRepo{db: db}
}

// --- rule configuration -----------------------------------------------------

func (r *HnRRepo) ListRules(ctx context.Context) ([]model.HnRRule, error) {
	query := `SELECT group_id, required_seed_hours, required_ratio, inactivity_grace_hours,
		max_days_to_satisfy, created_at, updated_at
		FROM hnr_rules ORDER BY group_id`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list hnr rules: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var rules []model.HnRRule
	for rows.Next() {
		var rule model.HnRRule
		if err := rows.Scan(
			&rule.GroupID, &rule.RequiredSeedHours, &rule.RequiredRatio,
			&rule.InactivityGraceHours, &rule.MaxDaysToSatisfy, &rule.CreatedAt, &rule.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan hnr rule: %w", err)
		}
		rules = append(rules, rule)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate hnr rules: %w", err)
	}
	return rules, nil
}

func (r *HnRRepo) GetRuleForGroup(ctx context.Context, groupID int64) (*model.HnRRule, error) {
	query := `SELECT group_id, required_seed_hours, required_ratio, inactivity_grace_hours,
		max_days_to_satisfy, created_at, updated_at
		FROM hnr_rules WHERE group_id = $1`
	var rule model.HnRRule
	err := r.db.QueryRowContext(ctx, query, groupID).Scan(
		&rule.GroupID, &rule.RequiredSeedHours, &rule.RequiredRatio,
		&rule.InactivityGraceHours, &rule.MaxDaysToSatisfy, &rule.CreatedAt, &rule.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &rule, nil
}

func (r *HnRRepo) UpsertRule(ctx context.Context, rule *model.HnRRule) error {
	query := `INSERT INTO hnr_rules
		(group_id, required_seed_hours, required_ratio, inactivity_grace_hours, max_days_to_satisfy)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (group_id) DO UPDATE SET
			required_seed_hours = EXCLUDED.required_seed_hours,
			required_ratio = EXCLUDED.required_ratio,
			inactivity_grace_hours = EXCLUDED.inactivity_grace_hours,
			max_days_to_satisfy = EXCLUDED.max_days_to_satisfy,
			updated_at = NOW()
		RETURNING created_at, updated_at`
	return r.db.QueryRowContext(ctx, query,
		rule.GroupID, rule.RequiredSeedHours, rule.RequiredRatio,
		rule.InactivityGraceHours, rule.MaxDaysToSatisfy,
	).Scan(&rule.CreatedAt, &rule.UpdatedAt)
}

func (r *HnRRepo) DeleteRule(ctx context.Context, groupID int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM hnr_rules WHERE group_id = $1`, groupID)
	if err != nil {
		return fmt.Errorf("delete hnr rule: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete hnr rule rows affected: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// --- announce-path accounting ------------------------------------------------

// CreateIfNotExists inserts a new open hnr_records row for (userID, torrentID)
// unless one already exists (ON CONFLICT DO NOTHING on the unique pair) or the
// torrent is currently hnr_exempt. Returns whether a row was actually inserted.
func (r *HnRRepo) CreateIfNotExists(ctx context.Context, userID, torrentID int64, completedAt time.Time) (bool, error) {
	query := `INSERT INTO hnr_records (user_id, torrent_id, completed_at, last_seen_at)
		SELECT $1, $2, $3, $3
		WHERE NOT EXISTS (SELECT 1 FROM torrents WHERE id = $2 AND hnr_exempt = true)
		ON CONFLICT (user_id, torrent_id) DO NOTHING`
	res, err := r.db.ExecContext(ctx, query, userID, torrentID, completedAt)
	if err != nil {
		return false, fmt.Errorf("create hnr record: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("create hnr record rows affected: %w", err)
	}
	return n > 0, nil
}

// Accumulate is the one atomic statement fed by every seeding announce for an
// open record: it credits seeded_seconds and uploaded since the last seeding
// announce, capped at creditCap (crediting nothing across a longer gap — see
// hnr_seed_credit_cap_minutes), and in the same statement recovers a 'hnr'
// record straight back to 'active'. creditCap is passed as seconds rather than
// bound as a Postgres INTERVAL parameter, which keeps the arithmetic in plain
// epoch-seconds instead of relying on driver-specific interval encoding.
func (r *HnRRepo) Accumulate(ctx context.Context, userID, torrentID int64, uploadDelta int64, creditCap time.Duration, now time.Time) error {
	query := `UPDATE hnr_records SET
		seeded_seconds = seeded_seconds + CASE
			WHEN EXTRACT(EPOCH FROM ($4::timestamptz - last_seen_at)) <= $3::double precision
			THEN GREATEST(EXTRACT(EPOCH FROM ($4::timestamptz - last_seen_at))::bigint, 0)
			ELSE 0 END,
		uploaded = uploaded + $5,
		last_seen_at = $4,
		state = CASE WHEN state = 'hnr' THEN 'active' ELSE state END
		WHERE user_id = $1 AND torrent_id = $2 AND state IN ('active', 'hnr')`
	_, err := r.db.ExecContext(ctx, query, userID, torrentID, creditCap.Seconds(), now, uploadDelta)
	if err != nil {
		return fmt.Errorf("accumulate hnr record: %w", err)
	}
	return nil
}

// --- daemon inputs and transitions ------------------------------------------

func (r *HnRRepo) ListOpenForEvaluation(ctx context.Context) ([]repository.HnREvalInput, error) {
	query := `SELECT
		hr.id, hr.user_id, hr.torrent_id, hr.state, hr.completed_at, hr.last_seen_at,
		hr.seeded_seconds, hr.uploaded, hr.breached_at, hr.resolved_at,
		ru.required_seed_hours, ru.required_ratio, ru.inactivity_grace_hours, ru.max_days_to_satisfy,
		t.size, t.hnr_exempt
		FROM hnr_records hr
		JOIN users u ON u.id = hr.user_id
		JOIN torrents t ON t.id = hr.torrent_id
		LEFT JOIN hnr_rules ru ON ru.group_id = u.group_id
		WHERE hr.state IN ('active', 'hnr')`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list open hnr records: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var inputs []repository.HnREvalInput
	for rows.Next() {
		var in repository.HnREvalInput
		var seedHours, graceHours, maxDays sql.NullInt64
		var ratio sql.NullFloat64
		if err := rows.Scan(
			&in.Record.ID, &in.Record.UserID, &in.Record.TorrentID, &in.Record.State,
			&in.Record.CompletedAt, &in.Record.LastSeenAt, &in.Record.SeededSeconds, &in.Record.Uploaded,
			&in.Record.BreachedAt, &in.Record.ResolvedAt,
			&seedHours, &ratio, &graceHours, &maxDays,
			&in.TorrentSize, &in.TorrentExempt,
		); err != nil {
			return nil, fmt.Errorf("scan hnr eval input: %w", err)
		}
		if seedHours.Valid {
			in.Rule = &model.HnRRule{
				GroupID:              0, // not needed by the evaluator
				RequiredSeedHours:    int(seedHours.Int64),
				RequiredRatio:        ratio.Float64,
				InactivityGraceHours: int(graceHours.Int64),
				MaxDaysToSatisfy:     int(maxDays.Int64),
			}
		}
		inputs = append(inputs, in)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate hnr eval inputs: %w", err)
	}
	return inputs, nil
}

func markState(ctx context.Context, db *sql.DB, ids []int64, from []string, to, timestampCol string, now time.Time) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	query := fmt.Sprintf(`UPDATE hnr_records SET state = $1, %s = $2
		WHERE id = ANY($3) AND state = ANY($4)`, timestampCol)
	res, err := db.ExecContext(ctx, query, to, now, ids, from)
	if err != nil {
		return 0, fmt.Errorf("mark hnr records %s: %w", to, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("mark hnr records %s rows affected: %w", to, err)
	}
	return n, nil
}

func (r *HnRRepo) MarkBreached(ctx context.Context, ids []int64, now time.Time) (int64, error) {
	return markState(ctx, r.db, ids, []string{model.HnRStateActive}, model.HnRStateBreach, "breached_at", now)
}

func (r *HnRRepo) MarkSatisfied(ctx context.Context, ids []int64, now time.Time) (int64, error) {
	return markState(ctx, r.db, ids, []string{model.HnRStateActive, model.HnRStateBreach}, model.HnRStateSatisfied, "resolved_at", now)
}

func (r *HnRRepo) MarkWaived(ctx context.Context, ids []int64, now time.Time) (int64, error) {
	return markState(ctx, r.db, ids, []string{model.HnRStateActive, model.HnRStateBreach}, model.HnRStateWaived, "resolved_at", now)
}

func (r *HnRRepo) PurgeResolved(ctx context.Context, olderThan time.Time) (int64, error) {
	query := `DELETE FROM hnr_records
		WHERE state IN ('satisfied', 'cleared', 'waived') AND resolved_at < $1`
	res, err := r.db.ExecContext(ctx, query, olderThan)
	if err != nil {
		return 0, fmt.Errorf("purge resolved hnr records: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("purge resolved hnr records rows affected: %w", err)
	}
	return n, nil
}

// --- penalty ladder configuration --------------------------------------------

// ListStages and UpsertStage marshal/unmarshal restriction_types as a JSON
// array rather than a native Postgres TEXT[]: the pgx v5 stdlib driver this
// project uses does not decode a Postgres array into a Go []string on Scan
// (confirmed against a live Postgres — Scan fails with "unsupported Scan,
// storing driver.Value type string into type *[]string"), so a real array
// column would need a driver-specific codec. JSON-in-text sidesteps that
// entirely and mirrors the existing wait_time_tiers setting's approach to
// "a list of values in one column".
func (r *HnRRepo) ListStages(ctx context.Context) ([]model.HnRPenaltyStage, error) {
	query := `SELECT stage, min_active_hnr, min_days_in_prev, action, restriction_types,
		restriction_days, message_template, created_at, updated_at
		FROM hnr_penalty_stages ORDER BY stage`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list hnr penalty stages: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var stages []model.HnRPenaltyStage
	for rows.Next() {
		var s model.HnRPenaltyStage
		var restrictionTypesJSON string
		if err := rows.Scan(
			&s.Stage, &s.MinActiveHnR, &s.MinDaysInPrev, &s.Action, &restrictionTypesJSON,
			&s.RestrictionDays, &s.MessageTemplate, &s.CreatedAt, &s.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan hnr penalty stage: %w", err)
		}
		if err := json.Unmarshal([]byte(restrictionTypesJSON), &s.RestrictionTypes); err != nil {
			return nil, fmt.Errorf("decode hnr penalty stage restriction types: %w", err)
		}
		stages = append(stages, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate hnr penalty stages: %w", err)
	}
	return stages, nil
}

func (r *HnRRepo) UpsertStage(ctx context.Context, stage *model.HnRPenaltyStage) error {
	restrictionTypes := stage.RestrictionTypes
	if restrictionTypes == nil {
		restrictionTypes = []string{}
	}
	restrictionTypesJSON, err := json.Marshal(restrictionTypes)
	if err != nil {
		return fmt.Errorf("encode hnr penalty stage restriction types: %w", err)
	}

	query := `INSERT INTO hnr_penalty_stages
		(stage, min_active_hnr, min_days_in_prev, action, restriction_types, restriction_days, message_template)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (stage) DO UPDATE SET
			min_active_hnr = EXCLUDED.min_active_hnr,
			min_days_in_prev = EXCLUDED.min_days_in_prev,
			action = EXCLUDED.action,
			restriction_types = EXCLUDED.restriction_types,
			restriction_days = EXCLUDED.restriction_days,
			message_template = EXCLUDED.message_template,
			updated_at = NOW()
		RETURNING created_at, updated_at`
	return r.db.QueryRowContext(ctx, query,
		stage.Stage, stage.MinActiveHnR, stage.MinDaysInPrev, stage.Action,
		string(restrictionTypesJSON), stage.RestrictionDays, stage.MessageTemplate,
	).Scan(&stage.CreatedAt, &stage.UpdatedAt)
}

func (r *HnRRepo) DeleteStage(ctx context.Context, stage int) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM hnr_penalty_stages WHERE stage = $1`, stage)
	if err != nil {
		return fmt.Errorf("delete hnr penalty stage: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete hnr penalty stage rows affected: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// --- per-user ladder position -------------------------------------------------

func (r *HnRRepo) ActiveHnRCounts(ctx context.Context) (map[int64]int, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT user_id, COUNT(*) FROM hnr_records WHERE state = 'hnr' GROUP BY user_id`)
	if err != nil {
		return nil, fmt.Errorf("active hnr counts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	counts := make(map[int64]int)
	for rows.Next() {
		var userID int64
		var count int
		if err := rows.Scan(&userID, &count); err != nil {
			return nil, fmt.Errorf("scan active hnr count: %w", err)
		}
		counts[userID] = count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate active hnr counts: %w", err)
	}
	return counts, nil
}

func (r *HnRRepo) UsersOnLadder(ctx context.Context) ([]model.HnRUserState, error) {
	query := `SELECT user_id, stage, stage_entered_at, last_notified_stage, updated_at
		FROM hnr_user_state WHERE stage > 0`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list users on hnr ladder: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var states []model.HnRUserState
	for rows.Next() {
		var s model.HnRUserState
		if err := rows.Scan(&s.UserID, &s.Stage, &s.StageEnteredAt, &s.LastNotifiedStage, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan hnr user state: %w", err)
		}
		states = append(states, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate hnr user states: %w", err)
	}
	return states, nil
}

func (r *HnRRepo) GetUserState(ctx context.Context, userID int64) (*model.HnRUserState, error) {
	query := `SELECT user_id, stage, stage_entered_at, last_notified_stage, updated_at
		FROM hnr_user_state WHERE user_id = $1`
	var s model.HnRUserState
	err := r.db.QueryRowContext(ctx, query, userID).Scan(
		&s.UserID, &s.Stage, &s.StageEnteredAt, &s.LastNotifiedStage, &s.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// EnsureUserState writes now explicitly rather than relying on the column's
// DEFAULT NOW() — the caller is about to run decideHnRLadderStage against
// this same now, and a separately-read database clock could land a hair
// after it, failing a zero-day dwell check for a user reaching the ladder
// for the first time in this run (see the interface doc).
func (r *HnRRepo) EnsureUserState(ctx context.Context, userID int64, now time.Time) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO hnr_user_state (user_id, stage_entered_at) VALUES ($1, $2) ON CONFLICT (user_id) DO NOTHING`, userID, now)
	if err != nil {
		return fmt.Errorf("ensure hnr user state: %w", err)
	}
	return nil
}

// CASUserStage moves userID from expectedStage to newStage. It is a no-op
// (false, nil) if the user is no longer at expectedStage — another instance
// already moved them — which is what makes the daemon's escalation safe to
// run from more than one process at once.
func (r *HnRRepo) CASUserStage(ctx context.Context, userID int64, expectedStage, newStage int, now time.Time) (bool, error) {
	query := `UPDATE hnr_user_state SET stage = $3, stage_entered_at = $4, updated_at = $4
		WHERE user_id = $1 AND stage = $2`
	res, err := r.db.ExecContext(ctx, query, userID, expectedStage, newStage, now)
	if err != nil {
		return false, fmt.Errorf("cas hnr user stage: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("cas hnr user stage rows affected: %w", err)
	}
	return n > 0, nil
}

func (r *HnRRepo) SetLastNotifiedStage(ctx context.Context, userID int64, stage int) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE hnr_user_state SET last_notified_stage = $2, updated_at = NOW() WHERE user_id = $1`,
		userID, stage)
	if err != nil {
		return fmt.Errorf("set last notified hnr stage: %w", err)
	}
	return nil
}

// --- run bookkeeping ----------------------------------------------------------

func (r *HnRRepo) StartRun(ctx context.Context, trigger string, triggeredBy *int64) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO hnr_runs (run_trigger, triggered_by) VALUES ($1, $2) RETURNING id`,
		trigger, triggeredBy,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("start hnr run: %w", err)
	}
	return id, nil
}

func (r *HnRRepo) FinishRun(ctx context.Context, runID int64, status string, counts repository.HnRRunCounts, errMsg *string) error {
	query := `UPDATE hnr_runs SET
		finished_at = NOW(), status = $2, scanned = $3, breached = $4, satisfied = $5,
		stages_advanced = $6, stages_decayed = $7, purged = $8, error = $9
		WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query,
		runID, status, counts.Scanned, counts.Breached, counts.Satisfied,
		counts.StagesAdvanced, counts.StagesDecayed, counts.Purged, errMsg,
	)
	if err != nil {
		return fmt.Errorf("finish hnr run: %w", err)
	}
	return nil
}

func scanHnRRun(row interface{ Scan(...any) error }) (*model.HnRRun, error) {
	var run model.HnRRun
	if err := row.Scan(
		&run.ID, &run.StartedAt, &run.FinishedAt, &run.Status, &run.Trigger, &run.TriggeredBy,
		&run.Scanned, &run.Breached, &run.Satisfied, &run.StagesAdvanced, &run.StagesDecayed,
		&run.Purged, &run.Error,
	); err != nil {
		return nil, err
	}
	return &run, nil
}

const hnrRunColumns = `id, started_at, finished_at, status, run_trigger, triggered_by,
	scanned, breached, satisfied, stages_advanced, stages_decayed, purged, error`

func (r *HnRRepo) LastRun(ctx context.Context) (*model.HnRRun, bool, error) {
	query := fmt.Sprintf(`SELECT %s FROM hnr_runs ORDER BY started_at DESC LIMIT 1`, hnrRunColumns)
	run, err := scanHnRRun(r.db.QueryRowContext(ctx, query))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("last hnr run: %w", err)
	}
	return run, true, nil
}

func (r *HnRRepo) ListRuns(ctx context.Context, limit int) ([]model.HnRRun, error) {
	if limit <= 0 {
		limit = 20
	}
	query := fmt.Sprintf(`SELECT %s FROM hnr_runs ORDER BY started_at DESC LIMIT $1`, hnrRunColumns)
	rows, err := r.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("list hnr runs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var runs []model.HnRRun
	for rows.Next() {
		run, err := scanHnRRun(rows)
		if err != nil {
			return nil, fmt.Errorf("scan hnr run: %w", err)
		}
		runs = append(runs, *run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate hnr runs: %w", err)
	}
	return runs, nil
}

// --- member-facing read path --------------------------------------------------

const hnrRecordColumns = `hr.id, hr.user_id, hr.torrent_id, hr.state, hr.completed_at, hr.last_seen_at,
	hr.seeded_seconds, hr.uploaded, hr.breached_at, hr.resolved_at, t.name, t.size, t.hnr_exempt`

func scanHnRRecordWithTorrent(row interface{ Scan(...any) error }) (*model.HnRRecord, error) {
	var rec model.HnRRecord
	if err := row.Scan(
		&rec.ID, &rec.UserID, &rec.TorrentID, &rec.State, &rec.CompletedAt, &rec.LastSeenAt,
		&rec.SeededSeconds, &rec.Uploaded, &rec.BreachedAt, &rec.ResolvedAt,
		&rec.TorrentName, &rec.TorrentSize, &rec.TorrentExempt,
	); err != nil {
		return nil, err
	}
	return &rec, nil
}

// hnrStateOrder is used to sort the member's list breach-first, then
// monitored, then resolved — matching the page's required section order.
const hnrStateOrder = `CASE hr.state
	WHEN 'hnr' THEN 0 WHEN 'active' THEN 1 WHEN 'satisfied' THEN 2
	WHEN 'cleared' THEN 3 WHEN 'waived' THEN 4 ELSE 5 END`

func (r *HnRRepo) ListForUser(ctx context.Context, userID int64) ([]model.HnRRecord, error) {
	query := fmt.Sprintf(`SELECT %s FROM hnr_records hr
		JOIN torrents t ON t.id = hr.torrent_id
		WHERE hr.user_id = $1
		ORDER BY %s, hr.completed_at DESC`, hnrRecordColumns, hnrStateOrder)

	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("list hnr records for user: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var records []model.HnRRecord
	for rows.Next() {
		rec, err := scanHnRRecordWithTorrent(rows)
		if err != nil {
			return nil, fmt.Errorf("scan hnr record: %w", err)
		}
		records = append(records, *rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate hnr records: %w", err)
	}
	return records, nil
}

func (r *HnRRepo) GetForUser(ctx context.Context, userID, recordID int64) (*model.HnRRecord, error) {
	query := fmt.Sprintf(`SELECT %s FROM hnr_records hr
		JOIN torrents t ON t.id = hr.torrent_id
		WHERE hr.user_id = $1 AND hr.id = $2`, hnrRecordColumns)
	return scanHnRRecordWithTorrent(r.db.QueryRowContext(ctx, query, userID, recordID))
}

// LiveSeedingTorrentIDs is the real-time overlay: which of torrentIDs the user
// currently has an active seeding peer for, straight from peers — the one
// place in the schema with zero lag.
func (r *HnRRepo) LiveSeedingTorrentIDs(ctx context.Context, userID int64, torrentIDs []int64) (map[int64]bool, error) {
	result := make(map[int64]bool)
	if len(torrentIDs) == 0 {
		return result, nil
	}
	query := `SELECT DISTINCT torrent_id FROM peers WHERE user_id = $1 AND torrent_id = ANY($2) AND seeder = true`
	rows, err := r.db.QueryContext(ctx, query, userID, torrentIDs)
	if err != nil {
		return nil, fmt.Errorf("live seeding torrent ids: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var torrentID int64
		if err := rows.Scan(&torrentID); err != nil {
			return nil, fmt.Errorf("scan live seeding torrent id: %w", err)
		}
		result[torrentID] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate live seeding torrent ids: %w", err)
	}
	return result, nil
}

// GetRuleForUser resolves a rule via the user's current class, the same
// LEFT JOIN ListOpenForEvaluation uses. seedHours.Valid false means the
// class carries no rule — reported as (nil, nil), not an error, exactly
// what the shared evaluator treats as HnRStatusExempt. A genuinely missing
// user (the inner JOIN on users matches no row) is sql.ErrNoRows.
func (r *HnRRepo) GetRuleForUser(ctx context.Context, userID int64) (*model.HnRRule, error) {
	query := `SELECT ru.required_seed_hours, ru.required_ratio, ru.inactivity_grace_hours, ru.max_days_to_satisfy
		FROM users u
		LEFT JOIN hnr_rules ru ON ru.group_id = u.group_id
		WHERE u.id = $1`
	var seedHours, graceHours, maxDays sql.NullInt64
	var ratio sql.NullFloat64
	if err := r.db.QueryRowContext(ctx, query, userID).Scan(&seedHours, &ratio, &graceHours, &maxDays); err != nil {
		return nil, err
	}
	if !seedHours.Valid {
		return nil, nil
	}
	return &model.HnRRule{
		RequiredSeedHours:    int(seedHours.Int64),
		RequiredRatio:        ratio.Float64,
		InactivityGraceHours: int(graceHours.Int64),
		MaxDaysToSatisfy:     int(maxDays.Int64),
	}, nil
}

// --- clearing with bonus points ------------------------------------------------

// ClearRecord mirrors BonusRepo.PurchaseItem: one transaction, a
// compare-and-set that only clears a record still open and owned by userID, a
// race-safe spend against users.bonus_points, and a bonus_transactions ledger
// row. price is computed by the caller and never trusted from the client.
func (r *HnRRepo) ClearRecord(ctx context.Context, userID, recordID, price int64) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin clear tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var clearedID int64
	err = tx.QueryRowContext(ctx,
		`UPDATE hnr_records SET state = 'cleared', resolved_at = NOW()
		 WHERE id = $1 AND user_id = $2 AND state IN ('active', 'hnr')
		 RETURNING id`, recordID, userID).Scan(&clearedID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, repository.ErrHnRRecordNotClearable
	}
	if err != nil {
		return 0, fmt.Errorf("clear hnr record: %w", err)
	}

	var newBalance int64
	err = tx.QueryRowContext(ctx,
		`UPDATE users SET bonus_points = bonus_points - $1 WHERE id = $2 AND bonus_points >= $1
		 RETURNING bonus_points`, price, userID).Scan(&newBalance)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, repository.ErrInsufficientBonusPoints
	}
	if err != nil {
		return 0, fmt.Errorf("spend points for hnr clear: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO bonus_transactions (user_id, delta, reason, ref_id) VALUES ($1, $2, $3, $4)`,
		userID, -price, model.BonusReasonHnRClear, recordID); err != nil {
		return 0, fmt.Errorf("ledger hnr clear: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit clear tx: %w", err)
	}
	return newBalance, nil
}

// --- staff visibility -----------------------------------------------------------

func (r *HnRRepo) AdminList(ctx context.Context, opts repository.HnRAdminListOptions) ([]model.HnRRecord, int64, error) {
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

	where := `WHERE ($1::text IS NULL OR hr.state = $1)
		AND ($2::bigint IS NULL OR hr.user_id = $2)
		AND ($3::text = '' OR u.username ILIKE '%' || $3 || '%' OR t.name ILIKE '%' || $3 || '%')`

	var total int64
	countQuery := `SELECT COUNT(*) FROM hnr_records hr
		JOIN torrents t ON t.id = hr.torrent_id
		JOIN users u ON u.id = hr.user_id ` + where
	if err := r.db.QueryRowContext(ctx, countQuery, opts.State, opts.UserID, opts.Search).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count hnr admin list: %w", err)
	}

	query := fmt.Sprintf(`SELECT %s, u.username FROM hnr_records hr
		JOIN torrents t ON t.id = hr.torrent_id
		JOIN users u ON u.id = hr.user_id
		%s
		ORDER BY hr.completed_at DESC
		LIMIT $4 OFFSET $5`, hnrRecordColumns, where)

	rows, err := r.db.QueryContext(ctx, query, opts.State, opts.UserID, opts.Search, perPage, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list hnr admin records: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var records []model.HnRRecord
	for rows.Next() {
		var rec model.HnRRecord
		if err := rows.Scan(
			&rec.ID, &rec.UserID, &rec.TorrentID, &rec.State, &rec.CompletedAt, &rec.LastSeenAt,
			&rec.SeededSeconds, &rec.Uploaded, &rec.BreachedAt, &rec.ResolvedAt,
			&rec.TorrentName, &rec.TorrentSize, &rec.TorrentExempt, &rec.Username,
		); err != nil {
			return nil, 0, fmt.Errorf("scan hnr admin record: %w", err)
		}
		records = append(records, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate hnr admin records: %w", err)
	}
	return records, total, nil
}

func (r *HnRRepo) AggregateStats(ctx context.Context) (repository.HnRAggregateStats, error) {
	query := `SELECT
		COUNT(*) FILTER (WHERE state = 'hnr') AS active_hnr,
		COUNT(*) FILTER (WHERE state = 'active') AS monitored,
		COUNT(*) FILTER (WHERE state = 'satisfied') AS satisfied,
		COUNT(*) FILTER (WHERE state = 'cleared') AS cleared,
		COUNT(*) FILTER (WHERE state = 'waived') AS waived,
		COUNT(*) FILTER (WHERE breached_at >= date_trunc('day', NOW())) AS breached_today
		FROM hnr_records`
	var stats repository.HnRAggregateStats
	err := r.db.QueryRowContext(ctx, query).Scan(
		&stats.ActiveHnR, &stats.Monitored, &stats.Satisfied, &stats.Cleared, &stats.Waived, &stats.BreachedToday,
	)
	if err != nil {
		return stats, fmt.Errorf("hnr aggregate stats: %w", err)
	}
	return stats, nil
}

func (r *HnRRepo) TopOffenders(ctx context.Context, limit int) ([]repository.HnROffender, error) {
	if limit <= 0 {
		limit = 20
	}
	query := `SELECT hr.user_id, u.username,
		COUNT(*) FILTER (WHERE hr.state = 'hnr') AS active_hnr,
		COUNT(*) AS total_records,
		COALESCE(hus.stage, 0) AS stage
		FROM hnr_records hr
		JOIN users u ON u.id = hr.user_id
		LEFT JOIN hnr_user_state hus ON hus.user_id = hr.user_id
		GROUP BY hr.user_id, u.username, hus.stage
		HAVING COUNT(*) FILTER (WHERE hr.state = 'hnr') > 0
		ORDER BY active_hnr DESC, total_records DESC
		LIMIT $1`
	rows, err := r.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("top hnr offenders: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var offenders []repository.HnROffender
	for rows.Next() {
		var o repository.HnROffender
		if err := rows.Scan(&o.UserID, &o.Username, &o.ActiveHnR, &o.TotalRecords, &o.Stage); err != nil {
			return nil, fmt.Errorf("scan hnr offender: %w", err)
		}
		offenders = append(offenders, o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate hnr offenders: %w", err)
	}
	return offenders, nil
}

var _ repository.HnRRepository = (*HnRRepo)(nil)
