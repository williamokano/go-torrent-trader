package worker

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/hibiken/asynq"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository/postgres"
)

// anyGroupID returns a seeded group id. Groups come from migration 001 and
// are never touched by this package's tests.
func anyGroupID(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	var id int64
	if err := db.QueryRow(`SELECT id FROM groups ORDER BY level LIMIT 1`).Scan(&id); err != nil {
		t.Fatalf("no seeded group: %v", err)
	}
	return id
}

var cleanupUserCounter int

// newCleanupTestUser inserts a user through the real UserRepo — not raw SQL —
// so the fixture exercises the same Create path production code uses, then
// backdates created_at directly. Create always stamps "now", and step 4 of
// the cleanup handler only matches registrations older than 7 days.
// activatedAt is passed straight through to Create, mirroring how
// AuthService.Register stamps ActivatedAt itself for no-confirmation
// registrations (nil left as-is is the pending-confirmation state).
func newCleanupTestUser(t *testing.T, db *sql.DB, enabled bool, createdAt time.Time, activatedAt *time.Time) *model.User {
	t.Helper()
	cleanupUserCounter++
	name := fmt.Sprintf("cleanupuser%d", cleanupUserCounter)

	u := &model.User{
		Username:       name,
		Email:          name + "@example.test",
		PasswordHash:   "hash",
		PasswordScheme: "argon2id",
		GroupID:        anyGroupID(t, db),
		Enabled:        enabled,
		ActivatedAt:    activatedAt,
		CanDownload:    true,
		CanUpload:      true,
		CanChat:        true,
		CanForum:       true,
	}
	if err := postgres.NewUserRepo(db).Create(context.Background(), u); err != nil {
		t.Fatalf("creating user: %v", err)
	}

	if _, err := db.Exec(`UPDATE users SET created_at = $1 WHERE id = $2`, createdAt, u.ID); err != nil {
		t.Fatalf("backdating created_at: %v", err)
	}
	u.CreatedAt = createdAt

	return u
}

// banUser flips enabled=false the way the admin ban paths do (AdminService.
// UpdateUser / QuickBanUser: read-modify-write the whole row), without
// touching any other field — in particular, it must not clobber ActivatedAt.
func banUser(t *testing.T, repo *postgres.UserRepo, u *model.User) {
	t.Helper()
	ctx := context.Background()

	fresh, err := repo.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetByID before ban: %v", err)
	}
	fresh.Enabled = false
	if err := repo.Update(ctx, fresh); err != nil {
		t.Fatalf("banning user: %v", err)
	}
}

// TestCleanupHandler_PreservesActivatedButBannedUsers is the regression test
// for BE-8.19. Step 4 of the cleanup handler used to delete any row matching
// `enabled = false AND created_at < NOW() - 7 days`, with no way to tell a
// banned user apart from a registration that was genuinely never activated.
// Both admin ban paths (AdminService.UpdateUser and QuickBanUser) flip
// enabled to false without ever touching ActivatedAt, so banning a user
// whose account predates the 7-day window used to hard-delete them on the
// next cleanup run — cascading away their torrents/notes/warnings and
// erasing the ban record itself.
//
// A first fix (last_access IS NULL) only closed part of the gap: last_access
// is set by ActivityTracker on a *subsequent* authenticated request, so a
// user banned right after registering (no confirmation required) or right
// after their first login — before any follow-up request — would still read
// as "never activated" and get deleted. ActivatedAt closes that: it is
// stamped by AuthService at the moment of registration (no confirmation
// needed), email confirmation, or first login — whichever comes first — with
// no follow-up request required. This test proves both of those "activated
// but never made a second request" cases survive, while a genuinely abandoned
// registration does not — mirroring the spirit of
// TestUserRepoUpdate_DoesNotClobberPrivilegeFlags (BE-8.16).
func TestCleanupHandler_PreservesActivatedButBannedUsers(t *testing.T) {
	db := requireDB(t)
	ctx := context.Background()
	repo := postgres.NewUserRepo(db)

	oldEnough := time.Now().Add(-30 * 24 * time.Hour)
	activatedLongAgo := oldEnough.Add(time.Hour)

	// A user who logged in at some point after registering (ActivatedAt set,
	// exactly as AuthService.Login does), then was banned. No further
	// activity ever happened, so last_access is still NULL — the old fix
	// would have deleted this user; the ActivatedAt-based fix must not.
	bannedAfterLogin := newCleanupTestUser(t, db, true, oldEnough, &activatedLongAgo)
	banUser(t, repo, bannedAfterLogin)

	// A user banned immediately after a no-confirmation registration, before
	// ever making a single authenticated request. AuthService.Register stamps
	// ActivatedAt at creation time in this flow, so even though the account
	// never did anything beyond signing up, it is not "abandoned" — someone
	// signed up and was banned for it, and that ban record must survive.
	bannedRightAfterRegistration := newCleanupTestUser(t, db, true, oldEnough, &oldEnough)
	banUser(t, repo, bannedRightAfterRegistration)

	// A registration that was never activated at all: created disabled
	// (mirrors AuthService.Register's needsConfirmation path), never
	// confirmed or logged in, and simply abandoned. ActivatedAt stays nil,
	// and there is no pending email_confirmations row shielding it, so
	// nothing should stop cleanup from reclaiming it.
	abandoned := newCleanupTestUser(t, db, false, oldEnough, nil)

	deps := &WorkerDeps{
		PeerRepo:    &mockPeerRepo{},
		TorrentRepo: &mockTorrentRepo{},
		DB:          db,
	}
	handler := NewCleanupHandler(deps)
	if err := handler(ctx, asynq.NewTask(TaskCleanupPeers, nil)); err != nil {
		t.Fatalf("cleanup handler: %v", err)
	}

	if _, err := repo.GetByID(ctx, bannedAfterLogin.ID); err != nil {
		t.Errorf("user banned after a prior login was deleted by cleanup: %v", err)
	}
	if _, err := repo.GetByID(ctx, bannedRightAfterRegistration.ID); err != nil {
		t.Errorf("user banned immediately after registration was deleted by cleanup: %v", err)
	}
	if _, err := repo.GetByID(ctx, abandoned.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("genuinely abandoned unconfirmed registration survived cleanup: err=%v, want sql.ErrNoRows", err)
	}
}
