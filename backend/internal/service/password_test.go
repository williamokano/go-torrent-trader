package service

import (
	"strings"
	"testing"
)

func TestHashPassword_ReturnsArgon2idFormat(t *testing.T) {
	hash, err := HashPassword("testpassword123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.HasPrefix(hash, "$argon2id$") {
		t.Errorf("hash should start with $argon2id$, got: %s", hash)
	}

	parts := strings.Split(hash, "$")
	if len(parts) != 6 {
		t.Errorf("expected 6 parts in hash, got %d", len(parts))
	}
}

func TestHashPassword_UniqueSalts(t *testing.T) {
	h1, err := HashPassword("same-password")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	h2, err := HashPassword("same-password")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if h1 == h2 {
		t.Error("two hashes of the same password should differ (different salts)")
	}
}

func TestVerifyPassword_CorrectPassword(t *testing.T) {
	hash, err := HashPassword("mypassword")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ok, err := VerifyPassword("mypassword", hash)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("expected password to verify correctly")
	}
}

func TestVerifyPassword_WrongPassword(t *testing.T) {
	hash, err := HashPassword("correctpassword")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ok, err := VerifyPassword("wrongpassword", hash)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected wrong password to fail verification")
	}
}

func TestVerifyPassword_InvalidHash(t *testing.T) {
	_, err := VerifyPassword("anything", "not-a-valid-hash")
	if err == nil {
		t.Error("expected error for invalid hash format")
	}
}
