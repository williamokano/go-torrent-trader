package worker

import (
	"strings"
	"testing"
	"time"

	"github.com/robfig/cron/v3"
)

// registered returns every periodic job's cron spec, keyed by kind — after
// checking that the real River schedule builds from them, which is what parses
// each spec: a recorder alone would accept a malformed one.
func registered(t *testing.T) map[string]string {
	t.Helper()

	if _, err := PeriodicJobs(""); err != nil {
		t.Fatalf("PeriodicJobs: %v", err)
	}
	specs := map[string]string{}
	for _, spec := range periodicSpecs("") {
		if _, dup := specs[spec.args.Kind()]; dup {
			t.Fatalf("%s is scheduled twice", spec.args.Kind())
		}
		specs[spec.args.Kind()] = spec.cron
	}
	return specs
}

// The schedule is the half of the wiring the worker registration guards cannot
// reach. A handler registered in the mux that nothing ever
// schedules is exactly as dead as one the mux never registered — and the failure
// is quieter, because every handler test in this package still passes.
//
// The concrete regressions this catches: a registration block dropped in a
// rebase, and a cron spec typoed into something that still parses. "0 1 1 * *"
// mistyped as "0 1 * * 1" is every Monday rather than the first of the month,
// which is not an error a cron parser can report.
func TestPeriodicJobsScheduleEveryRecurringJob(t *testing.T) {
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

// Scheduled backups are opt-in: no BACKUP_SCHEDULE_CRON, no backup job.
func TestBackupIsScheduledOnlyWhenConfigured(t *testing.T) {
	for _, spec := range periodicSpecs("") {
		if spec.args.Kind() == TaskBackupDatabase {
			t.Fatal("a backup is scheduled with no BACKUP_SCHEDULE_CRON set")
		}
	}

	var got string
	for _, spec := range periodicSpecs("0 3 * * *") {
		if spec.args.Kind() == TaskBackupDatabase {
			got = spec.cron
		}
	}
	if got != "0 3 * * *" {
		t.Errorf("backup scheduled on %q, want the configured %q", got, "0 3 * * *")
	}
}

// A malformed BACKUP_SCHEDULE_CRON must stop the server from starting, not
// leave it running with backups silently off.
func TestPeriodicJobsRejectsAMalformedBackupCron(t *testing.T) {
	if _, err := PeriodicJobs("every night please"); err == nil {
		t.Fatal("PeriodicJobs accepted a malformed backup cron")
	}
}

// The schedule's comments reason in UTC ("hours past midnight UTC on purpose"),
// and asynq's scheduler ran in UTC. robfig/cron would otherwise use the
// process's local zone, so a host set to anything else would quietly shift
// every job.
func TestCronSpecsAreEvaluatedInUTC(t *testing.T) {
	schedule, err := parseCron("15 4 * * *")
	if err != nil {
		t.Fatalf("parseCron: %v", err)
	}
	spec, ok := schedule.(*cron.SpecSchedule)
	if !ok {
		t.Fatalf("schedule is %T, want *cron.SpecSchedule", schedule)
	}
	if spec.Location != time.UTC {
		t.Errorf("schedule location = %v, want UTC", spec.Location)
	}

	next := schedule.Next(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if want := time.Date(2026, 1, 1, 4, 15, 0, 0, time.UTC); !next.Equal(want) {
		t.Errorf("next run = %v, want %v", next, want)
	}
}

// An operator who writes an explicit zone into BACKUP_SCHEDULE_CRON gets it.
func TestCronSpecsKeepAnExplicitZone(t *testing.T) {
	schedule, err := parseCron("CRON_TZ=America/New_York 0 3 * * *")
	if err != nil {
		t.Fatalf("parseCron: %v", err)
	}
	spec, ok := schedule.(*cron.SpecSchedule)
	if !ok {
		t.Fatalf("schedule is %T, want *cron.SpecSchedule", schedule)
	}
	if spec.Location.String() != "America/New_York" {
		t.Errorf("schedule location = %v, want the configured America/New_York", spec.Location)
	}
}

// A job that is scheduled, or enqueued by name, but has no worker sits in
// river_job forever and never runs. Every handler test would still pass.
func TestEveryJobKindHasAWorker(t *testing.T) {
	workers := map[string]bool{}
	for _, r := range registrations(&WorkerDeps{}) {
		if workers[r.kind] {
			t.Errorf("%s is registered twice", r.kind)
		}
		workers[r.kind] = true
	}

	kinds := []string{TaskSendEmail, TaskConnectorDrain}
	for _, spec := range periodicSpecs("0 3 * * *") {
		kinds = append(kinds, spec.args.Kind())
	}
	for _, kind := range kinds {
		if !workers[kind] {
			t.Errorf("%s is enqueued or scheduled but has no worker — it will never run", kind)
		}
	}

	if _, err := NewWorkers(&WorkerDeps{}); err != nil {
		t.Fatalf("NewWorkers: %v", err)
	}
}

// Each periodic insert is unique within its own period, so a tick that fires
// twice (a leader handover between nodes whose clocks disagree) is one job.
// The bonus award is why this matters: a second run in the hour pays twice.
func TestPeriodicJobsDedupeWithinTheirPeriod(t *testing.T) {
	cases := map[string]time.Duration{
		"*/5 * * * *": 5 * time.Minute,
		"30 * * * *":  time.Hour,
		"0 */6 * * *": 6 * time.Hour,
		"0 5 * * *":   24 * time.Hour,
		// Monthly is capped: completed jobs are pruned within the day anyway.
		"0 1 1 * *": maxUniquePeriod,
	}
	for spec, want := range cases {
		schedule, err := parseCron(spec)
		if err != nil {
			t.Fatalf("parseCron(%q): %v", spec, err)
		}
		if got := uniquePeriod(schedule); got != want {
			t.Errorf("uniquePeriod(%q) = %s, want %s", spec, got, want)
		}
	}
}
