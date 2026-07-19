package model

import "time"

// ForumCategory is a display grouping for forums.
type ForumCategory struct {
	ID        int64
	Name      string
	SortOrder int
	CreatedAt time.Time
	Forums    []Forum // populated when listing categories with forums
}

// Forum is a discussion board within a category.
type Forum struct {
	ID            int64
	CategoryID    int64
	Name          string
	Description   string
	SortOrder     int
	TopicCount    int
	PostCount     int
	LastPostID    *int64
	MinGroupLevel int
	MinPostLevel  int
	CreatedAt     time.Time

	// Denormalized last post info (populated by queries)
	LastPostAt         *time.Time
	LastPostUsername   *string
	LastPostTopicID    *int64
	LastPostTopicTitle *string
}

// ForumTopic is a discussion topic within a forum.
type ForumTopic struct {
	ID         int64
	ForumID    int64
	UserID     int64
	Title      string
	Pinned     bool
	Locked     bool
	PostCount  int
	ViewCount  int
	LastPostID *int64
	LastPostAt *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time

	// Denormalized fields (populated by queries)
	Username         string
	LastPostUsername *string
	ForumName        string
}

// ForumPost is a reply within a topic.
type ForumPost struct {
	ID                 int64
	TopicID            int64
	UserID             int64
	Body               string
	MentionedUsernames []string
	ReplyToPostID      *int64
	EditedAt           *time.Time
	EditedBy           *int64
	DeletedAt          *time.Time `json:"deleted_at,omitempty"`
	DeletedBy          *int64     `json:"deleted_by,omitempty"`
	CreatedAt          time.Time

	// Denormalized fields (populated by queries)
	Username      string
	Avatar        *string
	GroupName     string
	UserCreatedAt time.Time
	UserPostCount int
	IsFirstPost   bool
}

// ForumPostEdit tracks edit history for a forum post. Edits store a
// reversible diff against the version immediately after them (BE-9.23)
// rather than a full OldBody snapshot, so repeated small edits to the same
// post don't multiply storage. OldBody/NewBody are populated by
// ForumService.ListPostEdits — by reconstruction for diff-based rows, or
// read straight from the row for rows written before this migration — so
// the JSON contract callers see (BE-9.7's edit-history viewer) is unchanged.
type ForumPostEdit struct {
	ID        int64     `json:"id"`
	PostID    int64     `json:"post_id"`
	EditedBy  *int64    `json:"edited_by"`
	OldBody   string    `json:"old_body"`
	NewBody   string    `json:"new_body"`
	CreatedAt time.Time `json:"created_at"`
	Username  string    `json:"username,omitempty"`

	// Reason is an optional, staff-only note left when a staff member edits
	// on behalf of another user. ListPostEdits itself is staff-gated, so
	// this is never exposed to the post's author or other regular users.
	Reason *string `json:"reason,omitempty"`

	// Diff is the raw reversible patch (new body -> old body) as read from
	// or written to forum_post_edits.diff. Meaningless when IsSnapshot is
	// true (legacy rows never populate it).
	Diff string `json:"-"`

	// IsSnapshot is true for legacy rows written before BE-9.23, which carry
	// a full OldBody/NewBody snapshot directly instead of a diff. Set by the
	// repository from whether the row's old_body column is non-NULL — not
	// inferred from Diff being empty, so a future diff-based row that
	// somehow ends up with an empty Diff can't be silently misread as a
	// legacy snapshot (which would corrupt the backward-reconstruction walk
	// for every older edit in the chain).
	IsSnapshot bool `json:"-"`

	// ReconstructionFailed is set by ListPostEdits when a stored diff didn't
	// apply cleanly (a corrupt row, or two edits racing against the same
	// post — see EditPost's docs). OldBody/NewBody are unreliable for this
	// edit and everything older than it once this is true, so it renders as
	// "history unavailable" instead of either fabricated or absent content.
	ReconstructionFailed bool `json:"reconstruction_failed,omitempty"`
}

// ForumSearchResult represents a single search result from forum full-text search.
type ForumSearchResult struct {
	PostID     int64
	Body       string
	TopicID    int64
	TopicTitle string
	ForumID    int64
	ForumName  string
	UserID     int64
	Username   string
	CreatedAt  time.Time
	Snippet    string
	PostNumber int
}
