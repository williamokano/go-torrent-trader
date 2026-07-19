package service_test

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// --- mock saved message repo ---

type mockSavedMessageRepo struct {
	mu     sync.Mutex
	items  map[int64]*model.SavedMessage
	nextID int64

	createErr error
	updateErr error
	getErr    error
	deleteErr error
}

func newMockSavedMessageRepo() *mockSavedMessageRepo {
	return &mockSavedMessageRepo{items: map[int64]*model.SavedMessage{}, nextID: 1}
}

func (m *mockSavedMessageRepo) Create(_ context.Context, sm *model.SavedMessage) error {
	if m.createErr != nil {
		return m.createErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	sm.ID = m.nextID
	m.nextID++
	copy := *sm
	m.items[sm.ID] = &copy
	return nil
}

// Update is scoped to sm.UserID, mirroring the real repo's "AND user_id = $N"
// clause (see backend/internal/repository/postgres/saved_message.go) — a
// mismatched owner returns sql.ErrNoRows exactly like an empty RETURNING
// result would, so the service's TOCTOU handling in Update can be exercised
// without a real database.
func (m *mockSavedMessageRepo) Update(_ context.Context, sm *model.SavedMessage) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	existing, ok := m.items[sm.ID]
	if !ok || existing.UserID != sm.UserID {
		return sql.ErrNoRows
	}
	existing.ReceiverID = sm.ReceiverID
	existing.Subject = sm.Subject
	existing.Body = sm.Body
	existing.ParentID = sm.ParentID
	return nil
}

func (m *mockSavedMessageRepo) GetByID(_ context.Context, id int64) (*model.SavedMessage, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	sm, ok := m.items[id]
	if !ok {
		return nil, errors.New("not found")
	}
	copy := *sm
	return &copy, nil
}

func (m *mockSavedMessageRepo) ListByUser(_ context.Context, userID int64, kind model.SavedMessageKind, page, perPage int) ([]model.SavedMessage, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []model.SavedMessage
	for _, sm := range m.items {
		if sm.UserID == userID && sm.Kind == kind {
			result = append(result, *sm)
		}
	}
	total := int64(len(result))
	start := (page - 1) * perPage
	if start >= len(result) {
		return nil, total, nil
	}
	end := start + perPage
	if end > len(result) {
		end = len(result)
	}
	return result[start:end], total, nil
}

// Delete is scoped to userID for the same reason Update is scoped to
// sm.UserID above.
func (m *mockSavedMessageRepo) Delete(_ context.Context, id, userID int64) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	existing, ok := m.items[id]
	if !ok || existing.UserID != userID {
		return sql.ErrNoRows
	}
	delete(m.items, id)
	return nil
}

// --- helpers ---

func setupSavedMessageService() (*service.SavedMessageService, *mockMessageRepo) {
	messages := newMockMessageRepo()
	return service.NewSavedMessageService(
		newMockSavedMessageRepo(),
		newMockUserRepoForMessage(),
		messages,
	), messages
}

// --- drafts ---

func TestSaveDraft_Success(t *testing.T) {
	svc, _ := setupSavedMessageService()
	receiver := int64(2)

	sm, err := svc.Save(context.Background(), 1, model.SavedMessageKindDraft, service.SaveMessageRequest{
		ReceiverID: &receiver,
		Subject:    "Draft subject",
		Body:       "unfinished thought",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sm.Kind != model.SavedMessageKindDraft {
		t.Errorf("kind = %q, want draft", sm.Kind)
	}
	if sm.ReceiverID == nil || *sm.ReceiverID != receiver {
		t.Errorf("receiver_id = %v, want %d", sm.ReceiverID, receiver)
	}
}

func TestSaveDraft_AllowsEmptyBodyOrSubject(t *testing.T) {
	svc, _ := setupSavedMessageService()
	receiver := int64(2)

	// A draft is deliberately incomplete — only the receiver is set here.
	sm, err := svc.Save(context.Background(), 1, model.SavedMessageKindDraft, service.SaveMessageRequest{
		ReceiverID: &receiver,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sm.Subject != "" || sm.Body != "" {
		t.Errorf("expected blank subject/body to be preserved, got subject=%q body=%q", sm.Subject, sm.Body)
	}
}

func TestSaveDraft_RejectsEntirelyEmpty(t *testing.T) {
	svc, _ := setupSavedMessageService()

	_, err := svc.Save(context.Background(), 1, model.SavedMessageKindDraft, service.SaveMessageRequest{})
	if !errors.Is(err, service.ErrInvalidSavedMessage) {
		t.Errorf("expected ErrInvalidSavedMessage for an entirely empty draft, got %v", err)
	}
}

func TestSaveDraft_RejectsUnknownReceiver(t *testing.T) {
	svc, _ := setupSavedMessageService()
	receiver := int64(999)

	_, err := svc.Save(context.Background(), 1, model.SavedMessageKindDraft, service.SaveMessageRequest{
		ReceiverID: &receiver,
		Body:       "hi",
	})
	if !errors.Is(err, service.ErrInvalidSavedMessage) {
		t.Errorf("expected ErrInvalidSavedMessage for an unknown receiver, got %v", err)
	}
}

func TestSaveDraft_WithValidParent(t *testing.T) {
	svc, messages := setupSavedMessageService()
	parent := &model.Message{SenderID: 1, ReceiverID: 2, Subject: "orig", Body: "orig body"}
	if err := messages.Create(context.Background(), parent); err != nil {
		t.Fatalf("seed parent message: %v", err)
	}

	sm, err := svc.Save(context.Background(), 2, model.SavedMessageKindDraft, service.SaveMessageRequest{
		Body:     "replying...",
		ParentID: &parent.ID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sm.ParentID == nil || *sm.ParentID != parent.ID {
		t.Errorf("parent_id = %v, want %d", sm.ParentID, parent.ID)
	}
}

func TestSaveDraft_RejectsParentNotFound(t *testing.T) {
	svc, _ := setupSavedMessageService()
	missing := int64(999)

	_, err := svc.Save(context.Background(), 1, model.SavedMessageKindDraft, service.SaveMessageRequest{
		Body:     "hi",
		ParentID: &missing,
	})
	if !errors.Is(err, service.ErrInvalidSavedMessage) {
		t.Errorf("expected ErrInvalidSavedMessage for a missing parent, got %v", err)
	}
}

func TestSaveDraft_AllowsParentWhenUserIsSender(t *testing.T) {
	svc, messages := setupSavedMessageService()
	parent := &model.Message{SenderID: 1, ReceiverID: 2, Subject: "orig", Body: "orig body"}
	if err := messages.Create(context.Background(), parent); err != nil {
		t.Fatalf("seed parent message: %v", err)
	}

	// The sender of the parent message is a participant in that conversation,
	// so drafting a follow-up (e.g. "meant to add...") must be allowed too.
	_, err := svc.Save(context.Background(), 1, model.SavedMessageKindDraft, service.SaveMessageRequest{
		Body:     "hi",
		ParentID: &parent.ID,
	})
	if err != nil {
		t.Fatalf("sender of the parent message should be allowed to draft a follow-up: %v", err)
	}
}

func TestSaveDraft_RejectsParentWhenUserIsNotParticipant(t *testing.T) {
	svc, messages := setupSavedMessageService()
	parent := &model.Message{SenderID: 1, ReceiverID: 2, Subject: "orig", Body: "orig body"}
	if err := messages.Create(context.Background(), parent); err != nil {
		t.Fatalf("seed parent message: %v", err)
	}

	// mockUserRepoForMessage only seeds users 1 and 2, but the participant
	// check only cares about sender/receiver IDs, so a third user ID (3) is
	// sufficient here without needing to seed another user.
	_, err := svc.Save(context.Background(), 3, model.SavedMessageKindDraft, service.SaveMessageRequest{
		Body:     "hi",
		ParentID: &parent.ID,
	})
	if !errors.Is(err, service.ErrInvalidSavedMessage) {
		t.Errorf("expected ErrInvalidSavedMessage for a non-participant drafting a reply, got %v", err)
	}
}

// --- templates ---

func TestSaveTemplate_Success(t *testing.T) {
	svc, _ := setupSavedMessageService()

	sm, err := svc.Save(context.Background(), 1, model.SavedMessageKindTemplate, service.SaveMessageRequest{
		Subject: "Welcome",
		Body:    "Thanks for joining!",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sm.Kind != model.SavedMessageKindTemplate {
		t.Errorf("kind = %q, want template", sm.Kind)
	}
}

func TestSaveTemplate_RejectsEmptyBody(t *testing.T) {
	svc, _ := setupSavedMessageService()

	_, err := svc.Save(context.Background(), 1, model.SavedMessageKindTemplate, service.SaveMessageRequest{
		Subject: "Welcome",
	})
	if !errors.Is(err, service.ErrInvalidSavedMessage) {
		t.Errorf("expected ErrInvalidSavedMessage for an empty template body, got %v", err)
	}
}

func TestSaveTemplate_RejectsReceiver(t *testing.T) {
	svc, _ := setupSavedMessageService()
	receiver := int64(2)

	_, err := svc.Save(context.Background(), 1, model.SavedMessageKindTemplate, service.SaveMessageRequest{
		ReceiverID: &receiver,
		Body:       "hi",
	})
	if !errors.Is(err, service.ErrInvalidSavedMessage) {
		t.Errorf("expected ErrInvalidSavedMessage for a template with a receiver, got %v", err)
	}
}

func TestSaveTemplate_RejectsParentID(t *testing.T) {
	svc, _ := setupSavedMessageService()
	parent := int64(1)

	_, err := svc.Save(context.Background(), 1, model.SavedMessageKindTemplate, service.SaveMessageRequest{
		Body:     "hi",
		ParentID: &parent,
	})
	if !errors.Is(err, service.ErrInvalidSavedMessage) {
		t.Errorf("expected ErrInvalidSavedMessage for a template with a parent_id, got %v", err)
	}
}

// --- update ---

func TestUpdateDraft_Success(t *testing.T) {
	svc, _ := setupSavedMessageService()
	sm, _ := svc.Save(context.Background(), 1, model.SavedMessageKindDraft, service.SaveMessageRequest{Body: "v1"})

	updated, err := svc.Update(context.Background(), sm.ID, 1, model.SavedMessageKindDraft, service.SaveMessageRequest{Body: "v2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.Body != "v2" {
		t.Errorf("body = %q, want v2", updated.Body)
	}
}

func TestUpdateDraft_RejectsNonOwner(t *testing.T) {
	svc, _ := setupSavedMessageService()
	sm, _ := svc.Save(context.Background(), 1, model.SavedMessageKindDraft, service.SaveMessageRequest{Body: "v1"})

	_, err := svc.Update(context.Background(), sm.ID, 2, model.SavedMessageKindDraft, service.SaveMessageRequest{Body: "v2"})
	if !errors.Is(err, service.ErrSavedMessageNotFound) {
		t.Errorf("expected ErrSavedMessageNotFound for a non-owner update, got %v", err)
	}
}

// Narrow TOCTOU window: getOwnedOfKind's pre-check passes, but the row is
// gone (or its ownership changed) by the time Update's own user_id-scoped
// WHERE clause runs, so the repo returns sql.ErrNoRows. That must surface as
// ErrSavedMessageNotFound (404), not a generic wrapped error (500) — Delete
// already got this right; Update previously did not.
func TestUpdateDraft_MapsRaceWithDeleteToNotFound(t *testing.T) {
	repo := newMockSavedMessageRepo()
	svc := service.NewSavedMessageService(repo, newMockUserRepoForMessage(), newMockMessageRepo())
	sm, err := svc.Save(context.Background(), 1, model.SavedMessageKindDraft, service.SaveMessageRequest{Body: "v1"})
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	repo.updateErr = sql.ErrNoRows

	_, err = svc.Update(context.Background(), sm.ID, 1, model.SavedMessageKindDraft, service.SaveMessageRequest{Body: "v2"})
	if !errors.Is(err, service.ErrSavedMessageNotFound) {
		t.Errorf("expected ErrSavedMessageNotFound when Update races a delete, got %v", err)
	}
}

func TestUpdateDraft_RejectsKindMismatch(t *testing.T) {
	svc, _ := setupSavedMessageService()
	sm, _ := svc.Save(context.Background(), 1, model.SavedMessageKindDraft, service.SaveMessageRequest{Body: "v1"})

	// Trying to update a draft through the templates endpoint must not find it.
	_, err := svc.Update(context.Background(), sm.ID, 1, model.SavedMessageKindTemplate, service.SaveMessageRequest{Body: "v2"})
	if !errors.Is(err, service.ErrSavedMessageNotFound) {
		t.Errorf("expected ErrSavedMessageNotFound for a kind mismatch, got %v", err)
	}
}

// --- get ---

func TestGetSavedMessage_RejectsNonOwner(t *testing.T) {
	svc, _ := setupSavedMessageService()
	sm, _ := svc.Save(context.Background(), 1, model.SavedMessageKindTemplate, service.SaveMessageRequest{Body: "v1"})

	_, err := svc.Get(context.Background(), sm.ID, 2, model.SavedMessageKindTemplate)
	if !errors.Is(err, service.ErrSavedMessageNotFound) {
		t.Errorf("expected ErrSavedMessageNotFound, got %v", err)
	}
}

func TestGetSavedMessage_NotFound(t *testing.T) {
	svc, _ := setupSavedMessageService()

	_, err := svc.Get(context.Background(), 999, 1, model.SavedMessageKindDraft)
	if !errors.Is(err, service.ErrSavedMessageNotFound) {
		t.Errorf("expected ErrSavedMessageNotFound, got %v", err)
	}
}

// --- list ---

func TestListSavedMessages_FiltersByKind(t *testing.T) {
	svc, _ := setupSavedMessageService()
	if _, err := svc.Save(context.Background(), 1, model.SavedMessageKindDraft, service.SaveMessageRequest{Body: "d1"}); err != nil {
		t.Fatalf("save draft: %v", err)
	}
	if _, err := svc.Save(context.Background(), 1, model.SavedMessageKindTemplate, service.SaveMessageRequest{Body: "t1"}); err != nil {
		t.Fatalf("save template: %v", err)
	}

	drafts, total, err := svc.List(context.Background(), 1, model.SavedMessageKindDraft, 1, 25)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 1 || len(drafts) != 1 {
		t.Errorf("drafts = %d (total %d), want 1", len(drafts), total)
	}

	templates, total, err := svc.List(context.Background(), 1, model.SavedMessageKindTemplate, 1, 25)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 1 || len(templates) != 1 {
		t.Errorf("templates = %d (total %d), want 1", len(templates), total)
	}
}

func TestListSavedMessages_DefaultPagination(t *testing.T) {
	svc, _ := setupSavedMessageService()

	items, total, err := svc.List(context.Background(), 1, model.SavedMessageKindDraft, 0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 0 || len(items) != 0 {
		t.Errorf("expected no items, got %d (total %d)", len(items), total)
	}
}

// --- delete ---

func TestDeleteSavedMessage_Success(t *testing.T) {
	svc, _ := setupSavedMessageService()
	sm, _ := svc.Save(context.Background(), 1, model.SavedMessageKindDraft, service.SaveMessageRequest{Body: "v1"})

	if err := svc.Delete(context.Background(), sm.ID, 1, model.SavedMessageKindDraft); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err := svc.Get(context.Background(), sm.ID, 1, model.SavedMessageKindDraft)
	if !errors.Is(err, service.ErrSavedMessageNotFound) {
		t.Errorf("expected the deleted draft to be gone, got %v", err)
	}
}

func TestDeleteSavedMessage_RejectsNonOwner(t *testing.T) {
	svc, _ := setupSavedMessageService()
	sm, _ := svc.Save(context.Background(), 1, model.SavedMessageKindDraft, service.SaveMessageRequest{Body: "v1"})

	err := svc.Delete(context.Background(), sm.ID, 2, model.SavedMessageKindDraft)
	if !errors.Is(err, service.ErrSavedMessageNotFound) {
		t.Errorf("expected ErrSavedMessageNotFound for a non-owner delete, got %v", err)
	}
}

func TestDeleteSavedMessage_SurfacesRepositoryFailure(t *testing.T) {
	repo := newMockSavedMessageRepo()
	svc := service.NewSavedMessageService(repo, newMockUserRepoForMessage(), newMockMessageRepo())

	sm, err := svc.Save(context.Background(), 1, model.SavedMessageKindDraft, service.SaveMessageRequest{Body: "v1"})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	repo.deleteErr = errors.New("boom")

	err = svc.Delete(context.Background(), sm.ID, 1, model.SavedMessageKindDraft)
	if err == nil || errors.Is(err, service.ErrSavedMessageNotFound) {
		t.Errorf("expected the raw repository failure to surface, got %v", err)
	}
}

func TestDeleteSavedMessage_NotFound(t *testing.T) {
	svc, _ := setupSavedMessageService()

	err := svc.Delete(context.Background(), 999, 1, model.SavedMessageKindDraft)
	if !errors.Is(err, service.ErrSavedMessageNotFound) {
		t.Errorf("expected ErrSavedMessageNotFound, got %v", err)
	}
}
