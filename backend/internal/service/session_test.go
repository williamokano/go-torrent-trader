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
		ExpiresAt:        now.Add(AccessTokenTTL),
		RefreshExpiresAt: now.Add(RefreshTokenTTL),
	}
}

func TestSessionStore_CreateAndGet(t *testing.T) {
	store := NewSessionStore()
	sess := newTestSession("access-1", "refresh-1")
	store.Create(sess)

	got := store.GetByAccessToken("access-1")
	if got == nil {
		t.Fatal("expected session, got nil")
	}
	if got.UserID != 1 {
		t.Errorf("expected UserID=1, got %d", got.UserID)
	}
}

func TestSessionStore_GetByRefreshToken(t *testing.T) {
	store := NewSessionStore()
	sess := newTestSession("access-2", "refresh-2")
	store.Create(sess)

	got := store.GetByRefreshToken("refresh-2")
	if got == nil {
		t.Fatal("expected session, got nil")
	}
}

func TestSessionStore_GetExpired(t *testing.T) {
	store := NewSessionStore()
	now := time.Now()
	sess := &Session{
		UserID:           1,
		AccessToken:      "expired-access",
		RefreshToken:     "expired-refresh",
		ExpiresAt:        now.Add(-1 * time.Hour),
		RefreshExpiresAt: now.Add(-1 * time.Hour),
	}
	store.Create(sess)

	if store.GetByAccessToken("expired-access") != nil {
		t.Error("expected nil for expired access token")
	}
	if store.GetByRefreshToken("expired-refresh") != nil {
		t.Error("expected nil for expired refresh token")
	}
}

func TestSessionStore_Delete(t *testing.T) {
	store := NewSessionStore()
	sess := newTestSession("del-access", "del-refresh")
	store.Create(sess)

	store.Delete("del-access")

	if store.GetByAccessToken("del-access") != nil {
		t.Error("expected nil after delete")
	}
	if store.GetByRefreshToken("del-refresh") != nil {
		t.Error("expected nil for refresh token after delete")
	}
}

func TestSessionStore_Rotate(t *testing.T) {
	store := NewSessionStore()
	old := newTestSession("old-access", "old-refresh")
	store.Create(old)

	newSess := newTestSession("new-access", "new-refresh")
	store.Rotate("old-refresh", newSess)

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

func TestSessionStore_GetNotFound(t *testing.T) {
	store := NewSessionStore()
	if store.GetByAccessToken("nonexistent") != nil {
		t.Error("expected nil for nonexistent token")
	}
}
