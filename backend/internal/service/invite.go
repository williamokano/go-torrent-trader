package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

var (
	ErrNoInvitesRemaining = errors.New("no invites remaining")
	ErrInvalidInviteEmail = errors.New("invalid invite email")
	ErrInviteNotFound     = errors.New("invite not found")
	ErrInviteExpired      = errors.New("invite has expired")
	ErrInviteRedeemed     = errors.New("invite has already been redeemed")
)

const inviteExpiryDuration = 7 * 24 * time.Hour

// InviteService handles invitation business logic.
type InviteService struct {
	invites     repository.InviteRepository
	users       repository.UserRepository
	email       EmailSender
	eventBus    event.Bus
	siteBaseURL string
}

// NewInviteService creates a new InviteService.
func NewInviteService(invites repository.InviteRepository, users repository.UserRepository, email EmailSender, bus event.Bus, siteBaseURL string) *InviteService {
	return &InviteService{
		invites:     invites,
		users:       users,
		email:       email,
		eventBus:    bus,
		siteBaseURL: siteBaseURL,
	}
}

// SendInvite creates a new invite for the given email and sends the invite email.
func (s *InviteService) SendInvite(ctx context.Context, inviterID int64, email string) (*model.Invite, error) {
	if email == "" || !emailRe.MatchString(email) {
		return nil, fmt.Errorf("%w: email is required and must be valid", ErrInvalidInviteEmail)
	}

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
		Email:     email,
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

	// Send invite email
	inviteURL := fmt.Sprintf("%s/signup?invite=%s", s.siteBaseURL, token)
	htmlBody := fmt.Sprintf(
		`<h2>You've Been Invited!</h2>
<p>%s has invited you to join TorrentTrader.</p>
<p><a href="%s">Click here to create your account</a></p>
<p>This invitation expires in 7 days.</p>`,
		inviter.Username, inviteURL,
	)

	if err := s.email.Send(ctx, email, "You're Invited to TorrentTrader", htmlBody); err != nil {
		slog.Error("failed to send invite email", "invite_id", invite.ID, "error", err)
		// Non-fatal: invite is created even if email fails
	}

	s.eventBus.Publish(ctx, &event.InviteSentEvent{
		Base:     event.NewBase(event.InviteSent, event.Actor{ID: inviterID, Username: inviter.Username}),
		InviteID: invite.ID,
		Email:    email,
	})

	return invite, nil
}

// RedeemInvite validates an invite token. Returns the invite if valid.
// The actual user creation is handled by the registration flow.
func (s *InviteService) RedeemInvite(ctx context.Context, token string) (*model.Invite, error) {
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

// ListMyInvites returns a paginated list of invites sent by the given user.
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
