package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

// A live (non-deleted) post's response includes its resolved mentions.
func TestGetTopicIncludesMentionedUsernames(t *testing.T) {
	d := newForumDeps()
	d.topics.topics[100] = &model.ForumTopic{ID: 100, ForumID: 1, UserID: 9, CreatedAt: time.Now().Add(-time.Hour)}
	d.posts.list = []model.ForumPost{
		{ID: 501, TopicID: 100, UserID: 9, Body: "hi @carol", MentionedUsernames: []string{"carol"}},
	}
	d.posts.total = 1
	h := d.handler()

	req := withForumAuth(httptest.NewRequest(http.MethodGet, "/api/v1/forums/topics/100", nil), 7, memberPerms())
	req = withURLParam(req, "id", "100")
	w := httptest.NewRecorder()
	h.HandleGetTopic(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := decodeBody(t, w)
	post := body["posts"].([]interface{})[0].(map[string]interface{})
	mentioned, ok := post["mentioned_usernames"].([]interface{})
	if !ok || len(mentioned) != 1 || mentioned[0] != "carol" {
		t.Errorf("mentioned_usernames = %v, want [carol]", post["mentioned_usernames"])
	}
}

// A soft-deleted post's mentioned_usernames must be redacted the same way
// its body is — a non-staff reader who can't see what a post said must not
// be able to see who it mentioned either.
func TestGetTopicHidesDeletedPostMentionsFromMembers(t *testing.T) {
	d := newForumDeps()
	deletedAt := time.Now()
	deletedBy := int64(1)
	d.topics.topics[100] = &model.ForumTopic{ID: 100, ForumID: 1, UserID: 9, CreatedAt: time.Now().Add(-time.Hour)}
	d.posts.list = []model.ForumPost{
		{
			ID: 501, TopicID: 100, UserID: 9, Body: "naughty @carol",
			MentionedUsernames: []string{"carol"},
			DeletedAt:          &deletedAt, DeletedBy: &deletedBy,
		},
	}
	d.posts.total = 1
	h := d.handler()

	req := withForumAuth(httptest.NewRequest(http.MethodGet, "/api/v1/forums/topics/100", nil), 7, memberPerms())
	req = withURLParam(req, "id", "100")
	w := httptest.NewRecorder()
	h.HandleGetTopic(w, req)

	body := decodeBody(t, w)
	post := body["posts"].([]interface{})[0].(map[string]interface{})
	if post["body"] != "" {
		t.Errorf("body = %q, want it redacted for a non-staff reader", post["body"])
	}
	if mentioned := post["mentioned_usernames"]; mentioned != nil {
		t.Errorf("mentioned_usernames = %v, want redacted (nil) alongside the body", mentioned)
	}
}

// Staff can still see who a soft-deleted post mentioned, same as the body.
func TestGetTopicShowsDeletedPostMentionsToStaff(t *testing.T) {
	d := newForumDeps()
	deletedAt := time.Now()
	deletedBy := int64(1)
	d.topics.topics[100] = &model.ForumTopic{ID: 100, ForumID: 1, UserID: 7, CreatedAt: time.Now().Add(-time.Hour)}
	d.posts.list = []model.ForumPost{
		{
			ID: 501, TopicID: 100, UserID: 7, Body: "naughty @carol",
			MentionedUsernames: []string{"carol"},
			DeletedAt:          &deletedAt, DeletedBy: &deletedBy,
		},
	}
	d.posts.total = 1
	h := d.handler()

	req := withForumAuth(httptest.NewRequest(http.MethodGet, "/api/v1/forums/topics/100", nil), 1, staffPerms())
	req = withURLParam(req, "id", "100")
	w := httptest.NewRecorder()
	h.HandleGetTopic(w, req)

	body := decodeBody(t, w)
	post := body["posts"].([]interface{})[0].(map[string]interface{})
	mentioned, ok := post["mentioned_usernames"].([]interface{})
	if !ok || len(mentioned) != 1 || mentioned[0] != "carol" {
		t.Errorf("mentioned_usernames = %v, want staff to still see [carol]", post["mentioned_usernames"])
	}
}
