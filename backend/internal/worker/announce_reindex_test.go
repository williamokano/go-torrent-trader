package worker

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/riverqueue/river"

	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// captureLogs redirects the default slog handler for one test. The reindex job
// reports everything through slog, so an assertion about what it reported has
// nowhere else to look.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return &buf
}

// The options carry the whole safety story of this job. Without this, losing
// the attempt budget restores River's default of 25 — twenty-five full rebuilds
// of the largest table on the site, back to back, each failure leaving fresh
// wreckage for the next to clear — and every other test here stays green.
func TestAnnounceLogReindex_CarriesItsSafetyOptions(t *testing.T) {
	// A failed rebuild waits for next month. Retrying re-enters a job that just
	// ran long and failed, most likely for a reason that has not gone away.
	if got := (AnnounceLogReindexArgs{}).InsertOpts().MaxAttempts; got != 1 {
		t.Errorf("MaxAttempts = %d, want 1 — a failed rebuild must not retry", got)
	}
	// Bounds a wedged statement. Cancelling the context does abort the REINDEX
	// server-side, so this is a real bound and not just a Go-side giving up. Read
	// off the registered worker, because that is where River takes it from.
	w := registeredWorker[AnnounceLogReindexArgs](t, &WorkerDeps{})
	if got := w.Timeout(nil); got != announceReindexTimeout {
		t.Errorf("Timeout = %s, want %s", got, announceReindexTimeout)
	}
}

// The same guard the maintenance job has, for the same reason: every other test
// here calls the handler directly, which proves nothing about whether the worker
// will ever reach it. A dropped registration leaves this package green and the
// indexes growing forever.
func TestAnnounceLogReindexIsRegistered(t *testing.T) {
	w := registeredWorker[AnnounceLogReindexArgs](t, &WorkerDeps{})
	if err := w.Work(context.Background(), &river.Job[AnnounceLogReindexArgs]{}); err != nil {
		t.Errorf("registered worker returned %v with nothing wired, want nil", err)
	}
}

func TestAnnounceLogReindexRebuilds(t *testing.T) {
	events := &mockAnnounceEventRepo{
		reindexResult: repository.ReindexResult{BytesBefore: 900, BytesAfter: 500},
	}
	handler := NewAnnounceLogReindexHandler(&WorkerDeps{AnnounceEventRepo: events})

	if err := handler(context.Background(), nil); err != nil {
		t.Fatalf("handler returned %v, want nil", err)
	}
	if events.reindexCalls != 1 {
		t.Errorf("Reindex called %d times, want 1", events.reindexCalls)
	}
}

// A rebuild that fails has to fail the task, not be swallowed into a log line.
// The whole point of scheduling it is that nobody is watching: an operator who
// never learns the rebuild stopped working finds out from the disk usage
// instead, months later and with no way to tell when it started.
func TestAnnounceLogReindexSurfacesAFailureAsATaskFailure(t *testing.T) {
	wantErr := errors.New("could not obtain lock")
	events := &mockAnnounceEventRepo{reindexErr: wantErr}
	handler := NewAnnounceLogReindexHandler(&WorkerDeps{AnnounceEventRepo: events})

	err := handler(context.Background(), nil)
	if !errors.Is(err, wantErr) {
		t.Fatalf("handler returned %v, want it to surface %v", err, wantErr)
	}
}

// Cleanup progress is reported even when the run then fails, because "dropped
// three invalid indexes, then failed again" and "failed again having done
// nothing" are different situations, and the log is the only place an operator
// can tell them apart. That is why the handler logs the count *before* it checks
// the error, and asserting on the log is the only way to hold that ordering —
// checking the returned error alone stays green if the two are swapped.
func TestAnnounceLogReindexStillReportsCleanupWhenTheRebuildFails(t *testing.T) {
	logs := captureLogs(t)
	events := &mockAnnounceEventRepo{
		reindexResult: repository.ReindexResult{LeftoversDropped: 3},
		reindexErr:    errors.New("out of disk"),
	}
	handler := NewAnnounceLogReindexHandler(&WorkerDeps{AnnounceEventRepo: events})

	if err := handler(context.Background(), nil); err == nil {
		t.Fatal("handler returned nil, want the rebuild error")
	}
	if events.reindexCalls != 1 {
		t.Errorf("Reindex called %d times, want 1", events.reindexCalls)
	}

	out := logs.String()
	if !strings.Contains(out, "dropped invalid indexes") {
		t.Errorf("the cleanup was not reported when the rebuild failed; log was:\n%s", out)
	}
	if !strings.Contains(out, "count=3") {
		t.Errorf("the cleanup count was not reported; log was:\n%s", out)
	}
}

// An unwired deployment must not fail the task forever. The announce log is
// optional wiring, and a worker that reports a failed job every month for a
// feature the operator never enabled is noise that trains people to ignore it.
func TestAnnounceLogReindexIsInertWithoutTheRepo(t *testing.T) {
	handler := NewAnnounceLogReindexHandler(&WorkerDeps{})
	if err := handler(context.Background(), nil); err != nil {
		t.Errorf("handler returned %v with no repo wired, want nil", err)
	}
}
