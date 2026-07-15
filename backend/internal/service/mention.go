package service

import (
	"context"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/mention"
)

// publishMention parses @mentions from body and, if any exist, publishes a
// single UserMentionedEvent. Parsing here keeps the (possibly large) body off
// the event bus; `link` carries the source-specific ids the frontend needs to
// build the deep-link. Shared by the forum and comment publishers so the
// extract → guard → publish sequence can't drift between them.
func publishMention(ctx context.Context, bus event.Bus, actor event.Actor, source string, link map[string]any, contextTitle, body string) {
	names := mention.Extract(body)
	if len(names) == 0 {
		return
	}
	bus.Publish(ctx, &event.UserMentionedEvent{
		Base:               event.NewBase(event.UserMentioned, actor),
		Source:             source,
		MentionedUsernames: names,
		ContextTitle:       contextTitle,
		Link:               link,
	})
}
