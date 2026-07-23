package listener

import (
	"context"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

func moderationMsgEvent(actorID int64, uploaderID int64, assignedModID *int64) *event.TorrentModerationMessagePostedEvent {
	return &event.TorrentModerationMessagePostedEvent{
		Base:                event.NewBase(event.TorrentModerationMsg, actor(actorID, "someone")),
		TorrentID:           1,
		TorrentName:         "Thing",
		UploaderID:          uploaderID,
		AssignedModeratorID: assignedModID,
	}
}

func TestModerationMessageNotifiesModeratorWhenUploaderWrites(t *testing.T) {
	h := newNotifHarness(t, nil, nil)
	modID := int64(8)
	// Uploader (5) writes → moderator (8) is notified, uploader (actor) is not.
	h.bus.Publish(context.Background(), moderationMsgEvent(5, 5, &modID))

	if got := types(h.store.forUser(8)); len(got) != 1 || got[0] != model.NotifModerationMessage {
		t.Errorf("moderator notifs = %v, want [moderation_message]", got)
	}
	if got := h.store.forUser(5); len(got) != 0 {
		t.Errorf("uploader is the actor and must not be self-notified, got %v", got)
	}
}

func TestModerationMessageNotifiesUploaderWhenModeratorWrites(t *testing.T) {
	h := newNotifHarness(t, nil, nil)
	modID := int64(8)
	// Moderator (8) writes → uploader (5) is notified, moderator (actor) is not.
	h.bus.Publish(context.Background(), moderationMsgEvent(8, 5, &modID))

	if got := types(h.store.forUser(5)); len(got) != 1 || got[0] != model.NotifModerationMessage {
		t.Errorf("uploader notifs = %v, want [moderation_message]", got)
	}
	if got := h.store.forUser(8); len(got) != 0 {
		t.Errorf("moderator is the actor and must not be self-notified, got %v", got)
	}
}

func TestModerationMessageWithNoAssignedModerator(t *testing.T) {
	h := newNotifHarness(t, nil, nil)
	// No moderator assigned; uploader (5) writes → nobody else to notify.
	h.bus.Publish(context.Background(), moderationMsgEvent(5, 5, nil))

	if got := h.store.forUser(5); len(got) != 0 {
		t.Errorf("actor should not be notified, got %v", got)
	}
}

func moderatedEvent(actorID, uploaderID int64, decision string) *event.TorrentModeratedEvent {
	return &event.TorrentModeratedEvent{
		Base:        event.NewBase(event.TorrentModerated, actor(actorID, "mod")),
		TorrentID:   1,
		TorrentName: "Thing",
		UploaderID:  uploaderID,
		Decision:    decision,
	}
}

func TestTorrentModeratedNotifiesUploader(t *testing.T) {
	h := newNotifHarness(t, nil, nil)
	// A moderator (9) approves uploader 5's torrent → uploader is notified.
	h.bus.Publish(context.Background(), moderatedEvent(9, 5, "approved"))

	if got := types(h.store.forUser(5)); len(got) != 1 || got[0] != model.NotifModerationDecision {
		t.Errorf("uploader notifs = %v, want [moderation_decision]", got)
	}
}

func TestTorrentModeratedSkipsSelfApprover(t *testing.T) {
	h := newNotifHarness(t, nil, nil)
	// An Uploader-class member approves their own torrent (actor == uploader) →
	// no self-notification.
	h.bus.Publish(context.Background(), moderatedEvent(5, 5, "approved"))

	if got := h.store.forUser(5); len(got) != 0 {
		t.Errorf("self-approver should not be notified, got %v", got)
	}
}
