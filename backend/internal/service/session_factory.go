package service

import "fmt"

// NewSessionStore creates a SessionStore based on the provided configuration.
// Returns a RedisSessionStore for type "redis".
func NewSessionStore(cfg SessionStoreConfig) (SessionStore, error) {
	switch cfg.Type {
	case "redis":
		return NewRedisSessionStore(cfg.RedisURL, cfg.AccessTokenTTL, cfg.RefreshTokenTTL)
	default:
		return nil, fmt.Errorf("unknown session store type %q: must be \"redis\"", cfg.Type)
	}
}
