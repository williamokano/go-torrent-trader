package service_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// pagingRaceRepo simulates OFFSET/LIMIT drift: a notification inserted
// between two List() page fetches shifts every later row down by one, so
// the same row can be re-served on the next page. Its second page
// deliberately repeats the last ID from its first page before continuing
// with new ones, and its third page terminates the fetch loop.
type pagingRaceRepo struct {
	calls int
}

func (r *pagingRaceRepo) Create(_ context.Context, _ *model.Notification) error { return nil }
func (r *pagingRaceRepo) GetByID(_ context.Context, _ int64) (*model.Notification, error) {
	return nil, sql.ErrNoRows
}
func (r *pagingRaceRepo) MarkRead(_ context.Context, _, _ int64) error        { return nil }
func (r *pagingRaceRepo) MarkAllRead(_ context.Context, _ int64) error        { return nil }
func (r *pagingRaceRepo) CountUnread(_ context.Context, _ int64) (int, error) { return 0, nil }
func (r *pagingRaceRepo) DeleteOld(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}
func (r *pagingRaceRepo) CountUnreadSince(_ context.Context, _ int64, _ time.Time) (int, error) {
	return 0, nil
}
func (r *pagingRaceRepo) ListUnreadSince(_ context.Context, _ int64, _ time.Time, _ int) ([]model.Notification, error) {
	return nil, nil
}

func (r *pagingRaceRepo) List(_ context.Context, userID int64, opts repository.ListNotificationsOptions) ([]model.Notification, int64, error) {
	r.calls++
	notif := func(id int64) model.Notification {
		return model.Notification{
			ID:        id,
			UserID:    userID,
			Type:      model.NotifTopicReply,
			Data:      topicReplyData(1, "alice"),
			CreatedAt: time.Unix(int64(1000-id), 0),
		}
	}
	switch opts.Page {
	case 1:
		batch := make([]model.Notification, 100)
		for i := range batch {
			batch[i] = notif(int64(200 - i)) // IDs 200..101, newest first
		}
		return batch, 200, nil
	case 2:
		// Row 101 (the last of page 1) is re-served here because an insert
		// shifted the offset, followed by 99 genuinely new rows.
		batch := make([]model.Notification, 100)
		batch[0] = notif(101)
		for i := 1; i < 100; i++ {
			batch[i] = notif(int64(100 - i)) // IDs 99..1
		}
		return batch, 200, nil
	default:
		return nil, 200, nil // terminates the fetch loop
	}
}

func topicReplyData(topicID int64, actor string) json.RawMessage {
	return json.RawMessage(fmt.Sprintf(
		`{"topic_id": %d, "topic_title": "Topic %d", "actor_username": %q}`,
		topicID, topicID, actor,
	))
}

func TestListGrouped_CollapsesSameTopicReplies(t *testing.T) {
	ctx := context.Background()
	svc, _, _, _, _ := newTestNotifService()

	// Three topic_reply notifications for the same topic, from three
	// different actors, plus one for a different topic.
	if _, err := svc.Create(ctx, 2, 10, model.NotifTopicReply, topicReplyData(1, "alice")); err != nil {
		t.Fatalf("create error: %v", err)
	}
	if _, err := svc.Create(ctx, 2, 11, model.NotifTopicReply, topicReplyData(1, "bob")); err != nil {
		t.Fatalf("create error: %v", err)
	}
	if _, err := svc.Create(ctx, 2, 12, model.NotifTopicReply, topicReplyData(1, "carol")); err != nil {
		t.Fatalf("create error: %v", err)
	}
	if _, err := svc.Create(ctx, 2, 13, model.NotifTopicReply, topicReplyData(2, "dave")); err != nil {
		t.Fatalf("create error: %v", err)
	}

	groups, total, err := svc.ListGrouped(ctx, 2, 1, 25, false)
	if err != nil {
		t.Fatalf("list grouped error: %v", err)
	}
	if total != 2 {
		t.Fatalf("expected 2 groups (one per topic), got %d", total)
	}
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups returned, got %d", len(groups))
	}

	// Groups are sorted newest-first, so topic 2 (created last) comes first.
	topic2Group := groups[0]
	if topic2Group.Count != 1 {
		t.Errorf("topic 2 group count = %d, want 1", topic2Group.Count)
	}

	topic1Group := groups[1]
	if topic1Group.Count != 3 {
		t.Errorf("topic 1 group count = %d, want 3", topic1Group.Count)
	}
	if topic1Group.Key != "topic_reply:1" {
		t.Errorf("topic 1 group key = %q, want topic_reply:1", topic1Group.Key)
	}
	if len(topic1Group.Notifications) != 3 {
		t.Errorf("topic 1 group notifications = %d, want 3", len(topic1Group.Notifications))
	}
	wantActors := []string{"carol", "bob", "alice"} // most-recent-first
	if len(topic1Group.LastActors) != len(wantActors) {
		t.Fatalf("last actors = %v, want %v", topic1Group.LastActors, wantActors)
	}
	for i, a := range wantActors {
		if topic1Group.LastActors[i] != a {
			t.Errorf("last actors[%d] = %q, want %q (order: %v)", i, topic1Group.LastActors[i], a, topic1Group.LastActors)
		}
	}
}

func TestListGrouped_CapsLastActorsAtThree(t *testing.T) {
	ctx := context.Background()
	svc, _, _, _, _ := newTestNotifService()

	actors := []string{"a1", "a2", "a3", "a4", "a5"}
	for i, actor := range actors {
		if _, err := svc.Create(ctx, 2, int64(100+i), model.NotifTopicReply, topicReplyData(9, actor)); err != nil {
			t.Fatalf("create error: %v", err)
		}
	}

	groups, _, err := svc.ListGrouped(ctx, 2, 1, 25, false)
	if err != nil {
		t.Fatalf("list grouped error: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	g := groups[0]
	if g.Count != 5 {
		t.Errorf("count = %d, want 5 (all underlying notifications still counted)", g.Count)
	}
	if len(g.LastActors) != 3 {
		t.Fatalf("last actors = %v, want 3 entries max", g.LastActors)
	}
	want := []string{"a5", "a4", "a3"}
	for i, a := range want {
		if g.LastActors[i] != a {
			t.Errorf("last actors[%d] = %q, want %q", i, g.LastActors[i], a)
		}
	}
}

func TestListGrouped_NonTopicReplyTypesAreSingletons(t *testing.T) {
	ctx := context.Background()
	svc, _, _, _, _ := newTestNotifService()

	for i := 0; i < 3; i++ {
		if _, err := svc.Create(ctx, 2, 10, model.NotifMention, json.RawMessage(`{"actor_username":"alice"}`)); err != nil {
			t.Fatalf("create error: %v", err)
		}
	}

	groups, total, err := svc.ListGrouped(ctx, 2, 1, 25, false)
	if err != nil {
		t.Fatalf("list grouped error: %v", err)
	}
	if total != 3 {
		t.Fatalf("expected 3 singleton groups for non-topic_reply notifications, got %d", total)
	}
	for _, g := range groups {
		if g.Count != 1 {
			t.Errorf("expected singleton group, got count %d", g.Count)
		}
	}
}

// A mention arriving between two topic_reply notifications for the same
// topic must not split them into two groups -- grouping is keyed on
// type+topic, not on adjacency in the raw feed.
func TestListGrouped_InterleavedUnrelatedTypeDoesNotSplitTheGroup(t *testing.T) {
	ctx := context.Background()
	svc, _, _, _, _ := newTestNotifService()

	if _, err := svc.Create(ctx, 2, 10, model.NotifTopicReply, topicReplyData(1, "alice")); err != nil {
		t.Fatalf("create error: %v", err)
	}
	if _, err := svc.Create(ctx, 2, 11, model.NotifMention, json.RawMessage(`{"actor_username":"eve"}`)); err != nil {
		t.Fatalf("create error: %v", err)
	}
	if _, err := svc.Create(ctx, 2, 12, model.NotifTopicReply, topicReplyData(1, "bob")); err != nil {
		t.Fatalf("create error: %v", err)
	}

	groups, total, err := svc.ListGrouped(ctx, 2, 1, 25, false)
	if err != nil {
		t.Fatalf("list grouped error: %v", err)
	}
	if total != 2 {
		t.Fatalf("expected 2 groups (one topic_reply group of 2, one mention singleton), got %d", total)
	}
	var topicGroup *model.NotificationGroup
	for i := range groups {
		if groups[i].Type == model.NotifTopicReply {
			topicGroup = &groups[i]
		}
	}
	if topicGroup == nil {
		t.Fatal("expected a topic_reply group")
	}
	if topicGroup.Count != 2 {
		t.Errorf("topic_reply group count = %d, want 2 (interleaving a mention must not split it)", topicGroup.Count)
	}
}

// A topic_reply notification whose data has a malformed/missing topic_id
// must not crash grouping or silently merge with an unrelated group -- it
// falls back to being its own singleton, same as any other defensive path.
func TestListGrouped_MalformedTopicIDFallsBackToSingleton(t *testing.T) {
	ctx := context.Background()
	svc, _, _, _, _ := newTestNotifService()

	if _, err := svc.Create(ctx, 2, 10, model.NotifTopicReply, json.RawMessage(`{"actor_username":"alice"}`)); err != nil {
		t.Fatalf("create error: %v", err)
	}
	if _, err := svc.Create(ctx, 2, 11, model.NotifTopicReply, json.RawMessage(`{"topic_id":"not-a-number","actor_username":"bob"}`)); err != nil {
		t.Fatalf("create error: %v", err)
	}

	groups, total, err := svc.ListGrouped(ctx, 2, 1, 25, false)
	if err != nil {
		t.Fatalf("list grouped error: %v", err)
	}
	if total != 2 {
		t.Fatalf("expected 2 singleton groups (topic_id absent, then malformed), got %d", total)
	}
	for _, g := range groups {
		if g.Count != 1 {
			t.Errorf("expected singleton fallback group, got count %d", g.Count)
		}
	}
}

// Marking every underlying notification of a group as read should stop the
// group from being flagged unread -- the "unread" flag is derived, not
// separately stored.
func TestListGrouped_UnreadFlagReflectsUnderlyingReadState(t *testing.T) {
	ctx := context.Background()
	svc, _, _, _, _ := newTestNotifService()

	n1, err := svc.Create(ctx, 2, 10, model.NotifTopicReply, topicReplyData(1, "alice"))
	if err != nil {
		t.Fatalf("create error: %v", err)
	}
	n2, err := svc.Create(ctx, 2, 11, model.NotifTopicReply, topicReplyData(1, "bob"))
	if err != nil {
		t.Fatalf("create error: %v", err)
	}

	groups, _, err := svc.ListGrouped(ctx, 2, 1, 25, false)
	if err != nil {
		t.Fatalf("list grouped error: %v", err)
	}
	if !groups[0].Unread {
		t.Fatal("expected group to be unread before marking anything read")
	}

	if err := svc.MarkRead(ctx, 2, n1.ID); err != nil {
		t.Fatalf("mark read error: %v", err)
	}
	groups, _, err = svc.ListGrouped(ctx, 2, 1, 25, false)
	if err != nil {
		t.Fatalf("list grouped error: %v", err)
	}
	if !groups[0].Unread {
		t.Fatal("expected group to remain unread while one underlying notification is still unread")
	}

	if err := svc.MarkRead(ctx, 2, n2.ID); err != nil {
		t.Fatalf("mark read error: %v", err)
	}
	groups, _, err = svc.ListGrouped(ctx, 2, 1, 25, false)
	if err != nil {
		t.Fatalf("list grouped error: %v", err)
	}
	if groups[0].Unread {
		t.Fatal("expected group to be read once every underlying notification is read")
	}

	// The unread-count badge (used by the digest email too) must keep
	// counting individual rows regardless of grouping.
	count, err := svc.UnreadCount(ctx, 2)
	if err != nil {
		t.Fatalf("unread count error: %v", err)
	}
	if count != 0 {
		t.Errorf("unread count = %d, want 0", count)
	}
}

func TestListGrouped_Pagination(t *testing.T) {
	ctx := context.Background()
	svc, _, _, _, _ := newTestNotifService()

	// 5 distinct topics -> 5 groups.
	for i := int64(1); i <= 5; i++ {
		if _, err := svc.Create(ctx, 2, 10+i, model.NotifTopicReply, topicReplyData(i, "alice")); err != nil {
			t.Fatalf("create error: %v", err)
		}
	}

	page1, total, err := svc.ListGrouped(ctx, 2, 1, 2, false)
	if err != nil {
		t.Fatalf("list grouped error: %v", err)
	}
	if total != 5 {
		t.Fatalf("total = %d, want 5", total)
	}
	if len(page1) != 2 {
		t.Fatalf("page1 = %d items, want 2", len(page1))
	}

	page3, _, err := svc.ListGrouped(ctx, 2, 3, 2, false)
	if err != nil {
		t.Fatalf("list grouped error: %v", err)
	}
	if len(page3) != 1 {
		t.Fatalf("page3 = %d items, want 1 (5 groups over pages of 2)", len(page3))
	}

	pageOOB, _, err := svc.ListGrouped(ctx, 2, 4, 2, false)
	if err != nil {
		t.Fatalf("list grouped error: %v", err)
	}
	if len(pageOOB) != 0 {
		t.Fatalf("page beyond the last group = %d items, want 0", len(pageOOB))
	}
}

// The grouping window is bounded (see groupWindow in notification_group.go)
// so a user with an unusually large notification history doesn't turn
// every grouped-list request into an unbounded scan. This proves the cap
// actually applies and that it keeps the newest entries, not the oldest.
// fetchGroupWindow pages through List() with plain OFFSET/LIMIT, so a
// notification created between two of those page fetches can shift the
// offset and cause the same row to be re-served on the next page. This
// proves the dedup in fetchGroupWindow stops that duplicate from inflating
// a group's count or double-listing a notification in the expand view.
func TestListGrouped_DedupesRowsRepeatedAcrossPagesByOffsetDrift(t *testing.T) {
	ctx := context.Background()
	repo := &pagingRaceRepo{}
	svc := service.NewNotificationService(repo, newMockNotifPrefRepo(), nil, newMockTopicSubRepo(), nil, nil, nil)

	groups, total, err := svc.ListGrouped(ctx, 2, 1, 25, false)
	if err != nil {
		t.Fatalf("list grouped error: %v", err)
	}
	if total != 1 {
		t.Fatalf("total = %d, want 1 (all 199 unique rows share topic 1)", total)
	}
	if len(groups) != 1 {
		t.Fatalf("groups = %d, want 1", len(groups))
	}
	g := groups[0]
	if g.Count != 199 {
		t.Errorf("count = %d, want 199 (200 rows served across 2 pages minus 1 duplicate)", g.Count)
	}
	seen := make(map[int64]bool, len(g.Notifications))
	for _, n := range g.Notifications {
		if seen[n.ID] {
			t.Fatalf("notification %d appears more than once in the expand list", n.ID)
		}
		seen[n.ID] = true
	}
	if repo.calls != 3 {
		t.Errorf("expected List to be called 3 times (2 full pages + 1 terminating call), got %d", repo.calls)
	}
}

func TestListGrouped_CapsAtGroupWindow(t *testing.T) {
	const groupWindow = 200 // mirrors the unexported const in notification_group.go

	ctx := context.Background()
	svc, _, _, _, _ := newTestNotifService()

	for i := 0; i < groupWindow+5; i++ {
		data := json.RawMessage(fmt.Sprintf(`{"actor_username": "actor-%d"}`, i))
		if _, err := svc.Create(ctx, 2, 999, model.NotifMention, data); err != nil {
			t.Fatalf("create error: %v", err)
		}
	}

	// per_page is clamped to 100 (same cap as the existing List endpoint),
	// so pull the window across two full pages.
	page1, total, err := svc.ListGrouped(ctx, 2, 1, 100, false)
	if err != nil {
		t.Fatalf("list grouped error: %v", err)
	}
	if total != groupWindow {
		t.Fatalf("total groups = %d, want the window cap of %d", total, groupWindow)
	}
	page2, _, err := svc.ListGrouped(ctx, 2, 2, 100, false)
	if err != nil {
		t.Fatalf("list grouped error: %v", err)
	}
	groups := append(page1, page2...)
	if len(groups) != groupWindow {
		t.Fatalf("groups returned across both pages = %d, want %d", len(groups), groupWindow)
	}

	// The 5 oldest (actor-0..actor-4) must have been dropped, not the newest.
	seenActors := make(map[string]bool, len(groups))
	for _, g := range groups {
		var payload struct {
			ActorUsername string `json:"actor_username"`
		}
		if err := json.Unmarshal(g.Data, &payload); err != nil {
			t.Fatalf("unmarshal group data: %v", err)
		}
		seenActors[payload.ActorUsername] = true
	}
	for i := 0; i < 5; i++ {
		if seenActors[fmt.Sprintf("actor-%d", i)] {
			t.Errorf("expected the oldest notification actor-%d to be dropped by the window cap, but it was present", i)
		}
	}
	if !seenActors[fmt.Sprintf("actor-%d", groupWindow+4)] {
		t.Error("expected the newest notification (actor-204) to survive the window cap")
	}
}

// A single topic_reply group whose replies straddle the window boundary
// must be truncated in place, not dropped or duplicated: it should still
// show up as one group, just with a count capped at whatever fit in the
// window (its newest members), exactly like the documented trade-off in
// docs/IMPLEMENTATION_TASKS.md.
func TestListGrouped_GroupStraddlingWindowBoundaryIsTruncatedNotDropped(t *testing.T) {
	const groupWindow = 200 // mirrors the unexported const in notification_group.go

	ctx := context.Background()
	svc, _, _, _, _ := newTestNotifService()

	for i := 0; i < groupWindow+10; i++ {
		if _, err := svc.Create(ctx, 2, int64(1000+i), model.NotifTopicReply, topicReplyData(1, fmt.Sprintf("actor-%d", i))); err != nil {
			t.Fatalf("create error: %v", err)
		}
	}

	groups, total, err := svc.ListGrouped(ctx, 2, 1, 25, false)
	if err != nil {
		t.Fatalf("list grouped error: %v", err)
	}
	if total != 1 {
		t.Fatalf("total = %d, want 1 (all replies share the same topic)", total)
	}
	g := groups[0]
	if g.Count != groupWindow {
		t.Errorf("count = %d, want %d (truncated at the window, not dropped or duplicated)", g.Count, groupWindow)
	}
	if len(g.Notifications) != groupWindow {
		t.Errorf("underlying notifications = %d, want %d", len(g.Notifications), groupWindow)
	}
	// The kept notifications must be the newest ones, so the group's
	// representative data (used for the "N new replies" summary) reflects
	// the very last reply, not an arbitrary one from the middle.
	newestActor := fmt.Sprintf("actor-%d", groupWindow+9)
	if !strings.Contains(string(g.Data), newestActor) {
		t.Errorf("representative data = %s, want it to reflect the newest reply (%s)", g.Data, newestActor)
	}
}

func TestListGrouped_UnreadOnlyFiltersUnderlyingNotifications(t *testing.T) {
	ctx := context.Background()
	svc, _, _, _, _ := newTestNotifService()

	n1, err := svc.Create(ctx, 2, 10, model.NotifTopicReply, topicReplyData(1, "alice"))
	if err != nil {
		t.Fatalf("create error: %v", err)
	}
	if _, err := svc.Create(ctx, 2, 11, model.NotifTopicReply, topicReplyData(1, "bob")); err != nil {
		t.Fatalf("create error: %v", err)
	}
	if err := svc.MarkRead(ctx, 2, n1.ID); err != nil {
		t.Fatalf("mark read error: %v", err)
	}

	groups, total, err := svc.ListGrouped(ctx, 2, 1, 25, true)
	if err != nil {
		t.Fatalf("list grouped error: %v", err)
	}
	if total != 1 {
		t.Fatalf("expected 1 group under unread_only, got %d", total)
	}
	if groups[0].Count != 1 {
		t.Errorf("expected the unread-only group to contain just the unread notification, got count %d", groups[0].Count)
	}
}
