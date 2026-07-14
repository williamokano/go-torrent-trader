package postgres

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

// Mutating repository methods guard on RowsAffected and report sql.ErrNoRows
// when they changed nothing. That distinction is what lets a handler answer 404
// instead of 200, so a method that silently no-ops is a real defect. These
// tests pin the guard on every method that has one.
//
// The missing-ID cases and the "already in the target state" cases are both
// covered: several of these statements carry a guard in their WHERE clause
// (`AND deleted_at IS NULL`, `AND NOT dismissed`, `AND status = 'active'`) that
// makes a second call a no-op, and that idempotency guard is the point.

const missingID int64 = 999999

func TestTorrentRepoMutationsOnMissingRowReportNoRows(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewTorrentRepo(db)

	cases := map[string]func() error{
		"Delete":                  func() error { return repo.Delete(ctx, missingID) },
		"IncrementSeeders":        func() error { return repo.IncrementSeeders(ctx, missingID, 1) },
		"IncrementLeechers":       func() error { return repo.IncrementLeechers(ctx, missingID, 1) },
		"IncrementTimesCompleted": func() error { return repo.IncrementTimesCompleted(ctx, missingID) },
	}
	for name, call := range cases {
		t.Run(name, func(t *testing.T) {
			if err := call(); !errors.Is(err, sql.ErrNoRows) {
				t.Errorf("%s(missing id): err = %v, want sql.ErrNoRows", name, err)
			}
		})
	}
}

// TestTorrentRepoCountersClampAtZero pins the GREATEST(0, ...) in the counter
// updates: an over-decrement (a peer reaped twice, say) must floor at zero
// rather than wrap a swarm count negative.
func TestTorrentRepoCountersClampAtZero(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewTorrentRepo(db)
	uploader := newUser(t, db)
	tor := newTorrent(t, db, uploader.ID)

	if err := repo.IncrementSeeders(ctx, tor.ID, 1); err != nil {
		t.Fatalf("IncrementSeeders: %v", err)
	}
	// Decrement by more than is there.
	if err := repo.IncrementSeeders(ctx, tor.ID, -5); err != nil {
		t.Fatalf("IncrementSeeders(-5): %v", err)
	}
	if err := repo.IncrementLeechers(ctx, tor.ID, -3); err != nil {
		t.Fatalf("IncrementLeechers(-3): %v", err)
	}

	got, err := repo.GetByID(ctx, tor.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Seeders != 0 {
		t.Errorf("Seeders = %d after over-decrement, want 0 (clamped)", got.Seeders)
	}
	if got.Leechers != 0 {
		t.Errorf("Leechers = %d after over-decrement, want 0 (clamped)", got.Leechers)
	}
}

func TestUserRepoIncrementStatsGuardsAndClamps(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewUserRepo(db)

	if err := repo.IncrementStats(ctx, missingID, 10, 10); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("IncrementStats(missing id): err = %v, want sql.ErrNoRows", err)
	}

	u := newUser(t, db)
	if err := repo.IncrementStats(ctx, u.ID, 100, 50); err != nil {
		t.Fatalf("IncrementStats: %v", err)
	}
	// Over-subtracting must floor at zero, not underflow into a negative ratio.
	if err := repo.IncrementStats(ctx, u.ID, -1000, -1000); err != nil {
		t.Fatalf("IncrementStats(negative): %v", err)
	}

	got, err := repo.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Uploaded != 0 || got.Downloaded != 0 {
		t.Errorf("uploaded=%d downloaded=%d after over-subtraction, want 0/0 (clamped)", got.Uploaded, got.Downloaded)
	}
}

func TestNewsRepoMutationsOnMissingRowReportNoRows(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewNewsRepo(db)

	article := &model.NewsArticle{ID: missingID, Title: "ghost", Body: "body"}
	if err := repo.Update(ctx, article); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("Update(missing id): err = %v, want sql.ErrNoRows", err)
	}
	if err := repo.Delete(ctx, missingID); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("Delete(missing id): err = %v, want sql.ErrNoRows", err)
	}
}

// TestNewsRepoGetPublishedByIDHidesDrafts pins the published gate: an unpublished
// article must be invisible to the public getter but reachable by the admin one.
func TestNewsRepoGetPublishedByIDHidesDrafts(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewNewsRepo(db)
	author := newUser(t, db)

	draft := &model.NewsArticle{Title: "draft", Body: "unfinished", AuthorID: &author.ID, Published: false}
	if err := repo.Create(ctx, draft); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if _, err := repo.GetByID(ctx, draft.ID); err != nil {
		t.Errorf("GetByID on a draft: %v — admins must still see it", err)
	}
	if _, err := repo.GetPublishedByID(ctx, draft.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("GetPublishedByID on a draft: err = %v, want sql.ErrNoRows", err)
	}

	draft.Published = true
	if err := repo.Update(ctx, draft); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, err := repo.GetPublishedByID(ctx, draft.ID)
	if err != nil {
		t.Fatalf("GetPublishedByID after publishing: %v", err)
	}
	// The author name is a LEFT JOIN, so it must be resolved by the query.
	if got.AuthorName == nil || *got.AuthorName != author.Username {
		t.Errorf("AuthorName = %v, want %q", got.AuthorName, author.Username)
	}
}

// TestWarningRepoUpdateOnlyTouchesActiveWarnings pins the `AND status = 'active'`
// guard: a warning can only be lifted once, so a double-lift must not silently
// rewrite the original lift reason and attribution.
func TestWarningRepoUpdateOnlyTouchesActiveWarnings(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewWarningRepo(db)
	u := newUser(t, db)
	mod := newUser(t, db)
	w := newWarning(t, db, u.ID, "manual", "active", nil)

	w.Status = "lifted"
	w.LiftedAt = ptr(time.Now())
	w.LiftedBy = &mod.ID
	w.LiftedReason = ptr("first lift")
	if err := repo.Update(ctx, w); err != nil {
		t.Fatalf("Update: %v", err)
	}

	// The warning is no longer active, so a second Update matches no rows.
	w.LiftedReason = ptr("second lift")
	if err := repo.Update(ctx, w); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("Update on an already-lifted warning: err = %v, want sql.ErrNoRows", err)
	}

	got, err := repo.GetByID(ctx, w.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.LiftedReason == nil || *got.LiftedReason != "first lift" {
		t.Errorf("LiftedReason = %v, want %q — the rejected update must not have persisted",
			got.LiftedReason, "first lift")
	}

	if err := repo.Update(ctx, &model.Warning{ID: missingID, Status: "lifted"}); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("Update(missing id): err = %v, want sql.ErrNoRows", err)
	}
}

func TestReportRepoResolveOnMissingRowReportsNoRows(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	if err := NewReportRepo(db).Resolve(ctx, missingID, 1); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("Resolve(missing id): err = %v, want sql.ErrNoRows", err)
	}
}

// TestReportRepoExistsForSiteReports covers the NULL-torrent branch of
// ExistsByReporterAndTorrent: a site-wide report has torrent_id IS NULL, which
// `= NULL` would never match, so the query needs the separate IS NULL form.
func TestReportRepoExistsForSiteReports(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewReportRepo(db)
	reporter := newUser(t, db)
	uploader := newUser(t, db)
	tor := newTorrent(t, db, uploader.ID)

	// Nothing reported yet.
	exists, err := repo.ExistsByReporterAndTorrent(ctx, reporter.ID, nil)
	if err != nil {
		t.Fatalf("ExistsByReporterAndTorrent(nil): %v", err)
	}
	if exists {
		t.Error("a site report exists before any was filed")
	}

	// File a site-wide report (torrent_id NULL).
	site := &model.Report{ReporterID: reporter.ID, TorrentID: nil, Reason: "site problem"}
	if err := repo.Create(ctx, site); err != nil {
		t.Fatalf("Create(site report): %v", err)
	}

	exists, err = repo.ExistsByReporterAndTorrent(ctx, reporter.ID, nil)
	if err != nil {
		t.Fatalf("ExistsByReporterAndTorrent(nil): %v", err)
	}
	if !exists {
		t.Error("site report not found via the IS NULL branch")
	}

	// The site report must not be mistaken for a report against a torrent.
	exists, err = repo.ExistsByReporterAndTorrent(ctx, reporter.ID, &tor.ID)
	if err != nil {
		t.Fatalf("ExistsByReporterAndTorrent(torrent): %v", err)
	}
	if exists {
		t.Error("a site-wide report was matched as a report against a torrent")
	}

	// And a torrent report is scoped to its reporter.
	tr := &model.Report{ReporterID: reporter.ID, TorrentID: &tor.ID, Reason: "bad torrent"}
	if err := repo.Create(ctx, tr); err != nil {
		t.Fatalf("Create(torrent report): %v", err)
	}
	other := newUser(t, db)
	exists, err = repo.ExistsByReporterAndTorrent(ctx, other.ID, &tor.ID)
	if err != nil {
		t.Fatalf("ExistsByReporterAndTorrent(other reporter): %v", err)
	}
	if exists {
		t.Error("another user's report was attributed to this reporter")
	}
}

func TestBanRepoDeleteOnMissingRowReportsNoRows(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewBanRepo(db)
	if err := repo.DeleteEmailBan(ctx, missingID); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("DeleteEmailBan(missing id): err = %v, want sql.ErrNoRows", err)
	}
	if err := repo.DeleteIPBan(ctx, missingID); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("DeleteIPBan(missing id): err = %v, want sql.ErrNoRows", err)
	}
}

// TestBanRepoMatchingSemantics pins how the two ban checks actually match:
// emails by SQL LIKE against a stored pattern, IPs by CIDR containment. Both
// are easy to write in a way that is valid SQL but matches the wrong rows.
func TestBanRepoMatchingSemantics(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewBanRepo(db)

	if err := repo.CreateEmailBan(ctx, &model.BannedEmail{Pattern: "%@spam.test"}); err != nil {
		t.Fatalf("CreateEmailBan: %v", err)
	}
	if err := repo.CreateIPBan(ctx, &model.BannedIP{IPRange: "10.1.2.0/24"}); err != nil {
		t.Fatalf("CreateIPBan: %v", err)
	}

	emails := map[string]bool{
		"nuisance@spam.test": true,  // matches the wildcard pattern
		"user@good.test":     false, // different domain
		"spam.test":          false, // pattern requires an @ before the domain
	}
	for email, want := range emails {
		got, err := repo.IsEmailBanned(ctx, email)
		if err != nil {
			t.Fatalf("IsEmailBanned(%q): %v", email, err)
		}
		if got != want {
			t.Errorf("IsEmailBanned(%q) = %v, want %v", email, got, want)
		}
	}

	ips := map[string]bool{
		"10.1.2.7":   true,  // inside the /24
		"10.1.2.255": true,  // still inside
		"10.1.3.1":   false, // adjacent /24
		"192.0.2.1":  false, // unrelated
	}
	for ip, want := range ips {
		got, err := repo.IsIPBanned(ctx, ip)
		if err != nil {
			t.Fatalf("IsIPBanned(%q): %v", ip, err)
		}
		if got != want {
			t.Errorf("IsIPBanned(%q) = %v, want %v", ip, got, want)
		}
	}
}

// TestInviteRepoRedeemRejectsExpiredAndUsed pins the two guards in Redeem's
// WHERE clause. Without them an invite would be infinitely reusable, or usable
// long after it lapsed.
func TestInviteRepoRedeemRejectsExpiredAndUsed(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewInviteRepo(db)
	inviter := newUser(t, db)
	first := newUser(t, db)
	second := newUser(t, db)

	// An invite that lapsed an hour ago cannot be redeemed.
	expired := &model.Invite{
		InviterID: inviter.ID,
		Token:     uniq("expiredtok"),
		ExpiresAt: time.Now().Add(-time.Hour),
	}
	if err := repo.Create(ctx, expired); err != nil {
		t.Fatalf("Create(expired): %v", err)
	}
	if err := repo.Redeem(ctx, expired.Token, first.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("Redeem(expired): err = %v, want sql.ErrNoRows", err)
	}

	// A live invite redeems once...
	live := &model.Invite{
		InviterID: inviter.ID,
		Token:     uniq("livetok"),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	if err := repo.Create(ctx, live); err != nil {
		t.Fatalf("Create(live): %v", err)
	}
	if err := repo.Redeem(ctx, live.Token, first.ID); err != nil {
		t.Fatalf("Redeem(live): %v", err)
	}

	// ...and not twice: the second redeemer must be rejected, not overwrite the first.
	if err := repo.Redeem(ctx, live.Token, second.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("Redeem(already used): err = %v, want sql.ErrNoRows", err)
	}

	got, err := repo.GetByToken(ctx, live.Token)
	if err != nil {
		t.Fatalf("GetByToken: %v", err)
	}
	if !got.Redeemed {
		t.Error("Redeemed = false after a successful redeem")
	}
	if got.InviteeID == nil || *got.InviteeID != first.ID {
		t.Errorf("InviteeID = %v, want the first redeemer %d", got.InviteeID, first.ID)
	}

	// An unknown token is a miss, not a panic.
	if err := repo.Redeem(ctx, "no-such-token", first.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("Redeem(unknown token): err = %v, want sql.ErrNoRows", err)
	}

	// CountPendingByInviter counts only invites that are neither used nor lapsed:
	// the expired one and the redeemed one are both excluded.
	pending, err := repo.CountPendingByInviter(ctx, inviter.ID)
	if err != nil {
		t.Fatalf("CountPendingByInviter: %v", err)
	}
	if pending != 0 {
		t.Errorf("CountPendingByInviter = %d, want 0 (one expired, one redeemed)", pending)
	}
}

// TestRestrictionRepoLiftIsIdempotentlyGuarded pins the `AND lifted_at IS NULL`
// guard: lifting twice must not re-stamp the timestamp and re-attribute it.
func TestRestrictionRepoLiftIsIdempotentlyGuarded(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewRestrictionRepo(db)
	u := newUser(t, db)
	mod := newUser(t, db)

	r := &model.Restriction{UserID: u.ID, RestrictionType: "upload", Reason: "spam"}
	if err := repo.Create(ctx, r); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := repo.Lift(ctx, r.ID, &mod.ID); err != nil {
		t.Fatalf("Lift: %v", err)
	}
	if err := repo.Lift(ctx, r.ID, &mod.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("second Lift: err = %v, want sql.ErrNoRows", err)
	}
	if err := repo.Lift(ctx, missingID, &mod.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("Lift(missing id): err = %v, want sql.ErrNoRows", err)
	}

	// A lifted restriction is no longer active for its type.
	active, err := repo.HasActiveByType(ctx, u.ID, "upload")
	if err != nil {
		t.Fatalf("HasActiveByType: %v", err)
	}
	if active {
		t.Error("HasActiveByType = true after the restriction was lifted")
	}
	// ListActive filters on lifted_at IS NULL, so it must drop it too.
	list, err := repo.ListActive(ctx)
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("ListActive returned %d, want 0", len(list))
	}
	// But the history for the user still records it.
	history, err := repo.ListByUser(ctx, u.ID)
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("ListByUser returned %d, want 1", len(history))
	}
	if history[0].LiftedByUsername != mod.Username {
		t.Errorf("LiftedByUsername = %q, want %q (resolved via the LEFT JOIN)",
			history[0].LiftedByUsername, mod.Username)
	}
}

// TestCheatFlagRepoDismissIsGuarded pins the `AND NOT dismissed` guard: a flag
// already dismissed by one staffer must not be silently re-attributed to the next.
func TestCheatFlagRepoDismissIsGuarded(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewCheatFlagRepo(db)
	u := newUser(t, db)
	firstMod := newUser(t, db)
	secondMod := newUser(t, db)
	tor := newTorrent(t, db, u.ID)

	// details is JSONB, so it must be valid JSON.
	flag := &model.CheatFlag{
		UserID:    u.ID,
		TorrentID: &tor.ID,
		FlagType:  "impossible_speed",
		Details:   `{"speed_mbps": 9001}`,
	}
	if err := repo.Create(ctx, flag); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := repo.Dismiss(ctx, flag.ID, firstMod.ID); err != nil {
		t.Fatalf("Dismiss: %v", err)
	}
	if err := repo.Dismiss(ctx, flag.ID, secondMod.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("second Dismiss: err = %v, want sql.ErrNoRows", err)
	}
	if err := repo.Dismiss(ctx, missingID, firstMod.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("Dismiss(missing id): err = %v, want sql.ErrNoRows", err)
	}

	got, err := repo.GetByID(ctx, flag.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if !got.Dismissed {
		t.Error("Dismissed = false after Dismiss")
	}
	if got.DismissedBy == nil || *got.DismissedBy != firstMod.ID {
		t.Errorf("DismissedBy = %v, want the first dismisser %d", got.DismissedBy, firstMod.ID)
	}
	if got.DismisserName != firstMod.Username {
		t.Errorf("DismisserName = %q, want %q", got.DismisserName, firstMod.Username)
	}
	if got.TorrentName != tor.Name {
		t.Errorf("TorrentName = %q, want %q (resolved via the LEFT JOIN)", got.TorrentName, tor.Name)
	}

	// A dismissed flag no longer counts as a recent undismissed one, which is
	// what stops the cheat detector re-flagging within the cooldown window.
	recent, err := repo.HasRecentUndismissed(ctx, u.ID, tor.ID, "impossible_speed", 24)
	if err != nil {
		t.Fatalf("HasRecentUndismissed: %v", err)
	}
	if recent {
		t.Error("HasRecentUndismissed = true for a dismissed flag")
	}
}

// TestCheatFlagRepoGetByIDTolueratesNullTorrent covers the LEFT JOIN + COALESCE
// path: a flag not tied to a torrent must still load, with an empty torrent name.
func TestCheatFlagRepoGetByIDToleratesNullTorrent(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewCheatFlagRepo(db)
	u := newUser(t, db)

	flag := &model.CheatFlag{UserID: u.ID, TorrentID: nil, FlagType: "multi_ip", Details: `{}`}
	if err := repo.Create(ctx, flag); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetByID(ctx, flag.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.TorrentID != nil {
		t.Errorf("TorrentID = %v, want nil", got.TorrentID)
	}
	// COALESCE turns the NULL join into "", and DismissedBy is still NULL.
	if got.TorrentName != "" {
		t.Errorf("TorrentName = %q, want empty", got.TorrentName)
	}
	if got.DismisserName != "" {
		t.Errorf("DismisserName = %q, want empty", got.DismisserName)
	}

	if _, err := repo.GetByID(ctx, missingID); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("GetByID(missing id): err = %v, want sql.ErrNoRows", err)
	}
}

func TestSimpleDeletesOnMissingRowReportNoRows(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	cases := map[string]func() error{
		"ModNoteRepo.Delete":     func() error { return NewModNoteRepo(db).Delete(ctx, missingID) },
		"ChatMessageRepo.Delete": func() error { return NewChatMessageRepo(db).Delete(ctx, missingID) },
		"CommentRepo.Delete":     func() error { return NewCommentRepo(db).Delete(ctx, missingID) },
	}
	for name, call := range cases {
		t.Run(name, func(t *testing.T) {
			if err := call(); !errors.Is(err, sql.ErrNoRows) {
				t.Errorf("%s(missing id): err = %v, want sql.ErrNoRows", name, err)
			}
		})
	}

	// CategoryRepo.Delete is the odd one out: it reports a missing row with a
	// bare fmt.Errorf rather than sql.ErrNoRows, so a caller cannot errors.Is it
	// into a 404. Pinned as-is; changing it is a production change, not a test one.
	err := NewCategoryRepo(db).Delete(ctx, missingID)
	if err == nil {
		t.Fatal("CategoryRepo.Delete(missing id) = nil, want an error")
	}
	if errors.Is(err, sql.ErrNoRows) {
		t.Log("CategoryRepo.Delete now returns sql.ErrNoRows; this test's comment is stale")
	}
}

// TestNotificationMarkReadIsScopedToOwner is the security-relevant one: MarkRead
// takes both the notification id and the user id, and the WHERE clause must
// require both. If it matched on id alone, any user could mark another user's
// notification read.
func TestNotificationMarkReadIsScopedToOwner(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewNotificationRepo(db)
	owner := newUser(t, db)
	stranger := newUser(t, db)

	n := &model.Notification{UserID: owner.ID, Type: "pm", Data: []byte(`{"x":1}`)}
	if err := repo.Create(ctx, n); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// A stranger cannot touch it.
	if err := repo.MarkRead(ctx, stranger.ID, n.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("MarkRead(stranger): err = %v, want sql.ErrNoRows", err)
	}
	got, err := repo.GetByID(ctx, n.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Read {
		t.Error("a stranger managed to mark the owner's notification read")
	}

	// The owner can.
	if err := repo.MarkRead(ctx, owner.ID, n.ID); err != nil {
		t.Fatalf("MarkRead(owner): %v", err)
	}
	got, err = repo.GetByID(ctx, n.ID)
	if err != nil {
		t.Fatalf("GetByID after MarkRead: %v", err)
	}
	if !got.Read {
		t.Error("Read = false after the owner marked it read")
	}

	// A missing notification is a miss for anyone.
	if err := repo.MarkRead(ctx, owner.ID, missingID); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("MarkRead(missing id): err = %v, want sql.ErrNoRows", err)
	}
	if _, err := repo.GetByID(ctx, missingID); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("GetByID(missing id): err = %v, want sql.ErrNoRows", err)
	}
}

// TestMessageDeleteForUserRejectsThirdParties covers the fall-through in
// DeleteForUser: a user who is neither sender nor receiver must not be able to
// delete the message for either party.
func TestMessageDeleteForUserRejectsThirdParties(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewMessageRepo(db)
	sender := newUser(t, db)
	receiver := newUser(t, db)
	stranger := newUser(t, db)

	msg := newMessage(t, db, sender.ID, receiver.ID, "subject")

	if err := repo.DeleteForUser(ctx, msg.ID, stranger.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("DeleteForUser(stranger): err = %v, want sql.ErrNoRows", err)
	}

	// Neither side's tombstone was set.
	got, err := repo.GetByID(ctx, msg.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.SenderDeleted || got.ReceiverDeleted {
		t.Errorf("a third party deleted the message: sender_deleted=%v receiver_deleted=%v",
			got.SenderDeleted, got.ReceiverDeleted)
	}

	// A message that does not exist reports ErrNoRows from the lookup, not a panic.
	if err := repo.DeleteForUser(ctx, missingID, sender.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("DeleteForUser(missing id): err = %v, want sql.ErrNoRows", err)
	}
}

// TestMessageMarkAsReadIsScopedToReceiver: the sender must not be able to mark
// their own outgoing message as read on the receiver's behalf.
func TestMessageMarkAsReadIsScopedToReceiver(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewMessageRepo(db)
	sender := newUser(t, db)
	receiver := newUser(t, db)
	msg := newMessage(t, db, sender.ID, receiver.ID, "subject")

	// MarkAsRead does not report a miss (it has no RowsAffected guard), but it
	// must not have changed anything either.
	if err := repo.MarkAsRead(ctx, msg.ID, sender.ID); err != nil {
		t.Fatalf("MarkAsRead(sender): %v", err)
	}
	got, err := repo.GetByID(ctx, msg.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.IsRead {
		t.Error("the sender marked their own outgoing message as read")
	}

	if err := repo.MarkAsRead(ctx, msg.ID, receiver.ID); err != nil {
		t.Fatalf("MarkAsRead(receiver): %v", err)
	}
	got, err = repo.GetByID(ctx, msg.ID)
	if err != nil {
		t.Fatalf("GetByID after receiver read: %v", err)
	}
	if !got.IsRead {
		t.Error("IsRead = false after the receiver marked it read")
	}

	// Unread count drops to zero once read.
	unread, err := repo.CountUnread(ctx, receiver.ID)
	if err != nil {
		t.Fatalf("CountUnread: %v", err)
	}
	if unread != 0 {
		t.Errorf("CountUnread = %d, want 0", unread)
	}
}

// TestChatMuteDeleteIsNotFoundTolerant documents the deliberate asymmetry:
// ChatMuteRepo.Delete clears every mute for a user and has no RowsAffected
// guard, so unmuting an unmuted user is a no-op rather than an error.
func TestChatMuteDeleteIsNotFoundTolerant(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewChatMuteRepo(db)
	u := newUser(t, db)

	if err := repo.Delete(ctx, u.ID); err != nil {
		t.Errorf("Delete on an unmuted user: %v — want a tolerant no-op", err)
	}

	// Two overlapping mutes: Delete must clear both, not just the newest.
	for i := 0; i < 2; i++ {
		m := &model.ChatMute{UserID: u.ID, Reason: "spam", ExpiresAt: time.Now().Add(time.Hour)}
		if err := repo.Create(ctx, m); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}
	if err := repo.Delete(ctx, u.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	active, err := repo.GetActiveMute(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetActiveMute: %v", err)
	}
	// GetActiveMute signals "no mute" with (nil, nil), not an error.
	if active != nil {
		t.Errorf("GetActiveMute = %+v after Delete, want nil", active)
	}
}
