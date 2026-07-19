package postgres

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// blockingUserRepo wraps a real repository.UserRepository and gates its
// GetByID call: the first call closes reachedGate, then blocks until release
// is closed. ForumService.EditPost reads the post itself (the call that
// anchors its diff) before it ever reaches users.GetByID, so gating this
// call lets a test deterministically pause a goroutine after it has already
// captured the post's starting body, but before it attempts its write —
// reproducing the exact interleaving BE-9.25 closes without depending on
// goroutine-scheduling luck or artificial sleeps.
type blockingUserRepo struct {
	repository.UserRepository
	reachedGate chan struct{}
	release     <-chan struct{}
	once        sync.Once
}

func (b *blockingUserRepo) GetByID(ctx context.Context, id int64) (*model.User, error) {
	b.once.Do(func() { close(b.reachedGate) })
	<-b.release
	return b.UserRepository.GetByID(ctx, id)
}

// EditPost used to read a post, compute a diff, then write the edit-history
// row and the new body as two independent, non-transactional statements: a
// second edit racing in between could commit against the same stale body,
// leaving a diff on record that no longer matched the post's actual history
// (BE-9.25's origin — see the comment on ForumService.EditPost).
//
// This drives two genuinely-interleaved EditPost calls against a real
// Postgres: goroutine B reads the post before goroutine A's conflicting
// write commits, but B's own write only happens after — the precise
// interleaving that used to corrupt history. It proves the fix end to end:
// exactly one call lands, the other comes back as service.ErrPostEditConflict,
// the post ends up with the winner's body, and no phantom edit-history row
// is left behind for the rejected write.
func TestForumServiceEditPostConcurrentEditsOneWinsOneConflicts(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	f := newForumFixture(t, db)
	postID := newPost(t, db, f.TopicID, f.UserID, "original body")

	userRepo := NewUserRepo(db)
	postRepo := NewForumPostRepo(db)
	topicRepo := NewForumTopicRepo(db)
	forumRepo := NewForumRepo(db)

	reachedGate := make(chan struct{})
	release := make(chan struct{})
	gatedUsers := &blockingUserRepo{UserRepository: userRepo, reachedGate: reachedGate, release: release}

	// Both services share the same underlying *sql.DB (connection pool),
	// exactly like two concurrent HTTP requests would in production. Only
	// goroutine B's user repository is gated; goroutine A runs unimpeded.
	svcA := service.NewForumService(db, nil, forumRepo, topicRepo, postRepo, userRepo, nil, nil)
	svcB := service.NewForumService(db, nil, forumRepo, topicRepo, postRepo, gatedUsers, nil, nil)

	perms := model.Permissions{Level: 0, CanForum: true}

	type editResult struct {
		post *model.ForumPost
		err  error
	}
	bDone := make(chan editResult, 1)
	go func() {
		p, err := svcB.EditPost(ctx, postID, f.UserID, perms, "B's edit (stale, should conflict)", "")
		bDone <- editResult{p, err}
	}()

	// Block the test goroutine until B has read the post — still "original
	// body" — and is paused immediately before its own write. Also watch for
	// B finishing early (without ever reaching the gate) or simply taking too
	// long: either means EditPost's internal call order changed so gating
	// users.GetByID no longer sits after the post read and before the write,
	// and this test needs its gate moved rather than hanging silently for the
	// full `go test` timeout.
	select {
	case <-reachedGate:
	case b := <-bDone:
		t.Fatalf("goroutine B returned (post=%v, err=%v) before reaching the gate — "+
			"EditPost's call order changed, so gating users.GetByID no longer captures "+
			"the interleaving this test relies on; update the gate to match", b.post, b.err)
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for goroutine B to reach the gate — " +
			"EditPost's call order may have changed so users.GetByID (what this test " +
			"gates) is no longer reached before the write; update the gate to match")
	}

	aPost, err := svcA.EditPost(ctx, postID, f.UserID, perms, "A's edit (should win)", "")
	if err != nil {
		t.Fatalf("A's EditPost: unexpected error: %v", err)
	}
	if aPost.Body != "A's edit (should win)" {
		t.Fatalf("A's post body = %q, want %q", aPost.Body, "A's edit (should win)")
	}

	// A has committed; only now release B to attempt its now-stale write.
	close(release)
	b := <-bDone

	if !errors.Is(b.err, service.ErrPostEditConflict) {
		t.Fatalf("B's EditPost err = %v, want service.ErrPostEditConflict", b.err)
	}
	if b.post != nil {
		t.Errorf("B's EditPost returned a non-nil post on conflict: %+v", b.post)
	}

	final, err := postRepo.GetByID(ctx, postID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if final.Body != "A's edit (should win)" {
		t.Errorf("final body = %q, want %q — B's conflicting write must not have landed", final.Body, "A's edit (should win)")
	}

	// Exactly one edit-history row: B's CreateEdit must have been rolled back
	// along with its conflicting Update inside the same transaction, not left
	// dangling as a diff computed against a body the post never actually had.
	edits, err := postRepo.ListEdits(ctx, postID)
	if err != nil {
		t.Fatalf("ListEdits: %v", err)
	}
	if len(edits) != 1 {
		t.Fatalf("len(edits) = %d, want 1 (B's rejected edit must not leave a history row)", len(edits))
	}
	if edits[0].Diff == "" {
		t.Error("the surviving edit's diff is empty")
	}
}
