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
	"database/sql/driver"
	"fmt"
	"log/slog"
	"time"
)

// LockClass namespaces these locks inside Postgres' single global advisory-lock
// space, so a lock on instance 3 here can never collide with some other
// subsystem's lock on its own object 3. It spells "conn".
const LockClass = 0x636F6E6E

// advisoryObjSubID is what pg_locks records for the two-argument
// pg_advisory_lock(int4, int4) form. The single-bigint form uses 1, so
// including it keeps ownershipQuery from matching somebody else's lock that
// happens to share our numbers.
const advisoryObjSubID = 2

const (
	// confirmTimeout bounds the ownership check.
	confirmTimeout = 5 * time.Second
	// releaseTimeout bounds the unlock. Short: the caller is usually shutting
	// down or handing over, and waiting is worse than poisoning the connection.
	releaseTimeout = 5 * time.Second
	// acquireTimeout bounds waiting for a pool connection, so a saturated pool
	// wedges the supervise loop for a few seconds rather than forever.
	acquireTimeout = 10 * time.Second
)

// ownershipQuery asks Postgres whether this backend still holds the lock.
//
// It deliberately does not settle for "the session is alive". A liveness probe
// cannot see a lock that was released without the session dying — by an
// operator's pg_advisory_unlock_all(), by a DISCARD ALL, or by a connection
// pooler handing the session elsewhere — and each of those leaves this node
// convinced it is the owner while another node announces in parallel.
const ownershipQuery = `SELECT EXISTS (
	SELECT 1 FROM pg_locks
	WHERE locktype = 'advisory'
	  AND classid = $1::int8::oid
	  AND objid = $2::int8::oid
	  AND objsubid = $3
	  AND pid = pg_backend_pid()
	  AND granted
)`

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
// It never blocks on the lock itself: a node that loses simply retries later,
// which is what keeps a standby cheap.
func (l *Lease) TryAcquire(ctx context.Context) (bool, error) {
	if l.conn != nil {
		return false, fmt.Errorf("lease for instance %d is already held", l.instanceID)
	}

	// Bounded, because db.Conn blocks until the pool has room and each held
	// lease permanently occupies one connection.
	ctx, cancel := context.WithTimeout(ctx, acquireTimeout)
	defer cancel()

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

// Confirm re-checks that this node still holds the lock.
//
// This is the whole safety property. Postgres cannot tell us when ownership
// goes away, so the owner has to ask — and it has to ask about the lock, not
// about the socket. A node that cannot confirm must stop announcing
// immediately, or a partition produces two nodes both convinced they are the
// owner.
func (l *Lease) Confirm(ctx context.Context) error {
	if l.conn == nil {
		return fmt.Errorf("lease for instance %d is not held", l.instanceID)
	}

	ctx, cancel := context.WithTimeout(ctx, confirmTimeout)
	defer cancel()

	var held bool
	if err := l.conn.QueryRowContext(ctx, ownershipQuery,
		LockClass, l.instanceID, advisoryObjSubID,
	).Scan(&held); err != nil {
		return fmt.Errorf("confirm advisory lock: %w", err)
	}
	if !held {
		return fmt.Errorf("advisory lock for instance %d is no longer held", l.instanceID)
	}
	return nil
}

// Release gives up ownership.
//
// It explicitly unlocks rather than relying on the connection closing.
// (*sql.Conn).Close returns the connection to the pool — it does not end the
// Postgres session — so a session-scoped lock would survive, wedge every future
// acquire until ConnMaxLifetime recycled it, and meanwhile be handed to
// ordinary web traffic still holding a connector lock.
//
// If the unlock cannot be proven, the connection is poisoned so the pool
// discards it instead of reusing it. Killing the session is the one thing that
// definitely releases the lock.
func (l *Lease) Release() {
	if l.conn == nil {
		return
	}
	conn := l.conn
	l.conn = nil

	ctx, cancel := context.WithTimeout(context.Background(), releaseTimeout)
	defer cancel()

	var released bool
	if err := conn.QueryRowContext(ctx,
		`SELECT pg_advisory_unlock($1, $2)`, LockClass, l.instanceID,
	).Scan(&released); err != nil {
		slog.Warn("leader: failed to release advisory lock, discarding the connection",
			"instance_id", l.instanceID, "error", err)
	}

	if !released {
		if err := conn.Raw(func(any) error { return driver.ErrBadConn }); err != nil &&
			err != driver.ErrBadConn {
			slog.Warn("leader: failed to poison the lease connection",
				"instance_id", l.instanceID, "error", err)
		}
	}

	_ = conn.Close()
}

// Held reports whether this lease currently believes it owns the instance.
func (l *Lease) Held() bool {
	return l.conn != nil
}
