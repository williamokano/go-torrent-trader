package service_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

// setupCommentService's user repo is seeded with "sender" (id 1) and
// "receiver" (id 2) — see newMockUserRepoForMessage in message_test.go.

func TestCreateComment_SetsMentionedUsernames(t *testing.T) {
	svc := setupCommentService()

	comment, err := svc.CreateComment(context.Background(), 1, 10, "great work @sender")
	if err != nil {
		t.Fatalf("CreateComment: %v", err)
	}
	if !reflect.DeepEqual(comment.MentionedUsernames, []string{"sender"}) {
		t.Errorf("MentionedUsernames = %v, want [sender]", comment.MentionedUsernames)
	}
}

func TestCreateComment_NoMentionsSetsEmptyMentionedUsernames(t *testing.T) {
	svc := setupCommentService()

	comment, err := svc.CreateComment(context.Background(), 1, 10, "no mentions here")
	if err != nil {
		t.Fatalf("CreateComment: %v", err)
	}
	if comment.MentionedUsernames == nil {
		t.Error("MentionedUsernames should be an empty slice, not nil")
	}
	if len(comment.MentionedUsernames) != 0 {
		t.Errorf("MentionedUsernames = %v, want empty", comment.MentionedUsernames)
	}
}

func TestCreateComment_DropsUnresolvedMentions(t *testing.T) {
	svc := setupCommentService()

	comment, err := svc.CreateComment(context.Background(), 1, 10, "hey @totallyfakeuser")
	if err != nil {
		t.Fatalf("CreateComment: %v", err)
	}
	if len(comment.MentionedUsernames) != 0 {
		t.Errorf("MentionedUsernames = %v, want empty — the username doesn't exist", comment.MentionedUsernames)
	}
}

func TestUpdateComment_ReResolvesMentionedUsernamesOnEdit(t *testing.T) {
	svc := setupCommentService()

	created, err := svc.CreateComment(context.Background(), 1, 10, "no mentions at first")
	if err != nil {
		t.Fatalf("CreateComment: %v", err)
	}
	if len(created.MentionedUsernames) != 0 {
		t.Fatalf("precondition failed: expected no mentions, got %v", created.MentionedUsernames)
	}

	updated, err := svc.UpdateComment(context.Background(), created.ID, 10, model.Permissions{}, "edited to mention @sender")
	if err != nil {
		t.Fatalf("UpdateComment: %v", err)
	}
	if !reflect.DeepEqual(updated.MentionedUsernames, []string{"sender"}) {
		t.Errorf("after edit, MentionedUsernames = %v, want [sender]", updated.MentionedUsernames)
	}
}

func TestUpdateComment_RemovingMentionClearsMentionedUsernames(t *testing.T) {
	svc := setupCommentService()

	created, err := svc.CreateComment(context.Background(), 1, 10, "mentioning @sender")
	if err != nil {
		t.Fatalf("CreateComment: %v", err)
	}
	if len(created.MentionedUsernames) != 1 {
		t.Fatalf("precondition failed: expected one mention, got %v", created.MentionedUsernames)
	}

	updated, err := svc.UpdateComment(context.Background(), created.ID, 10, model.Permissions{}, "no more mentions")
	if err != nil {
		t.Fatalf("UpdateComment: %v", err)
	}
	if len(updated.MentionedUsernames) != 0 {
		t.Errorf("after removing the mention, MentionedUsernames = %v, want empty", updated.MentionedUsernames)
	}
}
