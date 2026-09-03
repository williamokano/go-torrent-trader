package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

// setupHnRServiceWithLadder wires HnRService with the same fakes as
// setupHnRService, but returns them too — the ladder tests below need to set
// up users and assert on warnings/restrictions/messages, not just the HnR
// records the plain setupHnRService callers care about.
func setupHnRServiceWithLadder() (svc *HnRService, hnr *fakeHnRRepo, users *mockUserRepoForRestrictions, warnRepo *mockWarningRepo, restrictionRepo *mockRestrictionRepo, msgRepo *mockMessageRepoForWarnings, bus event.Bus) {
	hnr = newFakeHnRRepo()
	users = newMockUserRepoForRestrictions()
	warnRepo = newMockWarningRepo()
	restrictionRepo = newMockRestrictionRepo()
	msgRepo = newMockMessageRepoForWarnings()
	bus = event.NewInMemoryBus()
	warnings := NewWarningService(warnRepo, users, msgRepo, bus)
	restrictions := NewRestrictionService(restrictionRepo, users, bus)
	svc = NewHnRService(nil, hnr, &fakeHnRGroupRepo{groups: hnrTestGroups()}, users, warnings, restrictions, nil, bus)
	return svc, hnr, users, warnRepo, restrictionRepo, msgRepo, bus
}

// standardLadder mirrors the plan's five-stage default: notice, warning,
// restriction, final notice, ban, escalating one active-hnr count at a time
// with no dwell requirement — tests that care about dwell timing set their
// own stages instead.
func standardLadder() []model.HnRPenaltyStage {
	return []model.HnRPenaltyStage{
		{Stage: 1, MinActiveHnR: 1, MinDaysInPrev: 0, Action: model.HnRActionNotify, MessageTemplate: "Notice: {{username}} has {{count}} active hit-and-runs."},
		{Stage: 2, MinActiveHnR: 2, MinDaysInPrev: 0, Action: model.HnRActionWarn, MessageTemplate: "Warning: {{username}} has {{count}} active hit-and-runs."},
		{Stage: 3, MinActiveHnR: 3, MinDaysInPrev: 0, Action: model.HnRActionRestrict, RestrictionTypes: []string{model.RestrictionTypeDownload}, RestrictionDays: 7, MessageTemplate: "Restricted: {{username}}."},
		{Stage: 4, MinActiveHnR: 4, MinDaysInPrev: 0, Action: model.HnRActionFinalNotice, MessageTemplate: "Final notice: {{username}}."},
		{Stage: 5, MinActiveHnR: 5, MinDaysInPrev: 0, Action: model.HnRActionBan, MessageTemplate: "Banned: {{username}}."},
	}
}

// climbLadder drives runLadder enough times to reach targetStage — escalation
// only ever advances one rung per call (see decideHnRLadderStage), so a test
// that needs a user at stage N from a standing start must call runLadder N
// times, exactly like N separate daemon runs would.
func climbLadder(t *testing.T, svc *HnRService, hnr *fakeHnRRepo, userID int64, targetStage int) {
	t.Helper()
	now := time.Now()
	for i := 0; i < targetStage; i++ {
		if _, _, err := svc.runLadder(context.Background(), now); err != nil {
			t.Fatalf("climbLadder step %d: %v", i, err)
		}
	}
	state, err := hnr.GetUserState(context.Background(), userID)
	if err != nil || state.Stage != targetStage {
		t.Fatalf("climbLadder: expected stage %d after %d runs, got %+v err=%v", targetStage, targetStage, state, err)
	}
}

func seedStages(t *testing.T, hnr *fakeHnRRepo, stages []model.HnRPenaltyStage) {
	t.Helper()
	for _, st := range stages {
		row := st
		if err := hnr.UpsertStage(context.Background(), &row); err != nil {
			t.Fatalf("seed stage %d: %v", st.Stage, err)
		}
	}
}

func TestHnRLadder_NoStagesConfiguredIsANoop(t *testing.T) {
	svc, _, _, _, _, _, _ := setupHnRServiceWithLadder()
	advanced, decayed, err := svc.runLadder(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("runLadder: %v", err)
	}
	if advanced != 0 || decayed != 0 {
		t.Fatalf("expected no transitions with no ladder configured, got advanced=%d decayed=%d", advanced, decayed)
	}
}

func TestHnRLadder_FirstBreachSendsANotification(t *testing.T) {
	svc, hnr, users, _, _, _, bus := setupHnRServiceWithLadder()
	seedStages(t, hnr, standardLadder())
	users.addUser(&model.User{ID: 1, Username: "alice", Enabled: true})

	var notified []*event.HnRStageChangedEvent
	bus.Subscribe(event.HnRStageChanged, func(_ context.Context, evt event.Event) error {
		notified = append(notified, evt.(*event.HnRStageChangedEvent))
		return nil
	})

	// One active-hnr breach for alice.
	hnr.records[1] = &model.HnRRecord{ID: 1, UserID: 1, TorrentID: 100, State: model.HnRStateBreach}
	hnr.nextID = 2

	advanced, decayed, err := svc.runLadder(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("runLadder: %v", err)
	}
	if advanced != 1 || decayed != 0 {
		t.Fatalf("expected 1 advance, got advanced=%d decayed=%d", advanced, decayed)
	}
	if len(notified) != 1 || notified[0].NewStage != 1 || notified[0].UserID != 1 {
		t.Fatalf("expected one stage-1 notification for user 1, got %+v", notified)
	}
	if notified[0].Message != "Notice: alice has 1 active hit-and-runs." {
		t.Errorf("unexpected message: %q", notified[0].Message)
	}

	state, err := hnr.GetUserState(context.Background(), 1)
	if err != nil || state.Stage != 1 {
		t.Fatalf("expected user state stage=1, got %+v err=%v", state, err)
	}
}

func TestHnRLadder_WarnStageIssuesAWarning(t *testing.T) {
	svc, hnr, users, warnRepo, _, msgRepo, _ := setupHnRServiceWithLadder()
	seedStages(t, hnr, standardLadder())
	users.addUser(&model.User{ID: 1, Username: "bob", Enabled: true})

	hnr.records[1] = &model.HnRRecord{ID: 1, UserID: 1, TorrentID: 100, State: model.HnRStateBreach}
	hnr.records[2] = &model.HnRRecord{ID: 2, UserID: 1, TorrentID: 101, State: model.HnRStateBreach}
	hnr.nextID = 3

	// Count of 2 supports stage 2 (warn), but escalation is one rung per
	// run — reach it the same way two daemon runs would.
	climbLadder(t, svc, hnr, 1, 2)

	warnings, err := warnRepo.ListByUser(context.Background(), 1, true)
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if len(warnings) != 1 || warnings[0].Type != model.WarningTypeHnR {
		t.Fatalf("expected one hnr warning, got %+v", warnings)
	}
	if len(msgRepo.messages) != 1 {
		t.Fatalf("expected a PM for the warning, got %d", len(msgRepo.messages))
	}

	u, _ := users.GetByID(context.Background(), 1)
	if !u.Warned {
		t.Error("expected user.Warned=true after an hnr warning")
	}
}

func TestHnRLadder_RestrictStageAppliesRestrictionWithExpiry(t *testing.T) {
	svc, hnr, users, _, restrictionRepo, _, _ := setupHnRServiceWithLadder()
	seedStages(t, hnr, standardLadder())
	users.addUser(&model.User{ID: 1, Username: "carol", Enabled: true, CanDownload: true})

	for i := int64(1); i <= 3; i++ {
		hnr.records[i] = &model.HnRRecord{ID: i, UserID: 1, TorrentID: 100 + i, State: model.HnRStateBreach}
	}
	hnr.nextID = 4

	climbLadder(t, svc, hnr, 1, 3)

	u, _ := users.GetByID(context.Background(), 1)
	if u.CanDownload {
		t.Error("expected CanDownload=false after the restrict stage")
	}

	active, err := restrictionRepo.ListActive(context.Background())
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if len(active) != 1 || active[0].Source != model.RestrictionSourceHnR || active[0].RestrictionType != model.RestrictionTypeDownload {
		t.Fatalf("expected one hnr-sourced download restriction, got %+v", active)
	}
	if active[0].ExpiresAt == nil {
		t.Error("expected an expiry from the stage's restriction_days")
	}
}

func TestHnRLadder_BanStageDisablesAccount(t *testing.T) {
	svc, hnr, users, warnRepo, _, msgRepo, _ := setupHnRServiceWithLadder()
	seedStages(t, hnr, standardLadder())
	users.addUser(&model.User{ID: 1, Username: "dave", Enabled: true})

	for i := int64(1); i <= 5; i++ {
		hnr.records[i] = &model.HnRRecord{ID: i, UserID: 1, TorrentID: 100 + i, State: model.HnRStateBreach}
	}
	hnr.nextID = 6

	climbLadder(t, svc, hnr, 1, 5)

	u, _ := users.GetByID(context.Background(), 1)
	if u.Enabled {
		t.Error("expected the account to be disabled after reaching the ban stage")
	}
	warnings, _ := warnRepo.ListByUser(context.Background(), 1, true)
	found := false
	for _, w := range warnings {
		if w.Type == model.WarningTypeHnRBan {
			found = true
		}
	}
	if !found {
		t.Errorf("expected an hnr_ban audit-trail warning, got %+v", warnings)
	}
	if len(msgRepo.messages) == 0 {
		t.Error("expected a PM explaining the ban")
	}
}

func TestHnRLadder_EscalationOnlyAdvancesOneStagePerRun(t *testing.T) {
	svc, hnr, users, _, _, _, _ := setupHnRServiceWithLadder()
	seedStages(t, hnr, standardLadder())
	users.addUser(&model.User{ID: 1, Username: "erin", Enabled: true})

	// Count already supports stage 5 (ban) on this user's very first run.
	for i := int64(1); i <= 5; i++ {
		hnr.records[i] = &model.HnRRecord{ID: i, UserID: 1, TorrentID: 100 + i, State: model.HnRStateBreach}
	}
	hnr.nextID = 6

	if _, _, err := svc.runLadder(context.Background(), time.Now()); err != nil {
		t.Fatalf("runLadder: %v", err)
	}

	state, err := hnr.GetUserState(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetUserState: %v", err)
	}
	if state.Stage != 1 {
		t.Fatalf("expected a single-rung advance to stage 1 despite a count that supports stage 5, got stage=%d", state.Stage)
	}

	u, _ := users.GetByID(context.Background(), 1)
	if !u.Enabled {
		t.Error("a user must never be banned in the same run they first appear on the ladder")
	}
}

func TestHnRLadder_DeescalationLiftsRestrictionsButNotBans(t *testing.T) {
	svc, hnr, users, _, restrictionRepo, _, _ := setupHnRServiceWithLadder()
	seedStages(t, hnr, standardLadder())
	users.addUser(&model.User{ID: 1, Username: "frank", Enabled: true, CanDownload: true})

	// Climb to stage 3 (restrict) over three runs, each one rung.
	for i := int64(1); i <= 3; i++ {
		hnr.records[i] = &model.HnRRecord{ID: i, UserID: 1, TorrentID: 100 + i, State: model.HnRStateBreach}
	}
	hnr.nextID = 4
	now := time.Now()
	for step := 0; step < 3; step++ {
		if _, _, err := svc.runLadder(context.Background(), now); err != nil {
			t.Fatalf("runLadder step %d: %v", step, err)
		}
	}
	state, _ := hnr.GetUserState(context.Background(), 1)
	if state.Stage != 3 {
		t.Fatalf("expected to reach stage 3 after three runs, got %d", state.Stage)
	}
	u, _ := users.GetByID(context.Background(), 1)
	if u.CanDownload {
		t.Fatal("expected download restricted at stage 3")
	}

	// Two of the three obligations resolve; count drops to 1.
	hnr.records[1].State = model.HnRStateSatisfied
	hnr.records[2].State = model.HnRStateSatisfied

	advanced, decayed, err := svc.runLadder(context.Background(), now)
	if err != nil {
		t.Fatalf("runLadder (deescalation): %v", err)
	}
	if advanced != 0 || decayed != 1 {
		t.Fatalf("expected exactly one de-escalation, got advanced=%d decayed=%d", advanced, decayed)
	}

	state, _ = hnr.GetUserState(context.Background(), 1)
	if state.Stage != 1 {
		t.Fatalf("expected a direct drop to stage 1 (matching the remaining count of 1), got %d", state.Stage)
	}

	u, _ = users.GetByID(context.Background(), 1)
	if !u.CanDownload {
		t.Error("expected the download restriction to be lifted on de-escalation")
	}
	active, _ := restrictionRepo.ListActive(context.Background())
	if len(active) != 0 {
		t.Errorf("expected no active restrictions after de-escalation, got %+v", active)
	}
}

func TestHnRLadder_BanIsNeverAutoReversedByDeescalation(t *testing.T) {
	svc, hnr, users, _, _, _, _ := setupHnRServiceWithLadder()
	seedStages(t, hnr, standardLadder())
	users.addUser(&model.User{ID: 1, Username: "grace", Enabled: true})

	for i := int64(1); i <= 5; i++ {
		hnr.records[i] = &model.HnRRecord{ID: i, UserID: 1, TorrentID: 100 + i, State: model.HnRStateBreach}
	}
	hnr.nextID = 6
	now := time.Now()
	// Climb all five rungs, one run each.
	for step := 0; step < 5; step++ {
		if _, _, err := svc.runLadder(context.Background(), now); err != nil {
			t.Fatalf("runLadder step %d: %v", step, err)
		}
	}
	u, _ := users.GetByID(context.Background(), 1)
	if u.Enabled {
		t.Fatal("expected the account disabled after reaching the ban stage")
	}

	// Every obligation resolves.
	for i := int64(1); i <= 5; i++ {
		hnr.records[i].State = model.HnRStateSatisfied
	}
	if _, decayed, err := svc.runLadder(context.Background(), now); err != nil || decayed != 1 {
		t.Fatalf("runLadder (deescalation to 0): decayed=%d err=%v", decayed, err)
	}

	u, _ = users.GetByID(context.Background(), 1)
	if u.Enabled {
		t.Error("de-escalation must never re-enable a banned account")
	}
}

func TestHnRLadder_DoubleRunIsSafe(t *testing.T) {
	// Two goroutines racing to advance the same user off the same stage must
	// not both apply the stage action — the CAS in runLadder (GetUserState
	// then CASUserStage) is what prevents that, mirroring MarkBreached's
	// compare-and-swap semantics for the record-level state machine.
	svc, hnr, users, warnRepo, _, _, _ := setupHnRServiceWithLadder()
	seedStages(t, hnr, standardLadder())
	users.addUser(&model.User{ID: 1, Username: "henry", Enabled: true})
	hnr.records[1] = &model.HnRRecord{ID: 1, UserID: 1, TorrentID: 100, State: model.HnRStateBreach}
	hnr.records[2] = &model.HnRRecord{ID: 2, UserID: 1, TorrentID: 101, State: model.HnRStateBreach}
	hnr.nextID = 3

	// Prime to stage 1 first, so both racing calls below are contending for
	// the exact same transition (stage 1 -> 2, the warn stage) rather than
	// each doing distinct, uncontested work.
	climbLadder(t, svc, hnr, 1, 1)

	var wg sync.WaitGroup
	results := make([]int, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			advanced, _, err := svc.runLadder(context.Background(), time.Now())
			if err != nil {
				t.Errorf("concurrent runLadder: %v", err)
			}
			results[i] = advanced
		}(i)
	}
	wg.Wait()

	total := results[0] + results[1]
	if total != 1 {
		t.Fatalf("expected exactly one of the two concurrent runs to win the CAS and advance, got %d total", total)
	}

	warnings, _ := warnRepo.ListByUser(context.Background(), 1, true)
	if len(warnings) != 1 {
		t.Fatalf("expected exactly one hnr warning despite two concurrent runs, got %d", len(warnings))
	}
}

func TestHnRService_ListStages_OrderedByStage(t *testing.T) {
	svc, hnr, _, _, _, _, _ := setupHnRServiceWithLadder()
	seedStages(t, hnr, []model.HnRPenaltyStage{
		{Stage: 3, MinActiveHnR: 3, Action: model.HnRActionRestrict, RestrictionTypes: []string{model.RestrictionTypeDownload}},
		{Stage: 1, MinActiveHnR: 1, Action: model.HnRActionNotify},
	})

	stages, err := svc.ListStages(context.Background())
	if err != nil {
		t.Fatalf("ListStages: %v", err)
	}
	if len(stages) != 2 || stages[0].Stage != 1 || stages[1].Stage != 3 {
		t.Fatalf("expected stages ordered [1, 3], got %+v", stages)
	}
}

func TestHnRService_UpsertStage_Validation(t *testing.T) {
	svc, _, _, _, _, _, _ := setupHnRServiceWithLadder()

	cases := []struct {
		name string
		in   HnRStageInput
	}{
		{"unknown action", HnRStageInput{MinActiveHnR: 1, Action: "explode"}},
		{"restrict with no types", HnRStageInput{MinActiveHnR: 1, Action: model.HnRActionRestrict}},
		{"restrict with bad type", HnRStageInput{MinActiveHnR: 1, Action: model.HnRActionRestrict, RestrictionTypes: []string{"nonsense"}}},
		{"notify with types set", HnRStageInput{MinActiveHnR: 1, Action: model.HnRActionNotify, RestrictionTypes: []string{model.RestrictionTypeDownload}}},
		{"zero min_active_hnr", HnRStageInput{MinActiveHnR: 0, Action: model.HnRActionNotify}},
		{"negative min_days_in_prev", HnRStageInput{MinActiveHnR: 1, MinDaysInPrev: -1, Action: model.HnRActionNotify}},
		{"negative restriction_days", HnRStageInput{MinActiveHnR: 1, Action: model.HnRActionNotify, RestrictionDays: -1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := svc.UpsertStage(context.Background(), 1, tc.in); !errors.Is(err, ErrHnRInvalidStage) {
				t.Errorf("got %v, want ErrHnRInvalidStage", err)
			}
		})
	}

	if _, err := svc.UpsertStage(context.Background(), 0, HnRStageInput{MinActiveHnR: 1, Action: model.HnRActionNotify}); !errors.Is(err, ErrHnRInvalidStage) {
		t.Errorf("stage=0: got %v, want ErrHnRInvalidStage", err)
	}
}

func TestHnRService_UpsertStage_HappyPath(t *testing.T) {
	svc, _, _, _, _, _, _ := setupHnRServiceWithLadder()

	row, err := svc.UpsertStage(context.Background(), 2, HnRStageInput{
		MinActiveHnR: 2, MinDaysInPrev: 3, Action: model.HnRActionWarn, MessageTemplate: "hi",
	})
	if err != nil {
		t.Fatalf("UpsertStage: %v", err)
	}
	if row.Stage != 2 || row.MinActiveHnR != 2 {
		t.Errorf("unexpected row: %+v", row)
	}
}

func TestHnRService_DeleteStage(t *testing.T) {
	svc, hnr, _, _, _, _, _ := setupHnRServiceWithLadder()
	seedStages(t, hnr, []model.HnRPenaltyStage{{Stage: 1, MinActiveHnR: 1, Action: model.HnRActionNotify}})

	if err := svc.DeleteStage(context.Background(), 1); err != nil {
		t.Fatalf("DeleteStage: %v", err)
	}
	if err := svc.DeleteStage(context.Background(), 1); !errors.Is(err, ErrHnRStageNotFound) {
		t.Errorf("DeleteStage(missing) = %v, want ErrHnRStageNotFound", err)
	}
}
