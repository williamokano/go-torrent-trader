package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

var (
	ErrRestrictionNotFound    = errors.New("restriction not found")
	ErrInvalidRestriction     = errors.New("invalid restriction")
	ErrRestrictionAlreadyLifted = errors.New("restriction already lifted")
)

// RestrictionService handles per-user privilege restriction business logic.
type RestrictionService struct {
	restrictions repository.RestrictionRepository
	users        repository.UserRepository
	eventBus     event.Bus
}

// NewRestrictionService creates a new RestrictionService.
func NewRestrictionService(
	restrictions repository.RestrictionRepository,
	users repository.UserRepository,
	bus event.Bus,
) *RestrictionService {
	return &RestrictionService{
		restrictions: restrictions,
		users:        users,
		eventBus:     bus,
	}
}

// ApplyRestriction creates a new restriction and updates the user flag.
func (s *RestrictionService) ApplyRestriction(ctx context.Context, userID int64, restrictionType, reason string, expiresAt *time.Time, issuedByID *int64) (*model.Restriction, error) {
	if reason == "" {
		return nil, fmt.Errorf("%w: reason cannot be empty", ErrInvalidRestriction)
	}

	if !isValidRestrictionType(restrictionType) {
		return nil, fmt.Errorf("%w: invalid restriction type: %s", ErrInvalidRestriction, restrictionType)
	}

	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	restriction := &model.Restriction{
		UserID:          userID,
		RestrictionType: restrictionType,
		Reason:          reason,
		IssuedBy:        issuedByID,
		ExpiresAt:       expiresAt,
	}

	if err := s.restrictions.Create(ctx, restriction); err != nil {
		return nil, fmt.Errorf("create restriction: %w", err)
	}

	// Update the user flag.
	s.setUserFlag(user, restrictionType, false)
	if err := s.users.Update(ctx, user); err != nil {
		return nil, fmt.Errorf("update user flag: %w", err)
	}

	// Publish event.
	actor := s.actorFromUserID(ctx, issuedByID)
	s.eventBus.Publish(ctx, &event.RestrictionAppliedEvent{
		Base:            event.NewBase(event.RestrictionApplied, actor),
		RestrictionID:   restriction.ID,
		UserID:          userID,
		Username:        user.Username,
		RestrictionType: restrictionType,
		Reason:          reason,
	})

	return restriction, nil
}

// LiftRestriction lifts a restriction and restores the user flag if no other
// active restrictions of the same type exist.
func (s *RestrictionService) LiftRestriction(ctx context.Context, restrictionID int64, liftedByID *int64) error {
	restriction, err := s.restrictions.GetByID(ctx, restrictionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrRestrictionNotFound
		}
		return fmt.Errorf("get restriction: %w", err)
	}

	if restriction.LiftedAt != nil {
		return ErrRestrictionAlreadyLifted
	}

	if err := s.restrictions.Lift(ctx, restrictionID, liftedByID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrRestrictionAlreadyLifted
		}
		return fmt.Errorf("lift restriction: %w", err)
	}

	// Check if there are other active restrictions of the same type.
	if err := s.restoreUserFlagIfNone(ctx, restriction.UserID, restriction.RestrictionType); err != nil {
		return err
	}

	user, err := s.users.GetByID(ctx, restriction.UserID)
	if err != nil {
		return fmt.Errorf("get user for event: %w", err)
	}

	actor := s.actorFromUserID(ctx, liftedByID)
	s.eventBus.Publish(ctx, &event.RestrictionLiftedEvent{
		Base:            event.NewBase(event.RestrictionLifted, actor),
		RestrictionID:   restrictionID,
		UserID:          restriction.UserID,
		Username:        user.Username,
		RestrictionType: restriction.RestrictionType,
	})

	return nil
}

// ListByUser returns all restrictions for a user.
func (s *RestrictionService) ListByUser(ctx context.Context, userID int64) ([]model.Restriction, error) {
	return s.restrictions.ListByUser(ctx, userID)
}

// GetActiveRestrictions returns all currently active restrictions.
func (s *RestrictionService) GetActiveRestrictions(ctx context.Context) ([]model.Restriction, error) {
	return s.restrictions.ListActive(ctx)
}

// ResolveExpired lifts expired restrictions and restores user flags.
// Called by the maintenance job.
func (s *RestrictionService) ResolveExpired(ctx context.Context) (int, error) {
	expired, err := s.restrictions.DeleteExpired(ctx)
	if err != nil {
		return 0, err
	}

	// Track which user+type combos need flag checks.
	type userType struct {
		userID          int64
		restrictionType string
	}
	seen := make(map[userType]bool)

	for _, r := range expired {
		key := userType{r.UserID, r.RestrictionType}
		if seen[key] {
			continue
		}
		seen[key] = true

		if err := s.restoreUserFlagIfNone(ctx, r.UserID, r.RestrictionType); err != nil {
			slog.Error("failed to restore user flag after expired restriction",
				"user_id", r.UserID,
				"restriction_type", r.RestrictionType,
				"error", err,
			)
		}
	}

	return len(expired), nil
}

// restoreUserFlagIfNone checks if there are remaining active restrictions of
// the given type for the user. If not, sets the flag back to true.
func (s *RestrictionService) restoreUserFlagIfNone(ctx context.Context, userID int64, restrictionType string) error {
	active, err := s.restrictions.ListByUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("list user restrictions: %w", err)
	}

	hasActive := false
	for _, r := range active {
		if r.RestrictionType == restrictionType && r.LiftedAt == nil {
			hasActive = true
			break
		}
	}

	if !hasActive {
		user, err := s.users.GetByID(ctx, userID)
		if err != nil {
			return fmt.Errorf("get user: %w", err)
		}
		s.setUserFlag(user, restrictionType, true)
		if err := s.users.Update(ctx, user); err != nil {
			return fmt.Errorf("update user flag: %w", err)
		}
	}

	return nil
}

func (s *RestrictionService) setUserFlag(user *model.User, restrictionType string, value bool) {
	switch restrictionType {
	case model.RestrictionTypeDownload:
		user.CanDownload = value
	case model.RestrictionTypeUpload:
		user.CanUpload = value
	case model.RestrictionTypeChat:
		user.CanChat = value
	}
}

func (s *RestrictionService) actorFromUserID(ctx context.Context, userID *int64) event.Actor {
	if userID == nil {
		return event.Actor{ID: 0, Username: "System"}
	}
	actor := event.Actor{ID: *userID}
	if u, err := s.users.GetByID(ctx, *userID); err == nil {
		actor.Username = u.Username
	}
	return actor
}

func isValidRestrictionType(t string) bool {
	switch t {
	case model.RestrictionTypeDownload, model.RestrictionTypeUpload, model.RestrictionTypeChat:
		return true
	}
	return false
}
