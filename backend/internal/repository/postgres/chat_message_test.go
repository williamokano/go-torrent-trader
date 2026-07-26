package postgres

import (
	"context"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

// A system message has no author row: user_id goes in as NULL and reads back as
// 0, and the username reads back empty — the JOIN has nothing to resolve and the
// label is ChatService's job, not the query's.
func TestChatMessageRepoSystemMessageRoundTrip(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()
	repo := NewChatMessageRepo(db)

	author := newUser(t, db)
	if err := repo.Create(ctx, &model.ChatMessage{UserID: author.ID, Message: "hello"}); err != nil {
		t.Fatalf("Create(user message): %v", err)
	}

	system := &model.ChatMessage{Message: "New torrent: Thing", System: true}
	if err := repo.Create(ctx, system); err != nil {
		t.Fatalf("Create(system message): %v", err)
	}
	if system.ID == 0 || system.CreatedAt.IsZero() {
		t.Fatalf("Create did not populate the generated columns: %+v", system)
	}

	var storedUserID *int64
	if err := db.QueryRowContext(ctx, `SELECT user_id FROM chat_messages WHERE id = $1`, system.ID).
		Scan(&storedUserID); err != nil {
		t.Fatalf("reading stored user_id: %v", err)
	}
	if storedUserID != nil {
		t.Fatalf("stored user_id = %d, want SQL NULL", *storedUserID)
	}

	msgs, err := repo.ListRecent(ctx, 10)
	if err != nil {
		t.Fatalf("ListRecent: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("got %d messages, want 2", len(msgs))
	}

	var got *model.ChatMessage
	for i := range msgs {
		if msgs[i].System {
			got = &msgs[i]
		}
	}
	if got == nil {
		t.Fatal("system message missing from ListRecent")
	}
	if got.UserID != 0 {
		t.Fatalf("user_id = %d, want 0 for a NULL author", got.UserID)
	}
	// The label is an operator setting now, so the repository must hand the
	// system row back unlabelled and let ChatService name it. A username
	// synthesised in SQL would be a second source of truth that no setting
	// change could reach.
	if got.Username != "" {
		t.Fatalf("username = %q, want empty (ChatService supplies the label)", got.Username)
	}

	// The regular message must still resolve its author through the JOIN.
	for i := range msgs {
		if !msgs[i].System && msgs[i].Username != author.Username {
			t.Fatalf("user message username = %q, want %q", msgs[i].Username, author.Username)
		}
	}

	before, err := repo.ListBefore(ctx, system.ID, 10)
	if err != nil {
		t.Fatalf("ListBefore: %v", err)
	}
	if len(before) != 1 || before[0].System {
		t.Fatalf("ListBefore = %+v, want only the earlier user message", before)
	}

	// Staff can delete an announcement (BE-10.7). DeleteByUserID can never
	// reach one — a system row's user_id is NULL — so this is the only path,
	// and it must not be filtered out by the `system` flag.
	if err := repo.Delete(ctx, system.ID); err != nil {
		t.Fatalf("Delete(system message): %v", err)
	}
	remaining, err := repo.ListRecent(ctx, 10)
	if err != nil {
		t.Fatalf("ListRecent after delete: %v", err)
	}
	for _, m := range remaining {
		if m.System {
			t.Fatal("system message survived Delete")
		}
	}
}
