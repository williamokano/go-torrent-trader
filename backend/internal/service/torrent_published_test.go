package service

import (
	"context"
	"encoding/hex"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

// publishedHarness wires a TorrentService with moderation and records every
// TorrentPublished event the bus carries.
type publishedHarness struct {
	svc      *TorrentService
	repo     *memTorrentRepo
	settings *mockSiteSettingsRepo
	events   []*event.TorrentPublishedEvent
}

func newPublishedHarness(t *testing.T) *publishedHarness {
	t.Helper()

	repo := newMemTorrentRepo()
	bus := event.NewInMemoryBus()
	settingsRepo := newMockSiteSettingsRepo()

	svc := NewTorrentService(nil, repo, newMemUserRepo(), newMemStorage(),
		TorrentServiceConfig{}, bus, nil)
	svc.SetModerationRepo(&fakeModerationRepo{repo: repo})
	svc.SetSiteSettings(NewSiteSettingsService(settingsRepo, bus))

	h := &publishedHarness{svc: svc, repo: repo, settings: settingsRepo}
	bus.Subscribe(event.TorrentPublished, func(_ context.Context, evt event.Event) error {
		h.events = append(h.events, evt.(*event.TorrentPublishedEvent))
		return nil
	})
	return h
}

// setModeration flips the moderation master switch, which is what decides
// whether an upload publishes immediately or waits for approval.
func (h *publishedHarness) setModeration(t *testing.T, enabled bool) {
	t.Helper()
	value := "false"
	if enabled {
		value = "true"
	}
	if err := h.settings.Set(context.Background(), SettingModerationEnabled, value); err != nil {
		t.Fatalf("set moderation setting: %v", err)
	}
}

func TestApproveEmitsTorrentPublished(t *testing.T) {
	ctx := context.Background()
	h := newPublishedHarness(t)

	tor := &model.Torrent{
		Name:             "Some.Release-GROUP",
		InfoHash:         []byte{0xde, 0xad, 0xbe, 0xef},
		Size:             1234,
		FileCount:        3,
		CategoryID:       2,
		CategoryName:     "Movies",
		UploaderID:       5,
		UploaderName:     "alice",
		Free:             true,
		ModerationStatus: model.ModerationPending,
	}
	if err := h.repo.Create(ctx, tor); err != nil {
		t.Fatalf("create: %v", err)
	}

	if _, err := h.svc.ApproveTorrent(ctx, tor.ID, 9, model.Permissions{IsAdmin: true}); err != nil {
		t.Fatalf("approve: %v", err)
	}

	if len(h.events) != 1 {
		t.Fatalf("got %d TorrentPublished events, want exactly 1", len(h.events))
	}
	e := h.events[0]
	if e.TorrentID != tor.ID || e.Name != "Some.Release-GROUP" {
		t.Fatalf("event = %+v, want it to describe the approved torrent", e)
	}
	if e.InfoHashHex != hex.EncodeToString(tor.InfoHash) {
		t.Fatalf("info hash = %q, want hex of the raw hash", e.InfoHashHex)
	}
	if e.CategoryName != "Movies" || e.CategoryID != 2 {
		t.Fatalf("category = %d/%q, want 2/Movies", e.CategoryID, e.CategoryName)
	}
	if e.UploaderID != 5 || e.UploaderName != "alice" {
		t.Fatalf("uploader = %d/%q, want 5/alice", e.UploaderID, e.UploaderName)
	}
	if !e.Freeleech {
		t.Fatal("freeleech flag lost")
	}
	if e.Actor.ID != 9 {
		t.Fatalf("actor = %d, want the approver (9)", e.Actor.ID)
	}
	if e.PublishedAt.IsZero() {
		t.Fatal("published_at must be set")
	}
}

// The single most important guarantee in the whole feature: an anonymous
// torrent's uploader name must not travel on the event at all, so nothing
// downstream can leak it even by accident.
func TestApproveOfAnonymousTorrentCarriesNoUploaderName(t *testing.T) {
	ctx := context.Background()
	h := newPublishedHarness(t)

	tor := &model.Torrent{
		Name:             "Secret.Release-GROUP",
		UploaderID:       5,
		UploaderName:     "alice",
		Anonymous:        true,
		ModerationStatus: model.ModerationPending,
	}
	if err := h.repo.Create(ctx, tor); err != nil {
		t.Fatalf("create: %v", err)
	}

	if _, err := h.svc.ApproveTorrent(ctx, tor.ID, 9, model.Permissions{IsAdmin: true}); err != nil {
		t.Fatalf("approve: %v", err)
	}

	if len(h.events) != 1 {
		t.Fatalf("got %d events, want 1", len(h.events))
	}
	e := h.events[0]
	if !e.Anonymous {
		t.Fatal("anonymous flag lost")
	}
	if e.UploaderName != "" {
		t.Fatalf("uploader name = %q, want it dropped at the source for an anonymous upload", e.UploaderName)
	}
}

func TestRejectDoesNotEmitTorrentPublished(t *testing.T) {
	ctx := context.Background()
	h := newPublishedHarness(t)

	tor := &model.Torrent{Name: "Rejected", UploaderID: 5, ModerationStatus: model.ModerationPending}
	if err := h.repo.Create(ctx, tor); err != nil {
		t.Fatalf("create: %v", err)
	}

	if _, err := h.svc.RejectTorrent(ctx, tor.ID, 9, model.Permissions{IsModerator: true}); err != nil {
		t.Fatalf("reject: %v", err)
	}
	if len(h.events) != 0 {
		t.Fatalf("rejected torrent published %d events, want 0", len(h.events))
	}
}

// ErrNotPending already blocks a second approval, so there is no path to a
// double announcement — this pins that.
func TestReapproveDoesNotEmitTwice(t *testing.T) {
	ctx := context.Background()
	h := newPublishedHarness(t)

	tor := &model.Torrent{Name: "Twice", UploaderID: 5, ModerationStatus: model.ModerationPending}
	if err := h.repo.Create(ctx, tor); err != nil {
		t.Fatalf("create: %v", err)
	}

	if _, err := h.svc.ApproveTorrent(ctx, tor.ID, 9, model.Permissions{IsAdmin: true}); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if _, err := h.svc.ApproveTorrent(ctx, tor.ID, 9, model.Permissions{IsAdmin: true}); err == nil {
		t.Fatal("expected a second approval to be refused")
	}
	if len(h.events) != 1 {
		t.Fatalf("got %d events, want exactly 1", len(h.events))
	}
}

// A rejected torrent later approved is a legitimate first publish.
func TestRejectedThenApprovedEmitsOnce(t *testing.T) {
	ctx := context.Background()
	h := newPublishedHarness(t)

	tor := &model.Torrent{Name: "Second Chance", UploaderID: 5, ModerationStatus: model.ModerationPending}
	if err := h.repo.Create(ctx, tor); err != nil {
		t.Fatalf("create: %v", err)
	}

	if _, err := h.svc.RejectTorrent(ctx, tor.ID, 9, model.Permissions{IsModerator: true}); err != nil {
		t.Fatalf("reject: %v", err)
	}
	if _, err := h.svc.ApproveTorrent(ctx, tor.ID, 9, model.Permissions{IsAdmin: true}); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if len(h.events) != 1 {
		t.Fatalf("got %d events, want exactly 1", len(h.events))
	}
}

func TestUploadWithModerationOnDoesNotEmit(t *testing.T) {
	ctx := context.Background()
	h := newPublishedHarness(t)
	h.setModeration(t, true)

	_, err := h.svc.Upload(ctx, buildTorrentFile("Pending.Upload"), UploadTorrentRequest{
		Name: "Pending.Upload", CategoryID: 1,
	}, 5)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}

	if len(h.events) != 0 {
		t.Fatalf("a pending upload published %d events, want 0", len(h.events))
	}
}

func TestUploadWithModerationOffEmitsOnUpload(t *testing.T) {
	ctx := context.Background()
	h := newPublishedHarness(t)
	h.setModeration(t, false)

	torrent, err := h.svc.Upload(ctx, buildTorrentFile("Auto.Approved"), UploadTorrentRequest{
		Name: "Auto.Approved", CategoryID: 1,
	}, 5)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}

	if len(h.events) != 1 {
		t.Fatalf("got %d events, want 1 for an auto-approved upload", len(h.events))
	}
	e := h.events[0]
	if e.TorrentID != torrent.ID {
		t.Fatalf("event torrent = %d, want %d", e.TorrentID, torrent.ID)
	}
	if e.Actor.ID != 5 {
		t.Fatalf("actor = %d, want the uploader (5)", e.Actor.ID)
	}
}

func TestAnonymousAutoApprovedUploadCarriesNoUploaderName(t *testing.T) {
	ctx := context.Background()
	h := newPublishedHarness(t)
	h.setModeration(t, false)

	_, err := h.svc.Upload(ctx, buildTorrentFile("Anon.Upload"), UploadTorrentRequest{
		Name: "Anon.Upload", CategoryID: 1, Anonymous: true,
	}, 5)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}

	if len(h.events) != 1 {
		t.Fatalf("got %d events, want 1", len(h.events))
	}
	if h.events[0].UploaderName != "" {
		t.Fatalf("uploader name = %q, want it dropped for an anonymous upload", h.events[0].UploaderName)
	}
	if !h.events[0].Anonymous {
		t.Fatal("anonymous flag lost")
	}
}
