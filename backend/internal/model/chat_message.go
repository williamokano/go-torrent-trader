package model

import "time"

// SystemChatUsername is the *default* display name for system chat messages.
// They have no author row, so nothing resolves through the users JOIN and the
// name has to come from somewhere; operators override it with the
// chat_system_display_name site setting, and this is the fallback when it is
// unset or unreadable.
const SystemChatUsername = "System"

// ChatMessage represents a message in the site-wide shoutbox/chat.
type ChatMessage struct {
	ID     int64
	UserID int64 // 0 for system messages, which store NULL
	// Username is populated via JOIN for authored messages. System messages come
	// out of the repository blank and are labelled by ChatService.
	Username string
	Message  string
	// System marks an authorless announcement posted by the site itself (e.g.
	// the chat notification connector). A dedicated system *user* was rejected
	// because it would show up in member lists, counts and admin search.
	System    bool
	CreatedAt time.Time
}
