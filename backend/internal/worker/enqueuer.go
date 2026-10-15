package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/riverqueue/river"
)

// enqueueTimeout bounds one insert. The insert is detached from the caller's
// cancellation (see detach), so this is what stops a stalled database from
// holding a request open indefinitely.
const enqueueTimeout = 10 * time.Second

// detach gives an insert its own lifetime. Callers pass request contexts, and
// the work an enqueue follows has usually already committed — a registration's
// user row, a dispatch's delivery row. A client that disconnects at that moment
// must not cancel the insert and leave an account that never gets its
// confirmation email. asynq's enqueuers ignored the context altogether.
func detach(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), enqueueTimeout)
}

// RiverEmailEnqueuer implements service.TaskEnqueuer using River.
type RiverEmailEnqueuer struct {
	client JobInserter
}

// NewRiverEmailEnqueuer creates an enqueuer backed by the given River client.
func NewRiverEmailEnqueuer(client JobInserter) *RiverEmailEnqueuer {
	return &RiverEmailEnqueuer{client: client}
}

// EnqueueSendEmail enqueues an email sending job for background processing.
func (e *RiverEmailEnqueuer) EnqueueSendEmail(ctx context.Context, to, subject, body string) error {
	ctx, cancel := detach(ctx)
	defer cancel()
	if _, err := e.client.Insert(ctx, SendEmailArgs{To: to, Subject: subject, Body: body}, nil); err != nil {
		return fmt.Errorf("enqueue email job: %w", err)
	}
	return nil
}

// RiverConnectorEnqueuer implements ConnectorDrainEnqueuer (and so
// listener.DrainEnqueuer) using River.
type RiverConnectorEnqueuer struct {
	client JobInserter
}

// NewRiverConnectorEnqueuer creates an enqueuer backed by the given River client.
func NewRiverConnectorEnqueuer(client JobInserter) *RiverConnectorEnqueuer {
	return &RiverConnectorEnqueuer{client: client}
}

// EnqueueConnectorDrain queues a drain for one instance, collapsing a burst of
// dispatches into a single run.
//
// A duplicate is reported as success: that is the collapse doing its job (25
// approvals in a second produce one drain, which then delivers all 25 rows),
// not a failure to schedule work. River signals it on the result rather than
// as an error.
func (e *RiverConnectorEnqueuer) EnqueueConnectorDrain(ctx context.Context, instanceID int64, delay time.Duration) error {
	opts := drainInsertOpts(delay)
	opts.UniqueOpts = connectorDrainUnique
	return e.insert(ctx, instanceID, opts)
}

// EnqueueConnectorDrainFollowUp queues a retry from inside a drain that is
// still executing. It deliberately does not collapse — see enqueueDrain for why
// a retry must not wait behind, or be swallowed by, another drain.
func (e *RiverConnectorEnqueuer) EnqueueConnectorDrainFollowUp(ctx context.Context, instanceID int64, delay time.Duration) error {
	return e.insert(ctx, instanceID, drainInsertOpts(delay))
}

func (e *RiverConnectorEnqueuer) insert(ctx context.Context, instanceID int64, opts *river.InsertOpts) error {
	ctx, cancel := detach(ctx)
	defer cancel()
	if _, err := e.client.Insert(ctx, ConnectorDrainArgs{InstanceID: instanceID}, opts); err != nil {
		return fmt.Errorf("enqueue connector drain job: %w", err)
	}
	return nil
}

func drainInsertOpts(delay time.Duration) *river.InsertOpts {
	opts := &river.InsertOpts{}
	if delay > 0 {
		opts.ScheduledAt = time.Now().Add(delay)
	}
	return opts
}
