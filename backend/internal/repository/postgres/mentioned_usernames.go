package postgres

import (
	"encoding/json"
	"log/slog"
)

// marshalMentionedUsernames encodes resolved @mentions for the JSONB
// mentioned_usernames column (torrent_comments, forum_posts). A nil slice is
// normalized to an empty JSON array so the column is always a JSON array —
// matching its NOT NULL DEFAULT '[]'::jsonb — never the JSON null literal.
func marshalMentionedUsernames(usernames []string) ([]byte, error) {
	if usernames == nil {
		usernames = []string{}
	}
	return json.Marshal(usernames)
}

// scanMentionedUsernames decodes a mentioned_usernames JSONB column already
// read into raw bytes by a *sql.Row/*sql.Rows Scan. This never fails the
// caller: mentioned_usernames is a pure rendering aid (every write path
// through marshalMentionedUsernames only ever produces a valid JSON array of
// strings), so a row that somehow contains something else — hand-edited
// data, a future migration mistake — degrades to "no mentions on this row"
// rather than aborting the whole comment/post it belongs to, let alone the
// entire page it's listed on.
func scanMentionedUsernames(raw []byte) []string {
	var usernames []string
	if err := json.Unmarshal(raw, &usernames); err != nil {
		slog.Warn("malformed mentioned_usernames column, treating as empty", "raw", string(raw), "error", err)
		return []string{}
	}
	return usernames
}
