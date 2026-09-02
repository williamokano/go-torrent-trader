package postgres

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

func TestHnRRepo_RulesCRUD(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewHnRRepo(db)
	gid := groupIDBySlug(t, db, "user")

	rule := &model.HnRRule{
		GroupID: gid, RequiredSeedHours: 240, RequiredRatio: 1.0,
		InactivityGraceHours: 48, MaxDaysToSatisfy: 30,
	}
	if err := repo.UpsertRule(ctx, rule); err != nil {
		t.Fatalf("UpsertRule insert: %v", err)
	}
	if rule.CreatedAt.IsZero() {
		t.Fatal("UpsertRule did not populate timestamps")
	}

	rule.RequiredRatio = 0.8
	if err := repo.UpsertRule(ctx, rule); err != nil {
		t.Fatalf("UpsertRule update: %v", err)
	}

	got, err := repo.GetRuleForGroup(ctx, gid)
	if err != nil {
		t.Fatalf("GetRuleForGroup: %v", err)
	}
	if got.RequiredRatio != 0.8 {
		t.Fatalf("expected updated ratio 0.8, got %v", got.RequiredRatio)
	}

	rules, err := repo.ListRules(ctx)
	if err != nil {
		t.Fatalf("ListRules: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}

	if err := repo.DeleteRule(ctx, gid); err != nil {
		t.Fatalf("DeleteRule: %v", err)
	}
	if _, err := repo.GetRuleForGroup(ctx, gid); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("GetRuleForGroup(deleted) = %v, want sql.ErrNoRows", err)
	}
	if err := repo.DeleteRule(ctx, gid); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("DeleteRule(missing) = %v, want sql.ErrNoRows", err)
	}
}

func TestHnRRepo_CreateIfNotExists(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewHnRRepo(db)

	u := newUser(t, db)
	tor := newTorrent(t, db, u.ID)
	now := time.Now()

	created, err := repo.CreateIfNotExists(ctx, u.ID, tor.ID, now)
	if err != nil {
		t.Fatalf("CreateIfNotExists: %v", err)
	}
	if !created {
		t.Fatal("expected the first call to create a record")
	}

	created, err = repo.CreateIfNotExists(ctx, u.ID, tor.ID, now)
	if err != nil {
		t.Fatalf("CreateIfNotExists second call: %v", err)
	}
	if created {
		t.Fatal("expected the second call to be a no-op (ON CONFLICT DO NOTHING)")
	}

	records, err := repo.ListForUser(ctx, u.ID)
	if err != nil {
		t.Fatalf("ListForUser: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected exactly 1 record after two CreateIfNotExists calls, got %d", len(records))
	}
}

func TestHnRRepo_CreateIfNotExists_SkipsExemptTorrent(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewHnRRepo(db)

	u := newUser(t, db)
	tor := newTorrent(t, db, u.ID)
	if _, err := db.ExecContext(ctx, `UPDATE torrents SET hnr_exempt = true WHERE id = $1`, tor.ID); err != nil {
		t.Fatalf("flag exempt: %v", err)
	}

	created, err := repo.CreateIfNotExists(ctx, u.ID, tor.ID, time.Now())
	if err != nil {
		t.Fatalf("CreateIfNotExists: %v", err)
	}
	if created {
		t.Fatal("expected no record for an hnr_exempt torrent")
	}
}

func TestHnRRepo_Accumulate(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewHnRRepo(db)

	u := newUser(t, db)
	tor := newTorrent(t, db, u.ID)
	start := time.Now().Add(-time.Hour)

	if _, err := repo.CreateIfNotExists(ctx, u.ID, tor.ID, start); err != nil {
		t.Fatalf("CreateIfNotExists: %v", err)
	}
	// Backdate last_seen_at to `start` so the next Accumulate call has a real gap to credit.
	if _, err := db.ExecContext(ctx, `UPDATE hnr_records SET last_seen_at = $1 WHERE user_id = $2 AND torrent_id = $3`,
		start, u.ID, tor.ID); err != nil {
		t.Fatalf("backdate last_seen_at: %v", err)
	}

	// Within the credit cap: the whole gap is credited.
	mid := start.Add(10 * time.Minute)
	if err := repo.Accumulate(ctx, u.ID, tor.ID, 500, 45*time.Minute, mid); err != nil {
		t.Fatalf("Accumulate within cap: %v", err)
	}

	rec, err := repo.GetForUser(ctx, u.ID, mustRecordID(t, db, u.ID, tor.ID))
	if err != nil {
		t.Fatalf("GetForUser: %v", err)
	}
	if rec.SeededSeconds < 590 || rec.SeededSeconds > 610 {
		t.Errorf("expected ~600s credited, got %d", rec.SeededSeconds)
	}
	if rec.Uploaded != 500 {
		t.Errorf("expected uploaded=500, got %d", rec.Uploaded)
	}

	// Beyond the credit cap: nothing is credited for this gap, even though
	// wall-clock time passed — this is the anti-gaming guard.
	before := rec.SeededSeconds
	late := mid.Add(2 * time.Hour)
	if err := repo.Accumulate(ctx, u.ID, tor.ID, 100, 45*time.Minute, late); err != nil {
		t.Fatalf("Accumulate beyond cap: %v", err)
	}
	rec, err = repo.GetForUser(ctx, u.ID, rec.ID)
	if err != nil {
		t.Fatalf("GetForUser after gap: %v", err)
	}
	if rec.SeededSeconds != before {
		t.Errorf("expected no additional seed credit across a gap beyond the cap: before=%d after=%d", before, rec.SeededSeconds)
	}
	if rec.Uploaded != 600 {
		t.Errorf("upload should still accumulate across a stale gap: got %d", rec.Uploaded)
	}
}

func TestHnRRepo_Accumulate_RecoversBreachToActive(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewHnRRepo(db)

	u := newUser(t, db)
	tor := newTorrent(t, db, u.ID)
	if _, err := repo.CreateIfNotExists(ctx, u.ID, tor.ID, time.Now()); err != nil {
		t.Fatalf("CreateIfNotExists: %v", err)
	}
	recordID := mustRecordID(t, db, u.ID, tor.ID)

	if n, err := repo.MarkBreached(ctx, []int64{recordID}, time.Now()); err != nil || n != 1 {
		t.Fatalf("MarkBreached: n=%d err=%v", n, err)
	}

	rec, err := repo.GetForUser(ctx, u.ID, recordID)
	if err != nil {
		t.Fatalf("GetForUser: %v", err)
	}
	if rec.State != model.HnRStateBreach {
		t.Fatalf("expected state=hnr after MarkBreached, got %s", rec.State)
	}

	// A seeding announce is proof of resumed seeding: Accumulate must recover
	// the record straight back to active in the same statement.
	if err := repo.Accumulate(ctx, u.ID, tor.ID, 0, 45*time.Minute, time.Now()); err != nil {
		t.Fatalf("Accumulate: %v", err)
	}
	rec, err = repo.GetForUser(ctx, u.ID, recordID)
	if err != nil {
		t.Fatalf("GetForUser after recovery: %v", err)
	}
	if rec.State != model.HnRStateActive {
		t.Fatalf("expected state=active after a seeding announce recovered the breach, got %s", rec.State)
	}
}

// TestHnRRepo_Accumulate_ConcurrentAnnouncesNeverExceedWallClock is the
// concurrency guarantee the announce-path design depends on: two "peers" for
// the same (user, torrent) announcing concurrently must never let the
// credited seed time exceed the wall-clock time that actually elapsed. The
// single atomic UPDATE (no read-modify-write in Go) is what makes this true.
func TestHnRRepo_Accumulate_ConcurrentAnnouncesNeverExceedWallClock(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewHnRRepo(db)

	u := newUser(t, db)
	tor := newTorrent(t, db, u.ID)
	start := time.Now().Add(-time.Minute)
	if _, err := repo.CreateIfNotExists(ctx, u.ID, tor.ID, start); err != nil {
		t.Fatalf("CreateIfNotExists: %v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE hnr_records SET last_seen_at = $1 WHERE user_id = $2 AND torrent_id = $3`,
		start, u.ID, tor.ID); err != nil {
		t.Fatalf("backdate: %v", err)
	}

	const goroutines = 10
	var wg sync.WaitGroup
	wg.Add(goroutines)
	now := time.Now()
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			if err := repo.Accumulate(ctx, u.ID, tor.ID, 10, 45*time.Minute, now); err != nil {
				t.Errorf("concurrent Accumulate: %v", err)
			}
		}()
	}
	wg.Wait()

	rec, err := repo.GetForUser(ctx, u.ID, mustRecordID(t, db, u.ID, tor.ID))
	if err != nil {
		t.Fatalf("GetForUser: %v", err)
	}
	elapsed := int64(now.Sub(start).Seconds()) + 2 // small slack for wall-clock drift in the test itself
	if rec.SeededSeconds > elapsed {
		t.Errorf("credited %ds of seed time but only %ds of wall clock elapsed — concurrent announces double-credited",
			rec.SeededSeconds, elapsed)
	}
	if rec.Uploaded != 10*goroutines {
		t.Errorf("expected every concurrent upload delta counted (upload is additive, not gap-based): got %d, want %d",
			rec.Uploaded, 10*goroutines)
	}
}

func TestHnRRepo_MarkTransitions_AreCompareAndSwap(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewHnRRepo(db)

	u := newUser(t, db)
	tor := newTorrent(t, db, u.ID)
	if _, err := repo.CreateIfNotExists(ctx, u.ID, tor.ID, time.Now()); err != nil {
		t.Fatalf("CreateIfNotExists: %v", err)
	}
	recordID := mustRecordID(t, db, u.ID, tor.ID)

	// First MarkSatisfied succeeds (record is 'active').
	n, err := repo.MarkSatisfied(ctx, []int64{recordID}, time.Now())
	if err != nil || n != 1 {
		t.Fatalf("first MarkSatisfied: n=%d err=%v", n, err)
	}
	// A second instance racing the same transition sees nothing left to move —
	// this is what makes a double daemon run safe.
	n, err = repo.MarkSatisfied(ctx, []int64{recordID}, time.Now())
	if err != nil {
		t.Fatalf("second MarkSatisfied: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 rows affected on a repeat MarkSatisfied (already satisfied), got %d", n)
	}

	// MarkBreached must not touch an already-satisfied record either.
	n, err = repo.MarkBreached(ctx, []int64{recordID}, time.Now())
	if err != nil || n != 0 {
		t.Fatalf("MarkBreached on satisfied record: n=%d err=%v", n, err)
	}
}

func TestHnRRepo_ListOpenForEvaluation(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewHnRRepo(db)

	userGroup := groupIDBySlug(t, db, "user")
	vipGroup := groupIDBySlug(t, db, "vip")

	rule := &model.HnRRule{GroupID: userGroup, RequiredSeedHours: 240, RequiredRatio: 1.0, InactivityGraceHours: 48, MaxDaysToSatisfy: 30}
	if err := repo.UpsertRule(ctx, rule); err != nil {
		t.Fatalf("UpsertRule: %v", err)
	}

	// A "user"-class member has a rule.
	regular := newUser(t, db)
	if _, err := db.ExecContext(ctx, `UPDATE users SET group_id = $1 WHERE id = $2`, userGroup, regular.ID); err != nil {
		t.Fatalf("set group: %v", err)
	}
	regularTorrent := newTorrent(t, db, regular.ID)
	if _, err := repo.CreateIfNotExists(ctx, regular.ID, regularTorrent.ID, time.Now()); err != nil {
		t.Fatalf("create regular record: %v", err)
	}

	// A VIP has no rule at all — Rule must come back nil.
	vip := newUser(t, db)
	if _, err := db.ExecContext(ctx, `UPDATE users SET group_id = $1 WHERE id = $2`, vipGroup, vip.ID); err != nil {
		t.Fatalf("set vip group: %v", err)
	}
	vipTorrent := newTorrent(t, db, vip.ID)
	if _, err := repo.CreateIfNotExists(ctx, vip.ID, vipTorrent.ID, time.Now()); err != nil {
		t.Fatalf("create vip record: %v", err)
	}

	inputs, err := repo.ListOpenForEvaluation(ctx)
	if err != nil {
		t.Fatalf("ListOpenForEvaluation: %v", err)
	}
	if len(inputs) != 2 {
		t.Fatalf("expected 2 open records, got %d", len(inputs))
	}

	var sawRegularRule, sawVIPNilRule bool
	for _, in := range inputs {
		switch in.Record.UserID {
		case regular.ID:
			if in.Rule == nil || in.Rule.RequiredSeedHours != 240 {
				t.Errorf("expected the user-class rule for the regular member, got %+v", in.Rule)
			}
			sawRegularRule = true
		case vip.ID:
			if in.Rule != nil {
				t.Errorf("expected a nil rule for a VIP (no hnr_rules row), got %+v", in.Rule)
			}
			sawVIPNilRule = true
		}
		if in.TorrentSize != 1024 {
			t.Errorf("expected torrent size 1024 from the fixture, got %d", in.TorrentSize)
		}
	}
	if !sawRegularRule || !sawVIPNilRule {
		t.Fatalf("did not see both expected records: regularRule=%v vipNilRule=%v", sawRegularRule, sawVIPNilRule)
	}
}

func TestHnRRepo_PurgeResolved(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewHnRRepo(db)

	u := newUser(t, db)
	oldTorrent := newTorrent(t, db, u.ID)
	recentTorrent := newTorrent(t, db, u.ID)
	openTorrent := newTorrent(t, db, u.ID)

	if _, err := repo.CreateIfNotExists(ctx, u.ID, oldTorrent.ID, time.Now()); err != nil {
		t.Fatalf("create old: %v", err)
	}
	if _, err := repo.CreateIfNotExists(ctx, u.ID, recentTorrent.ID, time.Now()); err != nil {
		t.Fatalf("create recent: %v", err)
	}
	if _, err := repo.CreateIfNotExists(ctx, u.ID, openTorrent.ID, time.Now()); err != nil {
		t.Fatalf("create open: %v", err)
	}

	oldID := mustRecordID(t, db, u.ID, oldTorrent.ID)
	recentID := mustRecordID(t, db, u.ID, recentTorrent.ID)

	longAgo := time.Now().Add(-200 * 24 * time.Hour)
	if _, err := repo.MarkSatisfied(ctx, []int64{oldID}, longAgo); err != nil {
		t.Fatalf("mark old satisfied: %v", err)
	}
	if _, err := repo.MarkSatisfied(ctx, []int64{recentID}, time.Now()); err != nil {
		t.Fatalf("mark recent satisfied: %v", err)
	}

	cutoff := time.Now().Add(-180 * 24 * time.Hour)
	purged, err := repo.PurgeResolved(ctx, cutoff)
	if err != nil {
		t.Fatalf("PurgeResolved: %v", err)
	}
	if purged != 1 {
		t.Fatalf("expected 1 purged record, got %d", purged)
	}

	records, err := repo.ListForUser(ctx, u.ID)
	if err != nil {
		t.Fatalf("ListForUser: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 remaining records (recent satisfied + open), got %d", len(records))
	}
}

func TestHnRRepo_StagesCRUD(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewHnRRepo(db)

	// The migration already seeds 5 stages.
	stages, err := repo.ListStages(ctx)
	if err != nil {
		t.Fatalf("ListStages: %v", err)
	}
	if len(stages) != 5 {
		t.Fatalf("expected the 5 seeded default stages, got %d", len(stages))
	}

	stage := &model.HnRPenaltyStage{
		Stage: 6, MinActiveHnR: 2, MinDaysInPrev: 21, Action: model.HnRActionBan,
		RestrictionTypes: []string{}, MessageTemplate: "final",
	}
	if err := repo.UpsertStage(ctx, stage); err != nil {
		t.Fatalf("UpsertStage: %v", err)
	}
	if err := repo.DeleteStage(ctx, 6); err != nil {
		t.Fatalf("DeleteStage: %v", err)
	}
	if err := repo.DeleteStage(ctx, 6); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("DeleteStage(missing) = %v, want sql.ErrNoRows", err)
	}

	// restriction_types round-trips through the array codec correctly.
	third, err := repo.ListStages(ctx)
	if err != nil {
		t.Fatalf("ListStages: %v", err)
	}
	for _, s := range third {
		if s.Stage == 3 {
			if len(s.RestrictionTypes) != 3 {
				t.Errorf("expected seeded stage 3 to carry 3 restriction types, got %v", s.RestrictionTypes)
			}
		}
	}
}

func TestHnRRepo_UserLadderState(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewHnRRepo(db)

	u := newUser(t, db)

	if _, err := repo.GetUserState(ctx, u.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetUserState(absent) = %v, want sql.ErrNoRows", err)
	}

	if err := repo.EnsureUserState(ctx, u.ID); err != nil {
		t.Fatalf("EnsureUserState: %v", err)
	}
	// Idempotent.
	if err := repo.EnsureUserState(ctx, u.ID); err != nil {
		t.Fatalf("EnsureUserState (repeat): %v", err)
	}

	state, err := repo.GetUserState(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetUserState: %v", err)
	}
	if state.Stage != 0 {
		t.Fatalf("expected a freshly-ensured state at stage 0, got %d", state.Stage)
	}

	ok, err := repo.CASUserStage(ctx, u.ID, 0, 1, time.Now())
	if err != nil || !ok {
		t.Fatalf("CASUserStage 0->1: ok=%v err=%v", ok, err)
	}
	// A stale expected-stage must not apply — this is the cross-instance safety.
	ok, err = repo.CASUserStage(ctx, u.ID, 0, 2, time.Now())
	if err != nil {
		t.Fatalf("CASUserStage stale: %v", err)
	}
	if ok {
		t.Fatal("expected CASUserStage to fail when expectedStage no longer matches")
	}

	users, err := repo.UsersOnLadder(ctx)
	if err != nil {
		t.Fatalf("UsersOnLadder: %v", err)
	}
	if len(users) != 1 || users[0].Stage != 1 {
		t.Fatalf("expected 1 user at stage 1, got %+v", users)
	}

	if err := repo.SetLastNotifiedStage(ctx, u.ID, 1); err != nil {
		t.Fatalf("SetLastNotifiedStage: %v", err)
	}
	state, err = repo.GetUserState(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetUserState after notify: %v", err)
	}
	if state.LastNotifiedStage != 1 {
		t.Fatalf("expected last_notified_stage=1, got %d", state.LastNotifiedStage)
	}
}

func TestHnRRepo_RunBookkeeping(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewHnRRepo(db)

	if _, ok, err := repo.LastRun(ctx); err != nil || ok {
		t.Fatalf("LastRun before any run: ok=%v err=%v", ok, err)
	}

	runID, err := repo.StartRun(ctx, model.HnRRunTriggerSchedule, nil)
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}

	errMsg := "boom"
	if err := repo.FinishRun(ctx, runID, model.HnRRunStatusFailed, repository.HnRRunCounts{
		Scanned: 10, Breached: 2, Satisfied: 1, StagesAdvanced: 1, StagesDecayed: 0, Purged: 0,
	}, &errMsg); err != nil {
		t.Fatalf("FinishRun: %v", err)
	}

	last, ok, err := repo.LastRun(ctx)
	if err != nil || !ok {
		t.Fatalf("LastRun: ok=%v err=%v", ok, err)
	}
	if last.Status != model.HnRRunStatusFailed || last.Scanned != 10 || last.Error == nil || *last.Error != "boom" {
		t.Fatalf("unexpected run: %+v", last)
	}

	runs, err := repo.ListRuns(ctx, 5)
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
}

func TestHnRRepo_LiveSeedingTorrentIDs(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewHnRRepo(db)

	u := newUser(t, db)
	seeding := newTorrent(t, db, u.ID)
	notSeeding := newTorrent(t, db, u.ID)

	if err := NewPeerRepo(db).Upsert(ctx, &model.Peer{
		TorrentID: seeding.ID, UserID: u.ID, PeerID: []byte("peer-seeding-1234567"),
		IP: "127.0.0.1", Port: 6881, Seeder: true,
	}); err != nil {
		t.Fatalf("upsert seeding peer: %v", err)
	}
	if err := NewPeerRepo(db).Upsert(ctx, &model.Peer{
		TorrentID: notSeeding.ID, UserID: u.ID, PeerID: []byte("peer-leeching-123456"),
		IP: "127.0.0.1", Port: 6881, Seeder: false,
	}); err != nil {
		t.Fatalf("upsert leeching peer: %v", err)
	}

	live, err := repo.LiveSeedingTorrentIDs(ctx, u.ID, []int64{seeding.ID, notSeeding.ID})
	if err != nil {
		t.Fatalf("LiveSeedingTorrentIDs: %v", err)
	}
	if !live[seeding.ID] {
		t.Error("expected the seeding torrent to be reported live")
	}
	if live[notSeeding.ID] {
		t.Error("expected the leeching torrent to not be reported live")
	}
}

func TestHnRRepo_ClearRecord(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewHnRRepo(db)

	u := newUser(t, db)
	if _, err := db.ExecContext(ctx, `UPDATE users SET bonus_points = 100 WHERE id = $1`, u.ID); err != nil {
		t.Fatalf("set bonus points: %v", err)
	}
	tor := newTorrent(t, db, u.ID)
	if _, err := repo.CreateIfNotExists(ctx, u.ID, tor.ID, time.Now()); err != nil {
		t.Fatalf("CreateIfNotExists: %v", err)
	}
	recordID := mustRecordID(t, db, u.ID, tor.ID)

	newBalance, err := repo.ClearRecord(ctx, u.ID, recordID, 40)
	if err != nil {
		t.Fatalf("ClearRecord: %v", err)
	}
	if newBalance != 60 {
		t.Fatalf("expected balance 60 after spending 40 of 100, got %d", newBalance)
	}

	rec, err := repo.GetForUser(ctx, u.ID, recordID)
	if err != nil {
		t.Fatalf("GetForUser: %v", err)
	}
	if rec.State != model.HnRStateCleared {
		t.Fatalf("expected state=cleared, got %s", rec.State)
	}
	if rec.ResolvedAt == nil {
		t.Error("expected resolved_at to be set on clear (retention purge depends on it)")
	}

	// Already cleared: a second attempt must not succeed or double-charge.
	if _, err := repo.ClearRecord(ctx, u.ID, recordID, 40); !errors.Is(err, repository.ErrHnRRecordNotClearable) {
		t.Fatalf("re-clearing an already-cleared record: got %v, want ErrHnRRecordNotClearable", err)
	}
	var balance int64
	if err := db.QueryRowContext(ctx, `SELECT bonus_points FROM users WHERE id = $1`, u.ID).Scan(&balance); err != nil {
		t.Fatalf("read balance: %v", err)
	}
	if balance != 60 {
		t.Fatalf("balance must not change on a rejected re-clear, got %d", balance)
	}
}

func TestHnRRepo_ClearRecord_InsufficientPoints(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewHnRRepo(db)

	u := newUser(t, db)
	if _, err := db.ExecContext(ctx, `UPDATE users SET bonus_points = 10 WHERE id = $1`, u.ID); err != nil {
		t.Fatalf("set bonus points: %v", err)
	}
	tor := newTorrent(t, db, u.ID)
	if _, err := repo.CreateIfNotExists(ctx, u.ID, tor.ID, time.Now()); err != nil {
		t.Fatalf("CreateIfNotExists: %v", err)
	}
	recordID := mustRecordID(t, db, u.ID, tor.ID)

	if _, err := repo.ClearRecord(ctx, u.ID, recordID, 40); !errors.Is(err, repository.ErrInsufficientBonusPoints) {
		t.Fatalf("got %v, want ErrInsufficientBonusPoints", err)
	}

	// The whole transaction must have rolled back: the record is still open.
	rec, err := repo.GetForUser(ctx, u.ID, recordID)
	if err != nil {
		t.Fatalf("GetForUser: %v", err)
	}
	if rec.State != model.HnRStateActive {
		t.Fatalf("expected the record to remain active after a failed clear, got %s", rec.State)
	}
}

func TestHnRRepo_AdminListAndAggregates(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewHnRRepo(db)

	u1 := newUser(t, db)
	u2 := newUser(t, db)
	t1 := newTorrent(t, db, u1.ID)
	t2 := newTorrent(t, db, u1.ID)
	t3 := newTorrent(t, db, u2.ID)

	for _, tc := range []struct {
		userID, torrentID int64
	}{{u1.ID, t1.ID}, {u1.ID, t2.ID}, {u2.ID, t3.ID}} {
		if _, err := repo.CreateIfNotExists(ctx, tc.userID, tc.torrentID, time.Now()); err != nil {
			t.Fatalf("create: %v", err)
		}
	}
	id1 := mustRecordID(t, db, u1.ID, t1.ID)
	id2 := mustRecordID(t, db, u1.ID, t2.ID)
	if _, err := repo.MarkBreached(ctx, []int64{id1, id2}, time.Now()); err != nil {
		t.Fatalf("MarkBreached: %v", err)
	}

	stats, err := repo.AggregateStats(ctx)
	if err != nil {
		t.Fatalf("AggregateStats: %v", err)
	}
	if stats.ActiveHnR != 2 || stats.Monitored != 1 {
		t.Fatalf("unexpected aggregate stats: %+v", stats)
	}

	offenders, err := repo.TopOffenders(ctx, 10)
	if err != nil {
		t.Fatalf("TopOffenders: %v", err)
	}
	if len(offenders) != 1 || offenders[0].UserID != u1.ID || offenders[0].ActiveHnR != 2 {
		t.Fatalf("unexpected offenders: %+v", offenders)
	}

	state := model.HnRStateBreach
	records, total, err := repo.AdminList(ctx, repository.HnRAdminListOptions{State: &state, Page: 1, PerPage: 10})
	if err != nil {
		t.Fatalf("AdminList: %v", err)
	}
	if total != 2 || len(records) != 2 {
		t.Fatalf("expected 2 breached records, got total=%d len=%d", total, len(records))
	}
}

// mustRecordID looks up the id of the (user, torrent) hnr_records row a test
// just created via CreateIfNotExists — a thin helper so tests can chain
// further operations (MarkBreached, ClearRecord, ...) that need the id.
func mustRecordID(t *testing.T, db *sql.DB, userID, torrentID int64) int64 {
	t.Helper()
	var id int64
	if err := db.QueryRow(`SELECT id FROM hnr_records WHERE user_id = $1 AND torrent_id = $2`, userID, torrentID).Scan(&id); err != nil {
		t.Fatalf("looking up hnr_records id for user=%d torrent=%d: %v", userID, torrentID, err)
	}
	return id
}
