package service

import "fmt"

// NewSessionStore creates a SessionStore based on the provided configuration.
// Returns a MemorySessionStore for type "memory" and a RedisSessionStore for type "redis".
func NewSessionStore(cfg SessionStoreConfig) (SessionStore, error) {
	switch cfg.Type {
	case "memory":
		return NewMemorySessionStore(), nil
	case "redis":
		return NewRedisSessionStore(cfg.RedisURL, cfg.AccessTokenTTL, cfg.RefreshTokenTTL)
	default:
		return nil, fmt.Errorf("unknown session store type %q: must be \"memory\" or \"redis\"", cfg.Type)
	}
}
