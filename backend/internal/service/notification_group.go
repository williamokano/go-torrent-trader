package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// ErrGroupNotBatchable is returned by MarkGroupRead when key does not
// identify a batchable group. Only topic_reply groups are batchable today:
// every other notification type is always its own "single:<id>" singleton
// (see groupKeyFor), which the frontend never routes through this call --
// a singleton group renders with the existing single-notification
// MarkRead button instead (see NotificationsPage's renderGroup: the
// "Mark all read" / batch path only appears once g.Count > 1).
var ErrGroupNotBatchable = errors.New("notification group is not batchable")

// groupWindow bounds how many of a user's most recent notifications (across
// as many raw List() calls as it takes) are considered when building
// grouped results. Read notifications are purged after 90 days by
// DeleteOld (see the maintenance worker), so an active user is nowhere
// near this in practice; a user who has more than groupWindow matching
// notifications simply won't see their oldest ones folded into this
// endpoint's response (they remain fully visible, uncollapsed, on the
// existing GET /notifications endpoint, which this never touches).
//
// This bound is what keeps grouping a pure read-time / display concern:
// no new table, no write-path changes, no interface changes to
// NotificationRepository. If it ever proves insufficient, the natural
// follow-up is a SQL-level GROUP BY aggregate query.
const groupWindow = 200

// groupWindowPageSize is the page size used internally to fill groupWindow
// via the existing, unmodified List() method.
const groupWindowPageSize = 100

// maxGroupActors caps how many distinct "last actors" are surfaced per
// group, matching the acceptance criteria's "last few actors".
const maxGroupActors = 3

// topicReplyGroupPrefix is the prefix groupKeyFor uses to build a
// topic_reply group's Key ("topic_reply:<topic_id>"). Shared with
// ParseTopicReplyGroupKey so the two stay in sync by construction rather
// than by convention.
const topicReplyGroupPrefix = model.NotifTopicReply + ":"

// ListGrouped returns a page of read-time-collapsed notifications: multiple
// topic_reply notifications for the same topic are folded into one
// NotificationGroup (count + last few distinct actors + the underlying
// notifications for expand-on-click), while every other notification type
// passes through as its own singleton group.
//
// Deliberately built on top of the existing List/CountUnread methods rather
// than a new repository query: it keeps the write path (Create, the event
// listeners) and the on-disk shape of notifications completely untouched,
// so unread-count badges and the digest email continue to count individual
// rows exactly as they did before this story.
func (s *NotificationService) ListGrouped(ctx context.Context, userID int64, page, perPage int, unreadOnly bool) ([]model.NotificationGroup, int64, error) {
	if page <= 0 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 25
	}
	if perPage > 100 {
		perPage = 100
	}

	raw, err := s.fetchGroupWindow(ctx, userID, unreadOnly)
	if err != nil {
		return nil, 0, err
	}

	groups := groupNotifications(raw)
	total := int64(len(groups))

	start := (page - 1) * perPage
	if start >= len(groups) {
		return []model.NotificationGroup{}, total, nil
	}
	end := start + perPage
	if end > len(groups) {
		end = len(groups)
	}
	return groups[start:end], total, nil
}

// fetchGroupWindow pulls up to groupWindow of the user's most recent
// notifications (newest first) by repeatedly paging through the existing
// List method.
//
// List is plain OFFSET/LIMIT pagination, so a notification created between
// two of these page fetches can shift every row after it and be re-served
// on the next page -- the same row would then appear twice in all. Dedupe
// by ID defensively so a race like that can only ever under-count a page
// (an item briefly missing), never inflate a group's count or duplicate an
// entry in the expand-on-click list.
func (s *NotificationService) fetchGroupWindow(ctx context.Context, userID int64, unreadOnly bool) ([]model.Notification, error) {
	var all []model.Notification
	seen := make(map[int64]bool)
	for page := 1; len(all) < groupWindow; page++ {
		batch, total, err := s.notifications.List(ctx, userID, repository.ListNotificationsOptions{
			UnreadOnly: unreadOnly,
			Page:       page,
			PerPage:    groupWindowPageSize,
		})
		if err != nil {
			return nil, fmt.Errorf("fetch group window: %w", err)
		}
		for _, n := range batch {
			if seen[n.ID] {
				continue
			}
			seen[n.ID] = true
			all = append(all, n)
		}
		if int64(len(all)) >= total || len(batch) < groupWindowPageSize {
			break
		}
	}
	if len(all) > groupWindow {
		all = all[:groupWindow]
	}
	return all, nil
}

// groupNotifications folds a newest-first slice of notifications into
// groups, preserving overall recency order.
//
// raw is required to already be strictly newest-first (which List's
// `ORDER BY created_at DESC, id DESC` plus fetchGroupWindow's order-
// preserving dedup guarantee). That means the first occurrence of any
// group key is always its most recent notification, so a group's
// representative Data/LatestCreatedAt can be set once, at creation, instead
// of via a running "is this newer" comparison on every subsequent member.
func groupNotifications(raw []model.Notification) []model.NotificationGroup {
	groups := make([]*model.NotificationGroup, 0, len(raw))
	byKey := make(map[string]*model.NotificationGroup, len(raw))

	for _, n := range raw {
		payload := parseNotificationPayload(n.Data)
		key := groupKeyFor(n, payload.TopicID)
		g, exists := byKey[key]
		if !exists {
			g = &model.NotificationGroup{
				Key:             key,
				Type:            n.Type,
				Data:            n.Data,
				LatestCreatedAt: n.CreatedAt,
				LastActors:      make([]string, 0, maxGroupActors),
			}
			byKey[key] = g
			groups = append(groups, g)
		}

		g.Count++
		g.Notifications = append(g.Notifications, n)
		if !n.Read {
			g.Unread = true
		}
		if payload.ActorUsername != "" {
			g.LastActors = appendDistinctActor(g.LastActors, payload.ActorUsername)
		}
	}

	sort.SliceStable(groups, func(i, j int) bool {
		return groups[i].LatestCreatedAt.After(groups[j].LatestCreatedAt)
	})

	result := make([]model.NotificationGroup, len(groups))
	for i, g := range groups {
		result[i] = *g
	}
	return result
}

// groupKeyFor returns the collapsing key for a notification: topic_reply
// entries group by topic, everything else is its own singleton group so
// callers have one uniform shape to render.
func groupKeyFor(n model.Notification, topicID json.Number) string {
	if n.Type == model.NotifTopicReply && topicID != "" {
		return topicReplyGroupPrefix + topicID.String()
	}
	return fmt.Sprintf("single:%d", n.ID)
}

// ParseTopicReplyGroupKey extracts the topic ID from a topic_reply group
// key, as returned to callers via NotificationGroup.Key / groupKeyFor.
// Reports ok=false for any other key shape -- a "single:<id>" singleton, an
// unrecognized prefix, or a non-numeric/non-positive suffix -- since only
// topic_reply groups are batchable (see MarkGroupRead).
func ParseTopicReplyGroupKey(key string) (topicID int64, ok bool) {
	suffix, found := strings.CutPrefix(key, topicReplyGroupPrefix)
	if !found {
		return 0, false
	}
	id, err := strconv.ParseInt(suffix, 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

// MarkGroupRead marks every unread notification underlying the group
// identified by key as read in one set-based UPDATE (see
// repository.NotificationRepository.MarkTopicReplyGroupRead), returning the
// number of rows affected. Replaces what the frontend previously did by
// looping the single-notification MarkRead endpoint over every unread
// notification in a group.
//
// A count of 0 is a valid, non-error result: the group may already be
// fully read (e.g. a second click, or two tabs racing each other), which is
// exactly what makes this idempotent -- calling it again after everything
// is already read just matches zero rows.
//
// The match is live at execution time (see the "this is a live match"
// note on MarkTopicReplyGroupRead), so a reply landing in this topic
// between the caller's last group fetch and this call is included too --
// intentional, not a race bug.
func (s *NotificationService) MarkGroupRead(ctx context.Context, userID int64, key string) (int64, error) {
	topicID, ok := ParseTopicReplyGroupKey(key)
	if !ok {
		return 0, ErrGroupNotBatchable
	}
	return s.notifications.MarkTopicReplyGroupRead(ctx, userID, topicID)
}

// notificationPayload holds the handful of fields grouping cares about from
// a notification's freeform JSON data. Parsed once per notification (rather
// than once per field) since every notification type's data is a flat
// object carrying at most these two.
type notificationPayload struct {
	TopicID       json.Number `json:"topic_id"`
	ActorUsername string      `json:"actor_username"`
}

// parseNotificationPayload is best-effort: malformed or absent fields
// resolve to their zero value rather than an error, since grouping degrades
// gracefully either way (falls back to a singleton group / omits the actor).
func parseNotificationPayload(data json.RawMessage) notificationPayload {
	var payload notificationPayload
	_ = json.Unmarshal(data, &payload)
	return payload
}

// appendDistinctActor records actor as the next most-recent distinct actor,
// preserving recency order and capping at maxGroupActors.
func appendDistinctActor(actors []string, actor string) []string {
	for _, a := range actors {
		if a == actor {
			return actors
		}
	}
	if len(actors) >= maxGroupActors {
		return actors
	}
	return append(actors, actor)
}
