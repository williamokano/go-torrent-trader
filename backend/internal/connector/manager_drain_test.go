package connector

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/cluster"
)

// fakeDrainer records the owner's drain passes.
type fakeDrainer struct {
	mu    sync.Mutex
	calls []int64
	// next is what each pass returns, in order; after the last, (false, 0).
	next []drainResult
	// during, when set, runs inside each pass.
	during func(ctx context.Context)
}

type drainResult struct {
	again bool
	after time.Duration
}

func (d *fakeDrainer) DrainOwned(ctx context.Context, instanceID int64) (bool, time.Duration) {
	d.mu.Lock()
	d.calls = append(d.calls, instanceID)
	during := d.during
	var res drainResult
	if len(d.next) > 0 {
		res, d.next = d.next[0], d.next[1:]
	}
	d.mu.Unlock()
	if during != nil {
		during(ctx)
	}
	return res.again, res.after
}

func (d *fakeDrainer) count() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.calls)
}

// fakeNode is an in-memory cluster.Node.
type fakeNode struct {
	mu        sync.Mutex
	handlers  map[string][]cluster.Handler
	resync    []func()
	published []published
}

type published struct {
	topic   string
	payload json.RawMessage
}

func newFakeNode() *fakeNode { return &fakeNode{handlers: map[string][]cluster.Handler{}} }

func (n *fakeNode) Publish(topic string, v any) {
	payload, _ := json.Marshal(v)
	n.mu.Lock()
	defer n.mu.Unlock()
	n.published = append(n.published, published{topic: topic, payload: payload})
}

func (n *fakeNode) Subscribe(topic string, h cluster.Handler) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.handlers[topic] = append(n.handlers[topic], h)
}

func (n *fakeNode) OnResync(f func()) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.resync = append(n.resync, f)
}

// receive delivers a message as if another node had published it.
func (n *fakeNode) receive(topic string, v any) {
	payload, _ := json.Marshal(v)
	n.mu.Lock()
	handlers := append([]cluster.Handler(nil), n.handlers[topic]...)
	n.mu.Unlock()
	for _, h := range handlers {
		h(payload)
	}
}

func (n *fakeNode) fireResync() {
	n.mu.Lock()
	callbacks := append([]func(){}, n.resync...)
	n.mu.Unlock()
	for _, f := range callbacks {
		f()
	}
}

func (n *fakeNode) publishedOn(topic string) []json.RawMessage {
	n.mu.Lock()
	defer n.mu.Unlock()
	var out []json.RawMessage
	for _, p := range n.published {
		if p.topic == topic {
			out = append(out, p.payload)
		}
	}
	return out
}

// newDrainingHarness is a manager with a drainer and a cluster node, and a
// drain poll long enough that every pass a test sees was asked for.
func newDrainingHarness(t *testing.T, acquire bool) (*managerHarness, *fakeDrainer, *fakeNode) {
	t.Helper()
	h := newManagerHarness(t, acquire)
	drainer := &fakeDrainer{}
	node := newFakeNode()
	h.manager.SetDrainer(drainer)
	h.manager.UseCluster(node)
	h.manager.drainPoll = time.Hour
	h.manager.connectPoll = 5 * time.Millisecond
	return h, drainer, node
}

func TestOwnerDrainsOnTakingOwnership(t *testing.T) {
	h, drainer, _ := newDrainingHarness(t, true)
	h.repo.set(ircInstance(1, "Libera", `{}`))

	stop := h.run(t)
	defer stop()

	// Whatever queued up while nobody owned it goes out at once.
	waitFor(t, "a drain pass", func() bool { return drainer.count() == 1 })
}

func TestStandbyNeverDrains(t *testing.T) {
	h, drainer, node := newDrainingHarness(t, false)
	h.repo.set(ircInstance(1, "Libera", `{}`))

	stop := h.run(t)
	defer stop()

	waitFor(t, "not_owner status", func() bool { return h.stateOf(1) == StateNotOwner })
	h.manager.RequestDrain(1)
	node.receive(topicDrain, drainRequest{InstanceID: 1})
	time.Sleep(20 * time.Millisecond)
	if drainer.count() != 0 {
		t.Fatal("a node that does not own the instance drained it")
	}
	// It cannot deliver, so it passes the request on to whoever can.
	if got := node.publishedOn(topicDrain); len(got) != 1 || string(got[0]) != `{"instance_id":1}` {
		t.Fatalf("published %s, want one drain request for instance 1", got)
	}
}

func TestOwnerDrainsWhenAsked(t *testing.T) {
	h, drainer, node := newDrainingHarness(t, true)
	h.repo.set(ircInstance(1, "Libera", `{}`))

	stop := h.run(t)
	defer stop()
	waitFor(t, "the first pass", func() bool { return drainer.count() == 1 })

	h.manager.RequestDrain(1)
	waitFor(t, "a pass for the local request", func() bool { return drainer.count() == 2 })
	if got := node.publishedOn(topicDrain); len(got) != 0 {
		t.Fatalf("the owner broadcast its own drain request: %s", got)
	}

	node.receive(topicDrain, drainRequest{InstanceID: 1})
	waitFor(t, "a pass for another node's request", func() bool { return drainer.count() == 3 })
}

func TestOwnerRunsAgainWhenThePassAsks(t *testing.T) {
	h, drainer, _ := newDrainingHarness(t, true)
	drainer.next = []drainResult{{again: true, after: 0}, {again: true, after: 10 * time.Millisecond}}
	h.repo.set(ircInstance(1, "Libera", `{}`))

	stop := h.run(t)
	defer stop()

	waitFor(t, "three passes", func() bool { return drainer.count() == 3 })
}

func TestOwnerPollsAsABackstop(t *testing.T) {
	h, drainer, _ := newDrainingHarness(t, true)
	h.manager.drainPoll = 5 * time.Millisecond
	h.repo.set(ircInstance(1, "Libera", `{}`))

	stop := h.run(t)
	defer stop()

	waitFor(t, "repeated passes with nobody asking", func() bool { return drainer.count() >= 3 })
}

// A pass against a client still registering would reschedule every row as
// not ready; the loop waits for the connection instead.
func TestOwnerWaitsForTheConnectionBeforeDraining(t *testing.T) {
	h, drainer, _ := newDrainingHarness(t, true)
	h.conn.mu.Lock()
	h.conn.connected = false
	h.conn.mu.Unlock()
	h.repo.set(ircInstance(1, "Libera", `{}`))

	stop := h.run(t)
	defer stop()

	waitFor(t, "the client to start", func() bool { return h.conn.startCount() == 1 })
	time.Sleep(30 * time.Millisecond)
	if drainer.count() != 0 {
		t.Fatal("drained before the client was connected")
	}

	h.conn.mu.Lock()
	h.conn.connected = true
	h.conn.mu.Unlock()
	waitFor(t, "a pass once connected", func() bool { return drainer.count() == 1 })
}

// The lease is what makes this node the only one delivering, so a pass still
// running when it is released would overlap the next owner's.
func TestOwnerFinishesDrainingBeforeReleasingTheLease(t *testing.T) {
	h, drainer, _ := newDrainingHarness(t, true)
	h.manager.confirmInterval = 10 * time.Millisecond
	heldAtEnd := make(chan bool, 1)
	drainer.during = func(ctx context.Context) {
		<-ctx.Done()
		// Give a missing join the chance to release first.
		time.Sleep(10 * time.Millisecond)
		h.lease.mu.Lock()
		held := h.lease.held
		h.lease.mu.Unlock()
		select {
		case heldAtEnd <- held:
		default:
		}
	}
	h.lease.confirmFn = func() error { return context.DeadlineExceeded }
	h.repo.set(ircInstance(1, "Libera", `{}`))

	stop := h.run(t)
	defer stop()

	select {
	case held := <-heldAtEnd:
		if !held {
			t.Fatal("the lease was released while a drain pass was still running")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the drain pass never ended")
	}
}

func TestOwnerDrainSurvivesAPanickingPass(t *testing.T) {
	h, drainer, _ := newDrainingHarness(t, true)
	var once sync.Once
	drainer.during = func(context.Context) { once.Do(func() { panic("delivery bug") }) }
	h.repo.set(ircInstance(1, "Libera", `{}`))

	stop := h.run(t)
	defer stop()

	waitFor(t, "the panicking pass", func() bool { return drainer.count() == 1 })
	h.manager.RequestDrain(1)
	waitFor(t, "a pass after the panic", func() bool { return drainer.count() == 2 })
}

func TestConfigChangeReachesEveryNode(t *testing.T) {
	h, _, node := newDrainingHarness(t, true)

	stop := h.run(t)
	defer stop()

	h.manager.HandleConfigChanged()
	if got := node.publishedOn(topicReconcile); len(got) != 1 {
		t.Fatalf("published %d reconcile requests, want 1", len(got))
	}

	// Another node's edit: without the broadcast this would wait for the
	// one-minute tick.
	h.repo.set(ircInstance(1, "Libera", `{}`))
	node.receive(topicReconcile, struct{}{})
	waitFor(t, "the edited instance to start", func() bool { return h.conn.startCount() == 1 })
	if got := node.publishedOn(topicReconcile); len(got) != 1 {
		t.Fatal("applying another node's reconcile broadcast it again")
	}
}

func TestResyncReconcilesAndWakesDrains(t *testing.T) {
	h, drainer, node := newDrainingHarness(t, true)
	h.repo.set(ircInstance(1, "Libera", `{}`))

	stop := h.run(t)
	defer stop()
	waitFor(t, "the first pass", func() bool { return drainer.count() == 1 })

	h.repo.set(ircInstance(1, "Libera", `{}`), ircInstance(2, "OFTC", `{}`))
	node.fireResync()

	waitFor(t, "a pass for the woken drain", func() bool { return drainer.count() >= 2 })
	// The harness shares one exclusive lease between instances, so the second
	// cannot be owned while the first is; being supervised at all is what shows
	// the reconcile ran.
	waitFor(t, "the instance missed in the gap to be picked up", func() bool {
		_, known := h.manager.Status()[2]
		return known
	})
}

func TestManagerWithoutADrainerDoesNotDrain(t *testing.T) {
	h := newManagerHarness(t, true)
	h.repo.set(ircInstance(1, "Libera", `{}`))

	stop := h.run(t)
	defer stop()

	waitFor(t, "the client to start", func() bool { return h.conn.startCount() == 1 })
	// Nothing to assert on beyond not panicking, and a request being harmless.
	h.manager.RequestDrain(1)
}
