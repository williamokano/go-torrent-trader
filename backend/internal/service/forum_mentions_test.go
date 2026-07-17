package service

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

func forumUsersWithMentionTarget() *mockForumUserRepo {
	return &mockForumUserRepo{
		user:       &model.User{ID: 1, CanForum: true},
		byUsername: map[string]*model.User{"carol": {ID: 9, Username: "carol"}},
	}
}

func TestCreateTopic_SetsMentionedUsernames(t *testing.T) {
	forums := &mockForumRepo{forumByID: map[int64]*model.Forum{1: {ID: 1, MinGroupLevel: 0}}}
	svc := NewForumService(nil, nil, forums, &mockForumTopicRepo{}, &mockForumPostRepo{}, forumUsersWithMentionTarget(), nil, nil)

	_, post, err := svc.CreateTopic(context.Background(), 1, 1, model.Permissions{}, "Title", "opening post, hi @carol")
	if err != nil {
		t.Fatalf("CreateTopic: %v", err)
	}
	if !reflect.DeepEqual(post.MentionedUsernames, []string{"carol"}) {
		t.Errorf("MentionedUsernames = %v, want [carol]", post.MentionedUsernames)
	}
}

func TestCreateTopic_NoMentionsSetsEmptyMentionedUsernames(t *testing.T) {
	forums := &mockForumRepo{forumByID: map[int64]*model.Forum{1: {ID: 1, MinGroupLevel: 0}}}
	svc := NewForumService(nil, nil, forums, &mockForumTopicRepo{}, &mockForumPostRepo{}, forumUsersWithMentionTarget(), nil, nil)

	_, post, err := svc.CreateTopic(context.Background(), 1, 1, model.Permissions{}, "Title", "no mentions here")
	if err != nil {
		t.Fatalf("CreateTopic: %v", err)
	}
	if post.MentionedUsernames == nil {
		t.Error("MentionedUsernames should be an empty slice, not nil")
	}
	if len(post.MentionedUsernames) != 0 {
		t.Errorf("MentionedUsernames = %v, want empty", post.MentionedUsernames)
	}
}

func TestCreatePost_SetsMentionedUsernames(t *testing.T) {
	topics := &mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{1: {ID: 1, ForumID: 1}}}
	forums := &mockForumRepo{forumByID: map[int64]*model.Forum{1: {ID: 1, MinGroupLevel: 0}}}
	// Empty postByID: CreatePost's post-create GetByID misses, and it falls
	// back to returning the in-memory struct it just built — exactly what
	// this test wants to assert on.
	posts := &mockForumPostRepo{postByID: map[int64]*model.ForumPost{}}
	svc := NewForumService(nil, nil, forums, topics, posts, forumUsersWithMentionTarget(), nil, nil)

	post, err := svc.CreatePost(context.Background(), 1, 1, model.Permissions{}, "reply mentioning @carol", nil)
	if err != nil {
		t.Fatalf("CreatePost: %v", err)
	}
	if !reflect.DeepEqual(post.MentionedUsernames, []string{"carol"}) {
		t.Errorf("MentionedUsernames = %v, want [carol]", post.MentionedUsernames)
	}
}

func TestEditPost_ReResolvesMentionedUsernamesOnEdit(t *testing.T) {
	existing := &model.ForumPost{ID: 200, TopicID: 1, UserID: 1, Body: "no mentions at first"}
	topics := &mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{1: {ID: 1, ForumID: 1}}}
	forums := &mockForumRepo{forumByID: map[int64]*model.Forum{1: {ID: 1, MinGroupLevel: 0}}}
	posts := &mockForumPostRepo{postByID: map[int64]*model.ForumPost{200: existing}}
	svc := NewForumService(nil, nil, forums, topics, posts, forumUsersWithMentionTarget(), nil, nil)

	updated, err := svc.EditPost(context.Background(), 200, 1, model.Permissions{}, "edited to mention @carol")
	if err != nil {
		t.Fatalf("EditPost: %v", err)
	}
	if !reflect.DeepEqual(updated.MentionedUsernames, []string{"carol"}) {
		t.Errorf("after edit, MentionedUsernames = %v, want [carol]", updated.MentionedUsernames)
	}
}

// A failed mention resolution must abort before any write, not land between
// CreateEdit and Update — otherwise edit history would record an edit that
// never actually reached the post (see the devil's-advocate review finding
// this reordering fixed).
func TestEditPost_ResolveFailureRecordsNoEditHistory(t *testing.T) {
	existing := &model.ForumPost{ID: 200, TopicID: 1, UserID: 1, Body: "original"}
	topics := &mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{1: {ID: 1, ForumID: 1}}}
	forums := &mockForumRepo{forumByID: map[int64]*model.Forum{1: {ID: 1, MinGroupLevel: 0}}}
	posts := &mockForumPostRepo{postByID: map[int64]*model.ForumPost{200: existing}}
	users := &mockForumUserRepo{
		user:         &model.User{ID: 1, CanForum: true},
		usernamesErr: errors.New("db unavailable"),
	}
	svc := NewForumService(nil, nil, forums, topics, posts, users, nil, nil)

	_, err := svc.EditPost(context.Background(), 200, 1, model.Permissions{}, "edited to mention @carol")
	if err == nil {
		t.Fatal("expected an error when mention resolution fails")
	}
	if posts.createdEdit != nil {
		t.Errorf("CreateEdit was called (%+v) despite resolution failing first — the edit-history/update pair can now diverge", posts.createdEdit)
	}
	if posts.updated != nil {
		t.Errorf("Update was called despite resolution failing")
	}
	if existing.Body != "original" {
		t.Errorf("post body mutated to %q despite the failed edit", existing.Body)
	}
}

func TestEditPost_DoesNotPublishMentionNotification(t *testing.T) {
	// The create-vs-edit notification split from the plan: EditPost updates
	// mentioned_usernames for rendering but must never call publishMention —
	// only CreateTopic/CreatePost notify, so editing a post to add a brand
	// new @mention does not spam a notification on every save.
	existing := &model.ForumPost{ID: 200, TopicID: 1, UserID: 1, Body: "no mentions at first"}
	topics := &mockForumTopicRepo{topicByID: map[int64]*model.ForumTopic{1: {ID: 1, ForumID: 1}}}
	forums := &mockForumRepo{forumByID: map[int64]*model.Forum{1: {ID: 1, MinGroupLevel: 0}}}
	posts := &mockForumPostRepo{postByID: map[int64]*model.ForumPost{200: existing}}
	bus := event.NewInMemoryBus()
	fired := false
	bus.Subscribe(event.UserMentioned, func(_ context.Context, _ event.Event) error {
		fired = true
		return nil
	})
	svc := NewForumService(nil, nil, forums, topics, posts, forumUsersWithMentionTarget(), nil, bus)

	if _, err := svc.EditPost(context.Background(), 200, 1, model.Permissions{}, "edited to mention @carol"); err != nil {
		t.Fatalf("EditPost: %v", err)
	}
	if fired {
		t.Error("EditPost must not publish a UserMentioned notification")
	}
}
