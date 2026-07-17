package service_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// setupMessageService's user repo is seeded with "sender" (id 1) and
// "receiver" (id 2) — see newMockUserRepoForMessage in message_test.go.

func TestSendMessage_SetsMentionedUsernames(t *testing.T) {
	svc := setupMessageService()

	msg, err := svc.SendMessage(context.Background(), 1, service.SendMessageRequest{
		ReceiverID: 2,
		Subject:    "hi",
		Body:       "hey @receiver",
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if !reflect.DeepEqual(msg.MentionedUsernames, []string{"receiver"}) {
		t.Errorf("MentionedUsernames = %v, want [receiver]", msg.MentionedUsernames)
	}
}

func TestSendMessage_NoMentionsSetsEmptyMentionedUsernames(t *testing.T) {
	svc := setupMessageService()

	msg, err := svc.SendMessage(context.Background(), 1, service.SendMessageRequest{
		ReceiverID: 2,
		Subject:    "hi",
		Body:       "no mentions here",
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if msg.MentionedUsernames == nil {
		t.Error("MentionedUsernames should be an empty slice, not nil")
	}
	if len(msg.MentionedUsernames) != 0 {
		t.Errorf("MentionedUsernames = %v, want empty", msg.MentionedUsernames)
	}
}

func TestSendMessage_DropsUnresolvedMentions(t *testing.T) {
	svc := setupMessageService()

	msg, err := svc.SendMessage(context.Background(), 1, service.SendMessageRequest{
		ReceiverID: 2,
		Subject:    "hi",
		Body:       "hey @totallyfakeuser",
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if len(msg.MentionedUsernames) != 0 {
		t.Errorf("MentionedUsernames = %v, want empty — the username doesn't exist", msg.MentionedUsernames)
	}
}

// PM mentions are links-only, by design: the recipient already gets a "new
// message" notification for the PM itself, so a mention notification would
// be redundant. This guards against a future change accidentally wiring
// publishMention into SendMessage (e.g. by copy-pasting from CreateComment).
func TestSendMessage_MentionDoesNotPublishUserMentionedEvent(t *testing.T) {
	bus := event.NewInMemoryBus()
	var fired bool
	bus.Subscribe(event.UserMentioned, func(_ context.Context, _ event.Event) error {
		fired = true
		return nil
	})

	svc := service.NewMessageService(newMockMessageRepo(), newMockUserRepoForMessage(), bus)

	_, err := svc.SendMessage(context.Background(), 1, service.SendMessageRequest{
		ReceiverID: 2,
		Subject:    "hi",
		Body:       "hey @receiver",
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if fired {
		t.Error("SendMessage published a UserMentionedEvent — PM mentions must be links-only, no notification")
	}
}
