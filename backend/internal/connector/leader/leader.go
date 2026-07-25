// Package leader elects the single node that owns a persistent connector.
//
// An IRC connection must be a singleton across the whole deployment: two nodes
// both connected would join the channel twice and announce every torrent twice.
// A Postgres advisory lock gives that for free — a single-process deployment
// acquires instantly and never notices there was an election.
package leader

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// LockClass namespaces these locks inside Postgres' single global advisory-lock
// space, so a lock on instance 3 here can never collide with some other
// subsystem's lock on its own object 3. It spells "conn".
const LockClass = 0x636F6E6E

// confirmTimeout bounds the liveness check on the held connection.
const confirmTimeout = 5 * time.Second

// Lease is one node's claim on one connector instance.
//
// The lock is held by a dedicated *sql.Conn rather than the pool, because a
// session-scoped advisory lock belongs to whichever connection took it — from
// the pool it could be released by an unrelated later query, or held by a
// connection that has already gone back into rotation.
type Lease struct {
	db         *sql.DB
	instanceID int64
	conn       *sql.Conn
}

// NewLease creates an unacquired lease for one instance.
func NewLease(db *sql.DB, instanceID int64) *Lease {
	return &Lease{db: db, instanceID: instanceID}
}

// TryAcquire attempts to take ownership, reporting whether it succeeded.
//
// It never blocks: a node that loses simply retries later, which is what keeps
// a standby cheap.
func (l *Lease) TryAcquire(ctx context.Context) (bool, error) {
	if l.conn != nil {
		return false, fmt.Errorf("lease for instance %d is already held", l.instanceID)
	}

	conn, err := l.db.Conn(ctx)
	if err != nil {
		return false, fmt.Errorf("open lease connection: %w", err)
	}

	var acquired bool
	if err := conn.QueryRowContext(ctx,
		`SELECT pg_try_advisory_lock($1, $2)`, LockClass, l.instanceID,
	).Scan(&acquired); err != nil {
		_ = conn.Close()
		return false, fmt.Errorf("acquire advisory lock: %w", err)
	}
	if !acquired {
		// Another node owns it. Give the connection straight back.
		_ = conn.Close()
		return false, nil
	}

	l.conn = conn
	return true, nil
}

// Confirm re-checks that ownership still holds, by proving the connection that
// took the lock is still alive.
//
// This is the whole safety property: the lock lives on that session, so if the
// session is gone the lock is gone — and Postgres has no way to tell us. A node
// that cannot confirm must stop announcing immediately, or a network partition
// produces two nodes both convinced they are the owner.
func (l *Lease) Confirm(ctx context.Context) error {
	if l.conn == nil {
		return fmt.Errorf("lease for instance %d is not held", l.instanceID)
	}

	ctx, cancel := context.WithTimeout(ctx, confirmTimeout)
	defer cancel()

	var one int
	if err := l.conn.QueryRowContext(ctx, `SELECT 1`).Scan(&one); err != nil {
		return fmt.Errorf("confirm advisory lock: %w", err)
	}
	return nil
}

// Release gives up ownership.
//
// It closes the connection rather than calling pg_advisory_unlock, because
// ending the session releases the lock anyway — and this way the crash path and
// the graceful path release by exactly the same mechanism, so there is only one
// behaviour to reason about.
func (l *Lease) Release() {
	if l.conn == nil {
		return
	}
	_ = l.conn.Close()
	l.conn = nil
}

// Held reports whether this lease currently believes it owns the instance.
func (l *Lease) Held() bool {
	return l.conn != nil
}
