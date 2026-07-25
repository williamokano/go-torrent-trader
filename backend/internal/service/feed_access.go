package service

import (
	"context"
	"fmt"
)

// FeedAccessService answers one question: may this member watch the live feeds?
//
// Access is all-or-nothing across every feed. A feed's filters decide what it
// carries, not who may read it, so there is deliberately no per-feed dimension —
// an operator grants the feeds to a class and can take them away from one member
// without moving them out of it.
type FeedAccessService struct {
	users  UserReader
	groups GroupReader
}

// UserReader and GroupReader are the slices of the repositories this needs.
// Narrowed so the check can be unit-tested without a database.
type UserReader interface {
	CanFeed(ctx context.Context, userID int64) (bool, int64, error)
}

// GroupReader reports whether a class has the feeds.
type GroupReader interface {
	CanFeed(ctx context.Context, groupID int64) (bool, error)
}

// NewFeedAccessService creates the check.
func NewFeedAccessService(users UserReader, groups GroupReader) *FeedAccessService {
	return &FeedAccessService{users: users, groups: groups}
}

// Allowed reports whether the user may watch live feeds.
//
// Both halves must agree: the class grants the privilege and the member must not
// have had it taken away. It reads live rows rather than the session's cached
// permissions, because a revoked privilege has to bite on the next connect
// rather than whenever the member next logs in — the same reason chat checks the
// user row on every message.
func (s *FeedAccessService) Allowed(ctx context.Context, userID int64) (bool, error) {
	if s == nil || s.users == nil || s.groups == nil {
		// Nothing wired to say otherwise. Failing open here would be a silent
		// authorization bypass, so an unconfigured check refuses.
		return false, nil
	}

	userAllows, groupID, err := s.users.CanFeed(ctx, userID)
	if err != nil {
		return false, fmt.Errorf("read user feed access: %w", err)
	}
	if !userAllows {
		return false, nil
	}

	groupAllows, err := s.groups.CanFeed(ctx, groupID)
	if err != nil {
		return false, fmt.Errorf("read group feed access: %w", err)
	}
	return groupAllows, nil
}
