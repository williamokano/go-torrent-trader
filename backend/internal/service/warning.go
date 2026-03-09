package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

var (
	ErrWarningNotFound = errors.New("warning not found")
	ErrInvalidWarning  = errors.New("invalid warning")
)

// WarningService handles user warning business logic.
type WarningService struct {
	warnings repository.WarningRepository
	users    repository.UserRepository
	messages repository.MessageRepository
	eventBus event.Bus
}

// NewWarningService creates a new WarningService.
func NewWarningService(
	warnings repository.WarningRepository,
	users repository.UserRepository,
	messages repository.MessageRepository,
	bus event.Bus,
) *WarningService {
	return &WarningService{
		warnings: warnings,
		users:    users,
		messages: messages,
		eventBus: bus,
	}
}

// IssueManualWarning creates a manual warning and sends a PM to the user.
func (s *WarningService) IssueManualWarning(ctx context.Context, userID int64, reason string, expiresAt *time.Time, issuedByID int64) (*model.Warning, error) {
	if reason == "" {
		return nil, fmt.Errorf("%w: reason cannot be empty", ErrInvalidWarning)
	}

	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("%w: user not found", ErrInvalidWarning)
	}

	w := &model.Warning{
		UserID:    userID,
		Type:      model.WarningTypeManual,
		Reason:    reason,
		IssuedBy:  &issuedByID,
		Status:    model.WarningStatusActive,
		ExpiresAt: expiresAt,
	}

	if err := s.warnings.Create(ctx, w); err != nil {
		return nil, fmt.Errorf("create warning: %w", err)
	}

	// Set warned flag on user
	user.Warned = true
	if expiresAt != nil {
		user.WarnUntil = expiresAt
	}
	if err := s.users.Update(ctx, user); err != nil {
		return nil, fmt.Errorf("update user warned flag: %w", err)
	}

	// Send PM to the warned user from the issuer
	s.sendPM(ctx, issuedByID, userID, "Warning Issued", reason)

	// Publish event
	actor := s.actorFromUserID(ctx, issuedByID)
	s.eventBus.Publish(ctx, &event.WarningIssuedEvent{
		Base:        event.NewBase(event.WarningIssued, actor),
		WarningID:   w.ID,
		UserID:      userID,
		Username:    user.Username,
		WarningType: model.WarningTypeManual,
	})

	// Re-fetch to get joined data
	return s.warnings.GetByID(ctx, w.ID)
}

// IssueRatioWarning creates a ratio warning (called by the ratio check job).
// The reason field stores the full warning message for user display.
func (s *WarningService) IssueRatioWarning(ctx context.Context, userID int64, message string) (*model.Warning, error) {
	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}

	w := &model.Warning{
		UserID: userID,
		Type:   model.WarningTypeRatioSoft,
		Reason: message,
		Status: model.WarningStatusActive,
	}

	if err := s.warnings.Create(ctx, w); err != nil {
		return nil, fmt.Errorf("create ratio warning: %w", err)
	}

	// Set warned flag on user
	user.Warned = true
	if err := s.users.Update(ctx, user); err != nil {
		return nil, fmt.Errorf("update user warned flag: %w", err)
	}

	// Publish event (system actor)
	actor := event.Actor{ID: 0, Username: "System"}
	s.eventBus.Publish(ctx, &event.WarningIssuedEvent{
		Base:        event.NewBase(event.WarningIssued, actor),
		WarningID:   w.ID,
		UserID:      userID,
		Username:    user.Username,
		WarningType: model.WarningTypeRatioSoft,
	})

	return w, nil
}

// EscalateRatioWarning escalates a ratio_soft warning to a ban.
func (s *WarningService) EscalateRatioWarning(ctx context.Context, warningID int64, banMessage string) error {
	w, err := s.warnings.GetByID(ctx, warningID)
	if err != nil {
		return fmt.Errorf("get warning: %w", err)
	}

	// Mark old warning as escalated
	w.Status = model.WarningStatusEscalated
	now := time.Now()
	w.LiftedAt = &now
	if err := s.warnings.Update(ctx, w); err != nil {
		return fmt.Errorf("escalate warning: %w", err)
	}

	// Disable the user
	user, err := s.users.GetByID(ctx, w.UserID)
	if err != nil {
		return fmt.Errorf("get user for ban: %w", err)
	}
	user.Enabled = false
	if err := s.users.Update(ctx, user); err != nil {
		return fmt.Errorf("disable user: %w", err)
	}

	// Create a ratio_ban warning record for audit trail
	banWarning := &model.Warning{
		UserID: w.UserID,
		Type:   model.WarningTypeRatioBan,
		Reason: banMessage,
		Status: model.WarningStatusActive,
	}
	if err := s.warnings.Create(ctx, banWarning); err != nil {
		return fmt.Errorf("create ban warning: %w", err)
	}

	// Publish event
	actor := event.Actor{ID: 0, Username: "System"}
	s.eventBus.Publish(ctx, &event.WarningIssuedEvent{
		Base:        event.NewBase(event.WarningIssued, actor),
		WarningID:   banWarning.ID,
		UserID:      w.UserID,
		Username:    user.Username,
		WarningType: model.WarningTypeRatioBan,
	})

	return nil
}

// ResolveWarning marks a warning as resolved (e.g., ratio improved).
func (s *WarningService) ResolveWarning(ctx context.Context, warningID int64) error {
	w, err := s.warnings.GetByID(ctx, warningID)
	if err != nil {
		return fmt.Errorf("get warning: %w", err)
	}

	w.Status = model.WarningStatusResolved
	now := time.Now()
	w.LiftedAt = &now
	if err := s.warnings.Update(ctx, w); err != nil {
		return fmt.Errorf("resolve warning: %w", err)
	}

	// Clear warned flag if no more active warnings
	if err := s.clearWarnedIfNone(ctx, w.UserID); err != nil {
		return err
	}

	return nil
}

// LiftWarning manually lifts an active warning.
func (s *WarningService) LiftWarning(ctx context.Context, warningID int64, liftedByID int64, reason string) error {
	w, err := s.warnings.GetByID(ctx, warningID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrWarningNotFound
		}
		return fmt.Errorf("get warning: %w", err)
	}

	if w.Status != model.WarningStatusActive {
		return fmt.Errorf("%w: warning is not active", ErrInvalidWarning)
	}

	w.Status = model.WarningStatusLifted
	now := time.Now()
	w.LiftedAt = &now
	w.LiftedBy = &liftedByID
	if reason != "" {
		w.LiftedReason = &reason
	}

	if err := s.warnings.Update(ctx, w); err != nil {
		return fmt.Errorf("lift warning: %w", err)
	}

	// Clear warned flag if no more active warnings
	if err := s.clearWarnedIfNone(ctx, w.UserID); err != nil {
		return err
	}

	// Publish event
	actor := s.actorFromUserID(ctx, liftedByID)
	username := w.Username
	if username == "" {
		if user, uErr := s.users.GetByID(ctx, w.UserID); uErr == nil {
			username = user.Username
		}
	}
	s.eventBus.Publish(ctx, &event.WarningLiftedEvent{
		Base:      event.NewBase(event.WarningLifted, actor),
		WarningID: warningID,
		UserID:    w.UserID,
		Username:  username,
	})

	return nil
}

// GetActiveWarnings returns active warnings for a user.
func (s *WarningService) GetActiveWarnings(ctx context.Context, userID int64) ([]model.Warning, error) {
	return s.warnings.ListByUser(ctx, userID, false)
}

// GetAllWarnings returns all warnings for a user (staff view).
func (s *WarningService) GetAllWarnings(ctx context.Context, userID int64) ([]model.Warning, error) {
	return s.warnings.ListByUser(ctx, userID, true)
}

// ListWarnings returns a paginated, filterable list of all warnings (admin view).
func (s *WarningService) ListWarnings(ctx context.Context, opts repository.ListWarningsOptions) ([]model.Warning, int64, error) {
	return s.warnings.ListAll(ctx, opts)
}

// GetActiveRatioWarning returns the active ratio_soft warning for a user, if any.
func (s *WarningService) GetActiveRatioWarning(ctx context.Context, userID int64) (*model.Warning, error) {
	w, err := s.warnings.GetActiveRatioWarning(ctx, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return w, nil
}

// GetUsersWithLowRatio returns users whose ratio is below threshold with enough downloads.
func (s *WarningService) GetUsersWithLowRatio(ctx context.Context, threshold float64, minDownloaded int64) ([]model.User, error) {
	return s.warnings.GetUsersWithLowRatio(ctx, threshold, minDownloaded)
}

// ReplaceTemplateVars replaces template variables in a message string.
func ReplaceTemplateVars(msg string, vars map[string]string) string {
	for k, v := range vars {
		msg = strings.ReplaceAll(msg, "{{"+k+"}}", v)
	}
	return msg
}

func (s *WarningService) clearWarnedIfNone(ctx context.Context, userID int64) error {
	count, err := s.warnings.CountActiveByUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("count active warnings: %w", err)
	}
	if count == 0 {
		user, err := s.users.GetByID(ctx, userID)
		if err != nil {
			return fmt.Errorf("get user: %w", err)
		}
		user.Warned = false
		user.WarnUntil = nil
		if err := s.users.Update(ctx, user); err != nil {
			return fmt.Errorf("clear user warned flag: %w", err)
		}
	}
	return nil
}

func (s *WarningService) sendPM(ctx context.Context, senderID, receiverID int64, subject, body string) {
	if senderID == receiverID {
		return
	}
	msg := &model.Message{
		SenderID:   senderID,
		ReceiverID: receiverID,
		Subject:    subject,
		Body:       body,
	}
	_ = s.messages.Create(ctx, msg)
}

func (s *WarningService) actorFromUserID(ctx context.Context, userID int64) event.Actor {
	actor := event.Actor{ID: userID}
	if u, err := s.users.GetByID(ctx, userID); err == nil {
		actor.Username = u.Username
	}
	return actor
}
