package service

import (
	"context"
	"fmt"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// BanService handles email and IP ban business logic.
type BanService struct {
	bans     repository.BanRepository
	eventBus event.Bus
}

// NewBanService creates a new BanService.
func NewBanService(bans repository.BanRepository, bus event.Bus) *BanService {
	return &BanService{bans: bans, eventBus: bus}
}

// BanEmail adds a new email ban pattern.
func (s *BanService) BanEmail(ctx context.Context, actorID int64, actorUsername string, ban *model.BannedEmail) error {
	ban.CreatedBy = &actorID
	if err := s.bans.CreateEmailBan(ctx, ban); err != nil {
		return fmt.Errorf("ban email: %w", err)
	}
	s.eventBus.Publish(ctx, &event.EmailBannedEvent{
		Base:    event.NewBase(event.EmailBanned, event.Actor{ID: actorID, Username: actorUsername}),
		Pattern: ban.Pattern,
	})
	return nil
}

// UnbanEmail removes an email ban.
func (s *BanService) UnbanEmail(ctx context.Context, actorID int64, actorUsername string, id int64) error {
	// Get the ban for the event before deleting
	bans, err := s.bans.ListEmailBans(ctx)
	if err != nil {
		return fmt.Errorf("list email bans: %w", err)
	}
	var pattern string
	for _, b := range bans {
		if b.ID == id {
			pattern = b.Pattern
			break
		}
	}

	if err := s.bans.DeleteEmailBan(ctx, id); err != nil {
		return fmt.Errorf("unban email: %w", err)
	}
	s.eventBus.Publish(ctx, &event.EmailUnbannedEvent{
		Base:    event.NewBase(event.EmailUnbanned, event.Actor{ID: actorID, Username: actorUsername}),
		Pattern: pattern,
	})
	return nil
}

// ListEmailBans returns all email bans.
func (s *BanService) ListEmailBans(ctx context.Context) ([]model.BannedEmail, error) {
	return s.bans.ListEmailBans(ctx)
}

// CheckEmail returns true if the email is banned.
func (s *BanService) CheckEmail(ctx context.Context, email string) (bool, error) {
	return s.bans.IsEmailBanned(ctx, email)
}

// BanIP adds a new IP ban.
func (s *BanService) BanIP(ctx context.Context, actorID int64, actorUsername string, ban *model.BannedIP) error {
	ban.CreatedBy = &actorID
	if err := s.bans.CreateIPBan(ctx, ban); err != nil {
		return fmt.Errorf("ban ip: %w", err)
	}
	s.eventBus.Publish(ctx, &event.IPBannedEvent{
		Base:    event.NewBase(event.IPBanned, event.Actor{ID: actorID, Username: actorUsername}),
		IPRange: ban.IPRange,
	})
	return nil
}

// UnbanIP removes an IP ban.
func (s *BanService) UnbanIP(ctx context.Context, actorID int64, actorUsername string, id int64) error {
	// Get the ban for the event before deleting
	bans, err := s.bans.ListIPBans(ctx)
	if err != nil {
		return fmt.Errorf("list ip bans: %w", err)
	}
	var ipRange string
	for _, b := range bans {
		if b.ID == id {
			ipRange = b.IPRange
			break
		}
	}

	if err := s.bans.DeleteIPBan(ctx, id); err != nil {
		return fmt.Errorf("unban ip: %w", err)
	}
	s.eventBus.Publish(ctx, &event.IPUnbannedEvent{
		Base:    event.NewBase(event.IPUnbanned, event.Actor{ID: actorID, Username: actorUsername}),
		IPRange: ipRange,
	})
	return nil
}

// ListIPBans returns all IP bans.
func (s *BanService) ListIPBans(ctx context.Context) ([]model.BannedIP, error) {
	return s.bans.ListIPBans(ctx)
}

// CheckIP returns true if the IP is banned.
func (s *BanService) CheckIP(ctx context.Context, ip string) (bool, error) {
	return s.bans.IsIPBanned(ctx, ip)
}

// IsEmailBanned implements the BanChecker interface.
func (s *BanService) IsEmailBanned(ctx context.Context, email string) (bool, error) {
	return s.bans.IsEmailBanned(ctx, email)
}

// IsIPBanned implements the BanChecker interface.
func (s *BanService) IsIPBanned(ctx context.Context, ip string) (bool, error) {
	return s.bans.IsIPBanned(ctx, ip)
}
