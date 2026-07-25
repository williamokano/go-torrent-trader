package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/hibiken/asynq"

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

func (h *drainHarness) run(t *testing.T) {
	t.Helper()
	task, err := NewConnectorDrainTask(1, 0, true)
	if err != nil {
		t.Fatalf("NewConnectorDrainTask: %v", err)
	}
	if err := NewConnectorDrainHandler(h.deps)(context.Background(), task); err != nil {
		t.Fatalf("drain handler returned an error: %v", err)
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

func TestConnectorDrainTaskPayloadIsMinimal(t *testing.T) {
	task, err := NewConnectorDrainTask(7, 0, true)
	if err != nil {
		t.Fatalf("NewConnectorDrainTask: %v", err)
	}
	if task.Type() != TaskConnectorDrain {
		t.Fatalf("type = %q, want %q", task.Type(), TaskConnectorDrain)
	}

	var payload map[string]any
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	// Everything else lives in connector_deliveries, so a task queued before a
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
	if len(h.enqueuer.calls) != 1 || h.enqueuer.calls[0].delay != connectorRateWindow {
		t.Fatalf("follow-up = %+v, want one queued for the next window", h.enqueuer.calls)
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
	if len(h.enqueuer.calls) != 1 || h.enqueuer.calls[0].delay != connectorRateWindow {
		t.Fatalf("follow-up = %+v, want one queued for the next window", h.enqueuer.calls)
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

func TestDrainQueuesFollowUpWhenBatchIsFull(t *testing.T) {
	h := newDrainHarness(t, fmt.Sprintf(`{"rate_per_min":%d}`, connectorDrainBatch*2))
	for i := 1; i <= connectorDrainBatch; i++ {
		h.deliveries.add(1, announcementN(i), 0)
	}

	h.run(t)

	if len(h.enqueuer.calls) != 1 || h.enqueuer.calls[0].delay != 0 {
		t.Fatalf("follow-up = %+v, want one queued immediately for the rest of the backlog", h.enqueuer.calls)
	}
}

// The two enqueue paths differ only in whether asynq is allowed to collapse
// duplicates, and getting that backwards is invisible without a real client:
// the uniqueness lock is held for the whole duration of the running task, so a
// collapsing enqueue from inside the handler is always rejected and the retry
// silently never happens.
func TestConnectorEnqueuerCollapsesDispatchesButNotFollowUps(t *testing.T) {
	mr := miniredis.RunT(t)
	client := asynq.NewClient(asynq.RedisClientOpt{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	inspector := asynq.NewInspector(asynq.RedisClientOpt{Addr: mr.Addr()})
	t.Cleanup(func() { _ = inspector.Close() })

	enqueuer := NewAsynqConnectorEnqueuer(client)
	ctx := context.Background()

	// The dispatcher path collapses: a burst of approvals produces one drain.
	if err := enqueuer.EnqueueConnectorDrain(ctx, 1, 0); err != nil {
		t.Fatalf("first dispatch enqueue: %v", err)
	}
	if err := enqueuer.EnqueueConnectorDrain(ctx, 1, 0); err != nil {
		t.Fatalf("a collapsed duplicate must be reported as success, got: %v", err)
	}
	pending, err := inspector.ListPendingTasks("default")
	if err != nil {
		t.Fatalf("listing pending tasks: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("%d pending drains, want 1: the burst should have collapsed", len(pending))
	}

	// The follow-up path must reach the queue even though the same uniqueness
	// key was just locked — asserting on the queue, not on the returned error,
	// because a collapsed enqueue reports success too.
	if err := enqueuer.EnqueueConnectorDrainFollowUp(ctx, 1, 30*time.Second); err != nil {
		t.Fatalf("follow-up enqueue: %v", err)
	}
	scheduled, err := inspector.ListScheduledTasks("default")
	if err != nil {
		t.Fatalf("listing scheduled tasks: %v", err)
	}
	if len(scheduled) != 1 {
		t.Fatalf("%d scheduled drains, want 1: the drain's own retry must not be collapsed away", len(scheduled))
	}
}

// --- guards ---

func TestDrainWithoutWiringIsANoOp(t *testing.T) {
	task, err := NewConnectorDrainTask(1, 0, true)
	if err != nil {
		t.Fatalf("NewConnectorDrainTask: %v", err)
	}
	if err := NewConnectorDrainHandler(&WorkerDeps{})(context.Background(), task); err != nil {
		t.Fatalf("an unwired deployment must not error: %v", err)
	}
}

func TestDrainIgnoresMalformedPayload(t *testing.T) {
	h := newDrainHarness(t, `{}`)
	task := asynq.NewTask(TaskConnectorDrain, []byte(`{not json`))

	if err := NewConnectorDrainHandler(h.deps)(context.Background(), task); err != nil {
		t.Fatalf("a malformed task payload must not error (it would only be retried): %v", err)
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
