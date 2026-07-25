package connector

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// Manager states, as reported to the admin panel.
const (
	// StateAcquiring: trying to become the owner of this instance.
	StateAcquiring = "acquiring"
	// StateNotOwner: another node owns it. The normal steady state on a standby.
	StateNotOwner = "not_owner"
	// StateConnecting: we own it and the client is coming up.
	StateConnecting = "connecting"
	// StateConnected: we own it and the connection is live.
	StateConnected = "connected"
	// StateReconnecting: we own it but the connection is currently down; the
	// client's own retry loop is working on it.
	StateReconnecting = "reconnecting"
	// StateStopped: deliberately not running (disabled, or shutting down).
	StateStopped = "stopped"
	// StateError: the client exited unexpectedly; a retry is scheduled.
	StateError = "error"
)

const (
	// acquireBackoffMin/Max bound how often a standby node re-checks whether the
	// owner has gone away.
	acquireBackoffMin = 5 * time.Second
	acquireBackoffMax = 60 * time.Second
	// stopGrace bounds how long a stopping instance is waited for, so one wedged
	// client cannot stall reconciliation for the others.
	stopGrace = 5 * time.Second
	// defaultConfirmInterval is how often the owner re-proves it still holds the
	// lock. The window between the lock actually being lost and us noticing is
	// the window in which two nodes could both be announcing, so it stays short.
	defaultConfirmInterval = 10 * time.Second
)

// ManagerStatus is one instance's lifecycle state on this node.
type ManagerStatus struct {
	State     string    `json:"state"`
	Since     time.Time `json:"since"`
	LastError string    `json:"last_error,omitempty"`
}

// Lease is the ownership primitive a persistent instance needs. connector/leader
// provides the Postgres advisory-lock implementation; the interface exists so
// the manager's loop can be tested without a database.
type Lease interface {
	TryAcquire(ctx context.Context) (bool, error)
	// Confirm re-proves ownership. Any error means ownership must be treated as
	// lost immediately.
	Confirm(ctx context.Context) error
	Release()
}

// LeaseFactory builds a lease for one instance.
type LeaseFactory func(instanceID int64) Lease

// connectionReporter is implemented by connectors that can say whether their
// live connection is currently up, which turns a coarse "running" into the
// connected/reconnecting distinction an admin actually wants to see.
type connectionReporter interface {
	Connected(instanceID int64) bool
}

// Manager owns the lifecycle of persistent connector instances: which ones
// should be running, which node runs them, and what to report about them.
type Manager struct {
	repo     repository.ConnectorRepository
	registry *Registry
	newLease LeaseFactory
	// confirmInterval is a field rather than a constant so a test can exercise
	// the ownership watchdog without waiting ten seconds. It is only ever set
	// before Run starts, so the supervise goroutines read it safely.
	confirmInterval time.Duration

	mu      sync.Mutex
	running map[int64]*managed
	status  map[int64]ManagerStatus
	// kinds remembers each managed instance's kind so Status can find its
	// connector without going back to the database on every poll.
	kinds map[int64]string

	// reconcile carries a nudge from the event bus to the run loop. Buffered
	// depth 1: several changes arriving together only need one reconcile.
	reconcile chan struct{}
}

type managed struct {
	cancel context.CancelFunc
	done   chan struct{}
	// fingerprint detects an edit, so an unchanged instance is not needlessly
	// disconnected and reconnected on every reconcile.
	fingerprint string
}

// NewManager creates a Manager.
func NewManager(repo repository.ConnectorRepository, registry *Registry, newLease LeaseFactory) *Manager {
	return &Manager{
		repo:            repo,
		registry:        registry,
		newLease:        newLease,
		confirmInterval: defaultConfirmInterval,
		running:         make(map[int64]*managed),
		status:          make(map[int64]ManagerStatus),
		kinds:           make(map[int64]string),
		reconcile:       make(chan struct{}, 1),
	}
}

// Run reconciles until ctx is cancelled, then stops every instance.
//
// The periodic tick is a backstop, not the mechanism: config changes arrive as
// events and reconcile immediately.
func (m *Manager) Run(ctx context.Context) {
	m.reconcileOnce(ctx)

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			m.stopAll()
			return
		case <-m.reconcile:
			m.reconcileOnce(ctx)
		case <-ticker.C:
			m.reconcileOnce(ctx)
		}
	}
}

// HandleConfigChanged asks for a reconcile. It never blocks: a pending nudge
// already covers whatever just changed.
func (m *Manager) HandleConfigChanged() {
	select {
	case m.reconcile <- struct{}{}:
	default:
	}
}

// Status returns this node's view of every managed instance.
//
// It is deliberately node-local: a standby correctly reports not_owner, which is
// the honest answer to "is this node announcing?" rather than a guess about the
// cluster.
func (m *Manager) Status() map[int64]ManagerStatus {
	m.mu.Lock()
	defer m.mu.Unlock()

	out := make(map[int64]ManagerStatus, len(m.status))
	for id, status := range m.status {
		// Upgrade the coarse "connected" to reflect the live socket, so a
		// dropped IRC connection shows as reconnecting rather than healthy.
		if status.State == StateConnected {
			if reporter, ok := m.reporterFor(id); ok && !reporter.Connected(id) {
				status.State = StateReconnecting
			}
		}
		out[id] = status
	}
	return out
}

func (m *Manager) reporterFor(instanceID int64) (connectionReporter, bool) {
	kind, ok := m.kinds[instanceID]
	if !ok {
		return nil, false
	}
	impl, ok := m.registry.Get(kind)
	if !ok {
		return nil, false
	}
	reporter, ok := impl.(connectionReporter)
	return reporter, ok
}

// reconcileOnce brings the running set in line with what the database says
// should be running.
func (m *Manager) reconcileOnce(ctx context.Context) {
	listCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	instances, err := m.repo.ListEnabled(listCtx)
	if err != nil {
		slog.Error("connector manager: failed to list enabled connectors", "error", err)
		return
	}

	wanted := make(map[int64]model.NotificationConnector)
	for _, inst := range instances {
		impl, ok := m.registry.Get(inst.Kind)
		if !ok {
			continue
		}
		if _, persistent := impl.(PersistentConnector); !persistent {
			continue
		}
		wanted[inst.ID] = inst
	}

	m.mu.Lock()
	// Stop anything that should no longer run, or whose config changed.
	stopping := make([]*managed, 0)
	for id, running := range m.running {
		inst, keep := wanted[id]
		if keep && running.fingerprint == fingerprint(inst) {
			delete(wanted, id) // already running with this exact config
			continue
		}
		running.cancel()
		stopping = append(stopping, running)
		delete(m.running, id)
		if !keep {
			delete(m.kinds, id)
			m.status[id] = ManagerStatus{State: StateStopped, Since: time.Now()}
		}
	}
	toStart := make([]model.NotificationConnector, 0, len(wanted))
	for _, inst := range wanted {
		toStart = append(toStart, inst)
	}
	m.mu.Unlock()

	// Wait for the old supervisors before starting the replacements. They still
	// hold their leases, so starting first would just make the new supervisor
	// fail to acquire and sit in not_owner for a backoff — an edit would appear
	// to take five seconds and briefly claim another node owned the instance.
	for _, entry := range stopping {
		waitForStop(entry)
	}

	for _, inst := range toStart {
		m.start(ctx, inst)
	}
}

// waitForStop waits for a supervisor to finish, bounded so a wedged client
// cannot stall reconciliation for every other instance.
func waitForStop(entry *managed) {
	select {
	case <-entry.done:
	case <-time.After(stopGrace):
		slog.Warn("connector manager: instance did not stop within the grace period")
	}
}

// start launches the supervise loop for one instance.
func (m *Manager) start(ctx context.Context, inst model.NotificationConnector) {
	instCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})

	m.mu.Lock()
	m.running[inst.ID] = &managed{cancel: cancel, done: done, fingerprint: fingerprint(inst)}
	m.kinds[inst.ID] = inst.Kind
	m.mu.Unlock()

	m.setStatus(inst.ID, StateAcquiring, "")

	go func() {
		defer close(done)
		m.supervise(instCtx, inst)
	}()
}

// supervise runs the acquire → own → run → lose cycle for one instance until
// its context is cancelled.
func (m *Manager) supervise(ctx context.Context, inst model.NotificationConnector) {
	backoff := acquireBackoffMin

	for ctx.Err() == nil {
		lease := m.newLease(inst.ID)

		acquired, err := lease.TryAcquire(ctx)
		if err != nil {
			m.setStatus(inst.ID, StateError, fmt.Sprintf("acquiring ownership: %v", err))
		} else if !acquired {
			m.setStatus(inst.ID, StateNotOwner, "")
		}
		if err != nil || !acquired {
			if !sleepCtx(ctx, backoff) {
				return
			}
			backoff = nextBackoff(backoff)
			continue
		}

		backoff = acquireBackoffMin
		m.runOwned(ctx, inst, lease)
		lease.Release()

		if !sleepCtx(ctx, acquireBackoffMin) {
			return
		}
	}
}

// runOwned runs the connector while ownership holds, and stops it the moment it
// cannot be confirmed.
func (m *Manager) runOwned(ctx context.Context, inst model.NotificationConnector, lease Lease) {
	impl, ok := m.registry.Get(inst.Kind)
	if !ok {
		m.setStatus(inst.ID, StateError, fmt.Sprintf("unknown connector kind %q", inst.Kind))
		return
	}
	persistent, ok := impl.(PersistentConnector)
	if !ok {
		m.setStatus(inst.ID, StateError, fmt.Sprintf("connector kind %q is not persistent", inst.Kind))
		return
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// The ownership watchdog. Losing the lock without noticing is the one
	// failure that produces two announcing nodes, so an unconfirmable lease
	// stops the client immediately rather than waiting for anything else.
	go func() {
		ticker := time.NewTicker(m.confirmInterval)
		defer ticker.Stop()
		for {
			select {
			case <-runCtx.Done():
				return
			case <-ticker.C:
				if err := lease.Confirm(runCtx); err != nil {
					slog.Warn("connector manager: lost ownership, stopping client",
						"instance_id", inst.ID, "error", err)
					m.setStatus(inst.ID, StateError, fmt.Sprintf("ownership lost: %v", err))
					cancel()
					return
				}
			}
		}
	}()

	m.setStatus(inst.ID, StateConnecting, "")

	err := func() (err error) {
		// Third-party client code runs here; a panic must not take the process
		// down with it.
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("connector %s panicked: %v", inst.Kind, r)
			}
		}()
		instance := Instance{ID: inst.ID, Name: inst.Name, Config: inst.Config}
		// Report connected optimistically: Start blocks for the connection's
		// whole life, and Status() consults the live socket for the real answer.
		m.setStatus(inst.ID, StateConnected, "")
		return persistent.Start(runCtx, instance)
	}()

	switch {
	case ctx.Err() != nil:
		m.setStatus(inst.ID, StateStopped, "")
	case err != nil:
		m.setStatus(inst.ID, StateError, err.Error())
	default:
		m.setStatus(inst.ID, StateStopped, "")
	}
}

func (m *Manager) stopAll() {
	m.mu.Lock()
	running := make([]*managed, 0, len(m.running))
	for id, entry := range m.running {
		running = append(running, entry)
		delete(m.running, id)
		m.status[id] = ManagerStatus{State: StateStopped, Since: time.Now()}
	}
	m.mu.Unlock()

	for _, entry := range running {
		entry.cancel()
	}
	// Wait briefly so IRC clients get to send QUIT and release their locks
	// before the process exits.
	for _, entry := range running {
		waitForStop(entry)
	}
}

func (m *Manager) setStatus(instanceID int64, state, lastError string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	previous := m.status[instanceID]
	// Keep Since meaning "in this state since", not "last touched".
	since := previous.Since
	if previous.State != state || since.IsZero() {
		since = time.Now()
	}
	m.status[instanceID] = ManagerStatus{State: state, Since: since, LastError: lastError}
}

// fingerprint detects an edit worth restarting for. The config and name are all
// that reach the connector; anything else can change without a reconnect.
func fingerprint(inst model.NotificationConnector) string {
	return inst.Name + "\x00" + string(inst.Config)
}

func nextBackoff(current time.Duration) time.Duration {
	next := current * 2
	if next > acquireBackoffMax {
		return acquireBackoffMax
	}
	return next
}

// sleepCtx waits for d, reporting false if the context ended first.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
