package postgres

import (
	"context"
	"fmt"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// An entry flagged staff-only is written with its flag and left out of a
// member's listing, whatever its event type; an entry inserted without the
// column is public, as every entry before migration 100 was.
func TestActivityLogStaffOnlyRows(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewActivityLogRepo(db)

	hidden := &model.ActivityLog{EventType: "torrent_uploaded", Message: "pending upload", StaffOnly: true}
	public := &model.ActivityLog{EventType: "torrent_uploaded", Message: "public upload"}
	for _, l := range []*model.ActivityLog{hidden, public} {
		if err := repo.Create(ctx, l); err != nil {
			t.Fatalf("create: %v", err)
		}
	}
	if _, err := db.Exec(`INSERT INTO activity_logs (event_type, message) VALUES ('torrent_uploaded', 'legacy')`); err != nil {
		t.Fatalf("raw insert: %v", err)
	}

	member, _, err := repo.List(ctx, repository.ListActivityLogsOptions{ExcludeStaffOnlyRows: true})
	if err != nil {
		t.Fatalf("member list: %v", err)
	}
	for _, l := range member {
		if l.Message == "pending upload" {
			t.Error("a staff-only entry reached a member's listing")
		}
	}
	if len(member) != 2 {
		t.Errorf("member sees %d entries, want the public and the legacy one", len(member))
	}

	staff, _, err := repo.List(ctx, repository.ListActivityLogsOptions{})
	if err != nil {
		t.Fatalf("staff list: %v", err)
	}
	found := false
	for _, l := range staff {
		if l.Message == "pending upload" {
			found = l.StaffOnly
		}
	}
	if !found {
		t.Error("staff listing lacks the entry, or lost its staff_only flag")
	}
}

// Hiding a torrent's entries reaches every upload, publish, edit and delete
// entry about it, and nothing about other torrents.
func TestActivityLogHideTorrentEntries(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewActivityLogRepo(db)

	meta := func(id int) *string { s := fmt.Sprintf(`{"torrent_id": %d}`, id); return &s }
	for _, l := range []*model.ActivityLog{
		{EventType: "torrent_published", Message: "alice uploaded torrent: T", Metadata: meta(7)},
		{EventType: "torrent_edited", Message: "alice edited torrent: T", Metadata: meta(7)},
		{EventType: "torrent_edited", Message: "alice edited torrent: U", Metadata: meta(8)},
	} {
		if err := repo.Create(ctx, l); err != nil {
			t.Fatalf("create: %v", err)
		}
	}
	if err := repo.HideTorrentEntries(ctx, 7); err != nil {
		t.Fatalf("hide: %v", err)
	}

	member, _, err := repo.List(ctx, repository.ListActivityLogsOptions{ExcludeStaffOnlyRows: true})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(member) != 1 || member[0].Message != "alice edited torrent: U" {
		t.Errorf("members see %v, want only the other torrent's entry", member)
	}
}
