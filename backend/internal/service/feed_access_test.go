package service

import (
	"context"
	"errors"
	"testing"
)

type stubFeedUsers struct {
	canFeed bool
	groupID int64
	err     error
	calls   int
}

func (s *stubFeedUsers) CanFeed(context.Context, int64) (bool, int64, error) {
	s.calls++
	return s.canFeed, s.groupID, s.err
}

type stubFeedGroups struct {
	canFeed bool
	err     error
	calls   int
}

func (s *stubFeedGroups) CanFeed(context.Context, int64) (bool, error) {
	s.calls++
	return s.canFeed, s.err
}

func TestFeedAccessNeedsBothHalves(t *testing.T) {
	cases := []struct {
		name       string
		user       bool
		group      bool
		wantAccess bool
	}{
		{"class grants and member keeps it", true, true, true},
		{"member had it revoked", false, true, false},
		{"class does not have the feeds", true, false, false},
		{"neither", false, false, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewFeedAccessService(
				&stubFeedUsers{canFeed: tc.user, groupID: 2},
				&stubFeedGroups{canFeed: tc.group},
			)
			allowed, err := svc.Allowed(context.Background(), 1)
			if err != nil {
				t.Fatalf("Allowed: %v", err)
			}
			if allowed != tc.wantAccess {
				t.Fatalf("allowed = %v, want %v", allowed, tc.wantAccess)
			}
		})
	}
}

// A revoked member is refused without the class being consulted: the answer
// cannot change, and this runs on every connect.
func TestFeedAccessStopsAtTheRevokedMember(t *testing.T) {
	groups := &stubFeedGroups{canFeed: true}
	svc := NewFeedAccessService(&stubFeedUsers{canFeed: false, groupID: 2}, groups)

	if allowed, _ := svc.Allowed(context.Background(), 1); allowed {
		t.Fatal("a revoked member must be refused")
	}
	if groups.calls != 0 {
		t.Fatalf("read the group %d times, want 0", groups.calls)
	}
}

// A check that cannot answer must not grant. The alternative is admitting
// someone whose access could not be confirmed.
func TestFeedAccessFailsClosedOnAReadError(t *testing.T) {
	svc := NewFeedAccessService(
		&stubFeedUsers{canFeed: true, groupID: 2, err: errors.New("database is down")},
		&stubFeedGroups{canFeed: true},
	)

	allowed, err := svc.Allowed(context.Background(), 1)
	if err == nil {
		t.Fatal("a failed lookup must be reported")
	}
	if allowed {
		t.Fatal("a failed lookup must not grant access")
	}
}

func TestFeedAccessFailsClosedOnAGroupReadError(t *testing.T) {
	svc := NewFeedAccessService(
		&stubFeedUsers{canFeed: true, groupID: 2},
		&stubFeedGroups{canFeed: true, err: errors.New("database is down")},
	)

	allowed, err := svc.Allowed(context.Background(), 1)
	if err == nil || allowed {
		t.Fatalf("allowed = %v, err = %v; want refused and reported", allowed, err)
	}
}

// An unwired check refuses rather than waving everyone through, so a missing
// dependency is a locked door instead of a silent authorization bypass.
func TestFeedAccessWithNothingWiredRefuses(t *testing.T) {
	for name, svc := range map[string]*FeedAccessService{
		"nil service": nil,
		"no repos":    NewFeedAccessService(nil, nil),
	} {
		allowed, err := svc.Allowed(context.Background(), 1)
		if err != nil || allowed {
			t.Fatalf("%s: allowed = %v, err = %v; want refused", name, allowed, err)
		}
	}
}
