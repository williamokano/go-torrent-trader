package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// topicReplyPayload builds the same shape the notification listener writes
// for a topic_reply notification (see listener/notification.go): a flat
// object carrying at least topic_id.
func topicReplyPayload(topicID int64) json.RawMessage {
	return json.RawMessage(fmt.Sprintf(`{"topic_id": %d, "topic_title": "Topic %d", "actor_username": "someone"}`, topicID, topicID))
}

// TestNotificationRepoMarkTopicReplyGroupRead_LeakScoping proves the batch
// UPDATE only ever touches the exact (user, type=topic_reply, topic_id)
// slice it was asked for: a different user's notifications, a different
// topic's notifications for the same user, an already-read row, and a
// different notification type that happens to carry the same topic_id (a
// forum_reply for the same topic also stores topic_id -- see
// listener/notification.go) must all survive untouched.
func TestNotificationRepoMarkTopicReplyGroupRead_LeakScoping(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewNotificationRepo(db)
	target := newUser(t, db)
	otherUser := newUser(t, db)

	// Two unread topic_reply notifications for target on topic 1 -- these
	// are the only rows that should end up marked read.
	n1 := &model.Notification{UserID: target.ID, Type: model.NotifTopicReply, Data: topicReplyPayload(1)}
	n2 := &model.Notification{UserID: target.ID, Type: model.NotifTopicReply, Data: topicReplyPayload(1)}
	if err := repo.Create(ctx, n1); err != nil {
		t.Fatalf("create n1: %v", err)
	}
	if err := repo.Create(ctx, n2); err != nil {
		t.Fatalf("create n2: %v", err)
	}

	// Already read, same user + topic: must not be double-counted or error.
	alreadyRead := &model.Notification{UserID: target.ID, Type: model.NotifTopicReply, Data: topicReplyPayload(1)}
	if err := repo.Create(ctx, alreadyRead); err != nil {
		t.Fatalf("create alreadyRead: %v", err)
	}
	if err := repo.MarkRead(ctx, target.ID, alreadyRead.ID); err != nil {
		t.Fatalf("mark alreadyRead: %v", err)
	}

	// Same user, different topic: must survive.
	otherTopic := &model.Notification{UserID: target.ID, Type: model.NotifTopicReply, Data: topicReplyPayload(2)}
	if err := repo.Create(ctx, otherTopic); err != nil {
		t.Fatalf("create otherTopic: %v", err)
	}

	// Same user, same topic, different type: forum_reply notifications also
	// carry topic_id -- the type filter must exclude it.
	otherType := &model.Notification{UserID: target.ID, Type: model.NotifForumReply, Data: topicReplyPayload(1)}
	if err := repo.Create(ctx, otherType); err != nil {
		t.Fatalf("create otherType: %v", err)
	}

	// Different user, same topic: must survive.
	otherUserSameTopic := &model.Notification{UserID: otherUser.ID, Type: model.NotifTopicReply, Data: topicReplyPayload(1)}
	if err := repo.Create(ctx, otherUserSameTopic); err != nil {
		t.Fatalf("create otherUserSameTopic: %v", err)
	}

	marked, err := repo.MarkTopicReplyGroupRead(ctx, target.ID, 1)
	if err != nil {
		t.Fatalf("MarkTopicReplyGroupRead: %v", err)
	}
	if marked != 2 {
		t.Fatalf("marked = %d, want 2 (only n1 and n2)", marked)
	}

	assertRead(t, ctx, repo, target.ID, n1.ID, true, "n1 (target user, topic 1, unread topic_reply)")
	assertRead(t, ctx, repo, target.ID, n2.ID, true, "n2 (target user, topic 1, unread topic_reply)")
	assertRead(t, ctx, repo, target.ID, alreadyRead.ID, true, "alreadyRead (was already read before the batch call)")
	assertRead(t, ctx, repo, target.ID, otherTopic.ID, false, "otherTopic (same user, different topic)")
	assertRead(t, ctx, repo, target.ID, otherType.ID, false, "otherType (same user/topic, different notification type)")
	assertRead(t, ctx, repo, otherUser.ID, otherUserSameTopic.ID, false, "otherUserSameTopic (different user, same topic)")
}

func assertRead(t *testing.T, ctx context.Context, repo repository.NotificationRepository, userID, notifID int64, want bool, label string) {
	t.Helper()
	n, err := repo.GetByID(ctx, notifID)
	if err != nil {
		t.Fatalf("GetByID(%d) for %s: %v", notifID, label, err)
	}
	if n.UserID != userID {
		t.Fatalf("%s belongs to user %d, expected %d", label, n.UserID, userID)
	}
	if n.Read != want {
		t.Errorf("%s: Read = %v, want %v", label, n.Read, want)
	}
}

// TestNotificationRepoMarkTopicReplyGroupRead_IsIdempotent proves a second
// call after everything in the group is already read matches zero rows and
// returns no error, rather than failing on already-read rows.
func TestNotificationRepoMarkTopicReplyGroupRead_IsIdempotent(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewNotificationRepo(db)
	u := newUser(t, db)

	for i := 0; i < 3; i++ {
		n := &model.Notification{UserID: u.ID, Type: model.NotifTopicReply, Data: topicReplyPayload(9)}
		if err := repo.Create(ctx, n); err != nil {
			t.Fatalf("create: %v", err)
		}
	}

	first, err := repo.MarkTopicReplyGroupRead(ctx, u.ID, 9)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	if first != 3 {
		t.Fatalf("first call marked = %d, want 3", first)
	}

	second, err := repo.MarkTopicReplyGroupRead(ctx, u.ID, 9)
	if err != nil {
		t.Fatalf("second call returned an error, want a no-op success: %v", err)
	}
	if second != 0 {
		t.Errorf("second call marked = %d, want 0 (nothing left unread)", second)
	}
}

// TestNotificationRepoMarkTopicReplyGroupRead_MatchesNIndividualMarkReads
// proves the set-based batch UPDATE produces the exact same end state as
// looping MarkRead once per notification (the pre-BE-9.26 frontend
// behavior): two users get an identical fixture of topic_reply
// notifications, interleaved with unrelated topics/types, and one is
// resolved via the batch call while the other is resolved via N individual
// MarkRead calls. The resulting read/unread state of every row must match
// between the two users.
func TestNotificationRepoMarkTopicReplyGroupRead_MatchesNIndividualMarkReads(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewNotificationRepo(db)

	// seedFixture builds an identical set of notifications for a user:
	// 5 unread topic_reply on topic 42 (the group under test), 1 unrelated
	// topic (topic 43), and 1 unrelated type -- returns the IDs of the 5
	// target-group notifications plus the 2 that must stay untouched.
	seedFixture := func(u *model.User) (groupIDs []int64, untouchedIDs []int64) {
		for i := 0; i < 5; i++ {
			n := &model.Notification{UserID: u.ID, Type: model.NotifTopicReply, Data: topicReplyPayload(42)}
			if err := repo.Create(ctx, n); err != nil {
				t.Fatalf("seed group notification: %v", err)
			}
			groupIDs = append(groupIDs, n.ID)
		}
		other := &model.Notification{UserID: u.ID, Type: model.NotifTopicReply, Data: topicReplyPayload(43)}
		if err := repo.Create(ctx, other); err != nil {
			t.Fatalf("seed other-topic notification: %v", err)
		}
		mention := &model.Notification{UserID: u.ID, Type: model.NotifMention, Data: json.RawMessage(`{"actor_username":"eve"}`)}
		if err := repo.Create(ctx, mention); err != nil {
			t.Fatalf("seed mention notification: %v", err)
		}
		return groupIDs, []int64{other.ID, mention.ID}
	}

	batchUser := newUser(t, db)
	batchGroupIDs, batchUntouchedIDs := seedFixture(batchUser)

	individualUser := newUser(t, db)
	individualGroupIDs, individualUntouchedIDs := seedFixture(individualUser)

	// The system under test: one set-based UPDATE.
	marked, err := repo.MarkTopicReplyGroupRead(ctx, batchUser.ID, 42)
	if err != nil {
		t.Fatalf("MarkTopicReplyGroupRead: %v", err)
	}
	if marked != int64(len(batchGroupIDs)) {
		t.Fatalf("marked = %d, want %d", marked, len(batchGroupIDs))
	}

	// The baseline being compared against: N individual MarkRead calls,
	// exactly what the frontend did before BE-9.26.
	for _, id := range individualGroupIDs {
		if err := repo.MarkRead(ctx, individualUser.ID, id); err != nil {
			t.Fatalf("MarkRead(%d): %v", id, err)
		}
	}

	for i, id := range batchGroupIDs {
		assertRead(t, ctx, repo, batchUser.ID, id, true, fmt.Sprintf("batch group notification %d", i))
	}
	for i, id := range individualGroupIDs {
		assertRead(t, ctx, repo, individualUser.ID, id, true, fmt.Sprintf("individual group notification %d", i))
	}
	for i, id := range batchUntouchedIDs {
		assertRead(t, ctx, repo, batchUser.ID, id, false, fmt.Sprintf("batch untouched notification %d", i))
	}
	for i, id := range individualUntouchedIDs {
		assertRead(t, ctx, repo, individualUser.ID, id, false, fmt.Sprintf("individual untouched notification %d", i))
	}
}

// TestNotificationRepoMarkTopicReplyGroupRead_MalformedTopicIDIsExcludedNotErrored
// proves a row whose data has no topic_id (or a non-numeric one) is simply
// excluded from the UPDATE rather than aborting it with a cast error --
// this repository compares topic_id as text specifically to avoid that
// failure mode.
func TestNotificationRepoMarkTopicReplyGroupRead_MalformedTopicIDIsExcludedNotErrored(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewNotificationRepo(db)
	u := newUser(t, db)

	valid := &model.Notification{UserID: u.ID, Type: model.NotifTopicReply, Data: topicReplyPayload(1)}
	if err := repo.Create(ctx, valid); err != nil {
		t.Fatalf("create valid: %v", err)
	}
	missing := &model.Notification{UserID: u.ID, Type: model.NotifTopicReply, Data: json.RawMessage(`{"actor_username":"alice"}`)}
	if err := repo.Create(ctx, missing); err != nil {
		t.Fatalf("create missing: %v", err)
	}
	malformed := &model.Notification{UserID: u.ID, Type: model.NotifTopicReply, Data: json.RawMessage(`{"topic_id":"not-a-number"}`)}
	if err := repo.Create(ctx, malformed); err != nil {
		t.Fatalf("create malformed: %v", err)
	}

	marked, err := repo.MarkTopicReplyGroupRead(ctx, u.ID, 1)
	if err != nil {
		t.Fatalf("MarkTopicReplyGroupRead must not error on malformed sibling rows: %v", err)
	}
	if marked != 1 {
		t.Fatalf("marked = %d, want 1 (only the row with a matching numeric topic_id)", marked)
	}
	assertRead(t, ctx, repo, u.ID, valid.ID, true, "valid")
	assertRead(t, ctx, repo, u.ID, missing.ID, false, "missing topic_id")
	assertRead(t, ctx, repo, u.ID, malformed.ID, false, "malformed topic_id")
}
