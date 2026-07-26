// Package target declares the PostgreSQL schema this tool writes into.
//
// docs/ARCHITECTURE.md "Shared Nothing": migration-tool is its own Go module
// and may not import backend/internal, so anything it needs from the target
// schema it declares itself. That independence has a cost — a declaration
// nothing checks drifts from the real schema silently, and three mapping rules
// in this package's first version named target columns that did not exist.
//
// So the declaration below is checked. target_drift_test.go reads
// backend/migrations when they are present and fails if this disagrees with
// them, which keeps the module standalone without letting it quietly rot.
package target

import "strings"

// Schema is the target's tables and their columns.
type Schema struct {
	tables map[string][]string
}

// HasTable reports whether the target has the named table.
func (s Schema) HasTable(name string) bool {
	_, ok := s.tables[name]
	return ok
}

// Has reports whether the target has the named column on the named table.
func (s Schema) Has(table, column string) bool {
	for _, c := range s.tables[table] {
		if strings.EqualFold(c, column) {
			return true
		}
	}
	return false
}

// Columns returns the named table's columns, sorted.
func (s Schema) Columns(table string) []string {
	return append([]string(nil), s.tables[table]...)
}

// Tables returns every declared table name, sorted.
func (s Schema) Tables() []string {
	names := make([]string, 0, len(s.tables))
	for name := range s.tables {
		names = append(names, name)
	}
	sortStrings(names)
	return names
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// Seeded names the tables the target ships with rows already in them, and what
// makes a legacy row collide with one.
//
// This matters because the mapping keeps legacy ids so foreign keys resolve.
// That is safe for tables the target creates empty and unsafe for these: the
// primary keys collide, the unique constraints collide, and an explicit-id
// insert does not advance the BIGSERIAL sequence, so the first row the running
// site creates afterwards collides too.
var Seeded = map[string]string{
	"groups":           "6 rows, with name and slug UNIQUE",
	"categories":       "20 rows at explicit ids 1-20, plus subcategories",
	"forum_categories": "3 rows",
	"countries":        "25 rows, with code UNIQUE",
	"languages":        "20 rows, with code UNIQUE",
}

// PostgreSQL returns the target schema. Only the tables the mapping targets are
// declared; the rest of the backend's schema is not this tool's business.
func PostgreSQL() Schema {
	return Schema{tables: map[string][]string{
		"users":                    {"activated_at", "avatar", "bonus_points", "can_chat", "can_download", "can_feed", "can_forum", "can_invite", "can_upload", "created_at", "disabled_until", "donor", "downloaded", "email", "enabled", "group_id", "id", "info", "invited_by", "invites", "ip", "last_access", "last_login", "parked", "passkey", "password_hash", "password_scheme", "title", "updated_at", "uploaded", "username", "warn_until", "warned"},
		"groups":                   {"can_comment", "can_download", "can_feed", "can_forum", "can_invite", "can_self_approve", "can_upload", "color", "created_at", "id", "is_admin", "is_immune", "is_moderator", "level", "name", "slug", "updated_at"},
		"torrents":                 {"anonymous", "approved_at", "approved_by", "assigned_moderator_id", "banned", "category_id", "comments_count", "created_at", "description", "file_count", "files", "free", "id", "info_hash", "leechers", "metadata", "moderation_status", "name", "nfo", "search_vector", "seeders", "silver", "size", "times_completed", "updated_at", "uploader_id", "visible"},
		"peers":                    {"agent", "downloaded", "id", "ip", "last_announce", "left_bytes", "peer_id", "port", "seeder", "started_at", "torrent_id", "uploaded", "user_id"},
		"transfer_history":         {"completed_at", "downloaded", "id", "last_announce", "seeder", "torrent_id", "uploaded", "user_id"},
		"messages":                 {"body", "created_at", "id", "is_read", "mentioned_usernames", "parent_id", "receiver_deleted", "receiver_id", "sender_deleted", "sender_id", "subject"},
		"saved_messages":           {"body", "created_at", "id", "kind", "parent_id", "receiver_id", "subject", "updated_at", "user_id", "version"},
		"forum_categories":         {"created_at", "id", "name", "sort_order"},
		"forums":                   {"category_id", "created_at", "description", "id", "last_post_id", "min_group_level", "min_post_level", "name", "post_count", "sort_order", "topic_count", "updated_at"},
		"forum_topics":             {"created_at", "forum_id", "id", "last_post_at", "last_post_by", "last_post_id", "locked", "pinned", "post_count", "search_vector", "title", "updated_at", "user_id", "view_count"},
		"forum_posts":              {"body", "created_at", "deleted_at", "deleted_by", "edited_at", "edited_by", "id", "mentioned_usernames", "reply_to_post_id", "search_vector", "topic_id", "updated_at", "user_id"},
		"categories":               {"created_at", "id", "image", "image_url", "metadata_schema", "name", "parent_id", "slug", "sort_order", "updated_at"},
		"torrent_comments":         {"body", "created_at", "id", "mentioned_usernames", "torrent_id", "updated_at", "user_id"},
		"torrent_ratings":          {"created_at", "id", "rating", "torrent_id", "updated_at", "user_id"},
		"reports":                  {"created_at", "id", "reason", "reporter_id", "resolved", "resolved_at", "resolved_by", "torrent_id"},
		"news":                     {"author_id", "body", "created_at", "id", "published", "title", "updated_at"},
		"chat_messages":            {"created_at", "id", "message", "system", "user_id"},
		"warnings":                 {"created_at", "expires_at", "id", "issued_by", "lifted_at", "lifted_by", "lifted_reason", "reason", "status", "type", "updated_at", "user_id"},
		"banned_ips":               {"created_at", "created_by", "id", "ip_range", "reason"},
		"banned_emails":            {"created_at", "created_by", "id", "pattern", "reason"},
		"countries":                {"code", "id", "name"},
		"languages":                {"code", "id", "name"},
		"invites":                  {"created_at", "email", "expires_at", "id", "inviter_id", "token", "used_at", "used_by_id"},
		"mod_notes":                {"author_id", "created_at", "id", "note", "user_id"},
		"notification_preferences": {"enabled", "notification_type", "user_id"},
	}}
}
