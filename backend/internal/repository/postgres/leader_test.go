package postgres

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/connector/leader"
)

// The lease lives here rather than in its own package's tests because it needs
// a real Postgres: an advisory lock is a property of a live session, and the
// whole point of the design is what happens to it when that session ends.

func TestLeaseIsExclusivePerInstance(t *testing.T) {
	db := requireDB(t)
	ctx := context.Background()

	first := leader.NewLease(db, 1)
	acquired, err := first.TryAcquire(ctx)
	if err != nil || !acquired {
		t.Fatalf("first TryAcquire = %v, %v; want true, nil", acquired, err)
	}
	defer first.Release()

	second := leader.NewLease(db, 1)
	acquired, err = second.TryAcquire(ctx)
	if err != nil {
		t.Fatalf("second TryAcquire errored: %v", err)
	}
	if acquired {
		second.Release()
		t.Fatal("two nodes both acquired the same instance: they would both announce")
	}
	if second.Held() {
		t.Fatal("a failed acquire must not leave the lease looking held")
	}
}

func TestLeaseDoesNotBlockOtherInstances(t *testing.T) {
	db := requireDB(t)
	ctx := context.Background()

	one := leader.NewLease(db, 1)
	if acquired, err := one.TryAcquire(ctx); err != nil || !acquired {
		t.Fatalf("acquiring instance 1: %v, %v", acquired, err)
	}
	defer one.Release()

	two := leader.NewLease(db, 2)
	acquired, err := two.TryAcquire(ctx)
	if err != nil || !acquired {
		t.Fatalf("instance 2 should be independently acquirable: %v, %v", acquired, err)
	}
	two.Release()
}

// Release must actually free the lock in Postgres, not merely hand the
// connection back to the pool.
//
// Asserting "a second lease can acquire" is not enough on its own: database/sql
// reuses free connections LIFO, so on an idle pool the second acquire lands on
// the very same session and succeeds re-entrantly even when the lock was never
// released. This checks pg_locks directly, which is the only version of the
// assertion that can fail.
func TestLeaseReleaseActuallyDropsTheLock(t *testing.T) {
	db := requireDB(t)
	ctx := context.Background()

	lease := leader.NewLease(db, 3)
	if acquired, err := lease.TryAcquire(ctx); err != nil || !acquired {
		t.Fatalf("TryAcquire: %v, %v", acquired, err)
	}
	if got := advisoryLockCount(t, db, 3); got != 1 {
		t.Fatalf("%d advisory locks while held, want 1", got)
	}

	lease.Release()
	if lease.Held() {
		t.Fatal("lease still reports held after Release")
	}

	if got := advisoryLockCount(t, db, 3); got != 0 {
		t.Fatalf("%d advisory locks after Release, want 0: the lock outlived the lease", got)
	}
}

// The hand-off another node actually performs: acquire from a *different*
// session than the one that just released.
func TestLeaseReleaseLetsAnotherSessionTakeOver(t *testing.T) {
	db := requireDB(t)
	ctx := context.Background()

	first := leader.NewLease(db, 30)
	if acquired, err := first.TryAcquire(ctx); err != nil || !acquired {
		t.Fatalf("first TryAcquire: %v, %v", acquired, err)
	}
	firstPID := backendPID(t, db, first)
	first.Release()

	// Hold the freed connection out of reach so the next acquire is forced onto
	// a new session, the way a second process would be.
	blockers := occupyConnections(t, db, 4)
	defer blockers()

	second := leader.NewLease(db, 30)
	acquired, err := second.TryAcquire(ctx)
	if err != nil || !acquired {
		t.Fatalf("a different session could not take over: %v, %v", acquired, err)
	}
	defer second.Release()

	if secondPID := backendPID(t, db, second); secondPID == firstPID {
		t.Skip("the pool handed back the same session; this run cannot prove the cross-session case")
	}
}

// A session that dies takes its locks with it — the crash path.
func TestLeaseLockDiesWithTheSession(t *testing.T) {
	db := requireDB(t)
	ctx := context.Background()

	lease := leader.NewLease(db, 32)
	if acquired, err := lease.TryAcquire(ctx); err != nil || !acquired {
		t.Fatalf("TryAcquire: %v, %v", acquired, err)
	}
	pid := backendPID(t, db, lease)

	if _, err := db.ExecContext(ctx, `SELECT pg_terminate_backend($1)`, pid); err != nil {
		t.Fatalf("terminating the lease backend: %v", err)
	}

	waitForLockCount(t, db, 32, 0)

	// And the owner notices, which is what makes it stop announcing.
	if err := lease.Confirm(ctx); err == nil {
		t.Fatal("Confirm must fail once the lease's session is gone")
	}
	lease.Release()
}

func TestLeaseConfirmSucceedsWhileHeldAndFailsAfterRelease(t *testing.T) {
	db := requireDB(t)
	ctx := context.Background()

	lease := leader.NewLease(db, 4)
	if acquired, err := lease.TryAcquire(ctx); err != nil || !acquired {
		t.Fatalf("TryAcquire: %v, %v", acquired, err)
	}

	if err := lease.Confirm(ctx); err != nil {
		t.Fatalf("Confirm while held = %v, want nil", err)
	}

	lease.Release()

	// After release there is no session to confirm against. The manager treats
	// any Confirm error as ownership lost and stops the client immediately.
	if err := lease.Confirm(ctx); err == nil {
		t.Fatal("Confirm must fail once the lease is no longer held")
	}
}

// --- helpers ---

// advisoryLockCount reports how many sessions hold the connector lock for an
// instance, read from Postgres rather than inferred from our own state.
func advisoryLockCount(t *testing.T, db *sql.DB, instanceID int64) int {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pg_locks
		WHERE locktype = 'advisory' AND classid = $1::int8::oid
		  AND objid = $2::int8::oid AND objsubid = 2 AND granted`,
		leader.LockClass, instanceID,
	).Scan(&count); err != nil {
		t.Fatalf("counting advisory locks: %v", err)
	}
	return count
}

func waitForLockCount(t *testing.T, db *sql.DB, instanceID int64, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if got := advisoryLockCount(t, db, instanceID); got == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("advisory lock count for instance %d never reached %d", instanceID, want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// backendPID finds the session holding the lease's lock.
func backendPID(t *testing.T, db *sql.DB, lease *leader.Lease) int {
	t.Helper()
	_ = lease
	var pid int
	if err := db.QueryRow(`SELECT pid FROM pg_locks
		WHERE locktype = 'advisory' AND classid = $1::int8::oid AND granted
		ORDER BY pid LIMIT 1`, leader.LockClass).Scan(&pid); err != nil {
		t.Fatalf("finding the lease backend pid: %v", err)
	}
	return pid
}

// occupyConnections holds n pool connections busy so the next acquire is forced
// onto a fresh session. Returns a func that frees them.
func occupyConnections(t *testing.T, db *sql.DB, n int) func() {
	t.Helper()
	conns := make([]*sql.Conn, 0, n)
	for i := 0; i < n; i++ {
		conn, err := db.Conn(context.Background())
		if err != nil {
			break
		}
		conns = append(conns, conn)
	}
	return func() {
		for _, conn := range conns {
			_ = conn.Close()
		}
	}
}

func TestLeaseCannotBeAcquiredTwiceByItself(t *testing.T) {
	db := requireDB(t)
	ctx := context.Background()

	lease := leader.NewLease(db, 5)
	if acquired, err := lease.TryAcquire(ctx); err != nil || !acquired {
		t.Fatalf("TryAcquire: %v, %v", acquired, err)
	}
	defer lease.Release()

	// Re-acquiring would leak the first connection, so it is an error rather
	// than a silent no-op.
	if _, err := lease.TryAcquire(ctx); err == nil {
		t.Fatal("expected re-acquiring a held lease to be refused")
	}
}
