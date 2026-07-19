package postgres

import (
	"context"
	"database/sql"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

func newSavedMessage(t *testing.T, db *sql.DB, userID int64, kind model.SavedMessageKind, receiverID *int64) *model.SavedMessage {
	t.Helper()

	sm := &model.SavedMessage{
		UserID:     userID,
		Kind:       kind,
		ReceiverID: receiverID,
		Subject:    "subject",
		Body:       "body",
	}
	if err := NewSavedMessageRepo(db).Create(context.Background(), sm); err != nil {
		t.Fatalf("creating saved message: %v", err)
	}
	return sm
}

func TestSavedMessageRepoCreateGetUpdate(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewSavedMessageRepo(db)
	alice := newUser(t, db)
	bob := newUser(t, db)

	draft := newSavedMessage(t, db, alice.ID, model.SavedMessageKindDraft, &bob.ID)

	got, err := repo.GetByID(ctx, draft.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.UserID != alice.ID || got.Kind != model.SavedMessageKindDraft {
		t.Errorf("user_id=%d kind=%q, want owner=%d kind=draft", got.UserID, got.Kind, alice.ID)
	}
	if got.ReceiverID == nil || *got.ReceiverID != bob.ID || got.ReceiverUsername != bob.Username {
		t.Errorf("receiver join not resolved: id=%v username=%q", got.ReceiverID, got.ReceiverUsername)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Error("expected created_at/updated_at to be populated")
	}

	got.Subject = "updated subject"
	got.Body = "updated body"
	got.ReceiverID = nil
	if err := repo.Update(ctx, got); err != nil {
		t.Fatalf("Update: %v", err)
	}

	reloaded, err := repo.GetByID(ctx, draft.ID)
	if err != nil {
		t.Fatalf("GetByID after update: %v", err)
	}
	if reloaded.Subject != "updated subject" || reloaded.Body != "updated body" {
		t.Errorf("update did not persist: subject=%q body=%q", reloaded.Subject, reloaded.Body)
	}
	if reloaded.ReceiverID != nil {
		t.Errorf("expected receiver_id cleared, got %v", reloaded.ReceiverID)
	}
}

func TestSavedMessageRepoGetByIDNotFound(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)

	repo := NewSavedMessageRepo(db)
	if _, err := repo.GetByID(context.Background(), 999999); err == nil {
		t.Error("expected an error for a non-existent saved message")
	}
}

// A template is never addressed to anyone, so receiver_id must stay NULL —
// this exercises the LEFT JOIN path where there's nothing to resolve.
func TestSavedMessageRepoTemplateHasNoReceiver(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewSavedMessageRepo(db)
	alice := newUser(t, db)
	tmpl := newSavedMessage(t, db, alice.ID, model.SavedMessageKindTemplate, nil)

	got, err := repo.GetByID(ctx, tmpl.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.ReceiverID != nil {
		t.Errorf("expected nil receiver_id for a template, got %v", got.ReceiverID)
	}
	if got.ReceiverUsername != "" {
		t.Errorf("expected empty receiver_username for a template, got %q", got.ReceiverUsername)
	}
}

func TestSavedMessageRepoListByUserFiltersByKindAndOwner(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewSavedMessageRepo(db)
	alice := newUser(t, db)
	bob := newUser(t, db)

	newSavedMessage(t, db, alice.ID, model.SavedMessageKindDraft, nil)
	newSavedMessage(t, db, alice.ID, model.SavedMessageKindDraft, nil)
	newSavedMessage(t, db, alice.ID, model.SavedMessageKindTemplate, nil)
	newSavedMessage(t, db, bob.ID, model.SavedMessageKindDraft, nil)

	drafts, total, err := repo.ListByUser(ctx, alice.ID, model.SavedMessageKindDraft, 1, 10)
	if err != nil {
		t.Fatalf("ListByUser(drafts): %v", err)
	}
	if total != 2 || len(drafts) != 2 {
		t.Errorf("alice's drafts = %d (total %d), want 2 — bob's draft and alice's template must not leak in", len(drafts), total)
	}

	templates, total, err := repo.ListByUser(ctx, alice.ID, model.SavedMessageKindTemplate, 1, 10)
	if err != nil {
		t.Fatalf("ListByUser(templates): %v", err)
	}
	if total != 1 || len(templates) != 1 {
		t.Errorf("alice's templates = %d (total %d), want 1", len(templates), total)
	}

	bobDrafts, total, err := repo.ListByUser(ctx, bob.ID, model.SavedMessageKindDraft, 1, 10)
	if err != nil {
		t.Fatalf("ListByUser(bob drafts): %v", err)
	}
	if total != 1 || len(bobDrafts) != 1 {
		t.Errorf("bob's drafts = %d (total %d), want 1", len(bobDrafts), total)
	}
}

func TestSavedMessageRepoDelete(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewSavedMessageRepo(db)
	alice := newUser(t, db)
	sm := newSavedMessage(t, db, alice.ID, model.SavedMessageKindDraft, nil)

	if err := repo.Delete(ctx, sm.ID, alice.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := repo.GetByID(ctx, sm.ID); err == nil {
		t.Error("GetByID succeeded after Delete")
	}
}

func TestSavedMessageRepoDeleteNotFound(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)

	repo := NewSavedMessageRepo(db)
	alice := newUser(t, db)
	if err := repo.Delete(context.Background(), 999999, alice.ID); err == nil {
		t.Error("expected an error deleting a non-existent saved message")
	}
}

// Delete and Update are scoped by user_id at the SQL level as defense-in-depth
// (see the comments on SavedMessageRepo.Update/Delete) — even though the
// service layer already checks ownership before calling either, the repo
// itself must refuse to touch a row it doesn't own if that check is ever
// bypassed or a future caller forgets it.
func TestSavedMessageRepoDeleteRefusesWrongOwner(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewSavedMessageRepo(db)
	alice := newUser(t, db)
	bob := newUser(t, db)
	sm := newSavedMessage(t, db, alice.ID, model.SavedMessageKindDraft, nil)

	if err := repo.Delete(ctx, sm.ID, bob.ID); err == nil {
		t.Error("expected an error deleting alice's draft as bob")
	}
	if _, err := repo.GetByID(ctx, sm.ID); err != nil {
		t.Errorf("alice's draft should have survived bob's delete attempt: %v", err)
	}
}

func TestSavedMessageRepoUpdateRefusesWrongOwner(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewSavedMessageRepo(db)
	alice := newUser(t, db)
	bob := newUser(t, db)
	sm := newSavedMessage(t, db, alice.ID, model.SavedMessageKindDraft, nil)

	forged := *sm
	forged.UserID = bob.ID
	forged.Subject = "hijacked"
	if err := repo.Update(ctx, &forged); err == nil {
		t.Error("expected an error updating alice's draft as bob")
	}

	reloaded, err := repo.GetByID(ctx, sm.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if reloaded.Subject == "hijacked" {
		t.Error("bob's update must not have applied to alice's draft")
	}
}

// A user's saved messages are personal, so deleting the user (a path this
// codebase doesn't currently exercise anywhere — users are only ever
// soft-disabled — but the FK is there defensively) must not leave orphaned
// drafts/templates behind.
func TestSavedMessageRepoUserDeletionCascades(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	alice := newUser(t, db)
	sm := newSavedMessage(t, db, alice.ID, model.SavedMessageKindDraft, nil)

	if _, err := db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", alice.ID); err != nil {
		t.Fatalf("deleting user: %v", err)
	}

	if _, err := NewSavedMessageRepo(db).GetByID(ctx, sm.ID); err == nil {
		t.Error("expected the draft to be gone after its owner was deleted (ON DELETE CASCADE)")
	}
}

// Unlike the owner, a draft's receiver is incidental — losing the user it
// pointed at should null the reference out, not delete or block deletion of
// the draft itself. Same for a reply-in-progress whose parent message is gone.
func TestSavedMessageRepoReceiverAndParentDeletionSetNull(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewSavedMessageRepo(db)
	alice := newUser(t, db)
	bob := newUser(t, db)
	parent := newMessage(t, db, alice.ID, bob.ID, "orig")

	sm := &model.SavedMessage{
		UserID:     alice.ID,
		Kind:       model.SavedMessageKindDraft,
		ReceiverID: &bob.ID,
		ParentID:   &parent.ID,
		Subject:    "reply in progress",
		Body:       "still writing",
	}
	if err := repo.Create(ctx, sm); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Delete the parent message first: messages.receiver_id has no ON DELETE
	// clause (defaults to RESTRICT), so bob can't be deleted while that
	// message still references him as its receiver.
	if _, err := db.ExecContext(ctx, "DELETE FROM messages WHERE id = $1", parent.ID); err != nil {
		t.Fatalf("deleting parent message: %v", err)
	}
	if _, err := db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", bob.ID); err != nil {
		t.Fatalf("deleting receiver: %v", err)
	}

	got, err := repo.GetByID(ctx, sm.ID)
	if err != nil {
		t.Fatalf("GetByID: %v — the draft itself must survive losing its receiver/parent", err)
	}
	if got.ReceiverID != nil {
		t.Errorf("expected receiver_id nulled out after the receiver was deleted, got %v", got.ReceiverID)
	}
	if got.ParentID != nil {
		t.Errorf("expected parent_id nulled out after the parent message was deleted, got %v", got.ParentID)
	}
}

// The service layer (build() in saved_message.go) refuses to build a template
// with a receiver or parent_id, but that's only a guarantee as good as every
// call site remembering to use it. The CHECK constraint in migration 063
// backstops that invariant at the schema level — this proves it actually
// rejects what the service would never send in the first place.
func TestSavedMessageRepoCheckConstraintRejectsAddressedTemplate(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	alice := newUser(t, db)
	bob := newUser(t, db)

	_, err := db.ExecContext(ctx,
		`INSERT INTO saved_messages (user_id, kind, receiver_id, subject, body) VALUES ($1, 'template', $2, 's', 'b')`,
		alice.ID, bob.ID,
	)
	if err == nil {
		t.Error("expected the CHECK constraint to reject a template with a receiver_id")
	}
}
