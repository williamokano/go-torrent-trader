package service

import (
	"context"
	"fmt"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/mention"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
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

// ResolveMentionedUsernames extracts @mentions from body and resolves them
// against real users, so rendering never has to re-look-up whether a token is
// a valid mention — the caller persists the returned (canonical, deduped)
// usernames alongside the content and the frontend links a token only if it's
// in that set. Extracts independently of publishMention (a second, cheap
// in-memory regex pass) rather than sharing one extraction: the notify-once
// pipeline and this persist-on-every-save pipeline are reasoned about and
// called from different places (edits call this but never publishMention),
// and coupling their call signatures together isn't worth trading for it.
func ResolveMentionedUsernames(ctx context.Context, users repository.UserRepository, body string) ([]string, error) {
	names := mention.Extract(body)
	if len(names) == 0 {
		return []string{}, nil
	}

	seen := make(map[string]bool, len(names))
	unique := make([]string, 0, len(names))
	for _, name := range names {
		if !seen[name] {
			seen[name] = true
			unique = append(unique, name)
		}
	}

	resolved, err := users.GetByUsernames(ctx, unique)
	if err != nil {
		return nil, fmt.Errorf("resolve mentioned usernames: %w", err)
	}

	usernames := make([]string, len(resolved))
	for i := range resolved {
		usernames[i] = resolved[i].Username
	}
	return usernames, nil
}
