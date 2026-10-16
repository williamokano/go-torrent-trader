package listener

import (
	"context"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// RegisterSessionRevocationListener ends every session of a member who is
// banned.
//
// UserBanned is published by every path that bans: the admin edit, the
// warning ladder, and the automatic ratio and hit-and-run bans. Quick-ban
// publishes its own event and deletes the sessions itself.
//
// It is the tidy half of the fix for #291, not the guarantee. The session
// store already refuses any session whose account is disabled, on every
// lookup, whichever code path disabled it. What deleting adds is that a ban
// means "logged out everywhere" rather than "locked out for now": when a
// temporary ban expires, the member signs in again instead of finding their
// old devices quietly working.
//
// It acts on the way back in as well: an unban deletes whatever sessions are
// left. That covers a ban that did not announce itself, and a member who
// logged out while banned — which cannot delete the session, because a
// disabled account's session no longer resolves — so nothing from before the
// ban can come back to life when the account is re-enabled.
func RegisterSessionRevocationListener(bus event.Bus, sessions service.SessionStore) {
	bus.Subscribe(event.UserBanned, func(_ context.Context, evt event.Event) error {
		if e, ok := evt.(*event.UserBannedEvent); ok {
			sessions.DeleteByUserID(e.UserID)
		}
		return nil
	})
	bus.Subscribe(event.UserUnbanned, func(_ context.Context, evt event.Event) error {
		if e, ok := evt.(*event.UserUnbannedEvent); ok {
			sessions.DeleteByUserID(e.UserID)
		}
		return nil
	})
}
