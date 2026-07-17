package postgres

import (
	"context"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

func TestUserRepoGetByUsernames(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewUserRepo(db)

	alice := newUser(t, db)
	bob := newUser(t, db)
	_ = newUser(t, db) // a third user that should never come back

	got, err := repo.GetByUsernames(ctx, []string{alice.Username, bob.Username, "nosuchuser"})
	if err != nil {
		t.Fatalf("GetByUsernames: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 resolved users, got %d: %+v", len(got), got)
	}
	names := map[string]bool{got[0].Username: true, got[1].Username: true}
	if !names[alice.Username] || !names[bob.Username] {
		t.Errorf("want alice and bob resolved, got %v", names)
	}
}

func TestUserRepoGetByUsernamesEmptyInput(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	repo := NewUserRepo(db)

	got, err := repo.GetByUsernames(context.Background(), nil)
	if err != nil {
		t.Fatalf("GetByUsernames(nil): %v", err)
	}
	if len(got) != 0 {
		t.Errorf("want no results for empty input, got %+v", got)
	}
}

func TestUserRepoGetByUsernamesAllUnknown(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	repo := NewUserRepo(db)

	got, err := repo.GetByUsernames(context.Background(), []string{"ghost1", "ghost2"})
	if err != nil {
		t.Fatalf("GetByUsernames: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("want no results for unknown usernames, got %+v", got)
	}
}

func TestCommentRepoMentionedUsernamesRoundTrip(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewCommentRepo(db)

	u := newUser(t, db)
	tor := newTorrent(t, db, u.ID)
	mentioned := newUser(t, db)

	c := &model.Comment{
		TorrentID:          tor.ID,
		UserID:             u.ID,
		Body:               "hey @" + mentioned.Username,
		MentionedUsernames: []string{mentioned.Username},
	}
	if err := repo.Create(ctx, c); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetByID(ctx, c.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if len(got.MentionedUsernames) != 1 || got.MentionedUsernames[0] != mentioned.Username {
		t.Errorf("GetByID mentioned_usernames = %v, want [%s]", got.MentionedUsernames, mentioned.Username)
	}

	list, _, err := repo.ListByTorrent(ctx, tor.ID, 1, 10)
	if err != nil {
		t.Fatalf("ListByTorrent: %v", err)
	}
	if len(list) != 1 || len(list[0].MentionedUsernames) != 1 || list[0].MentionedUsernames[0] != mentioned.Username {
		t.Errorf("ListByTorrent mentioned_usernames = %+v, want one entry [%s]", list, mentioned.Username)
	}

	// Editing to remove the mention clears the stored set — it isn't additive.
	c.Body = "no mentions anymore"
	c.MentionedUsernames = nil
	if err := repo.Update(ctx, c); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, err = repo.GetByID(ctx, c.ID)
	if err != nil {
		t.Fatalf("GetByID after update: %v", err)
	}
	if len(got.MentionedUsernames) != 0 {
		t.Errorf("after clearing, mentioned_usernames = %v, want empty", got.MentionedUsernames)
	}
}

func TestCommentRepoCreateWithNoMentionsStoresEmptyArray(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewCommentRepo(db)

	u := newUser(t, db)
	tor := newTorrent(t, db, u.ID)

	c := &model.Comment{TorrentID: tor.ID, UserID: u.ID, Body: "no mentions here"}
	if err := repo.Create(ctx, c); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetByID(ctx, c.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.MentionedUsernames == nil {
		t.Error("MentionedUsernames should unmarshal to an empty (non-nil) slice, not nil")
	}
	if len(got.MentionedUsernames) != 0 {
		t.Errorf("MentionedUsernames = %v, want empty", got.MentionedUsernames)
	}
}

// A malformed mentioned_usernames value (hand-edited data, a future migration
// mistake) must degrade to "no mentions" for that one row — mentioned_usernames
// is a pure rendering aid, so it must never take down the whole comment/post,
// let alone the page it's listed on.
func TestCommentRepoMalformedMentionedUsernamesDegradesGracefully(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewCommentRepo(db)

	u := newUser(t, db)
	tor := newTorrent(t, db, u.ID)
	c := &model.Comment{TorrentID: tor.ID, UserID: u.ID, Body: "fine"}
	if err := repo.Create(ctx, c); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := db.Exec(`UPDATE torrent_comments SET mentioned_usernames = '{"not":"an array"}' WHERE id = $1`, c.ID); err != nil {
		t.Fatalf("corrupting mentioned_usernames: %v", err)
	}

	got, err := repo.GetByID(ctx, c.ID)
	if err != nil {
		t.Fatalf("GetByID should degrade, not error, on malformed mentioned_usernames: %v", err)
	}
	if len(got.MentionedUsernames) != 0 {
		t.Errorf("MentionedUsernames = %v, want empty for a malformed row", got.MentionedUsernames)
	}

	list, total, err := repo.ListByTorrent(ctx, tor.ID, 1, 10)
	if err != nil {
		t.Fatalf("ListByTorrent should degrade, not error, on a malformed row: %v", err)
	}
	if total != 1 || len(list) != 1 {
		t.Fatalf("want the corrupted row still listed, got %d (total %d)", len(list), total)
	}
	if len(list[0].MentionedUsernames) != 0 {
		t.Errorf("ListByTorrent MentionedUsernames = %v, want empty for the malformed row", list[0].MentionedUsernames)
	}
}

func TestForumPostRepoMalformedMentionedUsernamesDegradesGracefully(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewForumPostRepo(db)
	f := newForumFixture(t, db)

	post := &model.ForumPost{TopicID: f.TopicID, UserID: f.UserID, Body: "fine"}
	if err := repo.Create(ctx, post); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := db.Exec(`UPDATE forum_posts SET mentioned_usernames = '"just a string"' WHERE id = $1`, post.ID); err != nil {
		t.Fatalf("corrupting mentioned_usernames: %v", err)
	}

	got, err := repo.GetByID(ctx, post.ID)
	if err != nil {
		t.Fatalf("GetByID should degrade, not error, on malformed mentioned_usernames: %v", err)
	}
	if len(got.MentionedUsernames) != 0 {
		t.Errorf("MentionedUsernames = %v, want empty for a malformed row", got.MentionedUsernames)
	}

	list, total, err := repo.ListByTopic(ctx, f.TopicID, 1, 10)
	if err != nil {
		t.Fatalf("ListByTopic should degrade, not error, on a malformed row: %v", err)
	}
	if total != 1 || len(list) != 1 {
		t.Fatalf("want the corrupted row still listed, got %d (total %d)", len(list), total)
	}
	if len(list[0].MentionedUsernames) != 0 {
		t.Errorf("ListByTopic MentionedUsernames = %v, want empty for the malformed row", list[0].MentionedUsernames)
	}
}

func TestForumPostRepoMentionedUsernamesRoundTrip(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewForumPostRepo(db)
	f := newForumFixture(t, db)
	mentioned := newUser(t, db)

	post := &model.ForumPost{
		TopicID:            f.TopicID,
		UserID:             f.UserID,
		Body:               "hey @" + mentioned.Username,
		MentionedUsernames: []string{mentioned.Username},
	}
	if err := repo.Create(ctx, post); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetByID(ctx, post.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if len(got.MentionedUsernames) != 1 || got.MentionedUsernames[0] != mentioned.Username {
		t.Errorf("GetByID mentioned_usernames = %v, want [%s]", got.MentionedUsernames, mentioned.Username)
	}

	list, _, err := repo.ListByTopic(ctx, f.TopicID, 1, 10)
	if err != nil {
		t.Fatalf("ListByTopic: %v", err)
	}
	var found bool
	for _, p := range list {
		if p.ID == post.ID {
			found = true
			if len(p.MentionedUsernames) != 1 || p.MentionedUsernames[0] != mentioned.Username {
				t.Errorf("ListByTopic mentioned_usernames = %v, want [%s]", p.MentionedUsernames, mentioned.Username)
			}
		}
	}
	if !found {
		t.Fatal("post not found in ListByTopic results")
	}

	got.Body = "edited, mentions someone else now"
	other := newUser(t, db)
	got.MentionedUsernames = []string{other.Username}
	if err := repo.Update(ctx, got); err != nil {
		t.Fatalf("Update: %v", err)
	}
	updated, err := repo.GetByID(ctx, post.ID)
	if err != nil {
		t.Fatalf("GetByID after update: %v", err)
	}
	if len(updated.MentionedUsernames) != 1 || updated.MentionedUsernames[0] != other.Username {
		t.Errorf("after edit, mentioned_usernames = %v, want [%s]", updated.MentionedUsernames, other.Username)
	}
}

func TestMessageRepoMentionedUsernamesRoundTrip(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewMessageRepo(db)

	sender := newUser(t, db)
	receiver := newUser(t, db)
	mentioned := newUser(t, db)

	msg := &model.Message{
		SenderID:           sender.ID,
		ReceiverID:         receiver.ID,
		Subject:            "hi",
		Body:               "hey @" + mentioned.Username,
		MentionedUsernames: []string{mentioned.Username},
	}
	if err := repo.Create(ctx, msg); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetByID(ctx, msg.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if len(got.MentionedUsernames) != 1 || got.MentionedUsernames[0] != mentioned.Username {
		t.Errorf("GetByID mentioned_usernames = %v, want [%s]", got.MentionedUsernames, mentioned.Username)
	}

	inbox, _, err := repo.ListInbox(ctx, receiver.ID, 1, 10)
	if err != nil {
		t.Fatalf("ListInbox: %v", err)
	}
	if len(inbox) != 1 || len(inbox[0].MentionedUsernames) != 1 || inbox[0].MentionedUsernames[0] != mentioned.Username {
		t.Errorf("ListInbox mentioned_usernames = %+v, want one entry [%s]", inbox, mentioned.Username)
	}

	outbox, _, err := repo.ListOutbox(ctx, sender.ID, 1, 10)
	if err != nil {
		t.Fatalf("ListOutbox: %v", err)
	}
	if len(outbox) != 1 || len(outbox[0].MentionedUsernames) != 1 || outbox[0].MentionedUsernames[0] != mentioned.Username {
		t.Errorf("ListOutbox mentioned_usernames = %+v, want one entry [%s]", outbox, mentioned.Username)
	}
}

func TestMessageRepoCreateWithNoMentionsStoresEmptyArray(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewMessageRepo(db)

	sender := newUser(t, db)
	receiver := newUser(t, db)

	msg := &model.Message{SenderID: sender.ID, ReceiverID: receiver.ID, Subject: "hi", Body: "no mentions here"}
	if err := repo.Create(ctx, msg); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetByID(ctx, msg.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.MentionedUsernames == nil {
		t.Error("MentionedUsernames should unmarshal to an empty (non-nil) slice, not nil")
	}
	if len(got.MentionedUsernames) != 0 {
		t.Errorf("MentionedUsernames = %v, want empty", got.MentionedUsernames)
	}
}

func TestMessageRepoMalformedMentionedUsernamesDegradesGracefully(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewMessageRepo(db)

	sender := newUser(t, db)
	receiver := newUser(t, db)
	msg := &model.Message{SenderID: sender.ID, ReceiverID: receiver.ID, Subject: "hi", Body: "fine"}
	if err := repo.Create(ctx, msg); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := db.Exec(`UPDATE messages SET mentioned_usernames = '{"not":"an array"}' WHERE id = $1`, msg.ID); err != nil {
		t.Fatalf("corrupting mentioned_usernames: %v", err)
	}

	got, err := repo.GetByID(ctx, msg.ID)
	if err != nil {
		t.Fatalf("GetByID should degrade, not error, on malformed mentioned_usernames: %v", err)
	}
	if len(got.MentionedUsernames) != 0 {
		t.Errorf("MentionedUsernames = %v, want empty for a malformed row", got.MentionedUsernames)
	}

	inbox, total, err := repo.ListInbox(ctx, receiver.ID, 1, 10)
	if err != nil {
		t.Fatalf("ListInbox should degrade, not error, on a malformed row: %v", err)
	}
	if total != 1 || len(inbox) != 1 {
		t.Fatalf("want the corrupted row still listed, got %d (total %d)", len(inbox), total)
	}
	if len(inbox[0].MentionedUsernames) != 0 {
		t.Errorf("ListInbox MentionedUsernames = %v, want empty for the malformed row", inbox[0].MentionedUsernames)
	}
}
