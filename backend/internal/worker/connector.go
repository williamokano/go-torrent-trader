package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/hibiken/asynq"

	"github.com/williamokano/go-torrent-trader/backend/internal/connector"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

// TaskConnectorDrain delivers whatever is pending for one connector instance.
const TaskConnectorDrain = "connector:drain"

const (
	// connectorMaxAttempts is when a delivery is dead-lettered. It stays in the
	// admin log as 'failed' rather than retrying forever.
	connectorMaxAttempts = 5
	// connectorBaseBackoff doubles per attempt up to connectorMaxBackoff:
	// 30s, 1m, 2m, 4m.
	connectorBaseBackoff = 30 * time.Second
	connectorMaxBackoff  = 10 * time.Minute
	// connectorDrainBatch bounds one run so a huge backlog cannot monopolise a
	// worker slot; whatever is left triggers an immediate follow-up drain.
	connectorDrainBatch = 100
	// connectorRateWindow is the window the per-instance rate budget is measured
	// over, and how long a budget-exhausted drain waits before trying again.
	connectorRateWindow = time.Minute
	// connectorDrainUnique collapses a burst of dispatches for one instance into
	// a single drain. asynq keys uniqueness on task type + payload, and the
	// payload is just the instance ID.
	connectorDrainUnique = 5 * time.Second
	// connectorDeliverTimeout bounds one Deliver call.
	connectorDeliverTimeout = 10 * time.Second
	// connectorClaimLease is how long a row is hidden from other drains while
	// one is delivering it. It comfortably covers connectorDeliverTimeout plus
	// scheduling jitter; if the worker dies mid-delivery the lease simply
	// expires and the row becomes due again.
	connectorClaimLease = 30 * time.Second
	// connectorDrainDeadline bounds a whole run. The row batch alone does not:
	// 100 rows against a black-holed endpoint is 100 × the deliver timeout, and
	// there are only ten asynq slots shared with every other job. Whatever is
	// left over is picked up by the immediate follow-up.
	connectorDrainDeadline = 60 * time.Second
	// connectorNotReadyRetry is how soon to look again when a connector says it
	// is not ready (an IRC client mid-reconnect, or a node that does not own the
	// connection). Short, because the condition usually clears in seconds.
	connectorNotReadyRetry = 15 * time.Second
	// connectorNotReadyJitter spreads the retries of instances recovering from
	// the same outage.
	connectorNotReadyJitter = 5 * time.Second
	// connectorNotReadyMaxAge stops "not ready" from meaning "never": a
	// destination that has not come up within this window is treated as a real
	// failure so the row dead-letters instead of being rescheduled forever.
	connectorNotReadyMaxAge = 15 * time.Minute
	// connectorDisabledGrace is how long a disabled instance's queued rows are
	// kept before being dead-lettered. Toggling an instance off to edit it is
	// routine, and losing the announcements queued in those few minutes would be
	// a nasty surprise; a week-long disable should not flush a stale flood on
	// re-enable either.
	connectorDisabledGrace = 15 * time.Minute
)

// ConnectorDrainPayload is the drain task's payload.
//
// It carries only the instance ID on purpose: every other piece of state lives
// in connector_deliveries, so a task queued before a deploy still means exactly
// the same thing after it. Never add a required field here.
type ConnectorDrainPayload struct {
	InstanceID int64 `json:"instance_id"`
}

// NewConnectorDrainTask builds a drain task, optionally deferred by delay.
//
// MaxRetry(0) is deliberate: retries are managed in the database, not by asynq.
// The delivery rows carry attempts and next_attempt_at, which is what lets one
// run see the whole pending set — required for both dedupe and coalescing, and
// something asynq's per-task retry cannot do.
//
// collapse adds the uniqueness window that turns a burst of dispatches into a
// single drain. It must be false for a drain scheduling its own follow-up:
// asynq holds the uniqueness lock for the whole duration of the running task
// (it is released in Done, not at dequeue) and the key is type+payload, which
// ProcessIn does not vary — so a collapsing enqueue from inside the handler is
// always rejected as a duplicate and the follow-up silently never happens.
// Concurrent drains are already made safe by ClaimForDelivery, so the follow-up
// does not need collapsing anyway.
func NewConnectorDrainTask(instanceID int64, delay time.Duration, collapse bool) (*asynq.Task, error) {
	payload, err := json.Marshal(ConnectorDrainPayload{InstanceID: instanceID})
	if err != nil {
		return nil, fmt.Errorf("marshal connector drain payload: %w", err)
	}
	opts := []asynq.Option{asynq.MaxRetry(0)}
	if collapse {
		opts = append(opts, asynq.Unique(connectorDrainUnique))
	}
	if delay > 0 {
		opts = append(opts, asynq.ProcessIn(delay))
	}
	return asynq.NewTask(TaskConnectorDrain, payload, opts...), nil
}

// NewConnectorDrainHandler returns the asynq handler that delivers pending
// announcements for one instance.
//
// It always returns nil. A connector failure is recorded on the delivery row and
// rescheduled from there; surfacing it to asynq as well would produce a second,
// competing retry schedule on top of the one in the database.
func NewConnectorDrainHandler(deps *WorkerDeps) func(ctx context.Context, t *asynq.Task) error {
	return func(ctx context.Context, t *asynq.Task) error {
		if deps.ConnectorRepo == nil || deps.ConnectorDeliveryRepo == nil || deps.ConnectorRegistry == nil {
			return nil
		}

		var payload ConnectorDrainPayload
		if err := json.Unmarshal(t.Payload(), &payload); err != nil {
			slog.Error("connector drain: invalid payload", "error", err)
			return nil
		}

		drainCtx, cancel := context.WithTimeout(ctx, connectorDrainDeadline)
		defer cancel()

		drainInstance(drainCtx, deps, payload.InstanceID)
		return nil
	}
}

func drainInstance(ctx context.Context, deps *WorkerDeps, instanceID int64) {
	now := time.Now()

	due, err := deps.ConnectorDeliveryRepo.ListDue(ctx, instanceID, now, connectorDrainBatch)
	if err != nil {
		slog.Error("connector drain: failed to list due deliveries", "instance_id", instanceID, "error", err)
		return
	}
	if len(due) == 0 {
		return
	}

	inst, err := deps.ConnectorRepo.GetByID(ctx, instanceID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// The instance was deleted; its rows went with it via cascade, and
			// anything we are holding is already gone.
			return
		}
		slog.Error("connector drain: failed to load instance", "instance_id", instanceID, "error", err)
		return
	}
	if !inst.Enabled {
		// Only give up on rows that have been waiting longer than the grace
		// period; anything newer stays pending so a brief disable-to-edit does
		// not throw the queue away.
		var stale []model.ConnectorDelivery
		for _, row := range due {
			if now.Sub(row.CreatedAt) > connectorDisabledGrace {
				stale = append(stale, row)
			}
		}
		failAll(ctx, deps, stale, "connector instance is disabled")
		return
	}
	impl, ok := deps.ConnectorRegistry.Get(inst.Kind)
	if !ok {
		failAll(ctx, deps, due, fmt.Sprintf("unknown connector kind %q", inst.Kind))
		return
	}

	budget, err := rateBudget(ctx, deps, inst.ID, inst.Config, now)
	if err != nil {
		slog.Error("connector drain: failed to compute rate budget", "instance_id", instanceID, "error", err)
		return
	}
	if budget <= 0 {
		// Nothing failed — the instance has simply said its piece for this
		// minute. Come back when the window rolls, without touching any row.
		enqueueDrain(ctx, deps, instanceID, connectorRateWindow)
		return
	}

	// With more due than budget, a kind a person reads spends its last unit of
	// budget on a single "+N more" summary, so nothing is dropped silently:
	// every row still ends up sent or coalesced. A machine-read kind cannot do
	// that — a summary is unrecoverable for a bot — so it spends the whole
	// budget on individual deliveries and leaves the rest pending for the next
	// window.
	retryIn := time.Duration(0)
	individual := len(due)
	var toCoalesce []model.ConnectorDelivery
	if len(due) > budget {
		if impl.Coalescable() {
			individual = budget - 1
			toCoalesce = due[individual:]
		} else {
			individual = budget
			retryIn = connectorRateWindow
		}
	}

	inject := connector.Instance{ID: inst.ID, Name: inst.Name, Config: inst.Config}
	secretFields := impl.SecretFields()

	for i := range due[:individual] {
		row := due[i]
		if !claim(ctx, deps, row, now) {
			// Another drain is already delivering this row.
			continue
		}
		announcement, err := decodeAnnouncement(row)
		if err != nil {
			// A payload that cannot be read will never become readable.
			markFailed(ctx, deps, row, connectorMaxAttempts, err.Error())
			continue
		}
		announcement.DeliveryKey = row.EventKey

		delivered := deliverOne(ctx, deps, impl, inject, announcement, row, inst.Config, secretFields)
		if delivered > 0 {
			retryIn = minNonZero(retryIn, delivered)
		}
	}

	if claimed := claimAll(ctx, deps, toCoalesce, now); len(claimed) > 0 {
		retryIn = minNonZero(retryIn, deliverCoalesced(ctx, deps, impl, inject, claimed, inst.Config, secretFields))
	}

	// Schedule at most one follow-up. Two enqueues here would collide anyway:
	// asynq keys uniqueness on type + payload, and the payload is just the
	// instance ID, so the second would be rejected as a duplicate.
	//
	// A full batch wins, because that work is due now while a retry is at least
	// 30s out — and the follow-up drain re-evaluates the retry when it runs. The
	// one gap is a full batch whose leftovers are *all* future-dated retries: the
	// follow-up then finds nothing due and schedules nothing, and the rows wait
	// for the maintenance sweep (≤5 min) to re-enqueue them. That is the sweep's
	// job, and it only costs extra latency in an already-failing instance.
	switch {
	case len(due) == connectorDrainBatch:
		enqueueDrain(ctx, deps, instanceID, 0)
	case retryIn > 0:
		enqueueDrain(ctx, deps, instanceID, retryIn)
	}
}

// deliverOne attempts a single delivery, returning the delay after which the row
// should be retried, or 0 when nothing more is needed (success or dead-letter).
func deliverOne(
	ctx context.Context,
	deps *WorkerDeps,
	impl connector.Connector,
	inst connector.Instance,
	a connector.Announcement,
	row model.ConnectorDelivery,
	rawConfig json.RawMessage,
	secretFields []string,
) time.Duration {
	deliverCtx, cancel := context.WithTimeout(ctx, connectorDeliverTimeout)
	defer cancel()

	if err := connector.SafeDeliver(deliverCtx, impl, inst, a); err != nil {
		return recordFailure(ctx, deps, row, err, rawConfig, secretFields)
	}

	if err := deps.ConnectorDeliveryRepo.MarkSent(ctx, row.ID, model.DeliverySent); err != nil {
		slog.Error("connector drain: failed to mark delivery sent",
			"instance_id", row.InstanceID, "delivery_id", row.ID, "error", err)
	}
	return 0
}

// deliverCoalesced sends one summary standing in for a batch of rows, then
// closes all of them as 'coalesced'. A failure applies to the whole batch: each
// row takes an attempt and retries on its own schedule.
func deliverCoalesced(
	ctx context.Context,
	deps *WorkerDeps,
	impl connector.Connector,
	inst connector.Instance,
	rows []model.ConnectorDelivery,
	rawConfig json.RawMessage,
	secretFields []string,
) time.Duration {
	// Drop unreadable rows first so the summary's count matches the rows it
	// actually stands for, and so one corrupt payload cannot dead-letter the
	// whole batch it happens to sit in.
	readable := make([]model.ConnectorDelivery, 0, len(rows))
	var announcement connector.Announcement
	for _, row := range rows {
		decoded, err := decodeAnnouncement(row)
		if err != nil {
			markFailed(ctx, deps, row, connectorMaxAttempts, err.Error())
			continue
		}
		readable = append(readable, row)
		// The newest readable row speaks for the batch.
		announcement = decoded
	}
	if len(readable) == 0 {
		return 0
	}

	representative := readable[len(readable)-1]
	announcement.Coalesced = len(readable)
	announcement.DeliveryKey = representative.EventKey

	deliverCtx, cancel := context.WithTimeout(ctx, connectorDeliverTimeout)
	defer cancel()

	if err := connector.SafeDeliver(deliverCtx, impl, inst, announcement); err != nil {
		retryIn := time.Duration(0)
		for _, row := range readable {
			retryIn = minNonZero(retryIn, recordFailure(ctx, deps, row, err, rawConfig, secretFields))
		}
		return retryIn
	}

	for _, row := range readable {
		if err := deps.ConnectorDeliveryRepo.MarkSent(ctx, row.ID, model.DeliveryCoalesced); err != nil {
			slog.Error("connector drain: failed to mark delivery coalesced",
				"instance_id", row.InstanceID, "delivery_id", row.ID, "error", err)
		}
	}
	return 0
}

// recordFailure persists a failed attempt and returns the retry delay (0 when
// the row was dead-lettered).
func recordFailure(
	ctx context.Context,
	deps *WorkerDeps,
	row model.ConnectorDelivery,
	deliverErr error,
	rawConfig json.RawMessage,
	secretFields []string,
) time.Duration {
	// "Not ready" is not a failure: an IRC client reconnecting, or a standby
	// node that does not own the connection, will be fine shortly. Counting it
	// against the five-attempt budget would let a routine reconnect dead-letter
	// a whole queue of good announcements, so the row is simply rescheduled —
	// but only up to a bounded age, so a destination that never comes up still
	// stops eventually.
	if errors.Is(deliverErr, connector.ErrNotReady) {
		return rescheduleNotReady(ctx, deps, row, deliverErr, rawConfig, secretFields)
	}

	// Every error path funnels through RedactError before it can reach a log
	// line or the last_error column — a Telegram bot token lives in the request
	// URL, and net/http puts that URL in the error.
	message := connector.RedactError(deliverErr, rawConfig, secretFields)
	attempts := row.Attempts + 1

	if attempts >= connectorMaxAttempts {
		markFailed(ctx, deps, row, attempts, message)
		slog.Warn("connector delivery dead-lettered",
			"instance_id", row.InstanceID, "delivery_id", row.ID,
			"attempts", attempts, "error", message)
		return 0
	}

	backoff := connectorBackoff(attempts)
	next := time.Now().Add(backoff)
	if err := deps.ConnectorDeliveryRepo.MarkFailedAttempt(ctx, row.ID, attempts, message, &next); err != nil {
		slog.Error("connector drain: failed to record delivery attempt",
			"instance_id", row.InstanceID, "delivery_id", row.ID, "error", err)
	}
	return backoff
}

// rescheduleNotReady defers a delivery whose destination is temporarily unable
// to take it, without consuming a retry attempt.
//
// It keeps whatever last_error the row already had: a row that failed four
// times with "connection refused" and is now merely waiting for a reconnect
// should still say why it failed, since the delivery log is the only place that
// history exists.
func rescheduleNotReady(
	ctx context.Context,
	deps *WorkerDeps,
	row model.ConnectorDelivery,
	deliverErr error,
	rawConfig json.RawMessage,
	secretFields []string,
) time.Duration {
	// A row with no creation time cannot be aged out — treat it as new rather
	// than as two thousand years old, which would dead-letter it immediately.
	age := time.Duration(0)
	if !row.CreatedAt.IsZero() {
		age = time.Since(row.CreatedAt)
	}
	if age > connectorNotReadyMaxAge {
		markFailed(ctx, deps, row, row.Attempts+1,
			connector.RedactError(deliverErr, rawConfig, secretFields))
		return 0
	}

	message := ""
	if row.LastError != nil {
		message = *row.LastError
	} else {
		message = connector.RedactError(deliverErr, rawConfig, secretFields)
	}

	// Jittered, so several instances coming back from the same outage do not
	// all retry on the same tick.
	delay := connectorNotReadyRetry + time.Duration(rand.Int64N(int64(connectorNotReadyJitter)))
	next := time.Now().Add(delay)
	if err := deps.ConnectorDeliveryRepo.MarkFailedAttempt(
		ctx, row.ID, row.Attempts, message, &next,
	); err != nil {
		slog.Error("connector drain: failed to reschedule a not-ready delivery",
			"instance_id", row.InstanceID, "delivery_id", row.ID, "error", err)
	}
	return delay
}

func markFailed(ctx context.Context, deps *WorkerDeps, row model.ConnectorDelivery, attempts int, message string) {
	if err := deps.ConnectorDeliveryRepo.MarkFailedAttempt(ctx, row.ID, attempts, message, nil); err != nil {
		slog.Error("connector drain: failed to dead-letter delivery",
			"instance_id", row.InstanceID, "delivery_id", row.ID, "error", err)
	}
}

// failAll dead-letters every row with the same reason, used when the instance
// itself cannot deliver at all (disabled, or its kind is no longer registered).
func failAll(ctx context.Context, deps *WorkerDeps, rows []model.ConnectorDelivery, message string) {
	for _, row := range rows {
		markFailed(ctx, deps, row, row.Attempts+1, message)
	}
}

// claim takes the delivery lease for one row, reporting whether this drain owns
// it. A lost race is not an error: the drain that won will deliver the row.
func claim(ctx context.Context, deps *WorkerDeps, row model.ConnectorDelivery, now time.Time) bool {
	claimed, err := deps.ConnectorDeliveryRepo.ClaimForDelivery(ctx, row.ID, now.Add(connectorClaimLease), now)
	if err != nil {
		slog.Error("connector drain: failed to claim delivery",
			"instance_id", row.InstanceID, "delivery_id", row.ID, "error", err)
		return false
	}
	return claimed
}

// claimAll returns the subset of rows this drain owns, so a coalesced summary
// only ever speaks for rows nobody else is delivering.
func claimAll(ctx context.Context, deps *WorkerDeps, rows []model.ConnectorDelivery, now time.Time) []model.ConnectorDelivery {
	claimed := make([]model.ConnectorDelivery, 0, len(rows))
	for _, row := range rows {
		if claim(ctx, deps, row, now) {
			claimed = append(claimed, row)
		}
	}
	return claimed
}

func rateBudget(ctx context.Context, deps *WorkerDeps, instanceID int64, cfg json.RawMessage, now time.Time) (int, error) {
	limit := connector.RatePerMin(cfg)
	sent, err := deps.ConnectorDeliveryRepo.CountSentSince(ctx, instanceID, now.Add(-connectorRateWindow))
	if err != nil {
		return 0, err
	}
	budget := limit - int(sent)
	if budget < 0 {
		budget = 0
	}
	return budget, nil
}

func decodeAnnouncement(row model.ConnectorDelivery) (connector.Announcement, error) {
	var a connector.Announcement
	if err := json.Unmarshal(row.Payload, &a); err != nil {
		return a, fmt.Errorf("unreadable delivery payload: %w", err)
	}
	return a, nil
}

// enqueueDrain schedules this drain's own follow-up. It uses the non-collapsing
// path because the uniqueness lock for this very task is still held.
func enqueueDrain(ctx context.Context, deps *WorkerDeps, instanceID int64, delay time.Duration) {
	if deps.ConnectorEnqueuer == nil {
		return
	}
	if err := deps.ConnectorEnqueuer.EnqueueConnectorDrainFollowUp(ctx, instanceID, delay); err != nil {
		slog.Error("connector drain: failed to re-enqueue drain",
			"instance_id", instanceID, "delay", delay, "error", err)
	}
}

// connectorBackoff is 30s doubling per attempt, capped at 10 minutes.
func connectorBackoff(attempts int) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	if attempts > 20 {
		return connectorMaxBackoff
	}
	backoff := connectorBaseBackoff << (attempts - 1)
	if backoff > connectorMaxBackoff || backoff <= 0 {
		return connectorMaxBackoff
	}
	return backoff
}

// minNonZero picks the sooner of two retry delays, ignoring "no retry needed".
func minNonZero(a, b time.Duration) time.Duration {
	switch {
	case a <= 0:
		return b
	case b <= 0:
		return a
	case b < a:
		return b
	default:
		return a
	}
}
