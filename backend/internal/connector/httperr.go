package connector

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// MaxDrainBytes is how much of a response body an HTTP connector reads before
// closing. Draining a little lets the connection be reused; draining all of it
// would let a hostile endpoint stream forever.
const MaxDrainBytes = 4 << 10

// StripURL unwraps *url.Error, which carries the full request URL.
//
// For every HTTP connector the URL is either sensitive or outright the
// credential: a Discord webhook URL *is* the secret, a Telegram bot token lives
// in the path, and plenty of generic receivers (Slack, Mattermost) do the same.
// This error is what lands in the delivery log for the whole retention window,
// so the URL never belongs in it. Keeping only the cause loses nothing worth
// reading.
//
// Redaction of configured secret values still runs on top of this — see
// RedactError. This is the guard for the part redaction cannot see, namely a
// URL the connector assembled rather than one stored in the config.
func StripURL(err error) error {
	var urlErr *url.Error
	if !errors.As(err, &urlErr) {
		return err
	}
	if urlErr.Err == nil {
		// Fail closed: the only thing left to report is the URL itself.
		return errors.New(urlErr.Op)
	}
	return fmt.Errorf("%s: %w", urlErr.Op, urlErr.Err)
}

// ErrorExcerpt renders a bounded, single-line excerpt of a response body for a
// delivery error.
//
// A bare status code is unactionable — "returned 400" tells an admin nothing,
// while the body usually says exactly which field the destination rejected. The
// excerpt is capped and stripped of line breaks; secret redaction and the
// overall length cap still run on top before anything is stored.
func ErrorExcerpt(body []byte, limit int) string {
	text := strings.TrimSpace(string(body))
	if text == "" {
		return ""
	}
	text = strings.Join(strings.Fields(text), " ")
	if len(text) > limit {
		return text[:limit] + "…"
	}
	return text
}
