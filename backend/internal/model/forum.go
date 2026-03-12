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
	LastPostAt       *time.Time
	LastPostUsername  *string
	LastPostTopicID  *int64
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
	LastPostUsername  *string
	ForumName        string
}

// ForumPost is a reply within a topic.
type ForumPost struct {
	ID             int64
	TopicID        int64
	UserID         int64
	Body           string
	ReplyToPostID  *int64
	EditedAt       *time.Time
	EditedBy       *int64
	CreatedAt      time.Time

	// Denormalized fields (populated by queries)
	Username       string
	Avatar         *string
	GroupName      string
	UserCreatedAt  time.Time
	UserPostCount  int
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
}
