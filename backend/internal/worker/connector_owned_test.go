package worker

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/connector"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

// persistentStub is a stubConnector that also holds a connection, as IRC does.
type persistentStub struct{ *stubConnector }

func (persistentStub) Start(ctx context.Context, _ connector.Instance) error {
	<-ctx.Done()
	return nil
}

// recordingOwner records drain requests handed to the owning node.
type recordingOwner struct {
	mu       sync.Mutex
	requests []int64
}

func (o *recordingOwner) RequestDrain(instanceID int64) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.requests = append(o.requests, instanceID)
}

// newPersistentHarness is newDrainHarness with instance 1 as a persistent kind.
func newPersistentHarness(t *testing.T, config string) (*drainHarness, *recordingOwner) {
	t.Helper()
	h := newDrainHarness(t, config)
	h.conn.kind = "irc"
	h.instance.Kind = "irc"
	registry := connector.NewRegistry()
	registry.Register(persistentStub{h.conn})
	h.deps.ConnectorRegistry = registry
	owner := &recordingOwner{}
	h.deps.OwnerDrains = owner
	return h, owner
}

// A queued drain can land on any node, and only the owner can deliver to a
// persistent destination (#294): it must hand off instead of trying.
func TestQueuedDrainHandsPersistentInstanceToOwner(t *testing.T) {
	h, owner := newPersistentHarness(t, `{}`)
	row := h.deliveries.add(1, announcementN(1), 0)

	h.run(t)

	if len(h.conn.delivered) != 0 {
		t.Fatal("a queued drain delivered to a persistent connector")
	}
	if len(owner.requests) != 1 || owner.requests[0] != 1 {
		t.Fatalf("drain requests = %v, want one for instance 1", owner.requests)
	}
	got := h.deliveries.rows[row.ID]
	if got.Status != model.DeliveryPending || got.Attempts != 0 || got.NextAttemptAt != nil {
		t.Fatalf("handing off touched the row: %+v", got)
	}
	h.assertNotSnoozed(t)
	if len(h.enqueuer.calls) != 0 {
		t.Fatalf("handing off queued %+v", h.enqueuer.calls)
	}
}

// With no node able to deliver, nothing else would ever end a row: the queue
// keeps the not-ready age limit.
func TestQueuedDrainAgesOutRowsNobodyDelivered(t *testing.T) {
	h, _ := newPersistentHarness(t, `{}`)
	old := h.deliveries.add(1, announcementN(1), 0)
	old.CreatedAt = time.Now().Add(-connectorNotReadyMaxAge - time.Minute)
	fresh := h.deliveries.add(1, announcementN(2), 0)

	h.run(t)

	if got := h.deliveries.rows[old.ID]; got.Status != model.DeliveryFailed || got.Attempts != 1 {
		t.Fatalf("old row = %+v, want it failed with an attempt counted", got)
	}
	if h.deliveries.rows[old.ID].LastError == nil {
		t.Fatal("an aged-out row must say why")
	}
	if got := h.deliveries.rows[fresh.ID]; got.Status != model.DeliveryPending {
		t.Fatalf("fresh row = %+v, want it left for the owner", got)
	}
}

// The disabled-instance grace still applies to persistent kinds, and a
// disabled instance has no owner to hand anything to.
func TestQueuedDrainStillFailsStaleRowsOfADisabledPersistentInstance(t *testing.T) {
	h, owner := newPersistentHarness(t, `{}`)
	h.instance.Enabled = false
	row := h.deliveries.add(1, announcementN(1), 0)
	row.CreatedAt = time.Now().Add(-connectorDisabledGrace - time.Minute)

	h.run(t)

	if h.deliveries.rows[row.ID].Status != model.DeliveryFailed {
		t.Fatal("a stale row of a disabled instance was not failed")
	}
	if len(owner.requests) != 0 {
		t.Fatalf("asked an owner to drain a disabled instance: %v", owner.requests)
	}
}

func TestOwnedDrainDeliversPersistentInstance(t *testing.T) {
	h, owner := newPersistentHarness(t, `{}`)
	row := h.deliveries.add(1, announcementN(1), 0)

	again, _ := NewOwnedDrainer(h.deps).DrainOwned(context.Background(), 1)

	if again {
		t.Error("nothing is left, but the drain asked to run again")
	}
	if len(h.conn.delivered) != 1 {
		t.Fatalf("delivered %d, want 1", len(h.conn.delivered))
	}
	if h.deliveries.rows[row.ID].Status != model.DeliverySent {
		t.Fatal("the delivered row was not marked sent")
	}
	if len(owner.requests) != 0 {
		t.Fatal("the owner asked itself to drain")
	}
}

// On the owner a retry is the loop's next wake-up, not a queued job that
// would only come back as a request to the same loop.
func TestOwnedDrainReturnsRetryInsteadOfQueueingIt(t *testing.T) {
	h, _ := newPersistentHarness(t, `{}`)
	h.conn.deliverFn = func(connector.Announcement) error { return context.DeadlineExceeded }
	h.deliveries.add(1, announcementN(1), 0)

	again, after := NewOwnedDrainer(h.deps).DrainOwned(context.Background(), 1)

	if !again || after != connectorBaseBackoff {
		t.Fatalf("DrainOwned = (%v, %v), want (true, %v)", again, after, connectorBaseBackoff)
	}
	if len(h.enqueuer.calls) != 0 {
		t.Fatalf("the owner queued %+v", h.enqueuer.calls)
	}
}

func TestOwnedDrainWaitsOutTheRateWindow(t *testing.T) {
	h, _ := newPersistentHarness(t, `{"rate_per_min":5}`)
	h.deliveries.sentInWindow = 5
	h.deliveries.add(1, announcementN(1), 0)

	again, after := NewOwnedDrainer(h.deps).DrainOwned(context.Background(), 1)

	if !again || after != connectorRateWindow {
		t.Fatalf("DrainOwned = (%v, %v), want (true, %v)", again, after, connectorRateWindow)
	}
}

// ctxStrictDeliveries refuses writes on a cancelled context, as the real
// repository's queries would.
type ctxStrictDeliveries struct{ *fakeDeliveryRepo }

func (r ctxStrictDeliveries) MarkSent(ctx context.Context, id int64, status string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return r.fakeDeliveryRepo.MarkSent(ctx, id, status)
}

// Ownership ends routinely — an edit restarts the client — and a line already
// sent must still be recorded as sent, or the next owner sends it again. The
// pass stops before claiming anything else.
func TestOwnedDrainFinishesTheRowItIsOnWhenOwnershipEnds(t *testing.T) {
	h, _ := newPersistentHarness(t, `{}`)
	h.deps.ConnectorDeliveryRepo = ctxStrictDeliveries{h.deliveries}
	ctx, cancel := context.WithCancel(context.Background())
	h.conn.deliverFn = func(connector.Announcement) error {
		cancel()
		return nil
	}
	first := h.deliveries.add(1, announcementN(1), 0)
	second := h.deliveries.add(1, announcementN(2), 0)

	NewOwnedDrainer(h.deps).DrainOwned(ctx, 1)

	if len(h.conn.delivered) != 1 {
		t.Fatalf("delivered %d after ownership ended, want only the row in flight", len(h.conn.delivered))
	}
	if h.deliveries.rows[first.ID].Status != model.DeliverySent {
		t.Fatal("the row delivered as ownership ended was not marked sent")
	}
	if got := h.deliveries.rows[second.ID]; got.Status != model.DeliveryPending || got.NextAttemptAt != nil {
		t.Fatalf("the next row was claimed after ownership ended: %+v", got)
	}
}

func TestOwnedDrainWithoutWiringIsANoOp(t *testing.T) {
	again, after := NewOwnedDrainer(&WorkerDeps{}).DrainOwned(context.Background(), 1)
	if again || after != 0 {
		t.Fatalf("DrainOwned = (%v, %v), want (false, 0)", again, after)
	}
}

// An owner that has just reconnected may be sending an old row at the moment a
// queued drain elsewhere decides it is too old: the row it holds is not failed.
func TestQueuedDrainDoesNotAgeOutARowTheOwnerIsDelivering(t *testing.T) {
	h, _ := newPersistentHarness(t, `{}`)
	row := h.deliveries.add(1, announcementN(1), 0)
	row.CreatedAt = time.Now().Add(-connectorNotReadyMaxAge - time.Minute)
	h.deliveries.refuseClaims = true // the owner holds it

	h.run(t)

	if got := h.deliveries.rows[row.ID]; got.Status != model.DeliveryPending {
		t.Fatalf("row = %+v, want it left to the owner delivering it", got)
	}
}

// A row partway up its retry ladder belongs to the owner's retries; a queued
// drain on another node gives up on it only much later.
func TestQueuedDrainLeavesARetryingRowToItsLadder(t *testing.T) {
	h, _ := newPersistentHarness(t, `{}`)
	retrying := h.deliveries.add(1, announcementN(1), 2)
	retrying.CreatedAt = time.Now().Add(-connectorNotReadyMaxAge - time.Minute)
	abandoned := h.deliveries.add(1, announcementN(2), 2)
	abandoned.CreatedAt = time.Now().Add(-connectorUnownedMaxAge - time.Minute)

	h.run(t)

	if got := h.deliveries.rows[retrying.ID]; got.Status != model.DeliveryPending {
		t.Fatalf("retrying row = %+v, want it left for the owner", got)
	}
	if got := h.deliveries.rows[abandoned.ID]; got.Status != model.DeliveryFailed || got.Attempts != 3 {
		t.Fatalf("abandoned row = %+v, want it failed", got)
	}
}
