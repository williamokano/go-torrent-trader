package postgres

import (
	"context"
	"testing"

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

// Releasing closes the session, and the lock goes with it — which is the same
// mechanism that frees it when a node crashes.
func TestLeaseReleaseLetsAnotherNodeTakeOver(t *testing.T) {
	db := requireDB(t)
	ctx := context.Background()

	first := leader.NewLease(db, 3)
	if acquired, err := first.TryAcquire(ctx); err != nil || !acquired {
		t.Fatalf("first TryAcquire: %v, %v", acquired, err)
	}
	first.Release()
	if first.Held() {
		t.Fatal("lease still reports held after Release")
	}

	second := leader.NewLease(db, 3)
	acquired, err := second.TryAcquire(ctx)
	if err != nil || !acquired {
		t.Fatalf("second node could not take over after release: %v, %v", acquired, err)
	}
	second.Release()
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
