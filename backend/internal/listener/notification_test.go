package listener

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// --- mocks ------------------------------------------------------------------

// mockNotifStore records what the listener asked to be created.
type mockNotifStore struct {
	mu      sync.Mutex
	created []model.Notification
}

func (m *mockNotifStore) Create(_ context.Context, n *model.Notification) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.created = append(m.created, *n)
	return nil
}
func (m *mockNotifStore) GetByID(_ context.Context, _ int64) (*model.Notification, error) {
	return nil, nil
}
func (m *mockNotifStore) List(_ context.Context, _ int64, _ repository.ListNotificationsOptions) ([]model.Notification, int64, error) {
	return nil, 0, nil
}
func (m *mockNotifStore) MarkRead(_ context.Context, _, _ int64) error        { return nil }
func (m *mockNotifStore) MarkAllRead(_ context.Context, _ int64) error        { return nil }
func (m *mockNotifStore) CountUnread(_ context.Context, _ int64) (int, error) { return 0, nil }
func (m *mockNotifStore) DeleteOld(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}

// forType returns the notifications created for a given recipient.
func (m *mockNotifStore) forUser(userID int64) []model.Notification {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []model.Notification
	for _, n := range m.created {
		if n.UserID == userID {
			out = append(out, n)
		}
	}
	return out
}

// mockNotifPrefs enables every notification type unless disabled explicitly.
type mockNotifPrefs struct {
	disabled map[string]bool
}

func (m *mockNotifPrefs) Get(_ context.Context, _ int64, _ string) (*model.NotificationPreference, error) {
	return nil, nil
}
func (m *mockNotifPrefs) GetAll(_ context.Context, _ int64) ([]model.NotificationPreference, error) {
	return nil, nil
}
func (m *mockNotifPrefs) Set(_ context.Context, _ int64, _ string, _ bool) error { return nil }
func (m *mockNotifPrefs) IsEnabled(_ context.Context, _ int64, notifType string) (bool, error) {
	return !m.disabled[notifType], nil
}

// mockTopicSubs returns a fixed subscriber list and records subscribe calls.
type mockTopicSubs struct {
	mu            sync.Mutex
	subscribers   []int64
	subscribeCall []int64 // user IDs passed to Subscribe
	listErr       error
}

func (m *mockTopicSubs) Subscribe(_ context.Context, userID, _ int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.subscribeCall = append(m.subscribeCall, userID)
	return nil
}
func (m *mockTopicSubs) Unsubscribe(_ context.Context, _, _ int64) error { return nil }
func (m *mockTopicSubs) IsSubscribed(_ context.Context, _, _ int64) (bool, error) {
	return false, nil
}
func (m *mockTopicSubs) ListSubscribers(_ context.Context, _ int64) ([]int64, error) {
	return m.subscribers, m.listErr
}

// mockNotifUsers resolves @mentions. Unknown usernames error, as the real
// repository does for a user that does not exist.
type mockNotifUsers struct {
	byName map[string]int64
}

func (m *mockNotifUsers) GetByUsername(_ context.Context, username string) (*model.User, error) {
	id, ok := m.byName[username]
	if !ok {
		return nil, errors.New("not found")
	}
	return &model.User{ID: id, Username: username}, nil
}
func (m *mockNotifUsers) GetByID(_ context.Context, _ int64) (*model.User, error) { return nil, nil }
func (m *mockNotifUsers) GetByEmail(_ context.Context, _ string) (*model.User, error) {
	return nil, nil
}
func (m *mockNotifUsers) GetByPasskey(_ context.Context, _ string) (*model.User, error) {
	return nil, nil
}
func (m *mockNotifUsers) Count(_ context.Context) (int64, error)                { return 0, nil }
func (m *mockNotifUsers) Create(_ context.Context, _ *model.User) error         { return nil }
func (m *mockNotifUsers) Update(_ context.Context, _ *model.User) error         { return nil }
func (m *mockNotifUsers) IncrementStats(_ context.Context, _, _, _ int64) error { return nil }
func (m *mockNotifUsers) List(_ context.Context, _ repository.ListUsersOptions) ([]model.User, int64, error) {
	return nil, 0, nil
}
func (m *mockNotifUsers) ListStaff(_ context.Context) ([]model.User, error) { return nil, nil }
func (m *mockNotifUsers) UpdateLastAccess(_ context.Context, _ int64) error { return nil }

// --- harness ----------------------------------------------------------------

type notifHarness struct {
	bus    event.Bus
	store  *mockNotifStore
	prefs  *mockNotifPrefs
	subs   *mockTopicSubs
	users  *mockNotifUsers
	pushed []int64 // user IDs that received a WebSocket push
	mu     sync.Mutex
}

func newNotifHarness(t *testing.T, subscribers []int64, usernames map[string]int64) *notifHarness {
	t.Helper()

	h := &notifHarness{
		bus:   event.NewInMemoryBus(),
		store: &mockNotifStore{},
		prefs: &mockNotifPrefs{disabled: map[string]bool{}},
		subs:  &mockTopicSubs{subscribers: subscribers},
		users: &mockNotifUsers{byName: usernames},
	}

	sendToUser := func(userID int64, _ []byte) {
		h.mu.Lock()
		defer h.mu.Unlock()
		h.pushed = append(h.pushed, userID)
	}

	// topics and forums are nil: Subscribe then skips its existence check, which
	// keeps these tests about event->notification mapping rather than topic lookup.
	svc := service.NewNotificationService(h.store, h.prefs, h.subs, nil, nil, sendToUser)

	// postRepo is unused by the listener; pass nil rather than a hollow mock.
	RegisterNotificationListeners(h.bus, svc, h.users, nil, h.subs)

	return h
}

func actor(id int64, username string) event.Actor {
	return event.Actor{ID: id, Username: username}
}

func postEvent(authorID int64, authorName string, body string, replyToUserID *int64) *event.ForumPostCreatedEvent {
	return &event.ForumPostCreatedEvent{
		Base:          event.NewBase(event.ForumPostCreated, actor(authorID, authorName)),
		PostID:        100,
		TopicID:       7,
		TopicTitle:    "A Topic",
		ForumID:       3,
		Body:          body,
		ReplyToUserID: replyToUserID,
	}
}

// types returns the notification types created for a user, for concise asserts.
func types(notifs []model.Notification) []string {
	out := make([]string, 0, len(notifs))
	for _, n := range notifs {
		out = append(out, n.Type)
	}
	return out
}

// --- tests ------------------------------------------------------------------

func TestForumPostNotifiesRepliedToUser(t *testing.T) {
	replyTo := int64(2)
	h := newNotifHarness(t, nil, nil)

	h.bus.Publish(context.Background(), postEvent(1, "alice", "thanks!", &replyTo))

	got := types(h.store.forUser(2))
	if len(got) != 1 || got[0] != model.NotifForumReply {
		t.Fatalf("user 2 got %v, want one %s", got, model.NotifForumReply)
	}
}

func TestForumPostNotifiesMentionedUsers(t *testing.T) {
	h := newNotifHarness(t, nil, map[string]int64{"bob": 2, "carol": 3})

	h.bus.Publish(context.Background(), postEvent(1, "alice", "hey @bob and @carol", nil))

	for _, id := range []int64{2, 3} {
		got := types(h.store.forUser(id))
		if len(got) != 1 || got[0] != model.NotifForumMention {
			t.Errorf("user %d got %v, want one %s", id, got, model.NotifForumMention)
		}
	}
}

// A mention of a username that doesn't resolve must be skipped, not crash the
// handler or abort the remaining notifications.
func TestForumPostSkipsUnknownMentionAndStillNotifiesSubscribers(t *testing.T) {
	h := newNotifHarness(t, []int64{4}, map[string]int64{"bob": 2})

	h.bus.Publish(context.Background(), postEvent(1, "alice", "hi @ghost and @bob", nil))

	if got := h.store.forUser(2); len(got) != 1 {
		t.Errorf("known mention @bob got %d notifications, want 1", len(got))
	}
	if got := types(h.store.forUser(4)); len(got) != 1 || got[0] != model.NotifTopicReply {
		t.Errorf("subscriber got %v, want one %s — an unresolvable mention must not abort the run", got, model.NotifTopicReply)
	}
}

func TestForumPostNotifiesTopicSubscribers(t *testing.T) {
	h := newNotifHarness(t, []int64{2, 3}, nil)

	h.bus.Publish(context.Background(), postEvent(1, "alice", "an update", nil))

	for _, id := range []int64{2, 3} {
		got := types(h.store.forUser(id))
		if len(got) != 1 || got[0] != model.NotifTopicReply {
			t.Errorf("subscriber %d got %v, want one %s", id, got, model.NotifTopicReply)
		}
	}
}

// The dedup rule is the subtle part: a user who is replied to AND mentioned AND
// subscribed is all three at once, and must still receive exactly one
// notification — the most specific one (forum_reply), not three.
func TestForumPostSendsOneNotificationToAUserMatchingEveryRule(t *testing.T) {
	replyTo := int64(2)
	h := newNotifHarness(t, []int64{2}, map[string]int64{"bob": 2})

	h.bus.Publish(context.Background(), postEvent(1, "alice", "@bob see above", &replyTo))

	got := types(h.store.forUser(2))
	if len(got) != 1 {
		t.Fatalf("user 2 got %d notifications (%v), want exactly 1 — reply/mention/subscription must dedup", len(got), got)
	}
	if got[0] != model.NotifForumReply {
		t.Errorf("user 2 got %s, want %s — the reply is the most specific match", got[0], model.NotifForumReply)
	}
}

// The author is excluded from every path: self-mention, own subscription, and
// even replying to their own post.
func TestForumPostNeverNotifiesTheAuthor(t *testing.T) {
	author := int64(1)
	h := newNotifHarness(t, []int64{1}, map[string]int64{"alice": 1})

	h.bus.Publish(context.Background(), postEvent(author, "alice", "replying to myself @alice", &author))

	if got := h.store.forUser(author); len(got) != 0 {
		t.Errorf("author got %d notifications (%v), want 0", len(got), types(got))
	}
}

func TestForumPostAutoSubscribesAuthor(t *testing.T) {
	h := newNotifHarness(t, nil, nil)

	h.bus.Publish(context.Background(), postEvent(1, "alice", "first", nil))

	h.subs.mu.Lock()
	defer h.subs.mu.Unlock()
	if len(h.subs.subscribeCall) != 1 || h.subs.subscribeCall[0] != 1 {
		t.Errorf("Subscribe calls = %v, want the author (1) subscribed to the topic", h.subs.subscribeCall)
	}
}

// A failure listing subscribers must not lose the notifications already created
// for the replied-to user.
func TestForumPostKeepsEarlierNotificationsWhenSubscriberLookupFails(t *testing.T) {
	replyTo := int64(2)
	h := newNotifHarness(t, nil, nil)
	h.subs.listErr = errors.New("db down")

	h.bus.Publish(context.Background(), postEvent(1, "alice", "hi", &replyTo))

	if got := types(h.store.forUser(2)); len(got) != 1 || got[0] != model.NotifForumReply {
		t.Errorf("user 2 got %v, want the forum_reply to survive a subscriber-lookup failure", got)
	}
}

// A user who turned a notification type off must not receive it.
func TestDisabledPreferenceSuppressesNotification(t *testing.T) {
	h := newNotifHarness(t, []int64{2}, nil)
	h.prefs.disabled[model.NotifTopicReply] = true

	h.bus.Publish(context.Background(), postEvent(1, "alice", "an update", nil))

	if got := h.store.forUser(2); len(got) != 0 {
		t.Errorf("user 2 got %v, want none — topic_reply is disabled in their preferences", types(got))
	}
}

func TestTopicCreatedAutoSubscribesCreator(t *testing.T) {
	h := newNotifHarness(t, nil, nil)

	h.bus.Publish(context.Background(), &event.ForumTopicCreatedEvent{
		Base:    event.NewBase(event.ForumTopicCreated, actor(9, "dave")),
		TopicID: 7,
	})

	h.subs.mu.Lock()
	defer h.subs.mu.Unlock()
	if len(h.subs.subscribeCall) != 1 || h.subs.subscribeCall[0] != 9 {
		t.Errorf("Subscribe calls = %v, want the topic creator (9)", h.subs.subscribeCall)
	}
}

func TestTorrentCommentedNotifiesUploader(t *testing.T) {
	h := newNotifHarness(t, nil, nil)

	h.bus.Publish(context.Background(), &event.TorrentCommentedEvent{
		Base:        event.NewBase(event.TorrentCommented, actor(1, "alice")),
		CommentID:   5,
		TorrentID:   11,
		TorrentName: "Some.Release",
		UploaderID:  2,
	})

	got := types(h.store.forUser(2))
	if len(got) != 1 || got[0] != model.NotifTorrentComment {
		t.Fatalf("uploader got %v, want one %s", got, model.NotifTorrentComment)
	}
}

func TestMessageSentNotifiesReceiver(t *testing.T) {
	h := newNotifHarness(t, nil, nil)

	h.bus.Publish(context.Background(), &event.MessageSentEvent{
		Base:       event.NewBase(event.MessageSent, actor(1, "alice")),
		MessageID:  5,
		ReceiverID: 2,
	})

	got := types(h.store.forUser(2))
	if len(got) != 1 || got[0] != model.NotifPMReceived {
		t.Fatalf("receiver got %v, want one %s", got, model.NotifPMReceived)
	}
}

func TestWarningIssuedNotifiesWarnedUser(t *testing.T) {
	h := newNotifHarness(t, nil, nil)

	h.bus.Publish(context.Background(), &event.WarningIssuedEvent{
		Base:        event.NewBase(event.WarningIssued, actor(1, "staff")),
		WarningID:   5,
		UserID:      2,
		WarningType: "ratio",
	})

	notifs := h.store.forUser(2)
	if len(notifs) != 1 || notifs[0].Type != model.NotifSystem {
		t.Fatalf("warned user got %v, want one %s", types(notifs), model.NotifSystem)
	}

	// The payload must carry enough for the UI to render the warning.
	var data map[string]interface{}
	if err := json.Unmarshal(notifs[0].Data, &data); err != nil {
		t.Fatalf("notification data is not valid JSON: %v", err)
	}
	if data["warning_type"] != "ratio" {
		t.Errorf("warning_type = %v, want \"ratio\"", data["warning_type"])
	}
}

// Every created notification is pushed over the WebSocket, which is how the
// unread badge updates without a refresh.
func TestNotificationIsPushedOverWebSocket(t *testing.T) {
	h := newNotifHarness(t, nil, nil)

	h.bus.Publish(context.Background(), &event.MessageSentEvent{
		Base:       event.NewBase(event.MessageSent, actor(1, "alice")),
		MessageID:  5,
		ReceiverID: 2,
	})

	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.pushed) != 1 || h.pushed[0] != 2 {
		t.Errorf("pushed to %v, want a push to the receiver (2)", h.pushed)
	}
}

// An email address in a post body is not a mention: the regex requires the @ to
// start the string or follow whitespace/an open paren.
func TestEmailAddressIsNotTreatedAsMention(t *testing.T) {
	h := newNotifHarness(t, nil, map[string]int64{"bob": 2})

	h.bus.Publish(context.Background(), postEvent(1, "alice", "mail me at alice@bob.example", nil))

	if got := h.store.forUser(2); len(got) != 0 {
		t.Errorf("user 2 got %v, want none — \"alice@bob\" is an email, not a mention", types(got))
	}
}
