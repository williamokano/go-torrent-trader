package listener

import (
	"context"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
	"github.com/williamokano/go-torrent-trader/backend/internal/testutil"
)

func TestBanEndsEveryOneOfTheMembersSessions(t *testing.T) {
	bus := event.NewInMemoryBus()
	sessions := testutil.NewMemorySessionStore()
	RegisterSessionRevocationListener(bus, sessions)

	now := time.Now()
	for _, s := range []*service.Session{
		{UserID: 7, AccessToken: "a1", RefreshToken: "r1", ExpiresAt: now.Add(time.Hour), RefreshExpiresAt: now.Add(time.Hour)},
		{UserID: 7, AccessToken: "a2", RefreshToken: "r2", ExpiresAt: now.Add(-time.Minute), RefreshExpiresAt: now.Add(time.Hour)},
		{UserID: 8, AccessToken: "a3", RefreshToken: "r3", ExpiresAt: now.Add(time.Hour), RefreshExpiresAt: now.Add(time.Hour)},
	} {
		if err := sessions.Create(s); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	bus.Publish(context.Background(), &event.UserBannedEvent{
		Base:   event.NewBase(event.UserBanned, event.Actor{ID: 1}),
		UserID: 7,
	})

	for _, token := range []string{"r1", "r2"} {
		if sess, _ := sessions.GetByRefreshToken(token); sess != nil {
			t.Errorf("the banned member's session %s survived the ban", token)
		}
	}
	if sess, _ := sessions.GetByRefreshToken("r3"); sess == nil {
		t.Error("another member's session was ended by someone else's ban")
	}
}

// Whatever survived the ban — a session the member "logged out" of while
// banned, which cannot be found to delete — is gone once they are let back in.
func TestUnbanEndsWhateverSessionsSurvivedTheBan(t *testing.T) {
	bus := event.NewInMemoryBus()
	sessions := testutil.NewMemorySessionStore()
	RegisterSessionRevocationListener(bus, sessions)

	now := time.Now()
	if err := sessions.Create(&service.Session{
		UserID: 7, AccessToken: "a", RefreshToken: "r",
		ExpiresAt: now.Add(time.Hour), RefreshExpiresAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	bus.Publish(context.Background(), &event.UserUnbannedEvent{
		Base: event.NewBase(event.UserUnbanned, event.Actor{ID: 1}), UserID: 7,
	})
	if sess, _ := sessions.GetByRefreshToken("r"); sess != nil {
		t.Error("a session from before the unban survived it")
	}
}
