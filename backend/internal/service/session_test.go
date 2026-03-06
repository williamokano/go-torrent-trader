package service

import (
	"testing"
	"time"
)

func newTestSession(accessToken, refreshToken string) *Session {
	now := time.Now()
	return &Session{
		UserID:           1,
		GroupID:          5,
		AccessToken:      accessToken,
		RefreshToken:     refreshToken,
		IP:               "127.0.0.1",
		CreatedAt:        now,
		LastActive:       now,
		ExpiresAt:        now.Add(DefaultAccessTokenTTL),
		RefreshExpiresAt: now.Add(DefaultRefreshTokenTTL),
	}
}

func TestMemorySessionStore_CreateAndGet(t *testing.T) {
	store := NewMemorySessionStore()
	sess := newTestSession("access-1", "refresh-1")
	if err := store.Create(sess); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := store.GetByAccessToken("access-1")
	if got == nil {
		t.Fatal("expected session, got nil")
	}
	if got.UserID != 1 {
		t.Errorf("expected UserID=1, got %d", got.UserID)
	}
}

func TestMemorySessionStore_GetByRefreshToken(t *testing.T) {
	store := NewMemorySessionStore()
	sess := newTestSession("access-2", "refresh-2")
	if err := store.Create(sess); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := store.GetByRefreshToken("refresh-2")
	if got == nil {
		t.Fatal("expected session, got nil")
	}
}

func TestMemorySessionStore_GetExpired(t *testing.T) {
	store := NewMemorySessionStore()
	now := time.Now()
	sess := &Session{
		UserID:           1,
		AccessToken:      "expired-access",
		RefreshToken:     "expired-refresh",
		ExpiresAt:        now.Add(-1 * time.Hour),
		RefreshExpiresAt: now.Add(-1 * time.Hour),
	}
	if err := store.Create(sess); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if store.GetByAccessToken("expired-access") != nil {
		t.Error("expected nil for expired access token")
	}
	if store.GetByRefreshToken("expired-refresh") != nil {
		t.Error("expected nil for expired refresh token")
	}
}

func TestMemorySessionStore_Delete(t *testing.T) {
	store := NewMemorySessionStore()
	sess := newTestSession("del-access", "del-refresh")
	if err := store.Create(sess); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	store.Delete("del-access")

	if store.GetByAccessToken("del-access") != nil {
		t.Error("expected nil after delete")
	}
	if store.GetByRefreshToken("del-refresh") != nil {
		t.Error("expected nil for refresh token after delete")
	}
}

func TestMemorySessionStore_Rotate(t *testing.T) {
	store := NewMemorySessionStore()
	old := newTestSession("old-access", "old-refresh")
	if err := store.Create(old); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	newSess := newTestSession("new-access", "new-refresh")
	if err := store.Rotate("old-refresh", newSess); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if store.GetByAccessToken("old-access") != nil {
		t.Error("old access token should be invalidated")
	}
	if store.GetByRefreshToken("old-refresh") != nil {
		t.Error("old refresh token should be invalidated")
	}
	if store.GetByAccessToken("new-access") == nil {
		t.Error("new access token should be valid")
	}
	if store.GetByRefreshToken("new-refresh") == nil {
		t.Error("new refresh token should be valid")
	}
}

func TestMemorySessionStore_GetNotFound(t *testing.T) {
	store := NewMemorySessionStore()
	if store.GetByAccessToken("nonexistent") != nil {
		t.Error("expected nil for nonexistent token")
	}
}

func TestMemorySessionStore_DeleteByUserID(t *testing.T) {
	store := NewMemorySessionStore()
	sess1 := newTestSession("a1", "r1")
	sess2 := newTestSession("a2", "r2")
	sess2.AccessToken = "a2"
	sess2.RefreshToken = "r2"
	_ = store.Create(sess1)
	_ = store.Create(sess2)

	store.DeleteByUserID(1)

	if store.GetByAccessToken("a1") != nil {
		t.Error("expected nil after DeleteByUserID")
	}
	if store.GetByAccessToken("a2") != nil {
		t.Error("expected nil after DeleteByUserID")
	}
}

func TestMemorySessionStore_DeleteByUserIDExcept(t *testing.T) {
	store := NewMemorySessionStore()
	sess1 := newTestSession("keep", "r-keep")
	sess2 := &Session{
		UserID:           1,
		GroupID:          5,
		AccessToken:      "remove",
		RefreshToken:     "r-remove",
		ExpiresAt:        time.Now().Add(DefaultAccessTokenTTL),
		RefreshExpiresAt: time.Now().Add(DefaultRefreshTokenTTL),
	}
	_ = store.Create(sess1)
	_ = store.Create(sess2)

	store.DeleteByUserIDExcept(1, "keep")

	if store.GetByAccessToken("keep") == nil {
		t.Error("expected kept session to remain")
	}
	if store.GetByAccessToken("remove") != nil {
		t.Error("expected removed session to be gone")
	}
}

func TestMemorySessionStore_TouchLastActive(t *testing.T) {
	store := NewMemorySessionStore()
	sess := newTestSession("touch-access", "touch-refresh")
	originalTime := sess.LastActive
	_ = store.Create(sess)

	// Small delay to ensure time difference
	store.TouchLastActive("touch-access")

	got := store.GetByAccessToken("touch-access")
	if got == nil {
		t.Fatal("expected session, got nil")
	}
	if got.LastActive.Before(originalTime) {
		t.Error("expected LastActive to be updated")
	}
}
