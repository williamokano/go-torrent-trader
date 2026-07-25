package postgres

import (
	"context"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

// A system message has no author row: user_id goes in as NULL and reads back as
// 0, with the username synthesised by the query rather than resolved by a JOIN.
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
	if got.Username != model.SystemChatUsername {
		t.Fatalf("username = %q, want %q", got.Username, model.SystemChatUsername)
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
}
