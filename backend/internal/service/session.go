package service

import (
	"time"
)

// SessionStore defines the interface for session persistence.
type SessionStore interface {
	Create(session *Session) error
	GetByAccessToken(token string) *Session
	GetByRefreshToken(token string) *Session
	Delete(accessToken string)
	DeleteByUserID(userID int64)
	DeleteByUserIDExcept(userID int64, keepAccessToken string)
	Rotate(oldRefreshToken string, newSession *Session) error
	TouchLastActive(accessToken string)
}

// Session represents an authenticated user session.
type Session struct {
	UserID           int64     `json:"user_id"`
	GroupID          int64     `json:"group_id"`
	AccessToken      string    `json:"access_token"`
	RefreshToken     string    `json:"refresh_token"`
	DeviceName       string    `json:"device_name"`
	IP               string    `json:"ip"`
	CreatedAt        time.Time `json:"created_at"`
	LastActive       time.Time `json:"last_active"`
	ExpiresAt        time.Time `json:"expires_at"`
	RefreshExpiresAt time.Time `json:"refresh_expires_at"`
}

// SessionStoreConfig holds configuration for the session store factory.
type SessionStoreConfig struct {
	Type            string        // "redis"
	RedisURL        string        // Redis connection URL
	AccessTokenTTL  time.Duration // TTL for access token keys
	RefreshTokenTTL time.Duration // TTL for refresh token keys
}
