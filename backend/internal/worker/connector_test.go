package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/williamokano/go-torrent-trader/backend/internal/connector"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

// --- mocks ---

type fakeConnectorRepo struct {
	rows map[int64]*model.NotificationConnector
	err  error
}

func (r *fakeConnectorRepo) Create(context.Context, *model.NotificationConnector) error { return nil }
func (r *fakeConnectorRepo) GetByID(_ context.Context, id int64) (*model.NotificationConnector, error) {
	if r.err != nil {
		return nil, r.err
	}
	c, ok := r.rows[id]
	if !ok {
		return nil, sql.ErrNoRows
	}
	return c, nil
}
func (r *fakeConnectorRepo) List(context.Context) ([]model.NotificationConnector, error) {
	return nil, nil
}
func (r *fakeConnectorRepo) ListEnabled(context.Context) ([]model.NotificationConnector, error) {
	return nil, nil
}
func (r *fakeConnectorRepo) Update(context.Context, *model.NotificationConnector) error { return nil }
func (r *fakeConnectorRepo) Delete(context.Context, int64) error                        { return nil }
func (r *fakeConnectorRepo) CountByKind(context.Context, string) (int64, error)         { return 0, nil }

type fakeDeliveryRepo struct {
	mu     sync.Mutex
	rows   map[int64]*model.ConnectorDelivery
	nextID int64
	// sentInWindow is what CountSentSince reports, so a test can pretend the
	// instance has already spent part of its budget this minute.
	sentInWindow int64
	listErr      error
	claimErr     error
	// refuseClaims simulates another drain having already taken every row.
	refuseClaims bool
	deleted      []time.Time
}

func newFakeDeliveryRepo() *fakeDeliveryRepo {
	return &fakeDeliveryRepo{rows: map[int64]*model.ConnectorDelivery{}, nextID: 1}
}

func (r *fakeDeliveryRepo) add(instanceID int64, a connector.Announcement, attempts int) *model.ConnectorDelivery {
	payload, _ := json.Marshal(a)
	d := &model.ConnectorDelivery{
		ID:         r.nextID,
		InstanceID: instanceID,
		EventKey:   connector.EventKey(a),
		EventType:  a.Event,
		Payload:    payload,
		Status:     model.DeliveryPending,
		Attempts:   attempts,
		CreatedAt:  time.Now(),
	}
	r.nextID++
	r.rows[d.ID] = d
	return d
}

func (r *fakeDeliveryRepo) InsertPending(context.Context, *model.ConnectorDelivery) (bool, error) {
	return true, nil
}

func (r *fakeDeliveryRepo) ListDue(_ context.Context, instanceID int64, now time.Time, limit int) ([]model.ConnectorDelivery, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.listErr != nil {
		return nil, r.listErr
	}
	var out []model.ConnectorDelivery
	for id := int64(1); id < r.nextID; id++ {
		d, ok := r.rows[id]
		if !ok || d.InstanceID != instanceID || d.Status != model.DeliveryPending {
			continue
		}
		if d.NextAttemptAt != nil && d.NextAttemptAt.After(now) {
			continue
		}
		out = append(out, *d)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (r *fakeDeliveryRepo) ClaimForDelivery(_ context.Context, id int64, leaseUntil, now time.Time) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.claimErr != nil {
		return false, r.claimErr
	}
	d, ok := r.rows[id]
	if !ok || d.Status != model.DeliveryPending {
		return false, nil
	}
	if d.NextAttemptAt != nil && d.NextAttemptAt.After(now) {
		return false, nil
	}
	if r.refuseClaims {
		return false, nil
	}
	d.NextAttemptAt = &leaseUntil
	return true, nil
}

func (r *fakeDeliveryRepo) CountSentSince(context.Context, int64, time.Time) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.sentInWindow, nil
}

func (r *fakeDeliveryRepo) MarkSent(_ context.Context, id int64, status string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	d, ok := r.rows[id]
	// Mirrors the real repository: only a pending row can be closed, so a slow
	// drain cannot resurrect one another drain already dead-lettered.
	if !ok || d.Status != model.DeliveryPending {
		return sql.ErrNoRows
	}
	d.Status = status
	d.NextAttemptAt = nil
	return nil
}

func (r *fakeDeliveryRepo) MarkFailedAttempt(_ context.Context, id int64, attempts int, lastError string, next *time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	d, ok := r.rows[id]
	if !ok {
		return sql.ErrNoRows
	}
	d.Attempts = attempts
	msg := lastError
	d.LastError = &msg
	d.NextAttemptAt = next
	if next == nil {
		d.Status = model.DeliveryFailed
	} else {
		d.Status = model.DeliveryPending
	}
	return nil
}

func (r *fakeDeliveryRepo) ListByInstance(context.Context, int64, int, int) ([]model.ConnectorDelivery, int64, error) {
	return nil, 0, nil
}
func (r *fakeDeliveryRepo) LatestStatusByInstance(context.Context) (map[int64]model.ConnectorDelivery, error) {
	return nil, nil
}
func (r *fakeDeliveryRepo) InstancesWithDue(context.Context, time.Time) ([]int64, error) {
	return nil, nil
}
func (r *fakeDeliveryRepo) DeleteOld(_ context.Context, cutoff time.Time) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.deleted = append(r.deleted, cutoff)
	return 0, nil
}

type drainCall struct {
	instanceID int64
	delay      time.Duration
}

type fakeEnqueuer struct {
	mu        sync.Mutex
	calls     []drainCall
	followUps []drainCall
}

func (e *fakeEnqueuer) EnqueueConnectorDrain(_ context.Context, instanceID int64, delay time.Duration) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.calls = append(e.calls, drainCall{instanceID: instanceID, delay: delay})
	return nil
}

// EnqueueConnectorDrainFollowUp is what a drain uses to schedule its own next
// run; the tests assert through `calls` either way, but recording both keeps the
// distinction visible.
func (e *fakeEnqueuer) EnqueueConnectorDrainFollowUp(_ context.Context, instanceID int64, delay time.Duration) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	call := drainCall{instanceID: instanceID, delay: delay}
	e.calls = append(e.calls, call)
	e.followUps = append(e.followUps, call)
	return nil
}

type stubConnector struct {
	mu         sync.Mutex
	kind       string
	secrets    []string
	delivered  []connector.Announcement
	deliverFn  func(connector.Announcement) error
	noCoalesce bool
}

func (s *stubConnector) Kind() string                         { return s.kind }
func (s *stubConnector) Singleton() bool                      { return false }
func (s *stubConnector) SecretFields() []string               { return s.secrets }
func (s *stubConnector) Coalescable() bool                    { return !s.noCoalesce }
func (s *stubConnector) ValidateConfig(json.RawMessage) error { return nil }
func (s *stubConnector) Deliver(_ context.Context, _ connector.Instance, a connector.Announcement) error {
	s.mu.Lock()
	s.delivered = append(s.delivered, a)
	s.mu.Unlock()
	if s.deliverFn != nil {
		return s.deliverFn(a)
	}
	return nil
}

// --- harness ---

type drainHarness struct {
	deps       *WorkerDeps
	deliveries *fakeDeliveryRepo
	enqueuer   *fakeEnqueuer
	conn       *stubConnector
	instance   *model.NotificationConnector

	// snoozed is set when the last run ended by snoozing, to how long.
	snoozed *time.Duration
}

func newDrainHarness(t *testing.T, config string) *drainHarness {
	t.Helper()

	conn := &stubConnector{kind: "webhook", secrets: []string{"hmac_secret"}}
	registry := connector.NewRegistry()
	registry.Register(conn)

	inst := &model.NotificationConnector{
		ID: 1, Kind: "webhook", Name: "Hook", Enabled: true,
		Config: json.RawMessage(config), Filters: json.RawMessage(`{}`),
	}
	deliveries := newFakeDeliveryRepo()
	enqueuer := &fakeEnqueuer{}

	return &drainHarness{
		deps: &WorkerDeps{
			ConnectorRegistry:     registry,
			ConnectorRepo:         &fakeConnectorRepo{rows: map[int64]*model.NotificationConnector{1: inst}},
			ConnectorDeliveryRepo: deliveries,
			ConnectorEnqueuer:     enqueuer,
		},
		deliveries: deliveries,
		enqueuer:   enqueuer,
		conn:       conn,
		instance:   inst,
	}
}

// run drives the drain the way River would: through the registered worker,
// for instance 1. A snooze is the drain asking to run again, so it is recorded
// rather than treated as a failure; any other error is one.
func (h *drainHarness) run(t *testing.T) {
	t.Helper()
	h.snoozed = nil
	job := &river.Job[ConnectorDrainArgs]{Args: ConnectorDrainArgs{InstanceID: 1}}
	err := registeredWorker[ConnectorDrainArgs](t, h.deps).Work(context.Background(), job)
	var snooze *rivertype.JobSnoozeError
	switch {
	case err == nil:
	case errors.As(err, &snooze):
		d := snooze.Duration
		h.snoozed = &d
	default:
		t.Fatalf("drain returned an error: %v", err)
	}
}

// assertSnoozed fails unless the last run snoozed for exactly want.
func (h *drainHarness) assertSnoozed(t *testing.T, want time.Duration) {
	t.Helper()
	if h.snoozed == nil {
		t.Fatalf("drain ended; want it to snooze for %v", want)
	}
	if *h.snoozed != want {
		t.Fatalf("drain snoozed for %v, want %v", *h.snoozed, want)
	}
}

// assertNotSnoozed fails if the last run snoozed.
func (h *drainHarness) assertNotSnoozed(t *testing.T) {
	t.Helper()
	if h.snoozed != nil {
		t.Fatalf("drain snoozed for %v; want it to end", *h.snoozed)
	}
}

func announcementN(n int) connector.Announcement {
	return connector.Announcement{
		Event:     connector.EventTorrentPublished,
		TorrentID: int64(n),
		Name:      fmt.Sprintf("Release.%d", n),
		Category:  "Movies",
		Uploader:  "alice",
		URL:       fmt.Sprintf("https://tracker.test/torrent/%d", n),
	}
}

// --- task construction ---

func TestConnectorDrainArgsAreMinimal(t *testing.T) {
	raw, err := json.Marshal(ConnectorDrainArgs{InstanceID: 7})
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	// Everything else lives in connector_deliveries, so a job queued before a
	// deploy still means the same thing after it.
	if len(payload) != 1 || payload["instance_id"].(float64) != 7 {
		t.Fatalf("payload = %v, want only instance_id", payload)
	}
}

// --- happy path ---

func TestDrainDeliversDueRowsInOrder(t *testing.T) {
	h := newDrainHarness(t, `{}`)
	first := h.deliveries.add(1, announcementN(1), 0)
	second := h.deliveries.add(1, announcementN(2), 0)

	h.run(t)

	if len(h.conn.delivered) != 2 {
		t.Fatalf("delivered %d announcements, want 2", len(h.conn.delivered))
	}
	if h.conn.delivered[0].TorrentID != 1 || h.conn.delivered[1].TorrentID != 2 {
		t.Fatalf("delivered out of order: %d then %d",
			h.conn.delivered[0].TorrentID, h.conn.delivered[1].TorrentID)
	}
	for _, row := range []*model.ConnectorDelivery{first, second} {
		if h.deliveries.rows[row.ID].Status != model.DeliverySent {
			t.Fatalf("row %d status = %q, want sent", row.ID, h.deliveries.rows[row.ID].Status)
		}
	}
}

// The delivery key is handed to the connector so a receiver can deduplicate a
// retried delivery on its own side.
func TestDrainPassesDeliveryKeyToConnector(t *testing.T) {
	h := newDrainHarness(t, `{}`)
	h.deliveries.add(1, announcementN(1), 0)

	h.run(t)

	if got := h.conn.delivered[0].DeliveryKey; got != "torrent.published:1" {
		t.Fatalf("delivery key = %q, want torrent.published:1", got)
	}
}

func TestDrainWithNothingDueDoesNothing(t *testing.T) {
	h := newDrainHarness(t, `{}`)

	h.run(t)

	if len(h.conn.delivered) != 0 {
		t.Fatal("nothing was due, so nothing should have been delivered")
	}
	if len(h.enqueuer.calls) != 0 {
		t.Fatal("nothing was due, so no follow-up drain should have been queued")
	}
}

// --- failure, backoff and dead-lettering ---

func TestDrainFailureSchedulesRetryWithoutLosingTheRow(t *testing.T) {
	h := newDrainHarness(t, `{}`)
	h.conn.deliverFn = func(connector.Announcement) error { return errors.New("endpoint down") }
	row := h.deliveries.add(1, announcementN(1), 0)

	before := time.Now()
	h.run(t)

	stored := h.deliveries.rows[row.ID]
	if stored.Attempts != 1 {
		t.Fatalf("attempts = %d, want 1", stored.Attempts)
	}
	if stored.Status != model.DeliveryPending {
		t.Fatalf("status = %q, want it to stay pending for the retry", stored.Status)
	}
	if stored.NextAttemptAt == nil {
		t.Fatal("next_attempt_at must be set so the row comes back")
	}
	gap := stored.NextAttemptAt.Sub(before)
	if gap < 29*time.Second || gap > 35*time.Second {
		t.Fatalf("first retry in %v, want about 30s", gap)
	}

	if len(h.enqueuer.calls) != 1 {
		t.Fatalf("queued %d follow-up drains, want 1", len(h.enqueuer.calls))
	}
	if h.enqueuer.calls[0].delay < 29*time.Second {
		t.Fatalf("follow-up delay = %v, want it to match the backoff", h.enqueuer.calls[0].delay)
	}
}

func TestDrainBackoffDoublesAndIsCapped(t *testing.T) {
	cases := []struct {
		attempts int
		want     time.Duration
	}{
		{1, 30 * time.Second},
		{2, time.Minute},
		{3, 2 * time.Minute},
		{4, 4 * time.Minute},
		{9, connectorMaxBackoff},
		{40, connectorMaxBackoff}, // guards the shift from overflowing
	}
	for _, c := range cases {
		if got := connectorBackoff(c.attempts); got != c.want {
			t.Errorf("connectorBackoff(%d) = %v, want %v", c.attempts, got, c.want)
		}
	}
}

func TestDrainDeadLettersAfterMaxAttempts(t *testing.T) {
	h := newDrainHarness(t, `{}`)
	h.conn.deliverFn = func(connector.Announcement) error { return errors.New("endpoint down") }
	row := h.deliveries.add(1, announcementN(1), connectorMaxAttempts-1)

	h.run(t)

	stored := h.deliveries.rows[row.ID]
	if stored.Status != model.DeliveryFailed {
		t.Fatalf("status = %q, want failed after %d attempts", stored.Status, connectorMaxAttempts)
	}
	if stored.NextAttemptAt != nil {
		t.Fatal("a dead-lettered row must not be scheduled again")
	}
	if stored.Attempts != connectorMaxAttempts {
		t.Fatalf("attempts = %d, want %d", stored.Attempts, connectorMaxAttempts)
	}
}

// last_error is shown in the admin log, so it must never carry a credential.
func TestDrainRedactsSecretsFromLastError(t *testing.T) {
	h := newDrainHarness(t, `{"url":"https://example.test/hook","hmac_secret":"s3cr3t"}`)
	h.conn.deliverFn = func(connector.Announcement) error {
		return errors.New("rejected: signature computed with s3cr3t did not match")
	}
	row := h.deliveries.add(1, announcementN(1), connectorMaxAttempts-1)

	h.run(t)

	stored := h.deliveries.rows[row.ID]
	if stored.LastError == nil {
		t.Fatal("last_error not recorded")
	}
	if strings.Contains(*stored.LastError, "s3cr3t") {
		t.Fatalf("last_error leaked the secret: %s", *stored.LastError)
	}
	if !strings.Contains(*stored.LastError, connector.Redacted) {
		t.Fatalf("last_error = %q, want the secret replaced with a marker", *stored.LastError)
	}
}

// A panic in third-party client code must degrade to a failed attempt, not take
// the worker down with every other queued task.
func TestDrainRecoversFromPanickingConnector(t *testing.T) {
	h := newDrainHarness(t, `{}`)
	h.conn.deliverFn = func(connector.Announcement) error { panic("client blew up") }
	row := h.deliveries.add(1, announcementN(1), 0)

	h.run(t)

	stored := h.deliveries.rows[row.ID]
	if stored.Attempts != 1 {
		t.Fatalf("attempts = %d, want the panic treated as a failed attempt", stored.Attempts)
	}
	if stored.LastError == nil || !strings.Contains(*stored.LastError, "panicked") {
		t.Fatalf("last_error = %v, want it to name the panic", stored.LastError)
	}
}

func TestDrainDeadLettersUnreadablePayload(t *testing.T) {
	h := newDrainHarness(t, `{}`)
	row := h.deliveries.add(1, announcementN(1), 0)
	row.Payload = json.RawMessage(`{not json`)

	h.run(t)

	stored := h.deliveries.rows[row.ID]
	if stored.Status != model.DeliveryFailed {
		t.Fatalf("status = %q, want failed: a payload that cannot be read never will be", stored.Status)
	}
	if len(h.conn.delivered) != 0 {
		t.Fatal("nothing should have been delivered for an unreadable payload")
	}
}

// --- instance state ---

// Toggling an instance off to edit it is routine, so recently queued rows wait
// rather than being thrown away.
func TestDrainKeepsRecentRowsForBrieflyDisabledInstance(t *testing.T) {
	h := newDrainHarness(t, `{}`)
	h.instance.Enabled = false
	row := h.deliveries.add(1, announcementN(1), 0)

	h.run(t)

	if len(h.conn.delivered) != 0 {
		t.Fatal("a disabled instance must not deliver")
	}
	if h.deliveries.rows[row.ID].Status != model.DeliveryPending {
		t.Fatalf("status = %q, want it left pending through a brief disable",
			h.deliveries.rows[row.ID].Status)
	}
}

// A long-disabled instance does eventually give up, so re-enabling it does not
// flush a flood of week-old announcements.
func TestDrainFailsStaleRowsForDisabledInstance(t *testing.T) {
	h := newDrainHarness(t, `{}`)
	h.instance.Enabled = false
	row := h.deliveries.add(1, announcementN(1), 0)
	row.CreatedAt = time.Now().Add(-2 * connectorDisabledGrace)

	h.run(t)

	stored := h.deliveries.rows[row.ID]
	if stored.Status != model.DeliveryFailed {
		t.Fatalf("status = %q, want failed", stored.Status)
	}
	if stored.LastError == nil || !strings.Contains(*stored.LastError, "disabled") {
		t.Fatalf("last_error = %v, want it to say the instance is disabled", stored.LastError)
	}
}

func TestDrainFailsRowsForUnknownKind(t *testing.T) {
	h := newDrainHarness(t, `{}`)
	h.instance.Kind = "carrier-pigeon"
	row := h.deliveries.add(1, announcementN(1), 0)

	h.run(t)

	stored := h.deliveries.rows[row.ID]
	if stored.Status != model.DeliveryFailed {
		t.Fatalf("status = %q, want failed", stored.Status)
	}
	if stored.LastError == nil || !strings.Contains(*stored.LastError, "unknown connector kind") {
		t.Fatalf("last_error = %v, want it to name the unknown kind", stored.LastError)
	}
}

// A deleted instance takes its rows with it via ON DELETE CASCADE, so there is
// nothing to do and nothing to complain about.
func TestDrainForDeletedInstanceIsANoOp(t *testing.T) {
	h := newDrainHarness(t, `{}`)
	h.deliveries.add(1, announcementN(1), 0)
	h.deps.ConnectorRepo = &fakeConnectorRepo{rows: map[int64]*model.NotificationConnector{}}

	h.run(t)

	if len(h.conn.delivered) != 0 {
		t.Fatal("nothing should be delivered for a deleted instance")
	}
}

// --- rate limiting and coalescing ---

// The plan's worked example: a budget of 3 against 6 due rows spends two
// messages individually and the third on a summary covering the rest, so every
// row ends up accounted for rather than dropped.
func TestDrainCoalescesWhenBudgetIsShort(t *testing.T) {
	h := newDrainHarness(t, `{"rate_per_min":3}`)
	var rows []*model.ConnectorDelivery
	for i := 1; i <= 6; i++ {
		rows = append(rows, h.deliveries.add(1, announcementN(i), 0))
	}

	h.run(t)

	if len(h.conn.delivered) != 3 {
		t.Fatalf("sent %d messages, want 3 (the whole budget)", len(h.conn.delivered))
	}
	summary := h.conn.delivered[2]
	if summary.Coalesced != 4 {
		t.Fatalf("summary Coalesced = %d, want 4", summary.Coalesced)
	}
	for i, row := range rows {
		want := model.DeliverySent
		if i >= 2 {
			want = model.DeliveryCoalesced
		}
		if got := h.deliveries.rows[row.ID].Status; got != want {
			t.Fatalf("row %d status = %q, want %q", i+1, got, want)
		}
	}
}

// A machine-read destination cannot use a "+N more" summary — a bot has no way
// to recover the torrents it stands for — so the whole budget goes on individual
// deliveries and the remainder waits for the next window.
func TestDrainDefersRatherThanCoalescingForMachineConsumers(t *testing.T) {
	h := newDrainHarness(t, `{"rate_per_min":3}`)
	h.conn.noCoalesce = true
	var rows []*model.ConnectorDelivery
	for i := 1; i <= 6; i++ {
		rows = append(rows, h.deliveries.add(1, announcementN(i), 0))
	}

	h.run(t)

	if len(h.conn.delivered) != 3 {
		t.Fatalf("sent %d messages, want the full budget of 3", len(h.conn.delivered))
	}
	for _, a := range h.conn.delivered {
		if a.Coalesced != 0 {
			t.Fatal("a non-coalescable connector must never receive a summary")
		}
	}
	for i, row := range rows {
		stored := h.deliveries.rows[row.ID]
		if i < 3 {
			if stored.Status != model.DeliverySent {
				t.Fatalf("row %d status = %q, want sent", i+1, stored.Status)
			}
			continue
		}
		// Deferred, not dropped: still pending, no attempt burned.
		if stored.Status != model.DeliveryPending || stored.Attempts != 0 {
			t.Fatalf("row %d = %+v, want it left pending for the next window", i+1, stored)
		}
	}
	// The same job waits out the window; nothing new is queued.
	h.assertSnoozed(t, connectorRateWindow)
	if len(h.enqueuer.calls) != 0 {
		t.Fatalf("queued %+v, want the drain to snooze rather than enqueue a copy", h.enqueuer.calls)
	}
}

// Budget of exactly 1 with several due rows: everything folds into one summary.
func TestDrainCoalescesEverythingWhenBudgetIsOne(t *testing.T) {
	h := newDrainHarness(t, `{"rate_per_min":1}`)
	for i := 1; i <= 3; i++ {
		h.deliveries.add(1, announcementN(i), 0)
	}

	h.run(t)

	if len(h.conn.delivered) != 1 {
		t.Fatalf("sent %d messages, want 1", len(h.conn.delivered))
	}
	if h.conn.delivered[0].Coalesced != 3 {
		t.Fatalf("Coalesced = %d, want 3", h.conn.delivered[0].Coalesced)
	}
}

// Already-spent budget is counted from the delivery log, so a burst spread over
// two drains still respects the per-minute limit.
func TestDrainRespectsBudgetAlreadySpentThisWindow(t *testing.T) {
	h := newDrainHarness(t, `{"rate_per_min":5}`)
	h.deliveries.sentInWindow = 5
	row := h.deliveries.add(1, announcementN(1), 0)

	h.run(t)

	if len(h.conn.delivered) != 0 {
		t.Fatal("the budget was already spent, so nothing should have been sent")
	}
	if h.deliveries.rows[row.ID].Status != model.DeliveryPending {
		t.Fatal("a rate-limited row must stay pending: nothing failed")
	}
	if h.deliveries.rows[row.ID].Attempts != 0 {
		t.Fatal("rate limiting must not burn a retry attempt")
	}
	// The same job waits out the window; nothing new is queued.
	h.assertSnoozed(t, connectorRateWindow)
	if len(h.enqueuer.calls) != 0 {
		t.Fatalf("queued %+v, want the drain to snooze rather than enqueue a copy", h.enqueuer.calls)
	}
}

// When the summary itself fails, each row it covered retries on its own.
func TestDrainCoalescedFailureRetriesEveryRow(t *testing.T) {
	h := newDrainHarness(t, `{"rate_per_min":1}`)
	h.conn.deliverFn = func(connector.Announcement) error { return errors.New("endpoint down") }
	var rows []*model.ConnectorDelivery
	for i := 1; i <= 3; i++ {
		rows = append(rows, h.deliveries.add(1, announcementN(i), 0))
	}

	h.run(t)

	for _, row := range rows {
		stored := h.deliveries.rows[row.ID]
		if stored.Attempts != 1 || stored.Status != model.DeliveryPending {
			t.Fatalf("row %d = %+v, want one failed attempt and still pending", row.ID, stored)
		}
	}
}

func TestDrainSnoozesImmediatelyWhenBatchIsFull(t *testing.T) {
	h := newDrainHarness(t, fmt.Sprintf(`{"rate_per_min":%d}`, connectorDrainBatch*2))
	for i := 1; i <= connectorDrainBatch; i++ {
		h.deliveries.add(1, announcementN(i), 0)
	}

	h.run(t)

	h.assertSnoozed(t, 0)
	if len(h.enqueuer.calls) != 0 {
		t.Fatalf("queued %+v, want the rest of the backlog handled by snoozing this drain", h.enqueuer.calls)
	}
}

// A full batch snoozes this drain, but a retry is a separate job. Before River
// the two shared one follow-up slot and the batch won, so a failure's retry
// waited for the maintenance sweep whenever the next run found only
// future-dated rows.
func TestDrainQueuesRetryEvenWhenBatchIsFull(t *testing.T) {
	h := newDrainHarness(t, fmt.Sprintf(`{"rate_per_min":%d}`, connectorDrainBatch*2))
	h.conn.deliverFn = func(a connector.Announcement) error {
		if a.TorrentID == 1 {
			return errors.New("one bad row")
		}
		return nil
	}
	for i := 1; i <= connectorDrainBatch; i++ {
		h.deliveries.add(1, announcementN(i), 0)
	}

	h.run(t)

	h.assertSnoozed(t, 0)
	if len(h.enqueuer.followUps) != 1 || h.enqueuer.followUps[0].delay < 29*time.Second {
		t.Fatalf("retries = %+v, want one queued for the failed row's backoff", h.enqueuer.followUps)
	}
}

// A dispatch that lands while a drain is running is skipped as a duplicate of
// that drain (River's uniqueness always covers running jobs), and the drain has
// already listed its rows. It must notice the newcomer on its way out rather
// than leave it for the maintenance sweep.
func TestDrainRunsAgainForRowsDispatchedWhileItRan(t *testing.T) {
	h := newDrainHarness(t, `{"rate_per_min":100}`)
	h.deliveries.add(1, announcementN(1), 0)
	dispatched := false
	h.conn.deliverFn = func(connector.Announcement) error {
		if !dispatched {
			dispatched = true
			h.deliveries.add(1, announcementN(2), 0)
		}
		return nil
	}

	h.run(t)

	h.assertSnoozed(t, connectorDrainRecheckDelay)
}

// The recheck looks only past the rows this drain listed. A row it already saw
// and could not claim is not news, and counting it would have the drain
// rescheduling itself every second for as long as the claim kept failing.
func TestDrainDoesNotRecheckForRowsItAlreadySaw(t *testing.T) {
	h := newDrainHarness(t, `{"rate_per_min":100}`)
	h.deliveries.add(1, announcementN(1), 0)
	h.deliveries.claimErr = errors.New("claim failed")

	h.run(t)

	h.assertNotSnoozed(t)
}

func TestDrainEndsWhenNothingIsLeft(t *testing.T) {
	h := newDrainHarness(t, `{"rate_per_min":100}`)
	h.deliveries.add(1, announcementN(1), 0)

	h.run(t)

	h.assertNotSnoozed(t)
	if len(h.enqueuer.calls) != 0 {
		t.Fatalf("queued %+v after delivering everything, want nothing", h.enqueuer.calls)
	}
}

// Shutdown cancels a drain that is still running. It must hand itself back
// rather than end: ended, its rows wait for the sweep, and a process that exits
// first leaves the job "running", which blocks every dispatch for the instance.
func TestDrainSnoozesWhenCancelled(t *testing.T) {
	h := newDrainHarness(t, `{"rate_per_min":100}`)
	h.deliveries.add(1, announcementN(1), 0)
	ctx, cancel := context.WithCancel(context.Background())
	h.conn.deliverFn = func(connector.Announcement) error {
		cancel()
		return nil
	}

	job := &river.Job[ConnectorDrainArgs]{Args: ConnectorDrainArgs{InstanceID: 1}}
	err := registeredWorker[ConnectorDrainArgs](t, h.deps).Work(ctx, job)
	var snooze *rivertype.JobSnoozeError
	if !errors.As(err, &snooze) || snooze.Duration != 0 {
		t.Fatalf("cancelled drain returned %v, want an immediate snooze", err)
	}
}

// fakeInserter records what an enqueuer asks River for.
type fakeInserter struct {
	args      []river.JobArgs
	opts      []*river.InsertOpts
	duplicate bool
	err       error
}

func (f *fakeInserter) Insert(_ context.Context, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	f.args = append(f.args, args)
	f.opts = append(f.opts, opts)
	if f.err != nil {
		return nil, f.err
	}
	return &rivertype.JobInsertResult{Job: &rivertype.JobRow{}, UniqueSkippedAsDuplicate: f.duplicate}, nil
}

// The two enqueue paths differ only in whether River may collapse duplicates,
// and getting that backwards is quiet. A collapsing retry would be swallowed by
// whatever drain is already queued or running for the instance; a dispatch that
// did not collapse would turn a burst of 25 approvals into 25 drains.
func TestConnectorEnqueuerCollapsesDispatchesButNotRetries(t *testing.T) {
	ins := &fakeInserter{}
	enqueuer := NewRiverConnectorEnqueuer(ins)
	ctx := context.Background()

	if err := enqueuer.EnqueueConnectorDrain(ctx, 1, 0); err != nil {
		t.Fatalf("dispatch enqueue: %v", err)
	}
	if err := enqueuer.EnqueueConnectorDrainFollowUp(ctx, 1, 30*time.Second); err != nil {
		t.Fatalf("retry enqueue: %v", err)
	}

	for i, args := range ins.args {
		if got, ok := args.(ConnectorDrainArgs); !ok || got.InstanceID != 1 {
			t.Fatalf("insert %d args = %#v, want ConnectorDrainArgs{InstanceID: 1}", i, args)
		}
	}

	dispatch := ins.opts[0].UniqueOpts
	if !dispatch.ByArgs {
		t.Error("dispatch must be unique by args, i.e. per instance")
	}
	if slices.Contains(dispatch.ByState, rivertype.JobStateCompleted) {
		t.Error("dispatch uniqueness covers completed jobs, so a finished drain would block the next one")
	}
	if !ins.opts[0].ScheduledAt.IsZero() {
		t.Error("a dispatch with no delay must run now")
	}

	retry := ins.opts[1]
	if retry.UniqueOpts.ByArgs || len(retry.UniqueOpts.ByState) != 0 {
		t.Errorf("retry carries uniqueness %+v; it must not be collapsed into another drain", retry.UniqueOpts)
	}
	if wait := time.Until(retry.ScheduledAt); wait < 29*time.Second || wait > 30*time.Second {
		t.Errorf("retry scheduled %v from now, want 30s", wait)
	}
}

// A collapsed duplicate is the collapse working, not a failure to schedule.
func TestConnectorEnqueuerReportsACollapsedDispatchAsSuccess(t *testing.T) {
	enqueuer := NewRiverConnectorEnqueuer(&fakeInserter{duplicate: true})
	if err := enqueuer.EnqueueConnectorDrain(context.Background(), 1, 0); err != nil {
		t.Fatalf("a collapsed duplicate must be reported as success, got: %v", err)
	}
}

// A dispatch follows a committed delivery row, and a registration email a
// committed user. A caller whose request is cancelled at that moment must not
// take the enqueue down with it.
func TestEnqueuersOutliveACancelledCaller(t *testing.T) {
	ins := &ctxCheckingInserter{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := NewRiverConnectorEnqueuer(ins).EnqueueConnectorDrain(ctx, 1, 0); err != nil {
		t.Fatalf("EnqueueConnectorDrain: %v", err)
	}
	if err := NewRiverEmailEnqueuer(ins).EnqueueSendEmail(ctx, "a@b.test", "Hi", "Body"); err != nil {
		t.Fatalf("EnqueueSendEmail: %v", err)
	}
	if ins.sawCancelled {
		t.Fatal("an insert ran on the caller's cancelled context")
	}
	if !ins.sawDeadline {
		t.Fatal("an insert ran with no deadline of its own")
	}
}

type ctxCheckingInserter struct {
	sawCancelled bool
	sawDeadline  bool
}

func (c *ctxCheckingInserter) Insert(ctx context.Context, _ river.JobArgs, _ *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	if ctx.Err() != nil {
		c.sawCancelled = true
	}
	if _, ok := ctx.Deadline(); ok {
		c.sawDeadline = true
	}
	return &rivertype.JobInsertResult{Job: &rivertype.JobRow{}}, nil
}

func TestConnectorEnqueuerReportsInsertFailure(t *testing.T) {
	enqueuer := NewRiverConnectorEnqueuer(&fakeInserter{err: errors.New("database down")})
	if err := enqueuer.EnqueueConnectorDrain(context.Background(), 1, 0); err == nil {
		t.Fatal("an insert failure must be reported")
	}
}

func TestEmailEnqueuerInsertsTheMessage(t *testing.T) {
	ins := &fakeInserter{}
	if err := NewRiverEmailEnqueuer(ins).EnqueueSendEmail(context.Background(), "a@b.test", "Hi", "Body"); err != nil {
		t.Fatalf("EnqueueSendEmail: %v", err)
	}
	want := SendEmailArgs{To: "a@b.test", Subject: "Hi", Body: "Body"}
	if len(ins.args) != 1 || ins.args[0] != want {
		t.Fatalf("inserted %#v, want %#v", ins.args, want)
	}

	if err := NewRiverEmailEnqueuer(&fakeInserter{err: errors.New("down")}).
		EnqueueSendEmail(context.Background(), "a@b.test", "Hi", "Body"); err == nil {
		t.Fatal("an insert failure must be reported")
	}
}

// --- guards ---

func TestDrainWithoutWiringIsANoOp(t *testing.T) {
	job := &river.Job[ConnectorDrainArgs]{Args: ConnectorDrainArgs{InstanceID: 1}}
	if err := NewConnectorDrainHandler(&WorkerDeps{})(context.Background(), job); err != nil {
		t.Fatalf("an unwired deployment must not error: %v", err)
	}
}

func TestDrainSurvivesListFailure(t *testing.T) {
	h := newDrainHarness(t, `{}`)
	h.deliveries.listErr = errors.New("database down")

	h.run(t)

	if len(h.conn.delivered) != 0 {
		t.Fatal("nothing should be delivered when the due list could not be read")
	}
}

func TestMinNonZero(t *testing.T) {
	cases := []struct{ a, b, want time.Duration }{
		{0, 0, 0},
		{0, time.Second, time.Second},
		{time.Second, 0, time.Second},
		{2 * time.Second, time.Second, time.Second},
		{time.Second, 2 * time.Second, time.Second},
	}
	for _, c := range cases {
		if got := minNonZero(c.a, c.b); got != c.want {
			t.Errorf("minNonZero(%v, %v) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

// --- concurrent drains ---

// Two drains for the same instance can genuinely overlap: a slow drain is still
// running when the next dispatch enqueues another. The per-row claim is what
// stops both from announcing the same torrent.
func TestDrainSkipsRowsClaimedByAnotherDrain(t *testing.T) {
	h := newDrainHarness(t, `{}`)
	row := h.deliveries.add(1, announcementN(1), 0)
	h.deliveries.refuseClaims = true

	h.run(t)

	if len(h.conn.delivered) != 0 {
		t.Fatal("a row another drain already claimed must not be delivered again")
	}
	if h.deliveries.rows[row.ID].Status != model.DeliveryPending {
		t.Fatal("losing the claim race must leave the row alone for the winner")
	}
}

func TestDrainClaimsBeforeDelivering(t *testing.T) {
	h := newDrainHarness(t, `{}`)
	row := h.deliveries.add(1, announcementN(1), 0)

	h.run(t)

	if len(h.conn.delivered) != 1 {
		t.Fatalf("delivered %d, want 1", len(h.conn.delivered))
	}
	// The successful delivery supersedes the lease.
	stored := h.deliveries.rows[row.ID]
	if stored.Status != model.DeliverySent || stored.NextAttemptAt != nil {
		t.Fatalf("row = %+v, want sent with the lease cleared", stored)
	}
}

// One unreadable row must not dead-letter the whole batch it happens to sit at
// the end of.
func TestDrainCoalescedSkipsUnreadableRepresentative(t *testing.T) {
	h := newDrainHarness(t, `{"rate_per_min":1}`)
	for i := 1; i <= 3; i++ {
		h.deliveries.add(1, announcementN(i), 0)
	}
	// The newest row is the natural representative; corrupt it.
	h.deliveries.rows[3].Payload = json.RawMessage(`{not json`)

	h.run(t)

	if len(h.conn.delivered) != 1 {
		t.Fatalf("delivered %d, want the summary to fall back to a readable row", len(h.conn.delivered))
	}
	if h.conn.delivered[0].Coalesced != 2 {
		t.Fatalf("Coalesced = %d, want 2: the summary must only speak for the rows it covers",
			h.conn.delivered[0].Coalesced)
	}
	if h.deliveries.rows[3].Status != model.DeliveryFailed {
		t.Fatalf("corrupt row status = %q, want failed", h.deliveries.rows[3].Status)
	}
	for _, id := range []int64{1, 2} {
		if h.deliveries.rows[id].Status != model.DeliveryCoalesced {
			t.Fatalf("row %d status = %q, want coalesced", id, h.deliveries.rows[id].Status)
		}
	}
}

func TestDrainCoalescedOnlySpeaksForRowsItClaimed(t *testing.T) {
	h := newDrainHarness(t, `{"rate_per_min":1}`)
	for i := 1; i <= 3; i++ {
		h.deliveries.add(1, announcementN(i), 0)
	}
	h.deliveries.refuseClaims = true

	h.run(t)

	if len(h.conn.delivered) != 0 {
		t.Fatal("with every row claimed elsewhere there is nothing to summarise")
	}
}

func TestDrainTreatsClaimFailureAsNotOwned(t *testing.T) {
	h := newDrainHarness(t, `{}`)
	h.deliveries.add(1, announcementN(1), 0)
	h.deliveries.claimErr = errors.New("database down")

	h.run(t)

	if len(h.conn.delivered) != 0 {
		t.Fatal("a failed claim must not be treated as ownership")
	}
}

// --- "not ready" (BE-10.2) ---

// A reconnecting IRC client is not a delivery failure. Counting it against the
// five-attempt budget would let a routine reconnect dead-letter a whole queue of
// perfectly good announcements.
func TestDrainNotReadyReschedulesWithoutBurningAnAttempt(t *testing.T) {
	h := newDrainHarness(t, `{}`)
	h.conn.deliverFn = func(connector.Announcement) error {
		return fmt.Errorf("%w: still connecting", connector.ErrNotReady)
	}
	row := h.deliveries.add(1, announcementN(1), 0)

	before := time.Now()
	h.run(t)

	stored := h.deliveries.rows[row.ID]
	if stored.Attempts != 0 {
		t.Fatalf("attempts = %d, want 0: not-ready must not consume the retry budget", stored.Attempts)
	}
	if stored.Status != model.DeliveryPending {
		t.Fatalf("status = %q, want it left pending", stored.Status)
	}
	if stored.NextAttemptAt == nil {
		t.Fatal("next_attempt_at must be set so the row comes back")
	}
	// Jittered, so assert the window rather than an exact value.
	gap := stored.NextAttemptAt.Sub(before)
	if gap < connectorNotReadyRetry || gap > connectorNotReadyRetry+connectorNotReadyJitter+time.Second {
		t.Fatalf("next attempt in %v, want between %v and %v",
			gap, connectorNotReadyRetry, connectorNotReadyRetry+connectorNotReadyJitter)
	}
	if len(h.enqueuer.calls) != 1 {
		t.Fatalf("queued %d follow-ups, want 1", len(h.enqueuer.calls))
	}
	if d := h.enqueuer.calls[0].delay; d < connectorNotReadyRetry || d > connectorNotReadyRetry+connectorNotReadyJitter {
		t.Fatalf("follow-up delay = %v, want it inside the jitter window", d)
	}
}

// The delivery log is the only record of why a row has been struggling, so a
// spell of "not ready" must not erase the real failure that preceded it.
func TestDrainNotReadyKeepsThePreviousFailureReason(t *testing.T) {
	h := newDrainHarness(t, `{}`)
	h.conn.deliverFn = func(connector.Announcement) error {
		return fmt.Errorf("%w: still connecting", connector.ErrNotReady)
	}
	row := h.deliveries.add(1, announcementN(1), 2)
	previous := "webhook returned 500"
	row.LastError = &previous

	h.run(t)

	stored := h.deliveries.rows[row.ID]
	if stored.LastError == nil || *stored.LastError != previous {
		t.Fatalf("last_error = %v, want the earlier failure preserved", stored.LastError)
	}
	if stored.Attempts != 2 {
		t.Fatalf("attempts = %d, want the existing count untouched", stored.Attempts)
	}
}

// A row with no creation time must be treated as new, not as two thousand years
// old — the latter dead-letters it on the first not-ready.
func TestDrainNotReadyToleratesAZeroCreatedAt(t *testing.T) {
	h := newDrainHarness(t, `{}`)
	h.conn.deliverFn = func(connector.Announcement) error {
		return fmt.Errorf("%w: still connecting", connector.ErrNotReady)
	}
	row := h.deliveries.add(1, announcementN(1), 0)
	row.CreatedAt = time.Time{}

	h.run(t)

	if got := h.deliveries.rows[row.ID].Status; got != model.DeliveryPending {
		t.Fatalf("status = %q, want it rescheduled rather than dead-lettered", got)
	}
}

// "Not ready" must not mean "never": a destination that has been down for a long
// time is a real failure and has to stop being retried.
func TestDrainNotReadyGivesUpOnAnOldDelivery(t *testing.T) {
	h := newDrainHarness(t, `{}`)
	h.conn.deliverFn = func(connector.Announcement) error {
		return fmt.Errorf("%w: still connecting", connector.ErrNotReady)
	}
	row := h.deliveries.add(1, announcementN(1), 0)
	row.CreatedAt = time.Now().Add(-2 * connectorNotReadyMaxAge)

	h.run(t)

	stored := h.deliveries.rows[row.ID]
	if stored.Status != model.DeliveryFailed {
		t.Fatalf("status = %q, want failed once it has waited past the cap", stored.Status)
	}
	if stored.NextAttemptAt != nil {
		t.Fatal("a dead-lettered row must not be scheduled again")
	}
}

// --- permanent failures (BE-10.4) ---

// A connector that knows the delivery can never succeed is believed at once.
// For a fan-out connector every pointless retry means the destinations that did
// work receive the same announcement again.
func TestDrainDeadLettersAPermanentFailureImmediately(t *testing.T) {
	h := newDrainHarness(t, `{}`)
	h.conn.deliverFn = func(connector.Announcement) error {
		return fmt.Errorf("%w: chat not found", connector.ErrPermanent)
	}
	row := h.deliveries.add(1, announcementN(1), 0)

	h.run(t)

	stored := h.deliveries.rows[row.ID]
	if stored.Status != model.DeliveryFailed {
		t.Fatalf("status = %q, want failed on the first permanent failure", stored.Status)
	}
	if stored.NextAttemptAt != nil {
		t.Fatal("a permanently failed delivery must not be scheduled again")
	}
	if stored.LastError == nil || !strings.Contains(*stored.LastError, "chat not found") {
		t.Fatalf("last_error = %v, want the reason preserved", stored.LastError)
	}
	if len(h.enqueuer.calls) != 0 {
		t.Fatal("nothing should be re-enqueued for a permanent failure")
	}
}

// A permanent failure still gets redacted, since the reason can quote a
// destination's response.
func TestDrainRedactsAPermanentFailure(t *testing.T) {
	h := newDrainHarness(t, `{"url":"https://example.test/hook","hmac_secret":"s3cr3tValue"}`)
	h.conn.deliverFn = func(connector.Announcement) error {
		return fmt.Errorf("%w: rejected token s3cr3tValue", connector.ErrPermanent)
	}
	row := h.deliveries.add(1, announcementN(1), 0)

	h.run(t)

	stored := h.deliveries.rows[row.ID]
	if stored.LastError == nil || strings.Contains(*stored.LastError, "s3cr3tValue") {
		t.Fatalf("last_error leaked the secret: %v", stored.LastError)
	}
}
