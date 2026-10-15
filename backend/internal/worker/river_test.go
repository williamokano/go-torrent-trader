package worker

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
)

// These run River itself against the real, migrated database. The other tests
// in this package check the options the code hands River; only these check
// that River does with them what the code assumes — the uniqueness that
// collapses dispatches, the schema the goose migrations created, and a job
// actually reaching its worker.

// requirePool opens a pgx pool on the test database and empties river_job
// before and after the test, so a test's leftover jobs are neither counted by
// the next one nor worked by its client.
func requirePool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	db := requireDB(t)
	clear := func() {
		if _, err := db.ExecContext(context.Background(), `DELETE FROM river_job`); err != nil {
			t.Fatalf("clearing river_job: %v", err)
		}
	}
	clear()
	t.Cleanup(clear)
	pool, err := pgxpool.New(context.Background(), testDSN)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func countDrainJobs(t *testing.T, instanceID int64, states ...string) int {
	t.Helper()
	var n int
	if err := testDB.QueryRowContext(context.Background(),
		`SELECT count(*) FROM river_job
		 WHERE kind = $1 AND (args->>'instance_id')::bigint = $2 AND state::text = ANY($3)`,
		TaskConnectorDrain, instanceID, states,
	).Scan(&n); err != nil {
		t.Fatalf("counting drain jobs: %v", err)
	}
	return n
}

// A burst of dispatches is one job; a retry is a second, separate one; and a
// finished drain does not block the next dispatch. The last is the subtle one:
// River's default unique states include completed, which would leave every
// instance unable to start a drain until River pruned the finished row.
func TestRiverCollapsesDispatchesButNotRetries(t *testing.T) {
	pool := requirePool(t)
	client, err := NewInsertClient(pool)
	if err != nil {
		t.Fatalf("NewInsertClient: %v", err)
	}
	enqueuer := NewRiverConnectorEnqueuer(client)
	ctx := context.Background()
	const instance = 9101

	for range 3 {
		if err := enqueuer.EnqueueConnectorDrain(ctx, instance, 0); err != nil {
			t.Fatalf("dispatch: %v", err)
		}
	}
	if n := countDrainJobs(t, instance, "available"); n != 1 {
		t.Fatalf("%d drains queued after a burst of 3 dispatches, want 1", n)
	}

	if err := enqueuer.EnqueueConnectorDrainFollowUp(ctx, instance, time.Minute); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if n := countDrainJobs(t, instance, "scheduled"); n != 1 {
		t.Fatalf("%d retries scheduled, want 1: a retry must not collapse into the queued drain", n)
	}

	// Finish the queued drain the way River would, then dispatch again.
	if _, err := testDB.ExecContext(ctx,
		`UPDATE river_job SET state = 'completed', finalized_at = now()
		 WHERE kind = $1 AND (args->>'instance_id')::bigint = $2 AND state = 'available'`,
		TaskConnectorDrain, instance,
	); err != nil {
		t.Fatalf("completing the drain: %v", err)
	}
	if err := enqueuer.EnqueueConnectorDrain(ctx, instance, 0); err != nil {
		t.Fatalf("dispatch after completion: %v", err)
	}
	if n := countDrainJobs(t, instance, "available"); n != 1 {
		t.Fatalf("%d drains queued after the previous one completed, want 1", n)
	}
}

// The full path: an email inserted through the insert client is picked up by
// the worker client and handed to the sender.
func TestRiverWorksAQueuedEmail(t *testing.T) {
	pool := requirePool(t)
	sent := make(chan string, 1)
	deps := &WorkerDeps{EmailSender: sendFunc(func(to string) { sent <- to })}

	workerClient, err := NewWorkerClient(pool, deps, nil, 1)
	if err != nil {
		t.Fatalf("NewWorkerClient: %v", err)
	}
	ctx := context.Background()
	if err := workerClient.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = workerClient.Stop(stopCtx)
	})

	insertClient, err := NewInsertClient(pool)
	if err != nil {
		t.Fatalf("NewInsertClient: %v", err)
	}
	if err := NewRiverEmailEnqueuer(insertClient).EnqueueSendEmail(ctx, "river@example.test", "Hi", "Body"); err != nil {
		t.Fatalf("EnqueueSendEmail: %v", err)
	}

	select {
	case to := <-sent:
		if to != "river@example.test" {
			t.Fatalf("sent to %q, want river@example.test", to)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("the queued email was never worked")
	}
}

// A snoozing job really does come back, and without spending its attempts:
// the connector drain allows exactly one, and snoozes routinely.
func TestRiverSnoozeDoesNotSpendTheOnlyAttempt(t *testing.T) {
	pool := requirePool(t)
	runs := make(chan int, 3)
	var count atomic.Int32
	workers := river.NewWorkers()
	river.AddWorker(workers, &funcWorker[snoozeTwiceArgs]{fn: func(_ context.Context, job *river.Job[snoozeTwiceArgs]) error {
		runs <- job.Attempt
		if count.Add(1) < 3 {
			return river.JobSnooze(0)
		}
		return nil
	}})

	client, err := river.NewClient(newRiverDriver(pool), workerConfig(workers, nil, 1))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	ctx := context.Background()
	if err := client.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = client.Stop(stopCtx)
	})

	if _, err := client.Insert(ctx, snoozeTwiceArgs{}, &river.InsertOpts{MaxAttempts: 1}); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	for i := 1; i <= 3; i++ {
		select {
		case attempt := <-runs:
			if attempt != 1 {
				t.Fatalf("run %d was attempt %d, want 1: a snooze must not count as an attempt", i, attempt)
			}
		case <-time.After(15 * time.Second):
			t.Fatalf("run %d never happened: a job with one attempt did not survive its snoozes", i)
		}
	}

	// And after two snoozes, the third run's success completes it.
	deadline := time.Now().Add(15 * time.Second)
	for {
		var state string
		if err := testDB.QueryRowContext(ctx,
			`SELECT state::text FROM river_job WHERE kind = $1`, snoozeTwiceArgs{}.Kind(),
		).Scan(&state); err != nil {
			t.Fatalf("reading job state: %v", err)
		}
		if state == "completed" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("job state = %q after its successful run, want completed", state)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

type snoozeTwiceArgs struct{}

func (snoozeTwiceArgs) Kind() string { return "test:snooze_twice" }

type sendFunc func(to string)

func (f sendFunc) Send(_ context.Context, to, _, _ string) error {
	f(to)
	return nil
}
