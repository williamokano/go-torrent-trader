package postgres

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// --- news -------------------------------------------------------------------

func TestNewsRepoCRUDAndPublishing(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewNewsRepo(db)
	author := newUser(t, db)

	draft := &model.NewsArticle{Title: "Draft", Body: "wip", AuthorID: &author.ID}
	if err := repo.Create(ctx, draft); err != nil {
		t.Fatalf("Create: %v", err)
	}
	live := &model.NewsArticle{Title: "Live", Body: "news", AuthorID: &author.ID, Published: true}
	if err := repo.Create(ctx, live); err != nil {
		t.Fatalf("Create(published): %v", err)
	}

	got, err := repo.GetByID(ctx, draft.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Title != "Draft" || got.AuthorName == nil || *got.AuthorName != author.Username {
		t.Errorf("title=%q author=%v — the author JOIN should resolve", got.Title, got.AuthorName)
	}

	// An unpublished article must not be reachable through the public getter.
	if _, err := repo.GetPublishedByID(ctx, draft.ID); err == nil {
		t.Error("GetPublishedByID returned a draft — unpublished news must stay hidden")
	}
	if _, err := repo.GetPublishedByID(ctx, live.ID); err != nil {
		t.Errorf("GetPublishedByID(live): %v", err)
	}

	published, total, err := repo.ListPublished(ctx, 1, 10)
	if err != nil {
		t.Fatalf("ListPublished: %v", err)
	}
	if total != 1 || len(published) != 1 || published[0].ID != live.ID {
		t.Errorf("ListPublished returned %d (total %d), want only the published article", len(published), total)
	}

	all, total, err := repo.List(ctx, repository.ListNewsOptions{Page: 1, PerPage: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 2 || len(all) != 2 {
		t.Errorf("List returned %d (total %d), want both articles", len(all), total)
	}

	drafts, _, err := repo.List(ctx, repository.ListNewsOptions{
		Published: ptr(false), Page: 1, PerPage: 10,
	})
	if err != nil {
		t.Fatalf("List(unpublished): %v", err)
	}
	if len(drafts) != 1 || drafts[0].ID != draft.ID {
		t.Errorf("published=false returned %d, want the draft", len(drafts))
	}

	draft.Title = "Published now"
	draft.Published = true
	if err := repo.Update(ctx, draft); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if _, err := repo.GetPublishedByID(ctx, draft.ID); err != nil {
		t.Errorf("the article was published but GetPublishedByID still hides it: %v", err)
	}

	if err := repo.Delete(ctx, draft.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := repo.GetByID(ctx, draft.ID); err == nil {
		t.Error("GetByID succeeded after Delete")
	}
}

// --- reports ----------------------------------------------------------------

func TestReportRepoCreateListResolve(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewReportRepo(db)
	reporter := newUser(t, db)
	staff := newUser(t, db)
	tor := newTorrent(t, db, reporter.ID)

	rep := &model.Report{ReporterID: reporter.ID, TorrentID: &tor.ID, Reason: "fake"}
	if err := repo.Create(ctx, rep); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetByID(ctx, rep.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	// The repository returns the raw row; ReporterUsername/TorrentName are
	// denormalised fields the service layer fills in, not this query.
	if got.Reason != "fake" || got.ReporterID != reporter.ID || got.TorrentID == nil || *got.TorrentID != tor.ID {
		t.Errorf("report not stored: reason=%q reporter=%d torrent=%v", got.Reason, got.ReporterID, got.TorrentID)
	}

	// Duplicate detection: the same reporter must not double-report a torrent.
	exists, err := repo.ExistsByReporterAndTorrent(ctx, reporter.ID, &tor.ID)
	if err != nil {
		t.Fatalf("ExistsByReporterAndTorrent: %v", err)
	}
	if !exists {
		t.Error("expected the existing report to be detected")
	}

	other := newUser(t, db)
	exists, err = repo.ExistsByReporterAndTorrent(ctx, other.ID, &tor.ID)
	if err != nil {
		t.Fatalf("ExistsByReporterAndTorrent(other): %v", err)
	}
	if exists {
		t.Error("a different reporter must be allowed to report the same torrent")
	}

	pending, total, err := repo.List(ctx, repository.ListReportsOptions{
		Status: ptr("pending"), Page: 1, PerPage: 10,
	})
	if err != nil {
		t.Fatalf("List(pending): %v", err)
	}
	if total != 1 || len(pending) != 1 {
		t.Errorf("pending = %d (total %d), want 1", len(pending), total)
	}

	if err := repo.Resolve(ctx, rep.ID, staff.ID); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	got, err = repo.GetByID(ctx, rep.ID)
	if err != nil {
		t.Fatalf("GetByID after resolve: %v", err)
	}
	if !got.Resolved || got.ResolvedBy == nil || *got.ResolvedBy != staff.ID || got.ResolvedAt == nil {
		t.Errorf("resolve did not persist: resolved=%v by=%v at=%v", got.Resolved, got.ResolvedBy, got.ResolvedAt)
	}

	pending, _, err = repo.List(ctx, repository.ListReportsOptions{
		Status: ptr("pending"), Page: 1, PerPage: 10,
	})
	if err != nil {
		t.Fatalf("List(pending) after resolve: %v", err)
	}
	if len(pending) != 0 {
		t.Errorf("got %d pending reports after resolving the only one", len(pending))
	}

	resolved, _, err := repo.List(ctx, repository.ListReportsOptions{
		Status: ptr("resolved"), Page: 1, PerPage: 10,
	})
	if err != nil {
		t.Fatalf("List(resolved): %v", err)
	}
	if len(resolved) != 1 {
		t.Errorf("got %d resolved reports, want 1", len(resolved))
	}
}

// --- messages ---------------------------------------------------------------

func newMessage(t *testing.T, db *sql.DB, from, to int64, subject string) *model.Message {
	t.Helper()

	m := &model.Message{
		SenderID:   from,
		ReceiverID: to,
		Subject:    subject,
		Body:       "body",
	}
	if err := NewMessageRepo(db).Create(context.Background(), m); err != nil {
		t.Fatalf("creating message: %v", err)
	}
	return m
}

func TestMessageRepoInboxOutboxAndUnread(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewMessageRepo(db)
	alice := newUser(t, db)
	bob := newUser(t, db)

	m := newMessage(t, db, alice.ID, bob.ID, "hello")

	got, err := repo.GetByID(ctx, m.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.SenderUsername != alice.Username || got.ReceiverUsername != bob.Username {
		t.Errorf("joins not resolved: sender=%q receiver=%q", got.SenderUsername, got.ReceiverUsername)
	}

	inbox, total, err := repo.ListInbox(ctx, bob.ID, 1, 10)
	if err != nil {
		t.Fatalf("ListInbox: %v", err)
	}
	if total != 1 || len(inbox) != 1 {
		t.Errorf("bob's inbox = %d (total %d), want 1", len(inbox), total)
	}

	// The sender's inbox must not contain their own outgoing message.
	senderInbox, _, err := repo.ListInbox(ctx, alice.ID, 1, 10)
	if err != nil {
		t.Fatalf("ListInbox(alice): %v", err)
	}
	if len(senderInbox) != 0 {
		t.Errorf("alice's inbox has %d messages, want 0 — she sent it", len(senderInbox))
	}

	outbox, total, err := repo.ListOutbox(ctx, alice.ID, 1, 10)
	if err != nil {
		t.Fatalf("ListOutbox: %v", err)
	}
	if total != 1 || len(outbox) != 1 {
		t.Errorf("alice's outbox = %d (total %d), want 1", len(outbox), total)
	}

	unread, err := repo.CountUnread(ctx, bob.ID)
	if err != nil {
		t.Fatalf("CountUnread: %v", err)
	}
	if unread != 1 {
		t.Errorf("bob's unread = %d, want 1", unread)
	}

	if err := repo.MarkAsRead(ctx, m.ID, bob.ID); err != nil {
		t.Fatalf("MarkAsRead: %v", err)
	}
	unread, err = repo.CountUnread(ctx, bob.ID)
	if err != nil {
		t.Fatalf("CountUnread after read: %v", err)
	}
	if unread != 0 {
		t.Errorf("bob's unread = %d after reading, want 0", unread)
	}
}

// Deleting a PM is per-side: it disappears for the deleter but the other party
// still has their copy.
func TestMessageRepoDeleteIsPerSide(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewMessageRepo(db)
	alice := newUser(t, db)
	bob := newUser(t, db)
	m := newMessage(t, db, alice.ID, bob.ID, "hello")

	if err := repo.DeleteForUser(ctx, m.ID, bob.ID); err != nil {
		t.Fatalf("DeleteForUser: %v", err)
	}

	inbox, _, err := repo.ListInbox(ctx, bob.ID, 1, 10)
	if err != nil {
		t.Fatalf("ListInbox: %v", err)
	}
	if len(inbox) != 0 {
		t.Errorf("bob still sees %d messages after deleting", len(inbox))
	}

	outbox, _, err := repo.ListOutbox(ctx, alice.ID, 1, 10)
	if err != nil {
		t.Fatalf("ListOutbox: %v", err)
	}
	if len(outbox) != 1 {
		t.Errorf("alice's outbox = %d — the receiver deleting must not remove the sender's copy", len(outbox))
	}
}

// --- comments ---------------------------------------------------------------

func TestCommentRepoCRUD(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewCommentRepo(db)
	u := newUser(t, db)
	tor := newTorrent(t, db, u.ID)

	c := &model.Comment{TorrentID: tor.ID, UserID: u.ID, Body: "nice release"}
	if err := repo.Create(ctx, c); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetByID(ctx, c.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Body != "nice release" || got.Username != u.Username {
		t.Errorf("body=%q username=%q — the user JOIN should resolve", got.Body, got.Username)
	}

	list, total, err := repo.ListByTorrent(ctx, tor.ID, 1, 10)
	if err != nil {
		t.Fatalf("ListByTorrent: %v", err)
	}
	if total != 1 || len(list) != 1 {
		t.Errorf("got %d comments (total %d), want 1", len(list), total)
	}

	c.Body = "edited"
	if err := repo.Update(ctx, c); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, err = repo.GetByID(ctx, c.ID)
	if err != nil {
		t.Fatalf("GetByID after update: %v", err)
	}
	if got.Body != "edited" {
		t.Errorf("body = %q, want edited", got.Body)
	}

	if err := repo.Delete(ctx, c.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := repo.GetByID(ctx, c.ID); err == nil {
		t.Error("GetByID succeeded after Delete")
	}
}

// --- invites ----------------------------------------------------------------

func TestInviteRepoCreateRedeemAndCount(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewInviteRepo(db)
	inviter := newUser(t, db)
	invitee := newUser(t, db)

	inv := &model.Invite{
		InviterID: inviter.ID,
		Token:     uniq("token"),
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
	}
	if err := repo.Create(ctx, inv); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetByToken(ctx, inv.Token)
	if err != nil {
		t.Fatalf("GetByToken: %v", err)
	}
	if got.InviterID != inviter.ID || got.Redeemed {
		t.Errorf("inviter=%d redeemed=%v, want an unredeemed invite from the inviter", got.InviterID, got.Redeemed)
	}

	pending, err := repo.CountPendingByInviter(ctx, inviter.ID)
	if err != nil {
		t.Fatalf("CountPendingByInviter: %v", err)
	}
	if pending != 1 {
		t.Errorf("pending = %d, want 1", pending)
	}

	if err := repo.Redeem(ctx, inv.Token, invitee.ID); err != nil {
		t.Fatalf("Redeem: %v", err)
	}

	got, err = repo.GetByToken(ctx, inv.Token)
	if err != nil {
		t.Fatalf("GetByToken after redeem: %v", err)
	}
	if !got.Redeemed || got.InviteeID == nil || *got.InviteeID != invitee.ID {
		t.Errorf("redeem did not persist: redeemed=%v invitee=%v", got.Redeemed, got.InviteeID)
	}

	// A redeemed invite is no longer pending.
	pending, err = repo.CountPendingByInviter(ctx, inviter.ID)
	if err != nil {
		t.Fatalf("CountPendingByInviter after redeem: %v", err)
	}
	if pending != 0 {
		t.Errorf("pending = %d after redemption, want 0", pending)
	}

	list, total, err := repo.ListByInviter(ctx, inviter.ID, 1, 10)
	if err != nil {
		t.Fatalf("ListByInviter: %v", err)
	}
	if total != 1 || len(list) != 1 {
		t.Errorf("got %d invites (total %d), want 1", len(list), total)
	}
	// InviteeName is a denormalised field the service resolves; the repository
	// returns the invitee's ID.
	if list[0].InviteeID == nil || *list[0].InviteeID != invitee.ID {
		t.Errorf("invitee id = %v, want %d", list[0].InviteeID, invitee.ID)
	}
}

func TestInviteRepoGetByIDAndDelete(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewInviteRepo(db)
	inviter := newUser(t, db)

	inv := &model.Invite{
		InviterID: inviter.ID,
		Token:     uniq("token"),
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
	}
	if err := repo.Create(ctx, inv); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetByID(ctx, inv.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.InviterID != inviter.ID || got.Token != inv.Token {
		t.Errorf("GetByID returned %+v, want inviter=%d token=%s", got, inviter.ID, inv.Token)
	}

	if err := repo.Delete(ctx, inv.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if _, err := repo.GetByID(ctx, inv.ID); err == nil {
		t.Error("GetByID succeeded after Delete")
	}
}

func TestInviteRepoDeleteNotFound(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewInviteRepo(db)
	if err := repo.Delete(ctx, 999999); err == nil {
		t.Error("expected error deleting a nonexistent invite")
	}
}

func TestInviteRepoDeleteRefusesRedeemed(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewInviteRepo(db)
	inviter := newUser(t, db)
	invitee := newUser(t, db)

	inv := &model.Invite{
		InviterID: inviter.ID,
		Token:     uniq("token"),
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
	}
	if err := repo.Create(ctx, inv); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := repo.Redeem(ctx, inv.Token, invitee.ID); err != nil {
		t.Fatalf("Redeem: %v", err)
	}

	// Delete is guarded exactly like Redeem — a redeemed invite cannot be
	// deleted, even by ID. This is what closes the revoke-vs-redeem race:
	// the guard is enforced by Postgres in the same statement, not by a
	// separate check the caller could race against.
	if err := repo.Delete(ctx, inv.ID); err == nil {
		t.Error("expected Delete to refuse a redeemed invite")
	}

	got, err := repo.GetByID(ctx, inv.ID)
	if err != nil {
		t.Fatalf("expected the redeemed invite to survive the refused delete, got err=%v", err)
	}
	if !got.Redeemed || got.InviteeID == nil || *got.InviteeID != invitee.ID {
		t.Errorf("redeemed invite should be untouched, got %+v", got)
	}
}
