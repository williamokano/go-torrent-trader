package leader

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// These cover the guard and error paths, which need no real database. The
// property that actually matters — that two nodes cannot both hold the same
// instance, and that the lock dies with its session — is proven against real
// Postgres in internal/repository/postgres/leader_test.go, because a mock
// cannot tell you anything about advisory-lock semantics.

func newMockDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("creating sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, mock
}

func TestTryAcquireSuccessHoldsTheLease(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("pg_try_advisory_lock").
		WithArgs(LockClass, int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(true))

	lease := NewLease(db, 7)
	acquired, err := lease.TryAcquire(context.Background())
	if err != nil || !acquired {
		t.Fatalf("TryAcquire = %v, %v; want true, nil", acquired, err)
	}
	if !lease.Held() {
		t.Fatal("lease should report itself held")
	}
	lease.Release()
}

// Losing the race must give the connection straight back, or a busy standby
// would slowly drain the pool re-checking every few seconds.
func TestTryAcquireFailureReleasesTheConnection(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("pg_try_advisory_lock").
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(false))

	lease := NewLease(db, 7)
	acquired, err := lease.TryAcquire(context.Background())
	if err != nil {
		t.Fatalf("TryAcquire errored: %v", err)
	}
	if acquired {
		t.Fatal("expected the lock to be reported as unavailable")
	}
	if lease.Held() {
		t.Fatal("a failed acquire must not leave the lease looking held")
	}
}

func TestTryAcquireQueryFailureIsAnError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("pg_try_advisory_lock").WillReturnError(errors.New("connection reset"))

	lease := NewLease(db, 7)
	acquired, err := lease.TryAcquire(context.Background())
	if err == nil {
		t.Fatal("expected the query failure to surface")
	}
	if acquired || lease.Held() {
		t.Fatal("a failed acquire must not look held")
	}
}

// Re-acquiring would abandon the connection holding the current lock.
func TestTryAcquireOnAHeldLeaseIsRefused(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("pg_try_advisory_lock").
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(true))

	lease := NewLease(db, 7)
	if _, err := lease.TryAcquire(context.Background()); err != nil {
		t.Fatalf("first TryAcquire: %v", err)
	}
	defer lease.Release()

	if _, err := lease.TryAcquire(context.Background()); err == nil {
		t.Fatal("expected re-acquiring a held lease to be refused")
	}
}

func TestConfirmRequiresAHeldLease(t *testing.T) {
	db, _ := newMockDB(t)

	lease := NewLease(db, 7)
	if err := lease.Confirm(context.Background()); err == nil {
		t.Fatal("Confirm on an unheld lease must be an error, not a silent pass")
	}
}

func TestConfirmSucceedsOnALiveSession(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("pg_try_advisory_lock").
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(true))
	mock.ExpectQuery("pg_locks").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery("pg_advisory_unlock").
		WillReturnRows(sqlmock.NewRows([]string{"pg_advisory_unlock"}).AddRow(true))

	lease := NewLease(db, 7)
	if _, err := lease.TryAcquire(context.Background()); err != nil {
		t.Fatalf("TryAcquire: %v", err)
	}
	defer lease.Release()

	if err := lease.Confirm(context.Background()); err != nil {
		t.Fatalf("Confirm = %v, want nil", err)
	}
}

// Confirm asks Postgres whether the lock is still held, so a lock that went
// away without the session dying is caught. Nothing else can detect that case,
// and missing it means two nodes announcing in parallel.
func TestConfirmReportsLossWhenTheLockIsGone(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("pg_try_advisory_lock").
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(true))
	mock.ExpectQuery("pg_locks").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery("pg_advisory_unlock").
		WillReturnRows(sqlmock.NewRows([]string{"pg_advisory_unlock"}).AddRow(true))

	lease := NewLease(db, 7)
	if _, err := lease.TryAcquire(context.Background()); err != nil {
		t.Fatalf("TryAcquire: %v", err)
	}
	defer lease.Release()

	if err := lease.Confirm(context.Background()); err == nil {
		t.Fatal("a lock released without the session dying must be reported as lost")
	}
}

// The whole safety property: an unprovable lease is a lost lease, and the
// manager stops announcing the moment this returns an error.
func TestConfirmFailureSurfaces(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("pg_try_advisory_lock").
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(true))
	mock.ExpectQuery("pg_locks").WillReturnError(errors.New("connection reset by peer"))

	lease := NewLease(db, 7)
	if _, err := lease.TryAcquire(context.Background()); err != nil {
		t.Fatalf("TryAcquire: %v", err)
	}
	defer lease.Release()

	if err := lease.Confirm(context.Background()); err == nil {
		t.Fatal("a dead session must not confirm ownership")
	}
}

func TestReleaseOnAnUnheldLeaseIsANoOp(t *testing.T) {
	db, _ := newMockDB(t)

	lease := NewLease(db, 7)
	lease.Release() // must not panic
	if lease.Held() {
		t.Fatal("lease should not report held")
	}
}

func TestReleaseIsIdempotent(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("pg_try_advisory_lock").
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(true))

	lease := NewLease(db, 7)
	if _, err := lease.TryAcquire(context.Background()); err != nil {
		t.Fatalf("TryAcquire: %v", err)
	}
	lease.Release()
	lease.Release()
	if lease.Held() {
		t.Fatal("lease should not report held after release")
	}
}
