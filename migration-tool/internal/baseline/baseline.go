// Package baseline holds the TorrentTrader 3.0 schema as shipped, so the tool
// can tell a stock install apart from one that years of mods have reshaped.
//
// The contents are transcribed from docs/FULL_FEATURE_DOCUMENTATION.md section 1,
// which is this repository's record of the original schema. Section 1.1 lists
// every table; section 1.2 gives columns for nine of them. Tables the document
// names but does not break down are recorded with ColumnsDocumented false — the
// tool then knows the table belongs to a stock install without pretending to
// know its columns, which keeps it from reporting every column of a table it
// cannot describe as a mod-added one.
package baseline

import (
	"sort"
	"strings"

	"github.com/williamokano/go-torrent-trader/migration-tool/internal/schema"
)

// Column is a baseline column: its structural facts plus the note the source
// document carried, which is worth showing to an operator reading a diff.
type Column struct {
	schema.Column
	Notes string
}

// Table is a baseline table.
type Table struct {
	Name    string
	Purpose string
	// Required marks a table whose absence means the migration cannot run:
	// either this is not a TorrentTrader database, or data every other table
	// refers to is missing.
	Required bool
	// ColumnsDocumented reports whether Columns describes the table in full.
	// When false, Columns is empty and the tool must not draw conclusions
	// about the table's columns.
	ColumnsDocumented bool
	Columns           []Column
}

// Column returns the named baseline column, matched case-insensitively.
func (t Table) Column(name string) (Column, bool) {
	for _, c := range t.Columns {
		if strings.EqualFold(c.Name, name) {
			return c, true
		}
	}
	return Column{}, false
}

// Schema is the TorrentTrader 3.0 baseline.
type Schema struct {
	Tables []Table
}

// Table returns the named baseline table, matched case-insensitively.
func (s Schema) Table(name string) (Table, bool) {
	for _, t := range s.Tables {
		if strings.EqualFold(t.Name, name) {
			return t, true
		}
	}
	return Table{}, false
}

// TableNames returns the baseline table names in sorted order.
func (s Schema) TableNames() []string {
	names := make([]string, 0, len(s.Tables))
	for _, t := range s.Tables {
		names = append(names, t.Name)
	}
	sort.Strings(names)
	return names
}

func col(name, typ, notes string) Column {
	return Column{Column: schema.Column{Name: name, Type: typ}, Notes: notes}
}

// documented builds a table whose columns are known in full.
func documented(name, purpose string, required bool, columns ...Column) Table {
	return Table{
		Name:              name,
		Purpose:           purpose,
		Required:          required,
		ColumnsDocumented: true,
		Columns:           columns,
	}
}

// named builds a table the baseline knows exists but cannot describe.
func named(name, purpose string, required bool) Table {
	return Table{Name: name, Purpose: purpose, Required: required}
}

// TorrentTrader30 returns the baseline schema. The value is rebuilt per call so
// no caller can mutate a shared copy.
func TorrentTrader30() Schema {
	return Schema{Tables: []Table{
		documented("users", "User accounts, stats, preferences", true,
			col("id", "int unsigned", "PK"),
			col("username", "varchar(40)", "UNIQUE"),
			col("password", "varchar(40)", "Hashed (SHA1/MD5/HMAC)"),
			col("secret", "varchar(20)", "Session/recovery token"),
			col("editsecret", "varchar(20)", "Email change token"),
			col("email", "varchar(80)", ""),
			col("status", "enum('pending','confirmed')", ""),
			col("enabled", "varchar(10)", "'yes'/'no'"),
			col("added", "datetime", "Registration date"),
			col("last_login", "datetime", ""),
			col("last_access", "datetime", ""),
			col("last_browse", "int", "Timestamp of last torrent browse"),
			col("ip", "varchar(39)", "Last known IP (IPv4/IPv6)"),
			col("class", "tinyint unsigned", "FK to groups.group_id"),
			col("privacy", "enum('strong','normal','low')", "Profile visibility"),
			col("stylesheet", "int", "FK to stylesheets.id"),
			col("language", "varchar(20)", "FK to languages.id"),
			col("uploaded", "bigint unsigned", "Total bytes uploaded"),
			col("downloaded", "bigint unsigned", "Total bytes downloaded"),
			col("passkey", "varchar(32)", "Tracker authentication key"),
			col("avatar", "varchar(100)", "Avatar URL"),
			col("title", "varchar(30)", "Custom title"),
			col("signature", "varchar(200)", ""),
			col("info", "text", "Bio/profile text"),
			col("country", "int unsigned", "FK to countries.id"),
			col("gender", "varchar(6)", ""),
			col("age", "int", ""),
			col("client", "varchar(25)", "Preferred torrent client"),
			col("donated", "int unsigned", "Donation amount"),
			col("warned", "char(3)", "'yes'/'no'"),
			col("forumbanned", "char(3)", "'yes'/'no'"),
			col("modcomment", "text", "Staff-only notes"),
			col("acceptpms", "enum('yes','no')", "Accept PMs from non-staff"),
			col("commentpm", "enum('yes','no')", "Notify on torrent comments"),
			col("notifs", "varchar(100)", "Notification preferences"),
			col("invited_by", "int", "FK to users.id (self-referencing)"),
			col("invitees", "varchar(100)", "Space-separated invited user IDs"),
			col("invites", "smallint", "Remaining invite count"),
			col("invitedate", "datetime", ""),
			col("team", "int unsigned", "FK to teams.id"),
			col("tzoffset", "int", "Timezone offset in hours"),
			col("hideshoutbox", "enum('yes','no')", ""),
			col("page", "text", "Current page (for who's online)"),
		),
		documented("groups", "Role definitions with granular permissions", true,
			col("group_id", "int", "PK"),
			col("level", "varchar(50)", "Group display name"),
			col("view_torrents", "enum('yes','no')", ""),
			col("edit_torrents", "enum('yes','no')", ""),
			col("delete_torrents", "enum('yes','no')", ""),
			col("view_users", "enum('yes','no')", ""),
			col("edit_users", "enum('yes','no')", ""),
			col("delete_users", "enum('yes','no')", ""),
			col("view_news", "enum('yes','no')", ""),
			col("edit_news", "enum('yes','no')", ""),
			col("delete_news", "enum('yes','no')", ""),
			col("can_upload", "enum('yes','no')", ""),
			col("can_download", "enum('yes','no')", ""),
			col("view_forum", "enum('yes','no')", ""),
			col("edit_forum", "enum('yes','no')", ""),
			col("delete_forum", "enum('yes','no')", ""),
			col("control_panel", "enum('yes','no')", ""),
			col("staff_page", "enum('yes','no')", ""),
			col("staff_public", "enum('yes','no')", ""),
			col("staff_sort", "tinyint unsigned", ""),
		),
		documented("torrents", "Core torrent metadata", true,
			col("id", "int unsigned", "PK"),
			col("info_hash", "varchar(40)", "UNIQUE (hex-encoded, 40 chars)"),
			col("name", "varchar(255)", "FULLTEXT indexed"),
			col("filename", "varchar(255)", "Original .torrent filename"),
			col("save_as", "varchar(255)", ""),
			col("descr", "text", "Description (BBCode)"),
			col("image1", "text", "Custom image URL/path"),
			col("image2", "text", "Custom image URL/path"),
			col("category", "int unsigned", "FK to categories.id"),
			col("torrentlang", "int unsigned", "FK to torrentlang.id"),
			col("size", "bigint unsigned", "Total bytes"),
			col("added", "datetime", ""),
			col("type", "enum('single','multi')", ""),
			col("numfiles", "int unsigned", ""),
			col("owner", "int unsigned", "FK to users.id"),
			col("anon", "enum('yes','no')", "Anonymous upload"),
			col("comments", "int unsigned", "Denormalized count"),
			col("views", "int unsigned", "Detail page views"),
			col("hits", "int unsigned", "Download link clicks"),
			col("times_completed", "int unsigned", "Denormalized completion count"),
			col("seeders", "int unsigned", "Denormalized seeder count"),
			col("leechers", "int unsigned", "Denormalized leecher count"),
			col("numratings", "int unsigned", ""),
			col("ratingsum", "int unsigned", "Sum for avg calculation"),
			col("visible", "enum('yes','no')", ""),
			col("banned", "enum('yes','no')", ""),
			col("nfo", "enum('yes','no')", "NFO file exists"),
			col("announce", "varchar(255)", "Primary announce URL"),
			col("external", "enum('yes','no')", "External tracker"),
			col("freeleech", "enum('0','1')", ""),
			col("last_action", "datetime", ""),
		),
		documented("peers", "Active peers (ephemeral tracker state)", true,
			col("id", "int unsigned", "PK"),
			col("torrent", "int unsigned", "FK to torrents.id"),
			col("peer_id", "varchar(40)", "BitTorrent peer ID"),
			col("ip", "varchar(64)", ""),
			col("port", "smallint unsigned", ""),
			col("uploaded", "bigint unsigned", "Per-session bytes up"),
			col("downloaded", "bigint unsigned", "Per-session bytes down"),
			col("to_go", "bigint unsigned", "Bytes remaining"),
			col("seeder", "enum('yes','no')", ""),
			col("started", "datetime", ""),
			col("last_action", "datetime", ""),
			col("connectable", "enum('yes','no')", "Port reachable"),
			col("client", "varchar(60)", "Client identifier"),
			col("userid", "varchar(32)", "FK to users.id"),
			col("passkey", "varchar(32)", ""),
		),
		documented("completed", "Download completion log per user/torrent", true,
			col("id", "int unsigned", "PK"),
			col("userid", "int", "FK to users.id"),
			col("torrentid", "int", "FK to torrents.id"),
			col("date", "datetime", ""),
		),
		documented("messages", "Private messages between users", false,
			col("id", "int unsigned", "PK"),
			col("sender", "int unsigned", "FK to users.id"),
			col("receiver", "int unsigned", "FK to users.id"),
			col("added", "datetime", ""),
			col("subject", "text", ""),
			col("msg", "text", "Body"),
			col("unread", "enum('yes','no')", ""),
			col("poster", "bigint unsigned", "Thread ID"),
			col("location", "enum('in','out','both','draft','template')", ""),
		),
		documented("forum_forums", "Forum categories", false,
			col("id", "int unsigned", "PK"),
			col("name", "varchar(60)", ""),
			col("description", "varchar(200)", ""),
			col("category", "tinyint", "FK to forumcats.id"),
			col("sort", "tinyint unsigned", "Display order"),
			col("minclassread", "tinyint unsigned", "Min group to read"),
			col("minclasswrite", "tinyint unsigned", "Min group to post"),
			col("guest_read", "enum('yes','no')", ""),
		),
		documented("forum_topics", "Forum threads", false,
			col("id", "int unsigned", "PK"),
			col("forumid", "int unsigned", "FK to forum_forums.id"),
			col("userid", "int unsigned", "Creator"),
			col("subject", "varchar(40)", ""),
			col("locked", "enum('yes','no')", ""),
			col("sticky", "enum('yes','no')", ""),
			col("moved", "enum('yes','no')", ""),
			col("views", "int", ""),
			col("lastpost", "int unsigned", "FK to forum_posts.id"),
		),
		documented("forum_posts", "Forum post content", false,
			col("id", "int unsigned", "PK"),
			col("topicid", "int unsigned", "FK to forum_topics.id"),
			col("userid", "int unsigned", "Author"),
			col("added", "datetime", ""),
			col("body", "longtext", "FULLTEXT indexed"),
			col("editedby", "int unsigned", "FK to users.id"),
			col("editedat", "datetime", ""),
		),

		// Section 1.1 names these tables but does not break down their columns.
		// Not required: a missing files table costs the per-torrent file
		// lists, which degrades the result without making it unloadable.
		named("files", "Individual files within a torrent", false),
		named("announce", "Tracker announce URLs per torrent", false),
		named("categories", "Torrent categories (hierarchical)", true),
		named("torrentlang", "Torrent language options", false),
		named("comments", "Comments on torrents and news", false),
		named("ratings", "Torrent ratings (1-5 stars)", false),
		named("reports", "Abuse reports (torrent/user/comment/forum)", false),
		named("forum_readposts", "Per-user read tracking for forum", false),
		named("forumcats", "Forum category groupings", false),
		named("shoutbox", "Chat/shoutbox messages", false),
		named("news", "Site news articles", false),
		named("polls", "Site polls (up to 20 options)", false),
		named("pollanswers", "Poll vote records", false),
		named("teams", "User teams/groups", false),
		named("bans", "IP address bans (range support)", false),
		named("email_bans", "Email/domain bans", false),
		named("warnings", "User warnings with expiry", false),
		named("blocks", "Sidebar widget configuration", false),
		named("faq", "FAQ categories and items", false),
		named("rules", "Site rules (role-gated visibility)", false),
		named("countries", "Country list with flags", false),
		named("languages", "UI language files", false),
		named("stylesheets", "Theme definitions", false),
		named("censor", "Word censor replacements", false),
		named("guests", "Guest visitor tracking", false),
		named("tasks", "Cron task scheduling", false),
		named("log", "Admin/system activity log", false),
		named("sqlerr", "SQL error log", false),
	}}
}
