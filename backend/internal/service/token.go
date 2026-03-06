package service

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

const tokenBytes = 32 // 32 bytes = 64 hex characters

// GenerateToken creates a cryptographically random 64-character hex token.
func GenerateToken() (string, error) {
	b := make([]byte, tokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return hex.EncodeToString(b), nil
}
