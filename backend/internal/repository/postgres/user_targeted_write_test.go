package postgres

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
	"github.com/williamokano/go-torrent-trader/backend/internal/testutil"
)

// staffActsDuringLogin hands Login the user it asked for, then lets a staff
// write land before Login goes on to check the password and record the
// login — the window a whole-row write used to reopen.
type staffActsDuringLogin struct {
	repository.UserRepository
	act func()
}

func (s *staffActsDuringLogin) GetByUsername(ctx context.Context, username string) (*model.User, error) {
	u, err := s.UserRepository.GetByUsername(ctx, username)
	if err == nil && s.act != nil {
		s.act()
		s.act = nil
	}
	return u, err
}

// A login reads the user, spends an Argon2 hash, then records the login. A
// ban, a demotion or a password reset that lands in between must still be
// there afterwards: the login writes only its own columns.
func TestLoginDoesNotRevertConcurrentStaffWrites(t *testing.T) {
	db := requireDB(t)
	ctx := context.Background()

	var otherGroup int64
	cases := map[string]struct {
		patch func() model.UserPatch
		check func(t *testing.T, got *model.User)
	}{
		"ban": {
			patch: model.DisablePatch,
			check: func(t *testing.T, got *model.User) {
				if got.Enabled {
					t.Error("the login re-enabled a banned account")
				}
			},
		},
		"demotion": {
			patch: func() model.UserPatch { return model.UserPatch{GroupID: model.To(otherGroup)} },
			check: func(t *testing.T, got *model.User) {
				if got.GroupID != otherGroup {
					t.Errorf("group = %d, want the demotion to %d kept", got.GroupID, otherGroup)
				}
			},
		},
		"password reset": {
			patch: func() model.UserPatch { return model.UserPatch{PasswordHash: model.To("reset-by-staff")} },
			check: func(t *testing.T, got *model.User) {
				if got.PasswordHash != "reset-by-staff" {
					t.Error("the login wrote the old password hash back")
				}
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			resetTestData(t, db)
			repo := NewUserRepo(db)
			u := newUser(t, db)
			hash, err := service.HashPassword("correct horse")
			if err != nil {
				t.Fatalf("hash: %v", err)
			}
			if err := repo.UpdateFields(ctx, u.ID, model.UserPatch{PasswordHash: model.To(hash)}); err != nil {
				t.Fatalf("set password: %v", err)
			}
			if err := db.QueryRow(`SELECT id FROM groups WHERE id <> $1 ORDER BY id LIMIT 1`, u.GroupID).Scan(&otherGroup); err != nil {
				t.Fatalf("other group: %v", err)
			}

			users := &staffActsDuringLogin{UserRepository: repo, act: func() {
				if err := repo.UpdateFields(ctx, u.ID, tc.patch()); err != nil {
					t.Fatalf("staff write: %v", err)
				}
			}}
			auth := service.NewAuthService(users, testutil.NewMemorySessionStore(), testutil.NewMemoryPasswordResetStore(),
				&testutil.NoopSender{}, "http://localhost", event.NewInMemoryBus())
			// The outcome of the login itself is not the point; only what it
			// leaves behind.
			_, _, _ = auth.Login(ctx, service.LoginRequest{Username: u.Username, Password: "correct horse"}, "10.0.0.1")

			got, err := repo.GetByID(ctx, u.ID)
			if err != nil {
				t.Fatalf("GetByID: %v", err)
			}
			tc.check(t, got)
		})
	}
}

// UpdateFields writes the patched columns and leaves every other one as the
// database has it — including values written after the caller read the row.
func TestUserRepoUpdateFieldsTouchesOnlyPatchedColumns(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewUserRepo(db)
	u := newUser(t, db)

	// Someone else changes the title after "u" was read.
	if err := repo.UpdateFields(ctx, u.ID, model.UserPatch{Title: model.To(ptr("set elsewhere"))}); err != nil {
		t.Fatalf("other write: %v", err)
	}
	if err := repo.UpdateFields(ctx, u.ID, model.UserPatch{Donor: model.To(true), WarnUntil: model.To[*time.Time](nil)}); err != nil {
		t.Fatalf("UpdateFields: %v", err)
	}

	got, err := repo.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if !got.Donor {
		t.Error("the patched column was not written")
	}
	if got.Title == nil || *got.Title != "set elsewhere" {
		t.Errorf("title = %v, want the other writer's value kept", got.Title)
	}
	if got.Username != u.Username || got.Email != u.Email || !got.Enabled {
		t.Errorf("an unpatched column changed: %+v", got)
	}
}

// Activation is stamped once: a later login or enable keeps the first time.
func TestUserRepoUpdateFieldsActivatesOnlyOnce(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewUserRepo(db)
	u := newUser(t, db)

	first := time.Now().Add(-time.Hour).Truncate(time.Second)
	for _, at := range []time.Time{first, time.Now()} {
		if err := repo.UpdateFields(ctx, u.ID, model.UserPatch{ActivateAt: model.To(at)}); err != nil {
			t.Fatalf("UpdateFields: %v", err)
		}
	}
	got, _ := repo.GetByID(ctx, u.ID)
	if got.ActivatedAt == nil || !got.ActivatedAt.Equal(first) {
		t.Errorf("activated_at = %v, want the first %v", got.ActivatedAt, first)
	}
}

func TestUserRepoUpdateFieldsEdges(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewUserRepo(db)

	if err := repo.UpdateFields(ctx, 1<<40, model.UserPatch{Donor: model.To(true)}); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("unknown id: err = %v, want sql.ErrNoRows", err)
	}
	u := newUser(t, db)
	before, _ := repo.GetByID(ctx, u.ID)
	if err := repo.UpdateFields(ctx, u.ID, model.UserPatch{}); err != nil {
		t.Fatalf("empty patch: %v", err)
	}
	after, _ := repo.GetByID(ctx, u.ID)
	if !after.UpdatedAt.Equal(before.UpdatedAt) {
		t.Error("an empty patch wrote the row")
	}
}
