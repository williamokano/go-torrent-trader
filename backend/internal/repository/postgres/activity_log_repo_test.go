package postgres

import (
	"context"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

func TestActivityLogListExcludeEventTypes(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	repo := NewActivityLogRepo(db)
	ctx := context.Background()

	entries := []model.ActivityLog{
		{EventType: "user_registered", Message: "alice joined the site"},
		{EventType: "backup_created", Message: "operator created database backup: x.dump"},
		{EventType: "cheat_flagged", Message: "cheat flag raised for bob"},
	}
	for i := range entries {
		if err := repo.Create(ctx, &entries[i]); err != nil {
			t.Fatalf("create entry %d: %v", i, err)
		}
	}

	logs, total, err := repo.List(ctx, repository.ListActivityLogsOptions{})
	if err != nil {
		t.Fatalf("list without exclusions: %v", err)
	}
	if total != 3 || len(logs) != 3 {
		t.Fatalf("expected 3 entries without exclusions, got total=%d len=%d", total, len(logs))
	}

	logs, total, err = repo.List(ctx, repository.ListActivityLogsOptions{
		ExcludeEventTypes: []string{"backup_created", "cheat_flagged"},
	})
	if err != nil {
		t.Fatalf("list with exclusions: %v", err)
	}
	if total != 1 || len(logs) != 1 {
		t.Fatalf("expected 1 entry with exclusions, got total=%d len=%d", total, len(logs))
	}
	if logs[0].EventType != "user_registered" {
		t.Errorf("expected user_registered to survive, got %s", logs[0].EventType)
	}

	// Exclusion combines with an explicit event_type filter: asking for an
	// excluded type yields nothing.
	et := "backup_created"
	logs, total, err = repo.List(ctx, repository.ListActivityLogsOptions{
		EventType:         &et,
		ExcludeEventTypes: []string{"backup_created", "cheat_flagged"},
	})
	if err != nil {
		t.Fatalf("list with filter+exclusions: %v", err)
	}
	if total != 0 || len(logs) != 0 {
		t.Fatalf("expected 0 entries when filtering for an excluded type, got total=%d len=%d", total, len(logs))
	}
}
