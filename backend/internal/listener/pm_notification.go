package listener

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// RegisterPMNotificationListener subscribes to MessageSent events and pushes
// real-time unread count updates to the receiver via WebSocket.
func RegisterPMNotificationListener(
	bus event.Bus,
	messageRepo repository.MessageRepository,
	sendToUser func(userID int64, payload []byte),
) {
	bus.Subscribe(event.MessageSent, func(ctx context.Context, evt event.Event) error {
		e := evt.(*event.MessageSentEvent)

		countCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		unread, err := messageRepo.CountUnread(countCtx, e.ReceiverID)
		if err != nil {
			slog.Error("pm_notification: failed to count unread messages",
				"receiver_id", e.ReceiverID,
				"error", err,
			)
			return err
		}

		payload, err := json.Marshal(map[string]interface{}{
			"type":         "pm_notification",
			"unread_count": unread,
		})
		if err != nil {
			slog.Error("pm_notification: failed to marshal payload", "error", err)
			return err
		}

		sendToUser(e.ReceiverID, payload)
		slog.Debug("pm_notification: sent unread count to user",
			"receiver_id", e.ReceiverID,
			"unread_count", unread,
		)
		return nil
	})
}
