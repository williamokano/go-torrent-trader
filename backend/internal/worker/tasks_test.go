package worker

import (
	"encoding/json"
	"testing"

	"github.com/riverqueue/river"
)

// Each job's attempt budget, written out rather than derived. River's default
// is 25 attempts, so an InsertOpts method lost in a refactor would not fail
// anything — it would quietly turn a "retry once" job into twenty-five runs.
// asynq counted retries rather than attempts, so MaxRetry(1) is MaxAttempts 2.
func TestJobsCarryTheirAttemptBudget(t *testing.T) {
	type jobWithOpts interface {
		river.JobArgs
		river.JobArgsWithInsertOpts
	}
	cases := []struct {
		args jobWithOpts
		want int
	}{
		{SendEmailArgs{}, 4},
		{CleanupPeersArgs{}, 2},
		{RatioWarningArgs{}, 2},
		{MaintenanceArgs{}, 2},
		{BackupArgs{}, 2},
		{PromotionArgs{}, 2},
		{BonusAwardArgs{}, 2},
		{InviteDistributionArgs{}, 2},
		{DigestArgs{}, 2},
		{AnnounceLogMaintenanceArgs{}, 2},
		{HnREvaluateArgs{}, 2},
		// One attempt: a failed rebuild waits for next month (see
		// AnnounceLogReindexArgs), and a drain's retries live in the database.
		{AnnounceLogReindexArgs{}, 1},
		{ConnectorDrainArgs{}, 1},
	}
	for _, tc := range cases {
		if got := tc.args.InsertOpts().MaxAttempts; got != tc.want {
			t.Errorf("%s: MaxAttempts = %d, want %d", tc.args.Kind(), got, tc.want)
		}
	}
}

// The email args are what sits in river_job.args, so their JSON names are the
// contract with any job already queued when a new version deploys.
func TestSendEmailArgsJSONShape(t *testing.T) {
	raw, err := json.Marshal(SendEmailArgs{To: "user@example.com", Subject: "Welcome", Body: "Hello!"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]string
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	want := map[string]string{"to": "user@example.com", "subject": "Welcome", "body": "Hello!"}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
	if len(got) != len(want) {
		t.Errorf("args = %v, want exactly %v", got, want)
	}
}

// Kinds are what River stores and routes on. They are kept from the asynq era,
// and a typo here would strand any job of that kind already queued.
func TestJobKinds(t *testing.T) {
	cases := map[string]river.JobArgs{
		TaskSendEmail:              SendEmailArgs{},
		TaskCleanupPeers:           CleanupPeersArgs{},
		TaskRatioWarning:           RatioWarningArgs{},
		TaskMaintenance:            MaintenanceArgs{},
		TaskBackupDatabase:         BackupArgs{},
		TaskPromotion:              PromotionArgs{},
		TaskBonusAward:             BonusAwardArgs{},
		TaskInviteDistribution:     InviteDistributionArgs{},
		TaskDigest:                 DigestArgs{},
		TaskAnnounceLogMaintenance: AnnounceLogMaintenanceArgs{},
		TaskAnnounceLogReindex:     AnnounceLogReindexArgs{},
		TaskHnREvaluate:            HnREvaluateArgs{},
		TaskConnectorDrain:         ConnectorDrainArgs{},
	}
	for want, args := range cases {
		if got := args.Kind(); got != want {
			t.Errorf("%T.Kind() = %q, want %q", args, got, want)
		}
	}
}
