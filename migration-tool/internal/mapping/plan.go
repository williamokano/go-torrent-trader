package mapping

// The proposed mapping from TorrentTrader 3.0 to this project's PostgreSQL
// schema. It is a proposal, not a decision: the generated file is meant to be
// read, argued with and edited before anything is written. Every rule carries
// the reason it exists, because a mapping an operator cannot audit is one they
// have to take on trust on the single night it matters.
//
// Target column names come from backend/migrations. Where the target schema has
// no home for a legacy column the rule says so plainly rather than inventing
// one — a skip with a reason is honest, a silent drop is not.

// Named transforms. A rule names one; the transformers added by the entity
// migrations implement it. They are constants so a rule can be found by
// reference rather than by grepping for a string, and plan_test.go rejects any
// rule naming a transform that is not on this list.
const (
	// TransformYesNoToBool converts ENUM('yes','no') to boolean.
	TransformYesNoToBool = "yes_no_to_bool"
	// TransformYesNoToBoolInverted converts ENUM('yes','no') to boolean and
	// negates it, for the columns whose sense is reversed in the target.
	TransformYesNoToBoolInverted = "yes_no_to_bool_inverted"
	// TransformZeroOneToBool converts ENUM('0','1') to boolean.
	TransformZeroOneToBool = "zero_one_to_bool"
	// TransformPositiveToBool converts a numeric amount to "is non-zero".
	TransformPositiveToBool = "positive_to_bool"
	// TransformHexToBytea converts a 40-character hex string to 20 raw bytes.
	TransformHexToBytea = "hex_to_bytea"
	// TransformTextToBytea stores a text column's bytes verbatim.
	TransformTextToBytea = "text_to_bytea"
	// TransformTextToInet parses a text address into an INET value.
	TransformTextToInet = "text_to_inet"
	// TransformLegacyHash carries a legacy password hash across unchanged and
	// records its scheme, so the backend can verify it once and re-hash.
	TransformLegacyHash = "legacy_hash"
	// TransformBBCodeToMarkdown converts BBCode body text to Markdown.
	TransformBBCodeToMarkdown = "bbcode_to_markdown"
	// TransformClassToGroupID resolves a legacy class number to a target
	// group id through the groups mapping.
	TransformClassToGroupID = "class_to_group_id"
	// TransformSlugify derives a URL slug from a display name.
	TransformSlugify = "slugify"
)

func mapTo(target, transform, comment string) Rule {
	return Rule{Action: ActionMap, Target: target, Transform: transform, Comment: comment}
}

func derive(target, comment string) Rule {
	return Rule{Action: ActionDerive, Target: target, Comment: comment}
}

func skip(comment string) Rule {
	return Rule{Action: ActionSkip, Comment: comment}
}

// Plan returns the proposed mapping, keyed by legacy table name.
func Plan() map[string]TablePlan {
	return map[string]TablePlan{
		"users":        usersPlan(),
		"groups":       groupsPlan(),
		"torrents":     torrentsPlan(),
		"peers":        peersPlan(),
		"completed":    completedPlan(),
		"messages":     messagesPlan(),
		"forum_forums": forumForumsPlan(),
		"forum_topics": forumTopicsPlan(),
		"forum_posts":  forumPostsPlan(),

		// Tables the baseline names but does not break down. The target is
		// settled here; the column rules come with the entity migrations that
		// implement each one.
		"categories":  {Action: TableMigrate, Target: "categories", Comment: "Hierarchical categories flatten onto categories.parent_id.\n\nThe target seeds 20 categories at explicit ids 1-20. Keeping legacy ids collides with them, so either merge onto the seeded rows or renumber and rewrite torrents.category_id to match. Reset the id sequence afterwards."},
		"comments":    {Action: TableMigrate, Target: "torrent_comments", Comment: "Legacy comments cover torrents and news; only the torrent rows have a home here. News comments have no target table."},
		"ratings":     {Action: TableMigrate, Target: "torrent_ratings", Comment: "torrents.numratings and torrents.ratingsum are recomputed from these rows rather than copied."},
		"files":       {Action: TableMigrate, Target: "torrents.files", Comment: "The target keeps the file list as JSONB on the torrent, so these rows are grouped by torrent rather than inserted one for one."},
		"reports":     {Action: TableMigrate, Target: "reports", Comment: ""},
		"news":        {Action: TableMigrate, Target: "news", Comment: ""},
		"shoutbox":    {Action: TableMigrate, Target: "chat_messages", Comment: ""},
		"warnings":    {Action: TableMigrate, Target: "warnings", Comment: ""},
		"bans":        {Action: TableMigrate, Target: "banned_ips", Comment: "banned_ips.ip_range is a Postgres CIDR, so a legacy range becomes one row rather than being expanded into many. A legacy first/last pair that is not a whole prefix has to be split into the CIDR blocks that cover it."},
		"email_bans":  {Action: TableMigrate, Target: "banned_emails", Comment: ""},
		"countries":   {Action: TableMigrate, Target: "countries", Comment: "The target seeds 25 countries with code UNIQUE. Match on code rather than inserting, or the run fails on the first duplicate. Nothing refers to these rows — the target does not put a country on the member record — so skipping the table entirely is reasonable."},
		"languages":   {Action: TableMigrate, Target: "languages", Comment: "The target seeds 20 languages with code UNIQUE. Match on code rather than inserting. As with countries, nothing in the target refers to these rows."},
		"forumcats":   {Action: TableMigrate, Target: "forum_categories", Comment: "The target seeds 3 forum categories. Merge onto them or insert alongside, then reset the id sequence."},
		"invites":     {Action: TableMigrate, Target: "invites", Comment: "Not part of the 3.0 baseline, but common enough as a mod to be worth naming a target for."},
		"teams":       {Action: TableReview, Comment: "Teams are proposed but not built, so the target schema has nowhere to put these rows yet. Keep the dump if you intend to load them when the feature lands."},
		"announce":    {Action: TableSkip, Comment: "Per-torrent announce URLs. The new tracker issues its own announce URL from the member's passkey."},
		"torrentlang": {Action: TableSkip, Comment: "Torrent language options have no target column."},

		// Deliberately dropped in the port — see docs/NOT_PORTING.md.
		"blocks":          {Action: TableSkip, Comment: "Sidebar widget system was not ported."},
		"stylesheets":     {Action: TableSkip, Comment: "Server-side theming was not ported; the new theme system is client-side."},
		"faq":             {Action: TableSkip, Comment: "Admin-managed FAQ was not ported."},
		"rules":           {Action: TableSkip, Comment: "Admin-managed rules pages were not ported."},
		"censor":          {Action: TableSkip, Comment: "Word censor was not ported."},
		"guests":          {Action: TableSkip, Comment: "Guest visitor tracking was not ported."},
		"polls":           {Action: TableSkip, Comment: "Polls were not ported. They may return; the rows are worth keeping in the dump."},
		"pollanswers":     {Action: TableSkip, Comment: "See polls."},
		"forum_readposts": {Action: TableSkip, Comment: "Per-user forum read tracking has no target table."},

		// Operational tables that describe the old installation, not its data.
		"tasks":  {Action: TableSkip, Comment: "Cron scheduling for the old PHP installation."},
		"log":    {Action: TableSkip, Comment: "Legacy admin log. The new activity log records events in a different shape; a copy would be unreadable alongside it."},
		"sqlerr": {Action: TableSkip, Comment: "Error log from the old installation."},
	}
}

func usersPlan() TablePlan {
	return TablePlan{
		Action:  TableMigrate,
		Target:  "users",
		Comment: "Members keep their id and passkey, so existing .torrent files keep announcing and every foreign key in the dump resolves without a translation table.",
		Derived: map[string]string{
			"password_scheme": "the scheme the legacy hash is in, so the backend can verify it once and re-hash to argon2id at next login",
			"bonus_points":    "0 — TorrentTrader 3.0 has no bonus economy",
			"parked":          "false",
			"updated_at":      "added, the registration date — the legacy schema records no modification time",
			"warn_until":      "NULL — legacy warnings are a flag with no expiry, so a warned member stays warned until staff clear it",
			"disabled_until":  "NULL — legacy enabled is permanent, not timed",
			"can_upload":      "true — the legacy schema grants this by class, not per member, and groups.can_upload already carries it",
			"can_download":    "true — see can_upload",
			"can_invite":      "true — see can_upload",
			"can_chat":        "true — no legacy per-member equivalent",
			"can_feed":        "true — no legacy per-member equivalent",
		},
		Columns: map[string]Rule{
			"id":           mapTo("id", "", "Kept, so foreign keys across the dump resolve without a translation table."),
			"username":     mapTo("username", "", "Legacy varchar(40) against a target varchar(20). Names longer than 20 characters have to be shortened before the run — nothing checks this yet, so find them with SELECT username FROM users WHERE CHAR_LENGTH(username) > 20."),
			"password":     mapTo("password_hash", TransformLegacyHash, "Carried across as-is. The backend verifies the legacy scheme once and re-hashes to argon2id, so nobody is asked to reset a password."),
			"secret":       skip("Legacy session token. Sessions are re-established at first login."),
			"editsecret":   skip("Email-change token; the new flow issues its own."),
			"email":        mapTo("email", "", ""),
			"status":       derive("activated_at", "ENUM('pending','confirmed') becomes a timestamp: confirmed accounts take their registration date, pending accounts stay NULL."),
			"enabled":      mapTo("enabled", TransformYesNoToBool, ""),
			"added":        mapTo("created_at", "", ""),
			"last_login":   mapTo("last_login", "", ""),
			"last_access":  mapTo("last_access", "", ""),
			"last_browse":  skip("Last-browse timestamp has no target column."),
			"ip":           mapTo("ip", TransformTextToInet, "An address the target's INET type rejects lands as NULL rather than failing the batch."),
			"class":        mapTo("group_id", TransformClassToGroupID, "Resolved through the groups mapping, which must run first."),
			"privacy":      skip("Profile visibility has no target setting."),
			"stylesheet":   skip("Per-user stylesheet; theming is client-side now."),
			"language":     skip("UI language has no target column on the member record."),
			"uploaded":     mapTo("uploaded", "", ""),
			"downloaded":   mapTo("downloaded", "", ""),
			"passkey":      mapTo("passkey", "", "Must survive the cutover: every .torrent file already downloaded announces with this key."),
			"avatar":       mapTo("avatar", "", ""),
			"title":        mapTo("title", "", ""),
			"signature":    skip("Forum signatures have no target column."),
			"info":         mapTo("info", "", ""),
			"country":      skip("The target keeps a countries table but does not put a country on the member record."),
			"gender":       skip("No target column."),
			"age":          skip("No target column."),
			"client":       skip("Preferred-client preference has no target column."),
			"donated":      mapTo("donor", TransformPositiveToBool, "Legacy stores an amount; the target records only whether the member donated, so any non-zero amount becomes true."),
			"warned":       mapTo("warned", TransformYesNoToBool, ""),
			"forumbanned":  mapTo("can_forum", TransformYesNoToBoolInverted, "Reversed sense: forumbanned='yes' becomes can_forum=false."),
			"modcomment":   derive("mod_notes", "Staff notes become one mod_notes row per member rather than a column."),
			"acceptpms":    skip("No target setting for refusing private messages."),
			"commentpm":    derive("notification_preferences", "Becomes a notification preference row; the exact shape is settled by the social-data migration."),
			"notifs":       derive("notification_preferences", "Legacy packed preference string, superseded by notification_preferences."),
			"invited_by":   mapTo("invited_by", "", ""),
			"invitees":     derive("users.invited_by", "Space-separated ids, redundant with invited_by. Read only as a cross-check that the two agree."),
			"invites":      mapTo("invites", "", ""),
			"invitedate":   skip("No target column."),
			"team":         skip("Teams are not in the target schema yet."),
			"tzoffset":     skip("Timezones are handled in the browser."),
			"hideshoutbox": skip("Client-side preference in the new UI."),
			"page":         skip("Who's-online scratch state."),
		},
	}
}

func groupsPlan() TablePlan {
	return TablePlan{
		Action:  TableMigrate,
		Target:  "groups",
		Comment: "The legacy permission flags are finer-grained than the target's. Several collapse onto is_moderator and is_admin, which loses detail — check the result against your staff roster before going live.\n\nThe target seeds this table with 6 groups whose name and slug are UNIQUE, so legacy ids and names collide. Decide per group whether to merge into the seeded row or insert alongside it, and reset the id sequence afterwards.",
		Derived: map[string]string{
			"slug":             "slugified from level",
			"level":            "the legacy group_id, which is also the class rank users.class refers to",
			"can_comment":      "true — TorrentTrader has no separate comment permission",
			"can_invite":       "true — invites are governed per member in the legacy schema",
			"can_feed":         "true — no legacy equivalent",
			"can_self_approve": "false — a new permission with no legacy counterpart; grant it deliberately",
			"is_immune":        "false — set this by hand for the staff groups that should not be ratio-watched",
			"color":            "NULL — legacy groups have no colour",
			"created_at":       "the run time",
			"updated_at":       "the run time",
		},
		Columns: map[string]Rule{
			"group_id":        mapTo("id", "", "Kept: users.class refers to it."),
			"level":           mapTo("name", "", "The display name, copied as-is. groups.slug is slugified from it."),
			"view_torrents":   skip("Every member can browse in the target."),
			"edit_torrents":   derive("is_moderator", "Folded into is_moderator with the other moderation flags."),
			"delete_torrents": derive("is_moderator", "See edit_torrents."),
			"view_users":      skip("Every member can view profiles in the target."),
			"edit_users":      derive("is_moderator", "See edit_torrents."),
			"delete_users":    derive("is_admin", "Deleting members is an admin power in the target."),
			"view_news":       skip("Every member can read news in the target."),
			"edit_news":       derive("is_moderator", "See edit_torrents."),
			"delete_news":     derive("is_moderator", "See edit_torrents."),
			"can_upload":      mapTo("can_upload", TransformYesNoToBool, ""),
			"can_download":    mapTo("can_download", TransformYesNoToBool, ""),
			"view_forum":      skip("Forum read access is governed by forums.min_group_level in the target."),
			"edit_forum":      mapTo("can_forum", TransformYesNoToBool, ""),
			"delete_forum":    derive("is_moderator", "See edit_torrents."),
			"control_panel":   mapTo("is_admin", TransformYesNoToBool, ""),
			"staff_page":      derive("is_moderator", "See edit_torrents."),
			"staff_public":    skip("Staff-page visibility has no target column."),
			"staff_sort":      skip("Display ordering of the staff page."),
		},
	}
}

func torrentsPlan() TablePlan {
	return TablePlan{
		Action:  TableMigrate,
		Target:  "torrents",
		Comment: "info_hash is the one column that cannot be got wrong: the target stores the 20 raw bytes where the legacy schema stores 40 hex characters, and a torrent whose hash does not survive stops announcing.",
		Derived: map[string]string{
			"silver":            "false — no legacy equivalent",
			"files":             "the legacy files table, grouped by torrent into one JSONB document",
			"moderation_status": "approved for a visible torrent, rejected for a banned one",
			"search_vector":     "maintained by the backend; leave it to the trigger rather than writing it",
		},
		Columns: map[string]Rule{
			"id":              mapTo("id", "", "Kept: peers, comments and completions all refer to it."),
			"info_hash":       mapTo("info_hash", TransformHexToBytea, "40 hex characters become the 20 bytes the tracker compares against. A row whose hash is not 40 valid hex characters must stop the run, not be skipped."),
			"name":            mapTo("name", "", ""),
			"filename":        derive("object storage", "The .torrent file itself moves to object storage; the row keeps no filename."),
			"save_as":         skip("No target column."),
			"descr":           mapTo("description", TransformBBCodeToMarkdown, ""),
			"image1":          derive("metadata", "Custom images go into the torrents.metadata JSONB document."),
			"image2":          derive("metadata", "See image1."),
			"category":        mapTo("category_id", "", "Resolved through the categories mapping, which must run first."),
			"torrentlang":     skip("No target column for torrent language."),
			"size":            mapTo("size", "", ""),
			"added":           mapTo("created_at", "", ""),
			"type":            skip("single/multi is implied by file_count in the target."),
			"numfiles":        mapTo("file_count", "", ""),
			"owner":           mapTo("uploader_id", "", ""),
			"anon":            mapTo("anonymous", TransformYesNoToBool, ""),
			"comments":        mapTo("comments_count", "", "Denormalised count. Worth recomputing after the comments migration rather than trusting the legacy value."),
			"views":           skip("Detail-page view counter has no target column."),
			"hits":            skip("Download-link counter has no target column."),
			"times_completed": mapTo("times_completed", "", ""),
			"seeders":         mapTo("seeders", "", "Recomputed by the tracker on the first announce; copied so the listing is not empty in the meantime."),
			"leechers":        mapTo("leechers", "", "See seeders."),
			"numratings":      derive("torrent_ratings", "Recomputed from the migrated rating rows."),
			"ratingsum":       derive("torrent_ratings", "See numratings."),
			"visible":         mapTo("visible", TransformYesNoToBool, ""),
			"banned":          mapTo("banned", TransformYesNoToBool, ""),
			"nfo":             derive("nfo", "Legacy stores a yes/no flag and keeps the NFO in a file; the target stores the text. The flag says which torrents have a file to read."),
			"announce":        skip("The new tracker issues its own announce URL."),
			"external":        skip("External-tracker flag has no target column."),
			"freeleech":       mapTo("free", TransformZeroOneToBool, ""),
			"last_action":     mapTo("updated_at", "", ""),
		},
	}
}

func peersPlan() TablePlan {
	return TablePlan{
		Action:  TableMigrate,
		Target:  "peers",
		Comment: "Peers are live tracker state with a short life. Migrating them keeps swarms intact across the cutover; skipping the table costs a few minutes of empty peer lists and nothing else.",
		Columns: map[string]Rule{
			"id":          mapTo("id", "", ""),
			"torrent":     mapTo("torrent_id", "", ""),
			"peer_id":     mapTo("peer_id", TransformTextToBytea, "The target stores the peer id as bytes."),
			"ip":          mapTo("ip", TransformTextToInet, "peers.ip is NOT NULL in the target, so a row with an unparseable address is dropped rather than nulled."),
			"port":        mapTo("port", "", ""),
			"uploaded":    mapTo("uploaded", "", ""),
			"downloaded":  mapTo("downloaded", "", ""),
			"to_go":       mapTo("left_bytes", "", ""),
			"seeder":      mapTo("seeder", TransformYesNoToBool, ""),
			"started":     mapTo("started_at", "", ""),
			"last_action": mapTo("last_announce", "", ""),
			"connectable": skip("Connectability checking was not ported."),
			"client":      mapTo("agent", "", ""),
			"userid":      mapTo("user_id", "", "Legacy stores this as a varchar; the target column is a bigint."),
			"passkey":     skip("Redundant: the member is already identified by user_id."),
		},
	}
}

func completedPlan() TablePlan {
	return TablePlan{
		Action:  TableMigrate,
		Target:  "transfer_history",
		Comment: "The legacy completion log becomes the target's transfer history. Legacy keeps no per-torrent byte counts, so the totals start at zero — members keep their overall ratio from users.uploaded and users.downloaded, not from these rows.\n\nBoth sides are unique on (member, torrent), so the rows transfer one for one. What does not transfer is a row whose member or torrent no longer exists: the target has real foreign keys where the legacy MyISAM schema had none, so orphaned rows have to be dropped rather than inserted.",
		Derived: map[string]string{
			"uploaded":      "0 — the legacy schema records no per-torrent totals",
			"downloaded":    "0 — see uploaded",
			"seeder":        "false",
			"last_announce": "date",
		},
		Columns: map[string]Rule{
			"id":        mapTo("id", "", ""),
			"userid":    mapTo("user_id", "", ""),
			"torrentid": mapTo("torrent_id", "", ""),
			"date":      mapTo("completed_at", "", ""),
		},
	}
}

func messagesPlan() TablePlan {
	return TablePlan{
		Action:  TableMigrate,
		Target:  "messages",
		Comment: "Only the rows a member actually sent or received. Drafts and templates go to saved_messages instead — see the note on location.",
		Derived: map[string]string{
			"parent_id":           "NULL — the legacy poster column is a thread id, which cannot be resolved back to the single parent the target threads on",
			"mentioned_usernames": "empty — the target populates this when a message is written",
		},
		Columns: map[string]Rule{
			"id":       mapTo("id", "", ""),
			"sender":   mapTo("sender_id", "", ""),
			"receiver": mapTo("receiver_id", "", ""),
			"added":    mapTo("created_at", "", ""),
			"subject":  mapTo("subject", "", "Legacy TEXT against a target varchar(255); longer subjects are truncated."),
			"msg":      mapTo("body", TransformBBCodeToMarkdown, ""),
			"unread":   mapTo("is_read", TransformYesNoToBoolInverted, "Reversed sense: unread='yes' becomes is_read=false."),
			"poster":   skip("Legacy thread id; the target threads messages through parent_id, which the legacy data cannot reconstruct."),
			"location": derive("sender_deleted, receiver_deleted", "The legacy mailbox enum encodes which side deleted the message: 'in' means the sender deleted it, 'out' means the receiver did, 'both' means neither — see FULL_FEATURE_DOCUMENTATION.md section 8.5 before 'correcting' this. Rows marked 'draft' or 'template' are not messages at all: they belong in saved_messages, whose kind column takes exactly those two values."),
		},
	}
}

func forumForumsPlan() TablePlan {
	return TablePlan{
		Action:  TableMigrate,
		Target:  "forums",
		Comment: "Read and write levels both carry across: minclassread becomes min_group_level and minclasswrite becomes min_post_level.",
		Derived: map[string]string{
			"topic_count":  "counted from the migrated topics",
			"post_count":   "counted from the migrated posts",
			"last_post_id": "the newest migrated post in the forum",
			"created_at":   "the run time — the legacy schema records no creation date for a forum",
			"updated_at":   "the run time",
		},
		Columns: map[string]Rule{
			"id":            mapTo("id", "", ""),
			"name":          mapTo("name", "", ""),
			"description":   mapTo("description", "", ""),
			"category":      mapTo("category_id", "", "Resolved through the forumcats mapping."),
			"sort":          mapTo("sort_order", "", ""),
			"minclassread":  mapTo("min_group_level", "", ""),
			"minclasswrite": mapTo("min_post_level", "", "The target gates starting a topic on this level. It does not gate replies, so a forum that was read-only to most members needs checking by hand."),
			"guest_read":    skip("The target has no guest access to forums."),
		},
	}
}

func forumTopicsPlan() TablePlan {
	return TablePlan{
		Action:  TableMigrate,
		Target:  "forum_topics",
		Comment: "",
		Derived: map[string]string{
			"post_count":    "counted from the migrated posts",
			"last_post_at":  "the created_at of the post last_post_id points to",
			"last_post_by":  "the author of the post last_post_id points to",
			"created_at":    "the added date of the topic's first post — legacy topics carry no timestamp of their own",
			"updated_at":    "last_post_at",
			"search_vector": "maintained by the backend",
		},
		Columns: map[string]Rule{
			"id":       mapTo("id", "", ""),
			"forumid":  mapTo("forum_id", "", ""),
			"userid":   mapTo("user_id", "", ""),
			"subject":  mapTo("title", "", ""),
			"locked":   mapTo("locked", TransformYesNoToBool, ""),
			"sticky":   mapTo("pinned", TransformYesNoToBool, ""),
			"moved":    skip("Moved-topic tombstones have no target representation."),
			"views":    mapTo("view_count", "", ""),
			"lastpost": mapTo("last_post_id", "", ""),
		},
	}
}

func forumPostsPlan() TablePlan {
	return TablePlan{
		Action:  TableMigrate,
		Target:  "forum_posts",
		Comment: "",
		Derived: map[string]string{
			"updated_at":          "editedat where the post was edited, otherwise added",
			"deleted_at":          "NULL — the legacy forum deletes posts outright, so nothing is soft-deleted on arrival",
			"deleted_by":          "NULL — see deleted_at",
			"reply_to_post_id":    "NULL — legacy posts are a flat list with no reply threading",
			"mentioned_usernames": "empty — the target populates this when a post is written",
			"search_vector":       "maintained by the backend",
		},
		Columns: map[string]Rule{
			"id":       mapTo("id", "", ""),
			"topicid":  mapTo("topic_id", "", ""),
			"userid":   mapTo("user_id", "", ""),
			"added":    mapTo("created_at", "", ""),
			"body":     mapTo("body", TransformBBCodeToMarkdown, ""),
			"editedby": mapTo("edited_by", "", ""),
			"editedat": mapTo("edited_at", "", ""),
		},
	}
}
