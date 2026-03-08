package listener

import (
	"context"
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
		if opts.ActorID != nil && l.ActorID != *opts.ActorID {
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

func setup() (*mockActivityLogRepo, event.Bus) {
	repo := &mockActivityLogRepo{}
	svc := service.NewActivityLogService(repo)
	bus := event.NewInMemoryBus()
	RegisterActivityLogListeners(bus, svc)
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
	if repo.logs[0].ActorID != 1 {
		t.Errorf("expected actor_id 1, got %d", repo.logs[0].ActorID)
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
		&event.CommentCreatedEvent{Base: event.NewBase(event.CommentCreated, event.Actor{ID: 1, Username: "bob"}), CommentID: 10, TorrentID: 5},
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
	if repo.logs[0].ActorID != 42 {
		t.Errorf("expected actor_id 42, got %d", repo.logs[0].ActorID)
	}
}
