package postgres

import (
	"encoding/json"
	"log/slog"
)

// MarshalMentionedUsernames encodes resolved @mentions for the JSONB
// mentioned_usernames column (torrent_comments, forum_posts, messages). A nil
// slice is normalized to an empty JSON array so the column is always a JSON
// array — matching its NOT NULL DEFAULT '[]'::jsonb — never the JSON null
// literal. Exported so the one-off backfill tool (cmd/backfill-mentions) uses
// the exact same encoding as the normal write paths, not a hand-rolled copy.
func MarshalMentionedUsernames(usernames []string) ([]byte, error) {
	if usernames == nil {
		usernames = []string{}
	}
	return json.Marshal(usernames)
}

// ScanMentionedUsernames decodes a mentioned_usernames JSONB column already
// read into raw bytes by a *sql.Row/*sql.Rows Scan. This never fails the
// caller: mentioned_usernames is a pure rendering aid (every write path
// through MarshalMentionedUsernames only ever produces a valid JSON array of
// strings), so a row that somehow contains something else — hand-edited
// data, a future migration mistake — degrades to "no mentions on this row"
// rather than aborting the whole comment/post it belongs to, let alone the
// entire page it's listed on.
func ScanMentionedUsernames(raw []byte) []string {
	var usernames []string
	if err := json.Unmarshal(raw, &usernames); err != nil {
		slog.Warn("malformed mentioned_usernames column, treating as empty", "raw", string(raw), "error", err)
		return []string{}
	}
	return usernames
}
