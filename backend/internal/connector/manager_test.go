package connector

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

// --- fakes ---

type fakeConnectorRepo struct {
	mu        sync.Mutex
	instances []model.NotificationConnector
	err       error
}

func (r *fakeConnectorRepo) set(instances ...model.NotificationConnector) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.instances = instances
}

func (r *fakeConnectorRepo) Create(context.Context, *model.NotificationConnector) error { return nil }
func (r *fakeConnectorRepo) GetByID(context.Context, int64) (*model.NotificationConnector, error) {
	return nil, errors.New("not implemented")
}
func (r *fakeConnectorRepo) List(context.Context) ([]model.NotificationConnector, error) {
	return nil, nil
}
func (r *fakeConnectorRepo) ListEnabled(context.Context) ([]model.NotificationConnector, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return nil, r.err
	}
	out := make([]model.NotificationConnector, len(r.instances))
	copy(out, r.instances)
	return out, nil
}
func (r *fakeConnectorRepo) Update(context.Context, *model.NotificationConnector) error { return nil }
func (r *fakeConnectorRepo) Delete(context.Context, int64) error                        { return nil }
func (r *fakeConnectorRepo) CountByKind(context.Context, string) (int64, error)         { return 0, nil }

// fakeLease models ownership without a database.
type fakeLease struct {
	mu        sync.Mutex
	acquire   bool
	held      bool
	acquired  int
	released  int
	confirmed int
	confirmFn func() error
}

// TryAcquire models real exclusivity: while the lease is held nobody else can
// take it. Without that, a test could not tell a clean hand-off from two
// supervisors running at once.
func (l *fakeLease) TryAcquire(context.Context) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.acquired++
	if !l.acquire || l.held {
		return false, nil
	}
	l.held = true
	return true, nil
}

func (l *fakeLease) Confirm(context.Context) error {
	l.mu.Lock()
	fn := l.confirmFn
	l.confirmed++
	l.mu.Unlock()
	if fn != nil {
		return fn()
	}
	return nil
}

func (l *fakeLease) Release() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.released++
	l.held = false
}

func (l *fakeLease) counts() (acquired, released, confirmed int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.acquired, l.released, l.confirmed
}

// fakePersistent records its lifecycle and blocks in Start like a real client.
type fakePersistent struct {
	mu        sync.Mutex
	starts    int
	ctxDone   int
	connected bool
	panicOn   bool
	startErr  error
	started   chan struct{}
}

func newFakePersistent() *fakePersistent {
	return &fakePersistent{connected: true, started: make(chan struct{}, 8)}
}

func (f *fakePersistent) Kind() string                         { return "irc" }
func (f *fakePersistent) Singleton() bool                      { return false }
func (f *fakePersistent) SecretFields() []string               { return nil }
func (f *fakePersistent) Coalescable() bool                    { return true }
func (f *fakePersistent) ValidateConfig(json.RawMessage) error { return nil }
func (f *fakePersistent) Deliver(context.Context, Instance, Announcement) error {
	return nil
}

func (f *fakePersistent) Connected(int64) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.connected
}

func (f *fakePersistent) Start(ctx context.Context, _ Instance) error {
	f.mu.Lock()
	f.starts++
	shouldPanic := f.panicOn
	err := f.startErr
	f.mu.Unlock()

	select {
	case f.started <- struct{}{}:
	default:
	}

	if shouldPanic {
		panic("client blew up")
	}
	if err != nil {
		return err
	}

	<-ctx.Done()
	f.mu.Lock()
	f.ctxDone++
	f.mu.Unlock()
	return ctx.Err()
}

func (f *fakePersistent) startCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.starts
}

func (f *fakePersistent) observedCancel() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ctxDone
}

// --- harness ---

func ircInstance(id int64, name, config string) model.NotificationConnector {
	return model.NotificationConnector{
		ID: id, Kind: "irc", Name: name, Enabled: true,
		Config: json.RawMessage(config), Filters: json.RawMessage(`{}`),
	}
}

type managerHarness struct {
	manager *Manager
	repo    *fakeConnectorRepo
	conn    *fakePersistent
	lease   *fakeLease
}

func newManagerHarness(t *testing.T, acquire bool) *managerHarness {
	t.Helper()

	conn := newFakePersistent()
	registry := NewRegistry()
	registry.Register(conn)

	repo := &fakeConnectorRepo{}
	lease := &fakeLease{acquire: acquire}

	return &managerHarness{
		manager: NewManager(repo, registry, func(int64) Lease { return lease }),
		repo:    repo,
		conn:    conn,
		lease:   lease,
	}
}

// run starts the manager and returns a stop func that waits for it to finish.
func (h *managerHarness) run(t *testing.T) func() {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		h.manager.Run(ctx)
	}()
	return func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("manager did not shut down")
		}
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		if cond() {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for %s", what)
		case <-time.After(time.Millisecond):
		}
	}
}

func (h *managerHarness) stateOf(id int64) string {
	return h.manager.Status()[id].State
}

// --- tests ---

func TestManagerStartsAnOwnedInstance(t *testing.T) {
	h := newManagerHarness(t, true)
	h.repo.set(ircInstance(1, "Libera", `{"server":"irc.test"}`))

	stop := h.run(t)
	defer stop()

	waitFor(t, "the client to start", func() bool { return h.conn.startCount() == 1 })
	waitFor(t, "connected status", func() bool { return h.stateOf(1) == StateConnected })
}

// A standby node must not connect; it just waits for the owner to go away.
func TestManagerDoesNotStartWhenAnotherNodeOwnsTheInstance(t *testing.T) {
	h := newManagerHarness(t, false)
	h.repo.set(ircInstance(1, "Libera", `{"server":"irc.test"}`))

	stop := h.run(t)
	defer stop()

	waitFor(t, "not_owner status", func() bool { return h.stateOf(1) == StateNotOwner })
	if h.conn.startCount() != 0 {
		t.Fatal("a node that does not own the lock must not connect")
	}
}

// The mandatory safety property: the lock lives on a database session, and
// Postgres cannot tell us when that session dies. A node that cannot re-prove
// ownership has to stop announcing at once, or a partition leaves two nodes
// both convinced they are the owner.
func TestManagerStopsTheClientWhenOwnershipCannotBeConfirmed(t *testing.T) {
	h := newManagerHarness(t, true)
	// Confirm fails from the very first tick.
	h.lease.confirmFn = func() error { return errors.New("connection reset") }
	h.repo.set(ircInstance(1, "Libera", `{"server":"irc.test"}`))

	// Shorten the watchdog so the test does not wait ten seconds. Safe to set
	// here: Run has not started, so nothing is reading it yet.
	h.manager.confirmInterval = 10 * time.Millisecond

	stop := h.run(t)
	defer stop()

	waitFor(t, "the client to start", func() bool { return h.conn.startCount() >= 1 })
	// The client's context must be cancelled — that is what makes it disconnect.
	waitFor(t, "the client's context to be cancelled", func() bool { return h.conn.observedCancel() >= 1 })
	// And the lease must be given back so another node can take over.
	waitFor(t, "the lease to be released", func() bool {
		_, released, _ := h.lease.counts()
		return released >= 1
	})
}

func TestManagerRestartsOnConfigChange(t *testing.T) {
	h := newManagerHarness(t, true)
	h.repo.set(ircInstance(1, "Libera", `{"server":"irc.test"}`))

	stop := h.run(t)
	defer stop()

	waitFor(t, "the first start", func() bool { return h.conn.startCount() == 1 })

	h.repo.set(ircInstance(1, "Libera", `{"server":"other.test"}`))
	h.manager.HandleConfigChanged()

	// The lease models real exclusivity, so a second start can only happen once
	// the first supervisor has released — i.e. this also proves the hand-off is
	// clean rather than two clients briefly coexisting.
	waitFor(t, "a restart with the new config", func() bool { return h.conn.startCount() == 2 })
	if h.conn.observedCancel() < 1 {
		t.Fatal("the old client must be stopped before the new one starts")
	}
}

// An unchanged instance must not be disconnected and reconnected every time
// something else in the panel is saved.
func TestManagerLeavesAnUnchangedInstanceAlone(t *testing.T) {
	h := newManagerHarness(t, true)
	h.repo.set(ircInstance(1, "Libera", `{"server":"irc.test"}`))

	stop := h.run(t)
	defer stop()

	waitFor(t, "the first start", func() bool { return h.conn.startCount() == 1 })

	for i := 0; i < 3; i++ {
		h.manager.HandleConfigChanged()
	}
	time.Sleep(50 * time.Millisecond)

	if got := h.conn.startCount(); got != 1 {
		t.Fatalf("started %d times, want 1: an unchanged config must not reconnect", got)
	}
}

func TestManagerStopsADisabledInstance(t *testing.T) {
	h := newManagerHarness(t, true)
	h.repo.set(ircInstance(1, "Libera", `{"server":"irc.test"}`))

	stop := h.run(t)
	defer stop()

	waitFor(t, "the client to start", func() bool { return h.conn.startCount() == 1 })

	h.repo.set() // disabled instances are simply absent from ListEnabled
	h.manager.HandleConfigChanged()

	waitFor(t, "stopped status", func() bool { return h.stateOf(1) == StateStopped })
	waitFor(t, "the client's context to be cancelled", func() bool { return h.conn.observedCancel() >= 1 })
}

// Third-party client code runs inside Start; a panic there must not take the
// process down.
func TestManagerRecoversFromAPanickingClient(t *testing.T) {
	h := newManagerHarness(t, true)
	h.conn.panicOn = true
	h.repo.set(ircInstance(1, "Libera", `{"server":"irc.test"}`))

	stop := h.run(t)
	defer stop()

	waitFor(t, "error status after the panic", func() bool {
		status := h.manager.Status()[1]
		return status.State == StateError && status.LastError != ""
	})
}

func TestManagerReportsStartFailure(t *testing.T) {
	h := newManagerHarness(t, true)
	h.conn.startErr = errors.New("connection refused")
	h.repo.set(ircInstance(1, "Libera", `{"server":"irc.test"}`))

	stop := h.run(t)
	defer stop()

	waitFor(t, "error status", func() bool {
		status := h.manager.Status()[1]
		return status.State == StateError && status.LastError == "connection refused"
	})
}

// A live-but-dropped connection should read as reconnecting, not as healthy.
func TestManagerStatusReflectsTheLiveSocket(t *testing.T) {
	h := newManagerHarness(t, true)
	h.repo.set(ircInstance(1, "Libera", `{"server":"irc.test"}`))

	stop := h.run(t)
	defer stop()

	waitFor(t, "connected status", func() bool { return h.stateOf(1) == StateConnected })

	h.conn.mu.Lock()
	h.conn.connected = false
	h.conn.mu.Unlock()

	if got := h.stateOf(1); got != StateReconnecting {
		t.Fatalf("state = %q, want %q once the socket is down", got, StateReconnecting)
	}
}

// Only persistent kinds are managed; a webhook has no connection to own.
func TestManagerIgnoresNonPersistentKinds(t *testing.T) {
	h := newManagerHarness(t, true)
	h.repo.set(model.NotificationConnector{
		ID: 2, Kind: "webhook", Name: "Hook", Enabled: true, Config: json.RawMessage(`{}`),
	})

	stop := h.run(t)
	defer stop()

	time.Sleep(50 * time.Millisecond)
	if _, ok := h.manager.Status()[2]; ok {
		t.Fatal("a non-persistent kind should not be managed at all")
	}
}

func TestManagerSurvivesRepositoryFailure(t *testing.T) {
	h := newManagerHarness(t, true)
	h.repo.err = errors.New("database down")

	stop := h.run(t)
	defer stop()

	time.Sleep(50 * time.Millisecond)
	if h.conn.startCount() != 0 {
		t.Fatal("nothing should start when the instance list could not be read")
	}
}

func TestManagerShutdownStopsEverything(t *testing.T) {
	h := newManagerHarness(t, true)
	h.repo.set(ircInstance(1, "Libera", `{"server":"irc.test"}`))

	stop := h.run(t)
	waitFor(t, "the client to start", func() bool { return h.conn.startCount() == 1 })

	stop()

	if h.conn.observedCancel() < 1 {
		t.Fatal("shutdown must cancel the client's context so it can send QUIT")
	}
	if got := h.stateOf(1); got != StateStopped {
		t.Fatalf("state = %q after shutdown, want %q", got, StateStopped)
	}
}
