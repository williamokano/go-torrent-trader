// Package listener contains event handlers (listeners) that react to domain
// events. Listeners are wired in main.go and bridge events to services or
// repositories. Services only publish events — they never subscribe.
package listener

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// RegisterActivityLogListeners subscribes event listeners that write entries
// to the product activity log. Each event type gets its own listener — a
// closure that knows how to build the log message for that specific event.
func RegisterActivityLogListeners(bus event.Bus, logSvc *service.ActivityLogService) {
	listen := func(evtType event.Type, buildMsg func(event.Event) (string, event.Actor)) {
		bus.Subscribe(evtType, func(ctx context.Context, evt event.Event) error {
			msg, actor := buildMsg(evt)
			metadata := marshalMetadata(evt)
			entry := &model.ActivityLog{
				EventType: string(evt.EventType()),
				ActorID:   actor.ID,
				Message:   msg,
				Metadata:  metadata,
			}
			if err := logSvc.Create(ctx, entry); err != nil {
				slog.Error("failed to write activity log", "event_type", string(evt.EventType()), "error", err)
				return fmt.Errorf("write activity log: %w", err)
			}
			return nil
		})
	}

	listen(event.UserRegistered, func(evt event.Event) (string, event.Actor) {
		e := evt.(*event.UserRegisteredEvent)
		return fmt.Sprintf("%s joined the site", e.Actor.Username), e.Actor
	})

	listen(event.UserBanned, func(evt event.Event) (string, event.Actor) {
		e := evt.(*event.UserBannedEvent)
		return fmt.Sprintf("%s banned %s", e.Actor.Username, e.Username), e.Actor
	})

	listen(event.UserWarned, func(evt event.Event) (string, event.Actor) {
		e := evt.(*event.UserWarnedEvent)
		return fmt.Sprintf("%s warned %s", e.Actor.Username, e.Username), e.Actor
	})

	listen(event.UserUnbanned, func(evt event.Event) (string, event.Actor) {
		e := evt.(*event.UserUnbannedEvent)
		return fmt.Sprintf("%s unbanned %s", e.Actor.Username, e.Username), e.Actor
	})

	listen(event.UserUnwarned, func(evt event.Event) (string, event.Actor) {
		e := evt.(*event.UserUnwarnedEvent)
		return fmt.Sprintf("%s removed warning from %s", e.Actor.Username, e.Username), e.Actor
	})

	listen(event.UserGroupChanged, func(evt event.Event) (string, event.Actor) {
		e := evt.(*event.UserGroupChangedEvent)
		return fmt.Sprintf("%s changed %s from %s to %s", e.Actor.Username, e.Username, e.OldGroupName, e.NewGroupName), e.Actor
	})

	listen(event.UserDeleted, func(evt event.Event) (string, event.Actor) {
		e := evt.(*event.UserDeletedEvent)
		return fmt.Sprintf("%s deleted %s", e.Actor.Username, e.Username), e.Actor
	})

	listen(event.TorrentUploaded, func(evt event.Event) (string, event.Actor) {
		e := evt.(*event.TorrentUploadedEvent)
		return fmt.Sprintf("%s uploaded torrent: %s", e.Actor.Username, e.TorrentName), e.Actor
	})

	listen(event.TorrentEdited, func(evt event.Event) (string, event.Actor) {
		e := evt.(*event.TorrentEditedEvent)
		return fmt.Sprintf("%s edited torrent: %s", e.Actor.Username, e.TorrentName), e.Actor
	})

	listen(event.TorrentDeleted, func(evt event.Event) (string, event.Actor) {
		e := evt.(*event.TorrentDeletedEvent)
		return fmt.Sprintf("%s deleted torrent: %s", e.Actor.Username, e.TorrentName), e.Actor
	})

	listen(event.TorrentReported, func(evt event.Event) (string, event.Actor) {
		e := evt.(*event.TorrentReportedEvent)
		return fmt.Sprintf("%s reported torrent #%d", e.Actor.Username, e.TorrentID), e.Actor
	})

	listen(event.ReportResolved, func(evt event.Event) (string, event.Actor) {
		e := evt.(*event.ReportResolvedEvent)
		return fmt.Sprintf("%s resolved report #%d", e.Actor.Username, e.ReportID), e.Actor
	})

	listen(event.CommentCreated, func(evt event.Event) (string, event.Actor) {
		e := evt.(*event.CommentCreatedEvent)
		return fmt.Sprintf("%s commented on torrent #%d", e.Actor.Username, e.TorrentID), e.Actor
	})

	listen(event.CommentDeleted, func(evt event.Event) (string, event.Actor) {
		e := evt.(*event.CommentDeletedEvent)
		return fmt.Sprintf("%s deleted comment on torrent #%d", e.Actor.Username, e.TorrentID), e.Actor
	})

	listen(event.ReseedRequested, func(evt event.Event) (string, event.Actor) {
		e := evt.(*event.ReseedRequestedEvent)
		return fmt.Sprintf("%s requested reseed for: %s", e.Actor.Username, e.TorrentName), e.Actor
	})

	listen(event.InviteSent, func(evt event.Event) (string, event.Actor) {
		e := evt.(*event.InviteSentEvent)
		return fmt.Sprintf("%s sent an invite to %s", e.Actor.Username, e.Email), e.Actor
	})

	listen(event.InviteRedeemed, func(evt event.Event) (string, event.Actor) {
		e := evt.(*event.InviteRedeemedEvent)
		return fmt.Sprintf("invite #%d was redeemed by user #%d", e.InviteID, e.InviteeID), e.Actor
	})
}

func marshalMetadata(evt event.Event) *string {
	data, err := json.Marshal(evt)
	if err != nil {
		return nil
	}
	s := string(data)
	return &s
}
