package listener

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

type mockActivityLogRepo struct {
	mu   sync.Mutex
	logs []*model.ActivityLog
	id   int64
}

func (m *mockActivityLogRepo) Create(_ context.Context, log *model.ActivityLog) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.id++
	log.ID = m.id
	m.logs = append(m.logs, log)
	return nil
}

func (m *mockActivityLogRepo) List(_ context.Context, opts repository.ListActivityLogsOptions) ([]model.ActivityLog, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var filtered []model.ActivityLog
	for _, l := range m.logs {
		if opts.EventType != nil && l.EventType != *opts.EventType {
			continue
		}
		if opts.ActorID != nil && (l.ActorID == nil || *l.ActorID != *opts.ActorID) {
			continue
		}
		filtered = append(filtered, *l)
	}

	total := int64(len(filtered))
	page := opts.Page
	if page <= 0 {
		page = 1
	}
	perPage := opts.PerPage
	if perPage <= 0 {
		perPage = 25
	}
	start := (page - 1) * perPage
	if start >= len(filtered) {
		return nil, total, nil
	}
	end := start + perPage
	if end > len(filtered) {
		end = len(filtered)
	}
	return filtered[start:end], total, nil
}

type mockUserRepo struct{}

func (r *mockUserRepo) GetByID(_ context.Context, id int64) (*model.User, error) {
	return &model.User{ID: id, Username: fmt.Sprintf("user%d", id)}, nil
}
func (r *mockUserRepo) GetByUsername(context.Context, string) (*model.User, error) {
	return nil, nil
}
func (r *mockUserRepo) GetByUsernames(context.Context, []string) ([]model.User, error) {
	return nil, nil
}
func (r *mockUserRepo) GetByEmail(context.Context, string) (*model.User, error) {
	return nil, nil
}
func (r *mockUserRepo) GetByPasskey(context.Context, string) (*model.User, error) {
	return nil, nil
}
func (r *mockUserRepo) Count(context.Context) (int64, error)      { return 0, nil }
func (r *mockUserRepo) Create(context.Context, *model.User) error { return nil }
func (r *mockUserRepo) Update(context.Context, *model.User) error { return nil }
func (r *mockUserRepo) IncrementStats(context.Context, int64, int64, int64) error {
	return nil
}
func (r *mockUserRepo) List(context.Context, repository.ListUsersOptions) ([]model.User, int64, error) {
	return nil, 0, nil
}
func (r *mockUserRepo) ListStaff(context.Context) ([]model.User, error) { return nil, nil }
func (r *mockUserRepo) UpdateLastAccess(context.Context, int64) error   { return nil }

func setup() (*mockActivityLogRepo, event.Bus) {
	repo := &mockActivityLogRepo{}
	svc := service.NewActivityLogService(repo)
	bus := event.NewInMemoryBus()
	RegisterActivityLogListeners(bus, svc, &mockUserRepo{})
	return repo, bus
}

func TestListener_UserRegistered(t *testing.T) {
	repo, bus := setup()

	bus.Publish(context.Background(), &event.UserRegisteredEvent{
		Base:   event.NewBase(event.UserRegistered, event.Actor{ID: 1, Username: "alice"}),
		UserID: 1,
	})

	if len(repo.logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(repo.logs))
	}
	if repo.logs[0].EventType != "user_registered" {
		t.Errorf("expected user_registered, got %s", repo.logs[0].EventType)
	}
	if repo.logs[0].Message != "alice joined the site" {
		t.Errorf("unexpected message: %s", repo.logs[0].Message)
	}
	if repo.logs[0].ActorID == nil || *repo.logs[0].ActorID != 1 {
		t.Errorf("expected actor_id 1, got %v", repo.logs[0].ActorID)
	}
}

func TestListener_InviteRevoked(t *testing.T) {
	repo, bus := setup()

	bus.Publish(context.Background(), &event.InviteRevokedEvent{
		Base:            event.NewBase(event.InviteRevoked, event.Actor{ID: 2, Username: "admin"}),
		InviteID:        7,
		InviterID:       1,
		InviterUsername: "alice",
	})

	if len(repo.logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(repo.logs))
	}
	if repo.logs[0].EventType != "invite_revoked" {
		t.Errorf("expected invite_revoked, got %s", repo.logs[0].EventType)
	}
	if repo.logs[0].Message != "admin revoked an invite issued by alice" {
		t.Errorf("unexpected message: %s", repo.logs[0].Message)
	}
}

func TestListener_InviteAutoGranted(t *testing.T) {
	repo, bus := setup()

	bus.Publish(context.Background(), &event.InviteAutoGrantedEvent{
		Base:     event.NewBase(event.InviteAutoGranted, event.Actor{ID: 0, Username: "System"}),
		UserID:   3,
		Username: "bob",
	})

	if len(repo.logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(repo.logs))
	}
	if repo.logs[0].EventType != "invite_auto_granted" {
		t.Errorf("expected invite_auto_granted, got %s", repo.logs[0].EventType)
	}
	if repo.logs[0].Message != "System granted an invite to bob via auto-distribution" {
		t.Errorf("unexpected message: %s", repo.logs[0].Message)
	}
	// A system actor (ID 0) must not be recorded as an actor_id, mirroring how
	// nil-issuedBy restriction events resolve to no actor.
	if repo.logs[0].ActorID != nil {
		t.Errorf("expected nil actor_id for a system event, got %v", *repo.logs[0].ActorID)
	}
}

func TestListener_InviteAutoGranted_ResolvesUsernameWhenEmpty(t *testing.T) {
	repo, bus := setup()

	bus.Publish(context.Background(), &event.InviteAutoGrantedEvent{
		Base:   event.NewBase(event.InviteAutoGranted, event.Actor{ID: 0, Username: "System"}),
		UserID: 9,
	})

	if len(repo.logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(repo.logs))
	}
	if repo.logs[0].Message != "System granted an invite to user9 via auto-distribution" {
		t.Errorf("unexpected message: %s", repo.logs[0].Message)
	}
}

func TestListener_TorrentUploaded(t *testing.T) {
	repo, bus := setup()

	bus.Publish(context.Background(), &event.TorrentUploadedEvent{
		Base:        event.NewBase(event.TorrentUploaded, event.Actor{ID: 5, Username: "bob"}),
		TorrentID:   10,
		TorrentName: "My.Torrent.2026",
	})

	if len(repo.logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(repo.logs))
	}
	if repo.logs[0].Message != "bob uploaded torrent: My.Torrent.2026" {
		t.Errorf("unexpected message: %s", repo.logs[0].Message)
	}
	if repo.logs[0].Metadata == nil {
		t.Error("expected metadata to be set")
	}
}

func TestListener_MultipleEventTypes(t *testing.T) {
	repo, bus := setup()

	events := []event.Event{
		&event.TorrentDeletedEvent{Base: event.NewBase(event.TorrentDeleted, event.Actor{ID: 2, Username: "admin"}), TorrentID: 5, TorrentName: "deleted.torrent"},
		&event.ReportResolvedEvent{Base: event.NewBase(event.ReportResolved, event.Actor{ID: 3, Username: "mod"}), ReportID: 7},
		&event.CommentCreatedEvent{Base: event.NewBase(event.CommentCreated, event.Actor{ID: 1, Username: "bob"}), CommentID: 10, TorrentID: 5, TorrentName: "Ubuntu 22.04"},
	}

	for _, evt := range events {
		bus.Publish(context.Background(), evt)
	}

	if len(repo.logs) != 3 {
		t.Errorf("expected 3 logs, got %d", len(repo.logs))
	}
}

func TestListener_ActorCarriesUsername(t *testing.T) {
	repo, bus := setup()

	bus.Publish(context.Background(), &event.TorrentEditedEvent{
		Base:        event.NewBase(event.TorrentEdited, event.Actor{ID: 42, Username: "editor"}),
		TorrentID:   7,
		TorrentName: "Edited Torrent",
	})

	if len(repo.logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(repo.logs))
	}
	if repo.logs[0].Message != "editor edited torrent: Edited Torrent" {
		t.Errorf("unexpected message: %s", repo.logs[0].Message)
	}
	if repo.logs[0].ActorID == nil || *repo.logs[0].ActorID != 42 {
		t.Errorf("expected actor_id 42, got %v", repo.logs[0].ActorID)
	}
}

func TestListener_BackupCreated(t *testing.T) {
	repo, bus := setup()

	bus.Publish(context.Background(), &event.BackupCreatedEvent{
		Base: event.NewBase(event.BackupCreated, event.Actor{ID: 1, Username: "alice"}),
		Name: "backup-20260714T031500Z-a1b2c3d4.dump",
		Size: 4096,
	})

	if len(repo.logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(repo.logs))
	}
	if repo.logs[0].EventType != "backup_created" {
		t.Errorf("expected backup_created, got %s", repo.logs[0].EventType)
	}
	if repo.logs[0].Message != "alice created database backup: backup-20260714T031500Z-a1b2c3d4.dump" {
		t.Errorf("unexpected message: %s", repo.logs[0].Message)
	}
}

func TestListener_BackupCreatedByScheduler(t *testing.T) {
	repo, bus := setup()

	// Scheduled backups have no human actor.
	bus.Publish(context.Background(), &event.BackupCreatedEvent{
		Base: event.NewBase(event.BackupCreated, event.Actor{Username: "system"}),
		Name: "backup-20260714T031500Z-a1b2c3d4.dump",
		Size: 4096,
	})

	if len(repo.logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(repo.logs))
	}
	if repo.logs[0].ActorID != nil {
		t.Errorf("expected nil actor_id for a scheduled backup, got %v", *repo.logs[0].ActorID)
	}
	if repo.logs[0].Message != "system created database backup: backup-20260714T031500Z-a1b2c3d4.dump" {
		t.Errorf("unexpected message: %s", repo.logs[0].Message)
	}
}

func TestListener_BackupDeleted(t *testing.T) {
	repo, bus := setup()

	bus.Publish(context.Background(), &event.BackupDeletedEvent{
		Base: event.NewBase(event.BackupDeleted, event.Actor{ID: 2, Username: "admin"}),
		Name: "backup-20260714T031500Z-a1b2c3d4.dump",
	})

	if len(repo.logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(repo.logs))
	}
	if repo.logs[0].Message != "admin deleted database backup: backup-20260714T031500Z-a1b2c3d4.dump" {
		t.Errorf("unexpected message: %s", repo.logs[0].Message)
	}
}

func TestListener_BackupDownloaded(t *testing.T) {
	repo, bus := setup()

	bus.Publish(context.Background(), &event.BackupDownloadedEvent{
		Base: event.NewBase(event.BackupDownloaded, event.Actor{ID: 3, Username: "admin"}),
		Name: "backup-20260714T031500Z-a1b2c3d4.dump",
	})

	if len(repo.logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(repo.logs))
	}
	if repo.logs[0].EventType != "backup_downloaded" {
		t.Errorf("expected backup_downloaded, got %s", repo.logs[0].EventType)
	}
	if repo.logs[0].Message != "admin downloaded database backup: backup-20260714T031500Z-a1b2c3d4.dump" {
		t.Errorf("unexpected message: %s", repo.logs[0].Message)
	}
}

// TorrentPublished is deliberately not logged: the activity log already records
// torrent_uploaded and torrent_moderated for the same two transitions, so a
// third row per publish would be pure noise. Keeping the event out also means
// its payload — which carries the anonymous flag — is never serialized into
// activity_logs.metadata at all.
func TestListener_TorrentPublishedIsNotLogged(t *testing.T) {
	repo, bus := setup()

	bus.Publish(context.Background(), &event.TorrentPublishedEvent{
		Base:         event.NewBase(event.TorrentPublished, event.Actor{ID: 9, Username: "mod"}),
		TorrentID:    42,
		Name:         "Some.Release-GROUP",
		UploaderID:   5,
		UploaderName: "alice",
	})

	if len(repo.logs) != 0 {
		t.Fatalf("torrent_published wrote %d activity log entries, want 0", len(repo.logs))
	}
}

// Chat deletions had no listener at all: both events were published and nothing
// subscribed, so a removed message — including a site announcement, which became
// deletable later — left no trace of who removed it.
func TestListener_ChatMessageDeleted(t *testing.T) {
	repo, bus := setup()

	// The service publishes the actor as an ID with no username, so the listener
	// has to resolve it. Without that the entry reads " deleted chat message #9".
	bus.Publish(context.Background(), &event.ChatMessageDeletedEvent{
		Base:      event.NewBase(event.ChatMessageDeleted, event.Actor{ID: 4}),
		MessageID: 9,
	})

	if len(repo.logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(repo.logs))
	}
	if got := repo.logs[0].EventType; got != "chat_message_deleted" {
		t.Errorf("event type = %s, want chat_message_deleted", got)
	}
	if want := "user4 deleted chat message #9"; repo.logs[0].Message != want {
		t.Errorf("message = %q, want %q", repo.logs[0].Message, want)
	}
	if repo.logs[0].ActorID == nil || *repo.logs[0].ActorID != 4 {
		t.Errorf("actor_id = %v, want 4", repo.logs[0].ActorID)
	}
}

func TestListener_ChatUserMessagesDeleted(t *testing.T) {
	repo, bus := setup()

	bus.Publish(context.Background(), &event.ChatUserMessagesDeletedEvent{
		Base:         event.NewBase(event.ChatUserMessagesDeleted, event.Actor{ID: 4}),
		TargetUserID: 11,
		Count:        12,
	})

	if len(repo.logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(repo.logs))
	}
	if want := "user4 deleted 12 chat messages from user11"; repo.logs[0].Message != want {
		t.Errorf("message = %q, want %q", repo.logs[0].Message, want)
	}
}

func TestListener_ChatUserMessagesDeleted_SingularReadsNaturally(t *testing.T) {
	repo, bus := setup()

	bus.Publish(context.Background(), &event.ChatUserMessagesDeletedEvent{
		Base:         event.NewBase(event.ChatUserMessagesDeleted, event.Actor{ID: 4}),
		TargetUserID: 11,
		Count:        1,
	})

	if want := "user4 deleted 1 chat message from user11"; repo.logs[0].Message != want {
		t.Errorf("message = %q, want %q", repo.logs[0].Message, want)
	}
}

// Deleting messages is moderation against one member, classified with warnings
// and restrictions rather than with mutes. Announcing it site-wide amplifies
// whatever was removed. Mutes stay public on purpose — see staff_only.go.
func TestChatDeletionsAreStaffOnlyButMutesAreNot(t *testing.T) {
	staffOnly := []event.Type{event.ChatMessageDeleted, event.ChatUserMessagesDeleted}
	for _, typ := range staffOnly {
		if !event.IsStaffOnly(typ) {
			t.Errorf("%s should be staff-only", typ)
		}
	}
	public := []event.Type{event.ChatUserMuted, event.ChatUserUnmuted}
	for _, typ := range public {
		if event.IsStaffOnly(typ) {
			t.Errorf("%s is public by design; changing it needs a deliberate decision", typ)
		}
	}
}
