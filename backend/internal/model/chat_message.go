package model

import "time"

// SystemChatUsername is the display name carried by system chat messages. They
// have no author row, so nothing resolves through the users JOIN.
const SystemChatUsername = "System"

// ChatMessage represents a message in the site-wide shoutbox/chat.
type ChatMessage struct {
	ID       int64
	UserID   int64  // 0 for system messages, which store NULL
	Username string // populated via JOIN; "System" for system messages
	Message  string
	// System marks an authorless announcement posted by the site itself (e.g.
	// the chat notification connector). A dedicated system *user* was rejected
	// because it would show up in member lists, counts and admin search.
	System    bool
	CreatedAt time.Time
}
