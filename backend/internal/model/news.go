package model

import "time"

// NewsArticle represents a news article.
type NewsArticle struct {
	ID         int64     `json:"id"`
	Title      string    `json:"title"`
	Body       string    `json:"body"`
	AuthorID   *int64    `json:"author_id"`
	Published  bool      `json:"published"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`

	// Joined fields (populated by queries with JOINs)
	AuthorName *string `json:"author_name,omitempty"`
}
