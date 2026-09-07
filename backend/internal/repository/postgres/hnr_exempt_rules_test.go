package postgres

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

func TestHnRRepo_ExemptRulesCRUD(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewHnRRepo(db)

	rule := &model.HnRExemptRule{Criterion: model.HnRExemptCriterionMinSeeders, Threshold: 50, Enabled: true}
	if err := repo.CreateExemptRule(ctx, rule); err != nil {
		t.Fatalf("CreateExemptRule: %v", err)
	}
	if rule.ID == 0 || rule.CreatedAt.IsZero() {
		t.Fatalf("CreateExemptRule did not populate id/timestamps: %+v", rule)
	}

	got, err := repo.GetExemptRule(ctx, rule.ID)
	if err != nil {
		t.Fatalf("GetExemptRule: %v", err)
	}
	if got.Criterion != model.HnRExemptCriterionMinSeeders || got.Threshold != 50 || !got.Enabled {
		t.Fatalf("round-trip mismatch: %+v", got)
	}

	rule.Threshold = 25
	rule.Enabled = false
	if err := repo.UpdateExemptRule(ctx, rule); err != nil {
		t.Fatalf("UpdateExemptRule: %v", err)
	}
	got, _ = repo.GetExemptRule(ctx, rule.ID)
	if got.Threshold != 25 || got.Enabled {
		t.Fatalf("update not persisted: %+v", got)
	}

	rules, err := repo.ListExemptRules(ctx)
	if err != nil || len(rules) != 1 {
		t.Fatalf("ListExemptRules: n=%d err=%v", len(rules), err)
	}

	if err := repo.DeleteExemptRule(ctx, rule.ID); err != nil {
		t.Fatalf("DeleteExemptRule: %v", err)
	}
	if _, err := repo.GetExemptRule(ctx, rule.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("GetExemptRule(deleted) = %v, want sql.ErrNoRows", err)
	}
	if err := repo.DeleteExemptRule(ctx, rule.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("DeleteExemptRule(missing) = %v, want sql.ErrNoRows", err)
	}
}

// setTorrentExempt forces a torrent's exemption state, standing in for whatever
// production path (staff edit, a prior auto pass) would have set it.
func setTorrentExempt(t *testing.T, db *sql.DB, torrentID int64, exempt bool, source any) {
	t.Helper()
	if _, err := db.Exec(
		`UPDATE torrents SET hnr_exempt = $1, hnr_exempt_source = $2 WHERE id = $3`,
		exempt, source, torrentID,
	); err != nil {
		t.Fatalf("set torrent exempt: %v", err)
	}
}

func torrentExemptState(t *testing.T, db *sql.DB, torrentID int64) (bool, *string) {
	t.Helper()
	var exempt bool
	var source *string
	if err := db.QueryRow(
		`SELECT hnr_exempt, hnr_exempt_source FROM torrents WHERE id = $1`, torrentID,
	).Scan(&exempt, &source); err != nil {
		t.Fatalf("read torrent exempt state: %v", err)
	}
	return exempt, source
}

func TestHnRRepo_ApplyExemptRules(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewHnRRepo(db)
	u := newUser(t, db)

	wellSeeded := newTorrent(t, db, u.ID) // will get 100 seeders → matches min_seeders 50
	tiny := newTorrent(t, db, u.ID)       // size 1024 → matches max_size_bytes 4096
	big := newTorrent(t, db, u.ID)        // size 1024 but we'll bump it past the size rule
	manualExempt := newTorrent(t, db, u.ID)
	manualNotExempt := newTorrent(t, db, u.ID)

	if _, err := db.ExecContext(ctx, `UPDATE torrents SET seeders = 100 WHERE id = $1`, wellSeeded.ID); err != nil {
		t.Fatalf("set seeders: %v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE torrents SET size = 1099511627776 WHERE id = $1`, big.ID); err != nil {
		t.Fatalf("set size: %v", err)
	}
	setTorrentExempt(t, db, manualExempt.ID, true, model.HnRExemptSourceManual)
	setTorrentExempt(t, db, manualNotExempt.ID, false, model.HnRExemptSourceManual)

	seedRule := &model.HnRExemptRule{Criterion: model.HnRExemptCriterionMinSeeders, Threshold: 50, Enabled: true}
	sizeRule := &model.HnRExemptRule{Criterion: model.HnRExemptCriterionMaxSize, Threshold: 4096, Enabled: true}
	if err := repo.CreateExemptRule(ctx, seedRule); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateExemptRule(ctx, sizeRule); err != nil {
		t.Fatal(err)
	}

	exempted, released, err := repo.ApplyExemptRules(ctx)
	if err != nil {
		t.Fatalf("ApplyExemptRules: %v", err)
	}
	if exempted != 2 || released != 0 {
		t.Fatalf("first pass: exempted=%d released=%d, want 2/0", exempted, released)
	}

	for _, tc := range []struct {
		name       string
		id         int64
		wantExempt bool
		wantSource *string
	}{
		{"well seeded → auto", wellSeeded.ID, true, strptr(model.HnRExemptSourceAuto)},
		{"tiny → auto", tiny.ID, true, strptr(model.HnRExemptSourceAuto)},
		{"big → untouched", big.ID, false, nil},
		{"manual exempt → untouched", manualExempt.ID, true, strptr(model.HnRExemptSourceManual)},
		{"manual not-exempt → untouched", manualNotExempt.ID, false, strptr(model.HnRExemptSourceManual)},
	} {
		exempt, source := torrentExemptState(t, db, tc.id)
		if exempt != tc.wantExempt || !eqStrPtr(source, tc.wantSource) {
			t.Errorf("%s: exempt=%v source=%v, want %v/%v", tc.name, exempt, derefOr(source), tc.wantExempt, derefOr(tc.wantSource))
		}
	}

	// A second pass with the rules unchanged is a no-op.
	exempted, released, err = repo.ApplyExemptRules(ctx)
	if err != nil || exempted != 0 || released != 0 {
		t.Fatalf("idempotent pass: exempted=%d released=%d err=%v", exempted, released, err)
	}

	// Disable both rules → the two auto exemptions are released; the manual one stays.
	seedRule.Enabled = false
	sizeRule.Enabled = false
	if err := repo.UpdateExemptRule(ctx, seedRule); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateExemptRule(ctx, sizeRule); err != nil {
		t.Fatal(err)
	}
	exempted, released, err = repo.ApplyExemptRules(ctx)
	if err != nil {
		t.Fatalf("release pass: %v", err)
	}
	if exempted != 0 || released != 2 {
		t.Fatalf("release pass: exempted=%d released=%d, want 0/2", exempted, released)
	}
	if exempt, source := torrentExemptState(t, db, wellSeeded.ID); exempt || source != nil {
		t.Errorf("released torrent still exempt=%v source=%v", exempt, derefOr(source))
	}
	if exempt, source := torrentExemptState(t, db, manualExempt.ID); !exempt || source == nil || *source != model.HnRExemptSourceManual {
		t.Errorf("manual exemption should survive the release pass: exempt=%v source=%v", exempt, derefOr(source))
	}
}

func strptr(s string) *string { return &s }

func eqStrPtr(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func derefOr(p *string) string {
	if p == nil {
		return "<nil>"
	}
	return *p
}
