package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

func TestResolveMentionedUsernames_ResolvesRealUsers(t *testing.T) {
	users := newMockUserRepoForMessage() // seeded with "sender" (id 1), "receiver" (id 2)

	got, err := service.ResolveMentionedUsernames(context.Background(), users, "hey @sender, check @receiver out")
	if err != nil {
		t.Fatalf("ResolveMentionedUsernames: %v", err)
	}
	want := map[string]bool{"sender": true, "receiver": true}
	if len(got) != 2 || !want[got[0]] || !want[got[1]] {
		t.Errorf("got %v, want [sender receiver] in some order", got)
	}
}

func TestResolveMentionedUsernames_DropsUnknownTokens(t *testing.T) {
	users := newMockUserRepoForMessage()

	got, err := service.ResolveMentionedUsernames(context.Background(), users, "hey @sender and @totallyfakeuser")
	if err != nil {
		t.Fatalf("ResolveMentionedUsernames: %v", err)
	}
	if len(got) != 1 || got[0] != "sender" {
		t.Errorf("got %v, want [sender] — the unknown token must not appear", got)
	}
}

func TestResolveMentionedUsernames_NoMentionsReturnsEmptyNotNil(t *testing.T) {
	users := newMockUserRepoForMessage()

	got, err := service.ResolveMentionedUsernames(context.Background(), users, "no mentions in this body")
	if err != nil {
		t.Fatalf("ResolveMentionedUsernames: %v", err)
	}
	if got == nil {
		t.Error("want a non-nil empty slice for no-mentions, got nil")
	}
	if len(got) != 0 {
		t.Errorf("got %v, want empty", got)
	}
}

func TestResolveMentionedUsernames_DedupesRepeatedMentions(t *testing.T) {
	users := newMockUserRepoForMessage()

	got, err := service.ResolveMentionedUsernames(context.Background(), users, "@sender @sender @sender")
	if err != nil {
		t.Fatalf("ResolveMentionedUsernames: %v", err)
	}
	if len(got) != 1 || got[0] != "sender" {
		t.Errorf("got %v, want a single deduped [sender]", got)
	}
}

func TestResolveMentionedUsernames_IgnoresEmailShapedText(t *testing.T) {
	users := newMockUserRepoForMessage()

	// "sender" is a real, resolvable username, but "x@sender" is not a
	// mention — the same left-boundary rule the backend regex enforces.
	got, err := service.ResolveMentionedUsernames(context.Background(), users, "contact me at x@sender")
	if err != nil {
		t.Fatalf("ResolveMentionedUsernames: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %v, want empty — x@sender is not a mention", got)
	}
}

var errMentionRepoBoom = errors.New("boom")

// failingUsersRepo wraps the message-test mock but fails username
// resolution, to prove ResolveMentionedUsernames propagates repo errors
// instead of swallowing them.
type failingUsersRepo struct {
	*mockUserRepoForMessage
}

func (f *failingUsersRepo) GetByUsernames(context.Context, []string) ([]model.User, error) {
	return nil, errMentionRepoBoom
}

func TestResolveMentionedUsernames_PropagatesRepoError(t *testing.T) {
	users := &failingUsersRepo{mockUserRepoForMessage: newMockUserRepoForMessage()}

	_, err := service.ResolveMentionedUsernames(context.Background(), users, "hey @sender")
	if err == nil {
		t.Fatal("expected an error to propagate")
	}
	if !errors.Is(err, errMentionRepoBoom) {
		t.Errorf("expected wrapped errMentionRepoBoom, got %v", err)
	}
}
