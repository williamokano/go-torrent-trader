package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// HnR errors are value errors (bad admin input), distinct from storage failures.
var (
	ErrHnRGroupNotFound    = fmt.Errorf("group not found")
	ErrHnRStaffGroup       = fmt.Errorf("staff groups cannot be subject to hit-and-run tracking")
	ErrHnRRuleNotFound     = fmt.Errorf("hit-and-run rule not found")
	ErrHnRInvalidThreshold = fmt.Errorf("hit-and-run thresholds must be zero or positive")
	ErrHnRStageNotFound    = fmt.Errorf("hit-and-run penalty stage not found")
	ErrHnRInvalidStage     = fmt.Errorf("invalid hit-and-run penalty stage")
	ErrHnRRecordNotFound   = fmt.Errorf("hit-and-run record not found")
)

// HnRRuleInput is the admin-supplied threshold set for one class.
type HnRRuleInput struct {
	RequiredSeedHours    int     `json:"required_seed_hours"`
	RequiredRatio        float64 `json:"required_ratio"`
	InactivityGraceHours int     `json:"inactivity_grace_hours"`
	MaxDaysToSatisfy     int     `json:"max_days_to_satisfy"`
}

// HnRRuleView is a rule joined with its group, for the admin UI.
type HnRRuleView struct {
	GroupID              int64   `json:"group_id"`
	GroupName            string  `json:"group_name"`
	GroupLevel           int     `json:"group_level"`
	IsStaff              bool    `json:"is_staff"`
	RequiredSeedHours    int     `json:"required_seed_hours"`
	RequiredRatio        float64 `json:"required_ratio"`
	InactivityGraceHours int     `json:"inactivity_grace_hours"`
	MaxDaysToSatisfy     int     `json:"max_days_to_satisfy"`
}

// HnRService handles hit-and-run tracking business logic: per-class rule
// configuration, the daemon's breach/satisfaction sweep, plus (built out
// across the feature's remaining pieces) the penalty ladder, the
// member-facing read path, and clearing with points. The announce-path
// accounting itself (CreateIfNotExists / Accumulate) is called directly by
// TrackerService against repository.HnRRepository, the same way it already
// talks to TransferHistoryRepository and AnnounceEventRepository — not
// routed through this service.
type HnRService struct {
	// db is used only for the daemon's cross-process advisory lock (see
	// RunDaemon) — every other operation goes through hnr, the repository
	// interface, exactly like TorrentService holds both db and a repository
	// for the same reason (a lock/transaction the interface doesn't expose).
	db     *sql.DB
	hnr    repository.HnRRepository
	groups repository.GroupRepository
	// users resolves a username for the penalty-ladder notification event —
	// the daemon otherwise only ever sees user ids (ActiveHnRCounts is keyed
	// by id, not joined to users).
	users repository.UserRepository
	// warnings and restrictions carry out the ladder's "warn"/"ban" and
	// "restrict" stage actions respectively, reusing the same services and
	// the same RestrictionSourceHnR attribution PR1 added — the ladder never
	// writes a warning or restriction row itself.
	warnings     *WarningService
	restrictions *RestrictionService
	settings     *SiteSettingsService
	// eventBus publishes HnRStageChangedEvent on every ladder transition, in
	// both directions, for the notification listener to turn into a
	// NotifHitAndRun entry — the same "service publishes, listener creates
	// the notification" shape used everywhere else in this codebase.
	eventBus event.Bus
}

// NewHnRService creates a new HnRService.
func NewHnRService(
	db *sql.DB,
	hnr repository.HnRRepository,
	groups repository.GroupRepository,
	users repository.UserRepository,
	warnings *WarningService,
	restrictions *RestrictionService,
	settings *SiteSettingsService,
	bus event.Bus,
) *HnRService {
	return &HnRService{
		db: db, hnr: hnr, groups: groups, users: users,
		warnings: warnings, restrictions: restrictions, settings: settings, eventBus: bus,
	}
}

// ListRules returns every rule joined with its group, ordered by level.
func (s *HnRService) ListRules(ctx context.Context) ([]HnRRuleView, error) {
	rules, err := s.hnr.ListRules(ctx)
	if err != nil {
		return nil, err
	}
	groups, err := s.groups.List(ctx)
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]model.Group, len(groups))
	for _, g := range groups {
		byID[g.ID] = g
	}

	views := make([]HnRRuleView, 0, len(rules))
	for _, r := range rules {
		g, ok := byID[r.GroupID]
		if !ok {
			continue // group vanished; rule will be cascaded away
		}
		views = append(views, HnRRuleView{
			GroupID:              r.GroupID,
			GroupName:            g.Name,
			GroupLevel:           g.Level,
			IsStaff:              g.IsAdmin || g.IsModerator,
			RequiredSeedHours:    r.RequiredSeedHours,
			RequiredRatio:        r.RequiredRatio,
			InactivityGraceHours: r.InactivityGraceHours,
			MaxDaysToSatisfy:     r.MaxDaysToSatisfy,
		})
	}
	sort.Slice(views, func(i, j int) bool {
		if views[i].GroupLevel != views[j].GroupLevel {
			return views[i].GroupLevel < views[j].GroupLevel
		}
		return views[i].GroupID < views[j].GroupID
	})
	return views, nil
}

// UpsertRule creates or updates a class's HnR rule, refusing staff groups and
// negative thresholds. A class with no rule is not subject to HnR at all —
// this is how "VIP has no hit-and-run" is expressed, with no special-case
// code anywhere else.
func (s *HnRService) UpsertRule(ctx context.Context, groupID int64, in HnRRuleInput) (*model.HnRRule, error) {
	group, err := s.groups.GetByID(ctx, groupID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrHnRGroupNotFound
		}
		return nil, fmt.Errorf("get group: %w", err)
	}
	if group.IsAdmin || group.IsModerator {
		return nil, ErrHnRStaffGroup
	}
	if in.RequiredSeedHours < 0 || in.RequiredRatio < 0 || in.InactivityGraceHours < 0 || in.MaxDaysToSatisfy < 0 {
		return nil, ErrHnRInvalidThreshold
	}

	rule := &model.HnRRule{
		GroupID:              groupID,
		RequiredSeedHours:    in.RequiredSeedHours,
		RequiredRatio:        in.RequiredRatio,
		InactivityGraceHours: in.InactivityGraceHours,
		MaxDaysToSatisfy:     in.MaxDaysToSatisfy,
	}
	if err := s.hnr.UpsertRule(ctx, rule); err != nil {
		return nil, fmt.Errorf("upsert hnr rule: %w", err)
	}
	return rule, nil
}

// DeleteRule removes a class from HnR tracking entirely.
func (s *HnRService) DeleteRule(ctx context.Context, groupID int64) error {
	if err := s.hnr.DeleteRule(ctx, groupID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrHnRRuleNotFound
		}
		return fmt.Errorf("delete hnr rule: %w", err)
	}
	return nil
}

// Advisory lock keys for RunDaemon's two-stage lock (see its doc comment).
// Arbitrary but fixed and distinct from every other advisory lock this
// application takes (e.g. announceReindexLockKey in the postgres package);
// spelled out from ASCII like that key is, for the same reason: a number
// that reads back as "hnr_q"/"hnr_r" is easier to recognise in pg_locks than
// an opaque one.
const (
	hnrDaemonQueueLockKey int64 = 0x686e725f71 // "hnr_q"
	hnrDaemonRunLockKey   int64 = 0x686e725f72 // "hnr_r"
)

// ErrHnRDaemonUnavailable means RunDaemon was called on an HnRService built
// without a database handle (db is nil) — the locking it needs has no
// connection to pin. Every other HnRService method works fine without one.
var ErrHnRDaemonUnavailable = errors.New("hnr daemon unavailable: no database handle for locking")

// HnRRunSummary is what one daemon invocation returns to its caller (the
// worker task, or the admin "run now" endpoint).
type HnRRunSummary struct {
	// Skipped is true when another instance was already queued (waiting or
	// about to wait) for its turn — this invocation dropped rather than
	// queuing behind the queue, since a third run has nothing to add.
	Skipped bool
	RunID   int64
	Counts  repository.HnRRunCounts
}

// RunDaemon evaluates every open hnr_records row and moves it to breach,
// satisfied, or waived as appropriate, then logs the run. It is safe to call
// concurrently, including from more than one process against the same
// database: a two-stage advisory lock on a pinned connection makes "only one
// running, a second waits and then runs immediately, a third drops" true
// across the whole deployment, not just within one Go process.
//
// The two locks exist because a single non-blocking try-lock cannot express
// "wait, but only one waiter": hnrDaemonQueueLockKey is the one waiting
// slot — held only while blocked on hnrDaemonRunLockKey, and released the
// instant that blocking acquire returns (success or failure), never while
// the run itself is in progress. A second invocation arriving while the
// first runs takes the free queue slot, then blocks on the run lock until
// the first releases it, then proceeds immediately. A third invocation's
// try-lock on the queue fails immediately (someone is already waiting), so
// it drops without ever touching the run lock.
func (s *HnRService) RunDaemon(ctx context.Context, trigger string, triggeredBy *int64) (HnRRunSummary, error) {
	if s.db == nil {
		return HnRRunSummary{}, ErrHnRDaemonUnavailable
	}

	// Pinned to one connection because both advisory locks are
	// session-scoped: taking them through the pool would let database/sql
	// hand the unlock to a different connection, which silently does
	// nothing and leaves the lock held until that session is recycled —
	// the pooling trap tasks/lessons.md records against the leader-election
	// test, and the same reasoning AnnounceEventRepo.Reindex documents.
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return HnRRunSummary{}, fmt.Errorf("hnr daemon: acquiring a connection for the run lock: %w", err)
	}
	defer func() { _ = conn.Close() }()

	var queued bool
	if err := conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock($1)`, hnrDaemonQueueLockKey).Scan(&queued); err != nil {
		return HnRRunSummary{}, fmt.Errorf("hnr daemon: taking the queue lock: %w", err)
	}
	if !queued {
		return HnRRunSummary{Skipped: true}, nil
	}
	releaseQueueLock := func() {
		// context.WithoutCancel: a cancellation arriving between statements
		// must not block this unlock — see the identical reasoning on
		// AnnounceEventRepo.Reindex's release.
		if _, err := conn.ExecContext(context.WithoutCancel(ctx),
			`SELECT pg_advisory_unlock($1)`, hnrDaemonQueueLockKey); err != nil {
			slog.Warn("hnr daemon: could not release the queue lock explicitly; "+
				"it is released when the connection closes", "error", err)
		}
	}

	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, hnrDaemonRunLockKey); err != nil {
		releaseQueueLock()
		return HnRRunSummary{}, fmt.Errorf("hnr daemon: taking the run lock: %w", err)
	}
	// The queue slot is only for waiting — release it now that we hold the
	// run lock and are about to actually run, so a new invocation arriving
	// mid-run can take the (now free) queue slot rather than being told a
	// waiter already exists.
	releaseQueueLock()
	defer func() {
		if _, err := conn.ExecContext(context.WithoutCancel(ctx),
			`SELECT pg_advisory_unlock($1)`, hnrDaemonRunLockKey); err != nil {
			slog.Warn("hnr daemon: could not release the run lock explicitly; "+
				"it is released when the connection closes", "error", err)
		}
	}()

	return s.runLocked(ctx, trigger, triggeredBy)
}

// runLocked is RunDaemon's body once the run lock is held — split out so the
// evaluate/mark logic can be unit-tested against a fake repository without a
// real database, while the locking itself is validated separately against a
// live Postgres instance (advisory locks have no meaningful fake).
func (s *HnRService) runLocked(ctx context.Context, trigger string, triggeredBy *int64) (HnRRunSummary, error) {
	runID, err := s.hnr.StartRun(ctx, trigger, triggeredBy)
	if err != nil {
		return HnRRunSummary{}, fmt.Errorf("hnr daemon: start run: %w", err)
	}

	counts, evalErr := s.evaluateAndMark(ctx)

	// The ladder reads ActiveHnRCounts, which reflects the breach/satisfy
	// transitions evaluateAndMark just made — so it runs after, in the same
	// sweep, not as a separate pass. Skipped when the sweep itself failed:
	// escalating against counts from a table evaluateAndMark couldn't
	// finish updating would risk acting on a stale picture.
	if evalErr == nil {
		advanced, decayed, err := s.runLadder(ctx, time.Now())
		if err != nil {
			evalErr = fmt.Errorf("ladder: %w", err)
		} else {
			counts.StagesAdvanced = advanced
			counts.StagesDecayed = decayed
		}
	}

	status := model.HnRRunStatusSuccess
	var errMsg *string
	if evalErr != nil {
		status = model.HnRRunStatusFailed
		msg := evalErr.Error()
		errMsg = &msg
	}
	if err := s.hnr.FinishRun(ctx, runID, status, counts, errMsg); err != nil {
		slog.Error("hnr daemon: failed to record run outcome", "run_id", runID, "error", err)
	}

	if evalErr != nil {
		return HnRRunSummary{RunID: runID, Counts: counts}, fmt.Errorf("hnr daemon: evaluate: %w", evalErr)
	}
	return HnRRunSummary{RunID: runID, Counts: counts}, nil
}

// evaluateAndMark is the daemon's core sweep: evaluate every open record
// against the shared evaluator and batch-transition it to breach, satisfied,
// or waived. Ladder-stage advancement/decay (PR4) and retention purging
// (PR6) extend this same run rather than living in a separate pass, so a
// single hnr_runs row still reflects one coherent sweep of the table.
func (s *HnRService) evaluateAndMark(ctx context.Context) (repository.HnRRunCounts, error) {
	var counts repository.HnRRunCounts

	inputs, err := s.hnr.ListOpenForEvaluation(ctx)
	if err != nil {
		return counts, fmt.Errorf("list open records: %w", err)
	}
	counts.Scanned = len(inputs)

	now := time.Now()
	var breachIDs, satisfiedIDs, waivedIDs []int64
	for _, in := range inputs {
		switch EvaluateHnRRecord(in, now) {
		case HnRStatusBreach:
			breachIDs = append(breachIDs, in.Record.ID)
		case HnRStatusSatisfied:
			satisfiedIDs = append(satisfiedIDs, in.Record.ID)
		case HnRStatusExempt:
			waivedIDs = append(waivedIDs, in.Record.ID)
		case HnRStatusMonitoring:
			// Nothing to do yet.
		}
	}

	if len(breachIDs) > 0 {
		n, err := s.hnr.MarkBreached(ctx, breachIDs, now)
		if err != nil {
			return counts, fmt.Errorf("mark breached: %w", err)
		}
		counts.Breached = int(n)
	}
	if len(satisfiedIDs) > 0 {
		n, err := s.hnr.MarkSatisfied(ctx, satisfiedIDs, now)
		if err != nil {
			return counts, fmt.Errorf("mark satisfied: %w", err)
		}
		counts.Satisfied = int(n)
	}
	if len(waivedIDs) > 0 {
		if _, err := s.hnr.MarkWaived(ctx, waivedIDs, now); err != nil {
			return counts, fmt.Errorf("mark waived: %w", err)
		}
	}

	return counts, nil
}

// HnRRecordView is one obligation as shown to the member who owns it: the
// stored record plus a live-evaluated DisplayStatus and, when the member's
// class currently carries a rule, the thresholds to show progress against.
// DisplayStatus is computed by the same EvaluateHnRRecord the daemon uses,
// called fresh on every request rather than read off the stored State — see
// the "Real-time member status" design note. It can disagree with State
// (State still "active" but DisplayStatus already "breach") whenever the
// daemon has not yet run since the grace deadline passed; it should never
// disagree in the other direction, because the announce path itself flips a
// breached record back to State "active" the instant a seeding announce
// proves the member resumed — DisplayStatus only ever gets ahead of State,
// never behind it.
type HnRRecordView struct {
	ID            int64      `json:"id"`
	TorrentID     int64      `json:"torrent_id"`
	TorrentName   string     `json:"torrent_name"`
	TorrentSize   int64      `json:"torrent_size"`
	State         string     `json:"state"`
	DisplayStatus string     `json:"display_status"` // breach | monitoring | satisfied | cleared | waived
	CompletedAt   time.Time  `json:"completed_at"`
	LastSeenAt    time.Time  `json:"last_seen_at"`
	SeededSeconds int64      `json:"seeded_seconds"`
	Uploaded      int64      `json:"uploaded"`
	BreachedAt    *time.Time `json:"breached_at,omitempty"`
	ResolvedAt    *time.Time `json:"resolved_at,omitempty"`
	// CurrentlySeeding is the live peers overlay: true when the member has an
	// active seeding peer for this torrent right now, straight from the
	// tracker's own peer table — the one place in the schema with zero lag.
	// It is what makes "start the client and the page clears" true even
	// before the accumulator's next credited announce lands.
	CurrentlySeeding bool `json:"currently_seeding"`

	// The class's current thresholds, omitted when the member's class
	// carries no rule at all (an exempt class, e.g. VIP) — nil, not zero
	// values, so the frontend can tell "no requirement" apart from "not
	// applicable".
	RequiredSeedHours    *int     `json:"required_seed_hours,omitempty"`
	RequiredRatio        *float64 `json:"required_ratio,omitempty"`
	InactivityGraceHours *int     `json:"inactivity_grace_hours,omitempty"`
}

// ListForUser returns every hit-and-run obligation the member has ever had
// (breach-first, then monitored, then resolved — see hnrStateOrder), with
// DisplayStatus evaluated live against their current class rule and the
// live seeding overlay, never the stale daemon-updated State column alone.
func (s *HnRService) ListForUser(ctx context.Context, userID int64) ([]HnRRecordView, error) {
	records, err := s.hnr.ListForUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list hnr records: %w", err)
	}
	if len(records) == 0 {
		return []HnRRecordView{}, nil
	}

	rule, err := s.hnr.GetRuleForUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get hnr rule for user: %w", err)
	}

	var openTorrentIDs []int64
	for _, rec := range records {
		if rec.State == model.HnRStateActive || rec.State == model.HnRStateBreach {
			openTorrentIDs = append(openTorrentIDs, rec.TorrentID)
		}
	}
	liveSeeding, err := s.hnr.LiveSeedingTorrentIDs(ctx, userID, openTorrentIDs)
	if err != nil {
		return nil, fmt.Errorf("live seeding overlay: %w", err)
	}

	now := time.Now()
	views := make([]HnRRecordView, 0, len(records))
	for _, rec := range records {
		views = append(views, buildHnRRecordView(rec, rule, liveSeeding[rec.TorrentID], now))
	}
	return views, nil
}

func buildHnRRecordView(rec model.HnRRecord, rule *model.HnRRule, seeding bool, now time.Time) HnRRecordView {
	view := HnRRecordView{
		ID: rec.ID, TorrentID: rec.TorrentID, TorrentName: rec.TorrentName, TorrentSize: rec.TorrentSize,
		State: rec.State, CompletedAt: rec.CompletedAt, LastSeenAt: rec.LastSeenAt,
		SeededSeconds: rec.SeededSeconds, Uploaded: rec.Uploaded,
		BreachedAt: rec.BreachedAt, ResolvedAt: rec.ResolvedAt,
		CurrentlySeeding: seeding,
	}
	if rule != nil {
		view.RequiredSeedHours = &rule.RequiredSeedHours
		view.RequiredRatio = &rule.RequiredRatio
		view.InactivityGraceHours = &rule.InactivityGraceHours
	}

	switch rec.State {
	case model.HnRStateActive, model.HnRStateBreach:
		in := repository.HnREvalInput{
			Record: rec, Rule: rule, TorrentSize: rec.TorrentSize, TorrentExempt: rec.TorrentExempt,
		}
		status := EvaluateHnRRecord(in, now)
		if seeding && status == HnRStatusBreach {
			// An active seeding peer right now outranks a stale
			// last_seen_at: the member has proven they are seeding, so the
			// display must not say breach even though the accumulator's
			// next credited announce (which would recover it for real)
			// hasn't landed yet.
			status = HnRStatusMonitoring
		}
		if status == HnRStatusExempt {
			// Open in storage but no longer applicable (class or torrent
			// changed since the snatch) — display it where it is headed
			// once the daemon catches up, not as still-open.
			view.DisplayStatus = model.HnRStateWaived
		} else {
			view.DisplayStatus = string(status)
		}
	default:
		// satisfied, cleared, waived: already resolved: nothing to
		// re-evaluate, and re-deriving it live risks disagreeing with the
		// terminal state actually on record.
		view.DisplayStatus = rec.State
	}
	return view
}

// LastRun and ListRuns expose the daemon's run log for staff visibility.
func (s *HnRService) LastRun(ctx context.Context) (*model.HnRRun, bool, error) {
	return s.hnr.LastRun(ctx)
}

func (s *HnRService) ListRuns(ctx context.Context, limit int) ([]model.HnRRun, error) {
	return s.hnr.ListRuns(ctx, limit)
}
