package service

import "testing"

func TestGenerateToken_Length(t *testing.T) {
	token, err := GenerateToken()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(token) != 64 {
		t.Errorf("expected 64-char hex token, got %d chars", len(token))
	}
}

func TestGenerateToken_Unique(t *testing.T) {
	t1, err := GenerateToken()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	t2, err := GenerateToken()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if t1 == t2 {
		t.Error("two generated tokens should be different")
	}
}

func TestGenerateToken_ValidHex(t *testing.T) {
	token, err := GenerateToken()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, c := range token {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("token contains non-hex character: %c", c)
		}
	}
}
