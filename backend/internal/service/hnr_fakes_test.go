package service

import (
	"context"
	"database/sql"
	"sync"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// fakeHnRRepo is a full in-memory repository.HnRRepository, behaviorally
// mirroring the real SQL (CAS semantics, credit-cap arithmetic, transactional
// clearing) rather than shortcutting it — the same reasoning tasks/lessons.md
// gives for every other fake in this package: a test double must fail the way
// production fails. It anchors every HnRService test added across the
// feature, not just the rule-CRUD ones.
type fakeHnRRepo struct {
	mu sync.Mutex

	rules   map[int64]model.HnRRule
	records map[int64]*model.HnRRecord
	nextID  int64

	stages     map[int]model.HnRPenaltyStage
	userStates map[int64]model.HnRUserState

	runs      []model.HnRRun
	nextRunID int64

	// Auxiliary fixtures a real join would read from other tables. Tests set
	// these directly; production reads users/torrents/peers instead.
	torrentSize   map[int64]int64
	torrentExempt map[int64]bool
	userGroup     map[int64]int64
	liveSeeding   map[int64]map[int64]bool
	bonusPoints   map[int64]int64

	// bonusTransactions records every ledger entry ClearRecord writes, for
	// tests that assert on it.
	bonusTransactions []model.BonusTransaction

	// Call counters so a caller can assert "this was never invoked" (e.g. the
	// HnR-disabled announce path) precisely, rather than only inferring it
	// from unchanged state.
	createCalls     int
	accumulateCalls int
}

func newFakeHnRRepo() *fakeHnRRepo {
	return &fakeHnRRepo{
		rules:         map[int64]model.HnRRule{},
		records:       map[int64]*model.HnRRecord{},
		nextID:        1,
		stages:        map[int]model.HnRPenaltyStage{},
		userStates:    map[int64]model.HnRUserState{},
		nextRunID:     1,
		torrentSize:   map[int64]int64{},
		torrentExempt: map[int64]bool{},
		userGroup:     map[int64]int64{},
		liveSeeding:   map[int64]map[int64]bool{},
		bonusPoints:   map[int64]int64{},
	}
}

// --- test-only fixture setters (not part of the interface) ---

func (f *fakeHnRRepo) setTorrent(torrentID, size int64, exempt bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.torrentSize[torrentID] = size
	f.torrentExempt[torrentID] = exempt
}

func (f *fakeHnRRepo) setUserGroup(userID, groupID int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.userGroup[userID] = groupID
}

func (f *fakeHnRRepo) setLiveSeeding(userID, torrentID int64, seeding bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.liveSeeding[userID] == nil {
		f.liveSeeding[userID] = map[int64]bool{}
	}
	f.liveSeeding[userID][torrentID] = seeding
}

func (f *fakeHnRRepo) recordByUserTorrent(userID, torrentID int64) *model.HnRRecord {
	for _, r := range f.records {
		if r.UserID == userID && r.TorrentID == torrentID {
			return r
		}
	}
	return nil
}

// --- rule configuration ---

func (f *fakeHnRRepo) ListRules(_ context.Context) ([]model.HnRRule, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]model.HnRRule, 0, len(f.rules))
	for _, r := range f.rules {
		out = append(out, r)
	}
	return out, nil
}

func (f *fakeHnRRepo) GetRuleForGroup(_ context.Context, groupID int64) (*model.HnRRule, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.rules[groupID]
	if !ok {
		return nil, sql.ErrNoRows
	}
	return &r, nil
}

func (f *fakeHnRRepo) UpsertRule(_ context.Context, r *model.HnRRule) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rules[r.GroupID] = *r
	return nil
}

func (f *fakeHnRRepo) DeleteRule(_ context.Context, groupID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.rules[groupID]; !ok {
		return sql.ErrNoRows
	}
	delete(f.rules, groupID)
	return nil
}

// --- announce-path accounting ---

func (f *fakeHnRRepo) CreateIfNotExists(_ context.Context, userID, torrentID int64, completedAt time.Time) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.createCalls++
	if f.torrentExempt[torrentID] {
		return false, nil
	}
	if f.recordByUserTorrent(userID, torrentID) != nil {
		return false, nil
	}
	rec := &model.HnRRecord{
		ID: f.nextID, UserID: userID, TorrentID: torrentID, State: model.HnRStateActive,
		CompletedAt: completedAt, LastSeenAt: completedAt,
	}
	f.records[rec.ID] = rec
	f.nextID++
	return true, nil
}

func (f *fakeHnRRepo) Accumulate(_ context.Context, userID, torrentID int64, uploadDelta int64, creditCap time.Duration, now time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.accumulateCalls++
	rec := f.recordByUserTorrent(userID, torrentID)
	if rec == nil || (rec.State != model.HnRStateActive && rec.State != model.HnRStateBreach) {
		return nil
	}
	gap := now.Sub(rec.LastSeenAt)
	if gap <= creditCap && gap > 0 {
		rec.SeededSeconds += int64(gap.Seconds())
	}
	rec.Uploaded += uploadDelta
	rec.LastSeenAt = now
	if rec.State == model.HnRStateBreach {
		rec.State = model.HnRStateActive
	}
	return nil
}

// --- daemon inputs and transitions ---

func (f *fakeHnRRepo) ListOpenForEvaluation(_ context.Context) ([]repository.HnREvalInput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []repository.HnREvalInput
	for _, rec := range f.records {
		if rec.State != model.HnRStateActive && rec.State != model.HnRStateBreach {
			continue
		}
		in := repository.HnREvalInput{
			Record:        *rec,
			TorrentSize:   f.torrentSize[rec.TorrentID],
			TorrentExempt: f.torrentExempt[rec.TorrentID],
		}
		if gid, ok := f.userGroup[rec.UserID]; ok {
			if rule, ok := f.rules[gid]; ok {
				r := rule
				in.Rule = &r
			}
		}
		out = append(out, in)
	}
	return out, nil
}

func (f *fakeHnRRepo) markState(ids []int64, from map[string]bool, to, tsField string, now time.Time) int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	var n int64
	for _, id := range ids {
		rec, ok := f.records[id]
		if !ok || !from[rec.State] {
			continue
		}
		rec.State = to
		switch tsField {
		case "breached_at":
			rec.BreachedAt = &now
		case "resolved_at":
			rec.ResolvedAt = &now
		}
		n++
	}
	return n
}

func (f *fakeHnRRepo) MarkBreached(_ context.Context, ids []int64, now time.Time) (int64, error) {
	return f.markState(ids, map[string]bool{model.HnRStateActive: true}, model.HnRStateBreach, "breached_at", now), nil
}

func (f *fakeHnRRepo) MarkSatisfied(_ context.Context, ids []int64, now time.Time) (int64, error) {
	return f.markState(ids, map[string]bool{model.HnRStateActive: true, model.HnRStateBreach: true}, model.HnRStateSatisfied, "resolved_at", now), nil
}

func (f *fakeHnRRepo) MarkWaived(_ context.Context, ids []int64, now time.Time) (int64, error) {
	return f.markState(ids, map[string]bool{model.HnRStateActive: true, model.HnRStateBreach: true}, model.HnRStateWaived, "resolved_at", now), nil
}

func (f *fakeHnRRepo) PurgeResolved(_ context.Context, olderThan time.Time) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	resolved := map[string]bool{model.HnRStateSatisfied: true, model.HnRStateCleared: true, model.HnRStateWaived: true}
	var n int64
	for id, rec := range f.records {
		if resolved[rec.State] && rec.ResolvedAt != nil && rec.ResolvedAt.Before(olderThan) {
			delete(f.records, id)
			n++
		}
	}
	return n, nil
}

// --- penalty ladder configuration ---

func (f *fakeHnRRepo) ListStages(_ context.Context) ([]model.HnRPenaltyStage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]model.HnRPenaltyStage, 0, len(f.stages))
	for _, s := range f.stages {
		out = append(out, s)
	}
	return out, nil
}

func (f *fakeHnRRepo) UpsertStage(_ context.Context, s *model.HnRPenaltyStage) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stages[s.Stage] = *s
	return nil
}

func (f *fakeHnRRepo) DeleteStage(_ context.Context, stage int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.stages[stage]; !ok {
		return sql.ErrNoRows
	}
	delete(f.stages, stage)
	return nil
}

// --- per-user ladder position ---

func (f *fakeHnRRepo) ActiveHnRCounts(_ context.Context) (map[int64]int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	counts := map[int64]int{}
	for _, rec := range f.records {
		if rec.State == model.HnRStateBreach {
			counts[rec.UserID]++
		}
	}
	return counts, nil
}

func (f *fakeHnRRepo) UsersOnLadder(_ context.Context) ([]model.HnRUserState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []model.HnRUserState
	for _, s := range f.userStates {
		if s.Stage > 0 {
			out = append(out, s)
		}
	}
	return out, nil
}

func (f *fakeHnRRepo) GetUserState(_ context.Context, userID int64) (*model.HnRUserState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.userStates[userID]
	if !ok {
		return nil, sql.ErrNoRows
	}
	return &s, nil
}

func (f *fakeHnRRepo) EnsureUserState(_ context.Context, userID int64, now time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.userStates[userID]; !ok {
		f.userStates[userID] = model.HnRUserState{UserID: userID, Stage: 0, StageEnteredAt: now}
	}
	return nil
}

func (f *fakeHnRRepo) CASUserStage(_ context.Context, userID int64, expectedStage, newStage int, now time.Time) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.userStates[userID]
	if !ok || s.Stage != expectedStage {
		return false, nil
	}
	s.Stage = newStage
	s.StageEnteredAt = now
	s.UpdatedAt = now
	f.userStates[userID] = s
	return true, nil
}

func (f *fakeHnRRepo) SetLastNotifiedStage(_ context.Context, userID int64, stage int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	s := f.userStates[userID]
	s.UserID = userID
	s.LastNotifiedStage = stage
	f.userStates[userID] = s
	return nil
}

// --- run bookkeeping ---

func (f *fakeHnRRepo) StartRun(_ context.Context, trigger string, triggeredBy *int64) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := f.nextRunID
	f.nextRunID++
	f.runs = append(f.runs, model.HnRRun{
		ID: id, StartedAt: time.Now(), Status: model.HnRRunStatusRunning,
		Trigger: trigger, TriggeredBy: triggeredBy,
	})
	return id, nil
}

func (f *fakeHnRRepo) FinishRun(_ context.Context, runID int64, status string, counts repository.HnRRunCounts, errMsg *string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.runs {
		if f.runs[i].ID == runID {
			now := time.Now()
			f.runs[i].FinishedAt = &now
			f.runs[i].Status = status
			f.runs[i].Scanned = counts.Scanned
			f.runs[i].Breached = counts.Breached
			f.runs[i].Satisfied = counts.Satisfied
			f.runs[i].StagesAdvanced = counts.StagesAdvanced
			f.runs[i].StagesDecayed = counts.StagesDecayed
			f.runs[i].Purged = counts.Purged
			f.runs[i].Error = errMsg
			return nil
		}
	}
	return sql.ErrNoRows
}

func (f *fakeHnRRepo) LastRun(_ context.Context) (*model.HnRRun, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.runs) == 0 {
		return nil, false, nil
	}
	last := f.runs[len(f.runs)-1]
	return &last, true, nil
}

func (f *fakeHnRRepo) ListRuns(_ context.Context, limit int) ([]model.HnRRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if limit <= 0 || limit > len(f.runs) {
		limit = len(f.runs)
	}
	out := make([]model.HnRRun, limit)
	for i := 0; i < limit; i++ {
		out[i] = f.runs[len(f.runs)-1-i]
	}
	return out, nil
}

// --- member-facing read path ---

// withTorrentJoin fills in the fields a real JOIN against torrents would
// populate — TorrentSize and TorrentExempt matter to the evaluator, so a
// caller reading them off a record returned by ListForUser/GetForUser must
// see what the fixtures say, exactly like the daemon path already does via
// ListOpenForEvaluation. Caller must hold f.mu.
func (f *fakeHnRRepo) withTorrentJoin(r model.HnRRecord) model.HnRRecord {
	r.TorrentSize = f.torrentSize[r.TorrentID]
	r.TorrentExempt = f.torrentExempt[r.TorrentID]
	return r
}

func (f *fakeHnRRepo) ListForUser(_ context.Context, userID int64) ([]model.HnRRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []model.HnRRecord
	for _, r := range f.records {
		if r.UserID == userID {
			out = append(out, f.withTorrentJoin(*r))
		}
	}
	return out, nil
}

func (f *fakeHnRRepo) GetForUser(_ context.Context, userID, recordID int64) (*model.HnRRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.records[recordID]
	if !ok || r.UserID != userID {
		return nil, sql.ErrNoRows
	}
	cp := f.withTorrentJoin(*r)
	return &cp, nil
}

func (f *fakeHnRRepo) GetRuleForUser(_ context.Context, userID int64) (*model.HnRRule, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	gid, ok := f.userGroup[userID]
	if !ok {
		return nil, nil
	}
	rule, ok := f.rules[gid]
	if !ok {
		return nil, nil
	}
	r := rule
	return &r, nil
}

func (f *fakeHnRRepo) LiveSeedingTorrentIDs(_ context.Context, userID int64, torrentIDs []int64) (map[int64]bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[int64]bool{}
	for _, tid := range torrentIDs {
		if f.liveSeeding[userID][tid] {
			out[tid] = true
		}
	}
	return out, nil
}

// --- clearing with bonus points ---

func (f *fakeHnRRepo) ClearRecord(_ context.Context, userID, recordID, price int64) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	rec, ok := f.records[recordID]
	if !ok || rec.UserID != userID || (rec.State != model.HnRStateActive && rec.State != model.HnRStateBreach) {
		return 0, repository.ErrHnRRecordNotClearable
	}
	if f.bonusPoints[userID] < price {
		return 0, repository.ErrInsufficientBonusPoints
	}
	f.bonusPoints[userID] -= price
	now := time.Now()
	rec.State = model.HnRStateCleared
	rec.ResolvedAt = &now
	f.bonusTransactions = append(f.bonusTransactions, model.BonusTransaction{
		UserID: userID, Delta: -price, Reason: model.BonusReasonHnRClear, RefID: &recordID, CreatedAt: now,
	})
	return f.bonusPoints[userID], nil
}

// --- staff visibility ---

func (f *fakeHnRRepo) AdminList(_ context.Context, opts repository.HnRAdminListOptions) ([]model.HnRRecord, int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var matched []model.HnRRecord
	for _, r := range f.records {
		if opts.State != nil && r.State != *opts.State {
			continue
		}
		if opts.UserID != nil && r.UserID != *opts.UserID {
			continue
		}
		matched = append(matched, f.withTorrentJoin(*r))
	}
	total := int64(len(matched))
	page, perPage := opts.Page, opts.PerPage
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 25
	}
	start := (page - 1) * perPage
	if start > len(matched) {
		start = len(matched)
	}
	end := start + perPage
	if end > len(matched) {
		end = len(matched)
	}
	return matched[start:end], total, nil
}

func (f *fakeHnRRepo) AggregateStats(_ context.Context) (repository.HnRAggregateStats, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var stats repository.HnRAggregateStats
	for _, r := range f.records {
		switch r.State {
		case model.HnRStateBreach:
			stats.ActiveHnR++
		case model.HnRStateActive:
			stats.Monitored++
		case model.HnRStateSatisfied:
			stats.Satisfied++
		case model.HnRStateCleared:
			stats.Cleared++
		case model.HnRStateWaived:
			stats.Waived++
		}
	}
	return stats, nil
}

func (f *fakeHnRRepo) TopOffenders(_ context.Context, limit int) ([]repository.HnROffender, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	byUser := map[int64]*repository.HnROffender{}
	for _, r := range f.records {
		o, ok := byUser[r.UserID]
		if !ok {
			o = &repository.HnROffender{UserID: r.UserID, Stage: f.userStates[r.UserID].Stage}
			byUser[r.UserID] = o
		}
		o.TotalRecords++
		if r.State == model.HnRStateBreach {
			o.ActiveHnR++
		}
	}
	var out []repository.HnROffender
	for _, o := range byUser {
		if o.ActiveHnR > 0 {
			out = append(out, *o)
		}
	}
	if limit > 0 && limit < len(out) {
		out = out[:limit]
	}
	return out, nil
}

var _ repository.HnRRepository = (*fakeHnRRepo)(nil)
