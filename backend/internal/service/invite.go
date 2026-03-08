package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

var (
	ErrNoInvitesRemaining = errors.New("no invites remaining")
	ErrInviteNotFound     = errors.New("invite not found")
	ErrInviteExpired      = errors.New("invite has expired")
	ErrInviteRedeemed     = errors.New("invite has already been redeemed")
)

const inviteExpiryDuration = 7 * 24 * time.Hour

// InviteService handles invitation business logic.
type InviteService struct {
	invites  repository.InviteRepository
	users    repository.UserRepository
	eventBus event.Bus
}

// NewInviteService creates a new InviteService.
func NewInviteService(invites repository.InviteRepository, users repository.UserRepository, bus event.Bus) *InviteService {
	return &InviteService{
		invites:  invites,
		users:    users,
		eventBus: bus,
	}
}

// CreateInvite generates a new invite token. The user shares the token/link themselves.
func (s *InviteService) CreateInvite(ctx context.Context, inviterID int64) (*model.Invite, error) {
	// Check if user has invites remaining
	inviter, err := s.users.GetByID(ctx, inviterID)
	if err != nil {
		return nil, fmt.Errorf("get inviter: %w", err)
	}
	if inviter.Invites <= 0 {
		return nil, ErrNoInvitesRemaining
	}

	// Generate invite token
	token, err := GenerateToken()
	if err != nil {
		return nil, fmt.Errorf("generate invite token: %w", err)
	}

	invite := &model.Invite{
		InviterID: inviterID,
		Token:     token,
		ExpiresAt: time.Now().Add(inviteExpiryDuration),
	}

	if err := s.invites.Create(ctx, invite); err != nil {
		return nil, fmt.Errorf("create invite: %w", err)
	}

	// Decrement user's invite count
	inviter.Invites--
	if err := s.users.Update(ctx, inviter); err != nil {
		return nil, fmt.Errorf("decrement invites: %w", err)
	}

	s.eventBus.Publish(ctx, &event.InviteCreatedEvent{
		Base:     event.NewBase(event.InviteCreated, event.Actor{ID: inviterID, Username: inviter.Username}),
		InviteID: invite.ID,
	})

	return invite, nil
}

// ValidateInvite checks if an invite token is valid without redeeming it.
func (s *InviteService) ValidateInvite(ctx context.Context, token string) (*model.Invite, error) {
	invite, err := s.invites.GetByToken(ctx, token)
	if err != nil {
		return nil, ErrInviteNotFound
	}

	if invite.Redeemed {
		return nil, ErrInviteRedeemed
	}

	if time.Now().After(invite.ExpiresAt) {
		return nil, ErrInviteExpired
	}

	return invite, nil
}

// RedeemInvite marks an invite as used by the given invitee.
func (s *InviteService) RedeemInvite(ctx context.Context, token string, inviteeID int64) (*model.Invite, error) {
	invite, err := s.ValidateInvite(ctx, token)
	if err != nil {
		return nil, err
	}

	if err := s.invites.Redeem(ctx, token, inviteeID); err != nil {
		return nil, fmt.Errorf("redeem invite: %w", err)
	}

	s.eventBus.Publish(ctx, &event.InviteRedeemedEvent{
		Base:      event.NewBase(event.InviteRedeemed, event.Actor{ID: inviteeID}),
		InviteID:  invite.ID,
		InviteeID: inviteeID,
		Token:     token,
	})

	return invite, nil
}

// ListMyInvites returns a paginated list of invites created by the given user.
func (s *InviteService) ListMyInvites(ctx context.Context, userID int64, page, perPage int) ([]model.Invite, int64, error) {
	if page <= 0 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 25
	}
	if perPage > 100 {
		perPage = 100
	}
	return s.invites.ListByInviter(ctx, userID, page, perPage)
}
