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
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// RegisterActivityLogListeners subscribes event listeners that write entries
// to the product activity log. Each event type gets its own listener — a
// closure that knows how to build the log message for that specific event.
func RegisterActivityLogListeners(bus event.Bus, logSvc *service.ActivityLogService, userRepo repository.UserRepository) {
	listen := func(evtType event.Type, buildMsg func(event.Event) (string, event.Actor)) {
		bus.Subscribe(evtType, func(ctx context.Context, evt event.Event) error {
			msg, actor := buildMsg(evt)
			metadata := marshalMetadata(evt)
			var actorID *int64
			if actor.ID != 0 {
				actorID = &actor.ID
			}
			entry := &model.ActivityLog{
				EventType: string(evt.EventType()),
				ActorID:   actorID,
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
		actor := resolveActor(userRepo, e.Actor)
		torrentName := nameOrID("torrent", e.TorrentName, e.TorrentID)
		return fmt.Sprintf("%s reported %s", actor, torrentName), e.Actor
	})

	listen(event.ReportResolved, func(evt event.Event) (string, event.Actor) {
		e := evt.(*event.ReportResolvedEvent)
		actor := resolveActor(userRepo, e.Actor)
		target := nameOrID("report", e.TorrentName, e.ReportID)
		actionDesc := "resolved"
		switch e.Action {
		case "warn":
			actionDesc = "resolved & warned uploader of"
		case "delete":
			actionDesc = "resolved & deleted"
		}
		return fmt.Sprintf("%s %s %s", actor, actionDesc, target), e.Actor
	})

	listen(event.CommentCreated, func(evt event.Event) (string, event.Actor) {
		e := evt.(*event.CommentCreatedEvent)
		actor := resolveActor(userRepo, e.Actor)
		torrentName := nameOrID("torrent", e.TorrentName, e.TorrentID)
		return fmt.Sprintf("%s commented on %s", actor, torrentName), e.Actor
	})

	listen(event.CommentDeleted, func(evt event.Event) (string, event.Actor) {
		e := evt.(*event.CommentDeletedEvent)
		actor := resolveActor(userRepo, e.Actor)
		torrentName := nameOrID("torrent", e.TorrentName, e.TorrentID)
		return fmt.Sprintf("%s deleted comment on %s", actor, torrentName), e.Actor
	})

	listen(event.ReseedRequested, func(evt event.Event) (string, event.Actor) {
		e := evt.(*event.ReseedRequestedEvent)
		return fmt.Sprintf("%s requested reseed for: %s", e.Actor.Username, e.TorrentName), e.Actor
	})

	listen(event.InviteCreated, func(evt event.Event) (string, event.Actor) {
		e := evt.(*event.InviteCreatedEvent)
		return fmt.Sprintf("%s created an invite", e.Actor.Username), e.Actor
	})

	listen(event.InviteRedeemed, func(evt event.Event) (string, event.Actor) {
		e := evt.(*event.InviteRedeemedEvent)
		invitee := e.InviteeUsername
		if invitee == "" {
			invitee = resolveUsername(userRepo, e.InviteeID)
		}
		return fmt.Sprintf("%s redeemed an invite", invitee), e.Actor
	})

	listen(event.RegistrationModeChanged, func(evt event.Event) (string, event.Actor) {
		e := evt.(*event.RegistrationModeChangedEvent)
		return fmt.Sprintf("%s changed registration mode from %s to %s", e.Actor.Username, e.OldMode, e.NewMode), e.Actor
	})

	listen(event.EmailBanned, func(evt event.Event) (string, event.Actor) {
		e := evt.(*event.EmailBannedEvent)
		return fmt.Sprintf("%s banned email pattern: %s", e.Actor.Username, e.Pattern), e.Actor
	})

	listen(event.EmailUnbanned, func(evt event.Event) (string, event.Actor) {
		e := evt.(*event.EmailUnbannedEvent)
		return fmt.Sprintf("%s unbanned email pattern: %s", e.Actor.Username, e.Pattern), e.Actor
	})

	listen(event.IPBanned, func(evt event.Event) (string, event.Actor) {
		e := evt.(*event.IPBannedEvent)
		return fmt.Sprintf("%s banned IP range: %s", e.Actor.Username, e.IPRange), e.Actor
	})

	listen(event.IPUnbanned, func(evt event.Event) (string, event.Actor) {
		e := evt.(*event.IPUnbannedEvent)
		return fmt.Sprintf("%s unbanned IP range: %s", e.Actor.Username, e.IPRange), e.Actor
	})

	// Note: MessageSent is intentionally NOT logged to the activity log.
	// Private messages must not appear in public logs or store content/metadata.

	listen(event.WarningIssued, func(evt event.Event) (string, event.Actor) {
		e := evt.(*event.WarningIssuedEvent)
		return fmt.Sprintf("%s issued %s warning to %s", e.Actor.Username, e.WarningType, e.Username), e.Actor
	})

	listen(event.WarningLifted, func(evt event.Event) (string, event.Actor) {
		e := evt.(*event.WarningLiftedEvent)
		return fmt.Sprintf("%s lifted warning from %s", e.Actor.Username, e.Username), e.Actor
	})

	listen(event.ChatUserMuted, func(evt event.Event) (string, event.Actor) {
		e := evt.(*event.ChatUserMutedEvent)
		actor := resolveActor(userRepo, e.Actor)
		target := resolveUsername(userRepo, e.TargetUserID)
		return fmt.Sprintf("%s muted %s in chat for %d minutes", actor, target, e.DurationMinutes), e.Actor
	})

	listen(event.ChatUserUnmuted, func(evt event.Event) (string, event.Actor) {
		e := evt.(*event.ChatUserUnmutedEvent)
		actor := resolveActor(userRepo, e.Actor)
		target := resolveUsername(userRepo, e.TargetUserID)
		return fmt.Sprintf("%s unmuted %s in chat", actor, target), e.Actor
	})

	listen(event.NewsPublished, func(evt event.Event) (string, event.Actor) {
		e := evt.(*event.NewsPublishedEvent)
		return fmt.Sprintf("%s published news: %s", e.Actor.Username, e.Title), e.Actor
	})

	listen(event.PasswordReset, func(evt event.Event) (string, event.Actor) {
		e := evt.(*event.PasswordResetEvent)
		return fmt.Sprintf("%s reset password for %s", e.Actor.Username, e.Username), e.Actor
	})

	listen(event.PasskeyReset, func(evt event.Event) (string, event.Actor) {
		e := evt.(*event.PasskeyResetEvent)
		return fmt.Sprintf("%s reset passkey for %s", e.Actor.Username, e.Username), e.Actor
	})

	listen(event.RestrictionApplied, func(evt event.Event) (string, event.Actor) {
		e := evt.(*event.RestrictionAppliedEvent)
		actor := resolveActor(userRepo, e.Actor)
		return fmt.Sprintf("%s restricted %s privilege for %s", actor, e.RestrictionType, e.Username), e.Actor
	})

	listen(event.RestrictionLifted, func(evt event.Event) (string, event.Actor) {
		e := evt.(*event.RestrictionLiftedEvent)
		actor := resolveActor(userRepo, e.Actor)
		return fmt.Sprintf("%s restored %s privilege for %s", actor, e.RestrictionType, e.Username), e.Actor
	})

	listen(event.UserQuickBanned, func(evt event.Event) (string, event.Actor) {
		e := evt.(*event.UserQuickBannedEvent)
		duration := "permanently"
		if e.DurationDays != nil {
			duration = fmt.Sprintf("for %d days", *e.DurationDays)
		}
		extras := ""
		if e.BanIP {
			extras += " +IP"
		}
		if e.BanEmail {
			extras += " +email"
		}
		return fmt.Sprintf("%s banned %s %s%s: %s", e.Actor.Username, e.Username, duration, extras, e.Reason), e.Actor
	})
}

// resolveUsername looks up a username by user ID, falling back to "User #ID" on error.
func resolveUsername(userRepo repository.UserRepository, userID int64) string {
	user, err := userRepo.GetByID(context.Background(), userID)
	if err != nil || user == nil {
		return fmt.Sprintf("User #%d", userID)
	}
	return user.Username
}

// resolveActor returns the actor's username, looking it up from the repo if not
// already populated in the event.
func resolveActor(userRepo repository.UserRepository, actor event.Actor) string {
	if actor.Username != "" {
		return actor.Username
	}
	return resolveUsername(userRepo, actor.ID)
}

// nameOrID returns the name if non-empty, otherwise a fallback like "torrent #42".
func nameOrID(kind, name string, id int64) string {
	if name != "" {
		return name
	}
	return fmt.Sprintf("%s #%d", kind, id)
}

func marshalMetadata(evt event.Event) *string {
	data, err := json.Marshal(evt)
	if err != nil {
		return nil
	}
	s := string(data)
	return &s
}
