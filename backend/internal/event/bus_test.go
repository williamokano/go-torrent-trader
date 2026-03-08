package event

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
)

func TestInMemoryBus_PublishCallsSubscribers(t *testing.T) {
	bus := NewInMemoryBus()

	var called int32
	bus.Subscribe(UserRegistered, func(_ context.Context, evt Event) error {
		atomic.AddInt32(&called, 1)
		if evt.EventType() != UserRegistered {
			t.Errorf("expected UserRegistered, got %s", evt.EventType())
		}
		return nil
	})

	bus.Publish(context.Background(), &UserRegisteredEvent{
		Base:   NewBase(UserRegistered, Actor{ID: 1, Username: "alice"}),
		UserID: 1,
	})

	if atomic.LoadInt32(&called) != 1 {
		t.Errorf("expected handler called once, got %d", called)
	}
}

func TestInMemoryBus_MultipleHandlers(t *testing.T) {
	bus := NewInMemoryBus()

	var count int32
	for i := 0; i < 3; i++ {
		bus.Subscribe(TorrentUploaded, func(_ context.Context, _ Event) error {
			atomic.AddInt32(&count, 1)
			return nil
		})
	}

	bus.Publish(context.Background(), &TorrentUploadedEvent{
		Base:        NewBase(TorrentUploaded, Actor{ID: 1, Username: "alice"}),
		TorrentID:   10,
		TorrentName: "test.torrent",
	})

	if atomic.LoadInt32(&count) != 3 {
		t.Errorf("expected 3 handlers called, got %d", count)
	}
}

func TestInMemoryBus_HandlerErrorDoesNotBlockOthers(t *testing.T) {
	bus := NewInMemoryBus()

	var secondCalled int32

	bus.Subscribe(UserRegistered, func(_ context.Context, _ Event) error {
		return errors.New("first handler fails")
	})
	bus.Subscribe(UserRegistered, func(_ context.Context, _ Event) error {
		atomic.AddInt32(&secondCalled, 1)
		return nil
	})

	bus.Publish(context.Background(), &UserRegisteredEvent{
		Base:   NewBase(UserRegistered, Actor{ID: 1, Username: "bob"}),
		UserID: 1,
	})

	if atomic.LoadInt32(&secondCalled) != 1 {
		t.Error("second handler should still be called after first fails")
	}
}

func TestInMemoryBus_NoSubscribers(t *testing.T) {
	bus := NewInMemoryBus()
	// Should not panic
	bus.Publish(context.Background(), &UserDeletedEvent{
		Base:     NewBase(UserDeleted, Actor{ID: 1, Username: "admin"}),
		UserID:   99,
		Username: "ghost",
	})
}

func TestInMemoryBus_DifferentEventTypes(t *testing.T) {
	bus := NewInMemoryBus()

	var uploadCalled, loginCalled int32

	bus.Subscribe(TorrentUploaded, func(_ context.Context, _ Event) error {
		atomic.AddInt32(&uploadCalled, 1)
		return nil
	})
	bus.Subscribe(UserBanned, func(_ context.Context, _ Event) error {
		atomic.AddInt32(&loginCalled, 1)
		return nil
	})

	bus.Publish(context.Background(), &TorrentUploadedEvent{
		Base:        NewBase(TorrentUploaded, Actor{ID: 1, Username: "alice"}),
		TorrentID:   1,
		TorrentName: "test",
	})

	if atomic.LoadInt32(&uploadCalled) != 1 {
		t.Error("upload handler should be called")
	}
	if atomic.LoadInt32(&loginCalled) != 0 {
		t.Error("login handler should NOT be called for upload event")
	}
}
