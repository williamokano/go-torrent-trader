package worker

import (
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/hibiken/asynq"
)

// recordingScheduler captures what RegisterPeriodicTasks asks for.
type recordingScheduler struct {
	specs map[string]string
}

func (r *recordingScheduler) Register(cronspec string, task *asynq.Task, _ ...asynq.Option) (string, error) {
	if r.specs == nil {
		r.specs = map[string]string{}
	}
	r.specs[task.Type()] = cronspec
	return task.Type(), nil
}

// registered runs the real registration against a recorder, and separately
// against a real asynq.Scheduler so the cron specs are actually parsed —
// asynq.Register rejects a malformed spec, which a recorder never would.
func registered(t *testing.T) map[string]string {
	t.Helper()

	mr := miniredis.RunT(t)
	real := asynq.NewScheduler(asynq.RedisClientOpt{Addr: mr.Addr()}, nil)
	if err := RegisterPeriodicTasks(real); err != nil {
		t.Fatalf("RegisterPeriodicTasks against a real scheduler: %v", err)
	}

	rec := &recordingScheduler{}
	if err := RegisterPeriodicTasks(rec); err != nil {
		t.Fatalf("RegisterPeriodicTasks: %v", err)
	}
	return rec.specs
}

// RegisterPeriodicTasks had no test at all, and it is the half of the wiring the
// mux guards cannot reach. A handler registered in the mux that nothing ever
// schedules is exactly as dead as one the mux never registered — and the failure
// is quieter, because every handler test in this package still passes.
//
// The concrete regressions this catches: a registration block dropped in a
// rebase, and a cron spec typoed into something that still parses. "0 1 1 * *"
// mistyped as "0 1 * * 1" is every Monday rather than the first of the month,
// which is not an error asynq can report.
func TestRegisterPeriodicTasksRegistersEveryRecurringJob(t *testing.T) {
	got := registered(t)

	// Every recurring task, with the schedule it is supposed to run on. Written
	// out rather than derived, because the point is to pin the *intent* — a spec
	// derived from the code it checks would agree with any typo.
	want := map[string]string{
		TaskCleanupPeers:           "*/15 * * * *",
		TaskRecalcStats:            "0 * * * *",
		TaskRatioWarning:           "0 */6 * * *",
		TaskMaintenance:            "*/5 * * * *",
		TaskPromotion:              "0 5 * * *",
		TaskBonusAward:             "30 * * * *",
		TaskInviteDistribution:     "30 5 * * *",
		TaskDigest:                 "0 6 * * *",
		TaskAnnounceLogMaintenance: "15 4 * * *",
		TaskAnnounceLogReindex:     "0 1 1 * *",
		TaskHnREvaluate:            "45 * * * *",
	}

	for taskType, wantSpec := range want {
		spec, ok := got[taskType]
		if !ok {
			t.Errorf("%s is not scheduled — it will never run", taskType)
			continue
		}
		if spec != wantSpec {
			t.Errorf("%s scheduled on %q, want %q", taskType, spec, wantSpec)
		}
	}
	for taskType := range got {
		if _, expected := want[taskType]; !expected {
			t.Errorf("%s is scheduled but not listed here — add it, with the schedule it should run on", taskType)
		}
	}
}

// The rebuild is the one recurring job whose runtime scales with the whole
// table rather than a slice of it, so it is the only one that can plausibly
// still be running when the next job starts. Its slot is load-bearing, and the
// comment above it in scheduler.go claims it is clear of the heavy jobs.
func TestAnnounceReindexIsScheduledClearOfTheHeavyJobs(t *testing.T) {
	all := registered(t)
	reindex := all[TaskAnnounceLogReindex]
	daily := map[string]string{}
	for taskType, spec := range all {
		if taskType != TaskAnnounceLogReindex {
			daily[taskType] = spec
		}
	}
	if reindex == "" {
		t.Fatal("the rebuild is not scheduled at all")
	}

	// Monthly, not daily: a full rebuild every night would be pure cost, since
	// the growth it corrects is gradual.
	if fields := len(strings.Fields(reindex)); fields != 5 {
		t.Fatalf("unexpected cron spec %q", reindex)
	}
	if dayOfMonth := strings.Fields(reindex)[2]; dayOfMonth == "*" {
		t.Errorf("the rebuild runs on every day-of-month (%q); it is meant to be monthly", reindex)
	}

	// And it must not share an hour with the heavy nightly jobs.
	reindexHour := strings.Fields(reindex)[1]
	for _, taskType := range []string{TaskAnnounceLogMaintenance, TaskPromotion, TaskInviteDistribution, TaskDigest} {
		spec, ok := daily[taskType]
		if !ok {
			continue
		}
		if strings.Fields(spec)[1] == reindexHour {
			t.Errorf("the rebuild shares hour %q with %s (%q); it is the one job that can overrun",
				reindexHour, taskType, spec)
		}
	}
}
