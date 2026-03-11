package listener

import (
	"context"
	"encoding/json"
	"log/slog"
	"regexp"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

var mentionRegex = regexp.MustCompile(`(?:^|[\s(])@(\w+)`)

// RegisterNotificationListeners subscribes to domain events and creates notifications.
func RegisterNotificationListeners(
	bus event.Bus,
	notifSvc *service.NotificationService,
	userRepo repository.UserRepository,
	postRepo repository.ForumPostRepository,
	topicSubRepo repository.TopicSubscriptionRepository,
) {
	// Auto-subscribe topic creator when a topic is created
	bus.Subscribe(event.ForumTopicCreated, func(_ context.Context, evt event.Event) error {
		e := evt.(*event.ForumTopicCreatedEvent)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := notifSvc.Subscribe(ctx, e.Actor.ID, e.TopicID); err != nil {
			slog.Error("notification: failed to auto-subscribe topic creator",
				"user_id", e.Actor.ID, "topic_id", e.TopicID, "error", err)
		}
		return nil
	})

	// Forum post created: triggers forum_reply, forum_mention, and topic_reply notifications
	bus.Subscribe(event.ForumPostCreated, func(_ context.Context, evt event.Event) error {
		e := evt.(*event.ForumPostCreatedEvent)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		authorID := e.Actor.ID

		// Auto-subscribe the post author to the topic
		if err := notifSvc.Subscribe(ctx, authorID, e.TopicID); err != nil {
			slog.Error("notification: failed to auto-subscribe post author",
				"user_id", authorID, "topic_id", e.TopicID, "error", err)
		}

		// Track who we've already notified to avoid duplicates
		notified := make(map[int64]bool)
		notified[authorID] = true // never notify the author

		// 1. forum_reply: notify the author of the replied-to post
		if e.ReplyToUserID != nil && *e.ReplyToUserID != authorID {
			data := marshalData(map[string]interface{}{
				"post_id":       e.PostID,
				"topic_id":      e.TopicID,
				"topic_title":   e.TopicTitle,
				"forum_id":      e.ForumID,
				"actor_id":      authorID,
				"actor_username": e.Actor.Username,
			})
			if _, err := notifSvc.Create(ctx, *e.ReplyToUserID, authorID, model.NotifForumReply, data); err != nil {
				slog.Error("notification: failed to create forum_reply", "error", err)
			}
			notified[*e.ReplyToUserID] = true
		}

		// 2. forum_mention: notify @mentioned users
		mentions := mentionRegex.FindAllStringSubmatch(e.Body, -1)
		for _, match := range mentions {
			username := match[1]
			user, err := userRepo.GetByUsername(ctx, username)
			if err != nil {
				continue // user doesn't exist
			}
			if notified[user.ID] {
				continue
			}
			data := marshalData(map[string]interface{}{
				"post_id":        e.PostID,
				"topic_id":       e.TopicID,
				"topic_title":    e.TopicTitle,
				"forum_id":       e.ForumID,
				"actor_id":       authorID,
				"actor_username": e.Actor.Username,
			})
			if _, err := notifSvc.Create(ctx, user.ID, authorID, model.NotifForumMention, data); err != nil {
				slog.Error("notification: failed to create forum_mention", "error", err)
			}
			notified[user.ID] = true
		}

		// 3. topic_reply: notify all topic subscribers (except author and already-notified)
		subscribers, err := topicSubRepo.ListSubscribers(ctx, e.TopicID)
		if err != nil {
			slog.Error("notification: failed to list topic subscribers",
				"topic_id", e.TopicID, "error", err)
			return nil
		}
		for _, subUserID := range subscribers {
			if notified[subUserID] {
				continue
			}
			data := marshalData(map[string]interface{}{
				"post_id":        e.PostID,
				"topic_id":       e.TopicID,
				"topic_title":    e.TopicTitle,
				"forum_id":       e.ForumID,
				"actor_id":       authorID,
				"actor_username": e.Actor.Username,
			})
			if _, err := notifSvc.Create(ctx, subUserID, authorID, model.NotifTopicReply, data); err != nil {
				slog.Error("notification: failed to create topic_reply", "error", err)
			}
			notified[subUserID] = true
		}

		return nil
	})

	// Torrent commented: notify the torrent uploader
	bus.Subscribe(event.TorrentCommented, func(_ context.Context, evt event.Event) error {
		e := evt.(*event.TorrentCommentedEvent)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		data := marshalData(map[string]interface{}{
			"comment_id":     e.CommentID,
			"torrent_id":     e.TorrentID,
			"torrent_name":   e.TorrentName,
			"actor_id":       e.Actor.ID,
			"actor_username": e.Actor.Username,
		})
		if _, err := notifSvc.Create(ctx, e.UploaderID, e.Actor.ID, model.NotifTorrentComment, data); err != nil {
			slog.Error("notification: failed to create torrent_comment", "error", err)
		}
		return nil
	})

	// PM received: notify the receiver
	bus.Subscribe(event.MessageSent, func(_ context.Context, evt event.Event) error {
		e := evt.(*event.MessageSentEvent)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		data := marshalData(map[string]interface{}{
			"message_id":     e.MessageID,
			"actor_id":       e.Actor.ID,
			"actor_username": e.Actor.Username,
		})
		if _, err := notifSvc.Create(ctx, e.ReceiverID, e.Actor.ID, model.NotifPMReceived, data); err != nil {
			slog.Error("notification: failed to create pm_received", "error", err)
		}
		return nil
	})

	// Warning issued: notify the warned user (system notification)
	bus.Subscribe(event.WarningIssued, func(_ context.Context, evt event.Event) error {
		e := evt.(*event.WarningIssuedEvent)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		data := marshalData(map[string]interface{}{
			"warning_id":   e.WarningID,
			"warning_type": e.WarningType,
		})
		if _, err := notifSvc.Create(ctx, e.UserID, e.Actor.ID, model.NotifSystem, data); err != nil {
			slog.Error("notification: failed to create system warning notification", "error", err)
		}
		return nil
	})
}

func marshalData(v interface{}) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		slog.Error("notification: failed to marshal data", "error", err)
		return json.RawMessage("{}")
	}
	return data
}
