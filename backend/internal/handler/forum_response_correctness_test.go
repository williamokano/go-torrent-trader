package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// #236. Three forum endpoints echo page and per_page, and they echoed the values as
// *requested* rather than as applied: the service clamped, the handler reported its
// own unclamped locals. An echo exists so a client can paginate off it, so
// `?per_page=500` reporting 500 while serving 100 rows made the client skip 400
// rows, and `?page=abc` reporting 0 made it loop — the discarded strconv.Atoi error
// leaving the local at its zero value.
//
// Asserted on all three, because the bug was three copies of the same mistake.
func TestPaginatedForumResponsesEchoTheAppliedValues(t *testing.T) {
	type endpoint struct {
		name string
		call func(*forumDeps, string) *httptest.ResponseRecorder
	}

	endpoints := []endpoint{{
		name: "list topics",
		call: func(d *forumDeps, query string) *httptest.ResponseRecorder {
			req := withForumAuth(httptest.NewRequest(http.MethodGet, "/api/v1/forums/1/topics"+query, nil), 7, memberPerms())
			req = withURLParam(req, "id", "1")
			w := httptest.NewRecorder()
			d.handler().HandleListTopics(w, req)
			return w
		},
	}, {
		name: "get topic",
		call: func(d *forumDeps, query string) *httptest.ResponseRecorder {
			d.topics.topics[100] = &model.ForumTopic{ID: 100, ForumID: 1, UserID: 7, Title: "A topic"}
			req := withForumAuth(httptest.NewRequest(http.MethodGet, "/api/v1/forums/topics/100"+query, nil), 7, memberPerms())
			req = withURLParam(req, "id", "100")
			w := httptest.NewRecorder()
			d.handler().HandleGetTopic(w, req)
			return w
		},
	}, {
		name: "search",
		call: func(d *forumDeps, query string) *httptest.ResponseRecorder {
			sep := "&"
			if query == "" {
				sep = "?"
				query = ""
			}
			url := "/api/v1/forums/search?q=hello" + strings.Replace(query, "?", sep, 1)
			req := withForumAuth(httptest.NewRequest(http.MethodGet, url, nil), 7, memberPerms())
			w := httptest.NewRecorder()
			d.handler().HandleSearchForum(w, req)
			return w
		},
	}}

	cases := []struct {
		name        string
		query       string
		wantPage    float64
		wantPerPage float64
	}{
		{name: "per_page above the cap", query: "?per_page=500", wantPage: 1, wantPerPage: 100},
		{name: "unparseable page", query: "?page=abc", wantPage: 1, wantPerPage: 25},
		{name: "zero page", query: "?page=0", wantPage: 1, wantPerPage: 25},
		{name: "negative per_page", query: "?per_page=-5", wantPage: 1, wantPerPage: 25},
		{name: "no params at all", query: "", wantPage: 1, wantPerPage: 25},
		{name: "values within range are honoured", query: "?page=3&per_page=10", wantPage: 3, wantPerPage: 10},
	}

	for _, ep := range endpoints {
		for _, tc := range cases {
			t.Run(ep.name+"/"+tc.name, func(t *testing.T) {
				d := newForumDeps()
				w := ep.call(d, tc.query)
				if w.Code != http.StatusOK {
					t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
				}
				body := decodeBody(t, w)
				if got := body["page"]; got != tc.wantPage {
					t.Errorf("page = %v, want %v — a client paginating off this field "+
						"computes the wrong offset", got, tc.wantPage)
				}
				if got := body["per_page"]; got != tc.wantPerPage {
					t.Errorf("per_page = %v, want %v", got, tc.wantPerPage)
				}
			})
		}
	}
}

// #235. can_create_topic tells the client whether to show the compose button, and it
// checked only the group level. CreateTopic additionally requires the member's own
// forum privilege, so a member whose posting had been revoked as a sanction was
// shown an enabled button, wrote a topic, and lost it to a 403 on submit — the worst
// possible ordering.
//
// The privilege has to be read from the member's row, not from Permissions:
// can_forum exists on both `groups` and `users`, and PermissionsFromGroup copies the
// class flag. Branching on that would still show the button to exactly the member
// the sanction was applied to.
func TestCanCreateTopicRespectsARevokedForumPrivilege(t *testing.T) {
	for _, tc := range []struct {
		name     string
		canForum bool
		want     bool
	}{
		{name: "privilege intact", canForum: true, want: true},
		{name: "privilege revoked", canForum: false, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := newForumDeps()
			d.users = newStubUserRepo(
				&model.User{ID: 7, Username: "member", GroupID: 5, CanForum: tc.canForum},
			)
			h := d.handler()

			req := withForumAuth(httptest.NewRequest(http.MethodGet, "/api/v1/forums/1/topics", nil), 7, memberPerms())
			req = withURLParam(req, "id", "1")
			w := httptest.NewRecorder()
			h.HandleListTopics(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
			}
			if got := decodeBody(t, w)["can_create_topic"]; got != tc.want {
				t.Errorf("can_create_topic = %v, want %v — the flag must answer the "+
					"same question the create call does", got, tc.want)
			}
		})
	}
}

// An anonymous caller cannot create a topic, and must not be reported as able to.
func TestCanCreateTopicIsFalseWhenNotAuthenticated(t *testing.T) {
	d := newForumDeps()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/forums/1/topics", nil)
	req = withURLParam(req, "id", "1")
	w := httptest.NewRecorder()
	d.handler().HandleListTopics(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if got := decodeBody(t, w)["can_create_topic"]; got != false {
		t.Errorf("can_create_topic = %v for an anonymous caller, want false", got)
	}
}

// #230. Five forum moderation paths record an optional reason on the activity-log
// entry. Three bounded it; rename and move passed it straight through, so a staff
// member could write an arbitrarily large string into an activity-log row on those
// two endpoints and not the other four. The cap exists to keep those rows bounded,
// so the inconsistency was the bug rather than the cap.
//
// Asserted on the event the activity-log listener consumes, which is where the
// reason actually lands. A multi-byte reason, so a byte-based cap would show up as a
// rune-count failure rather than passing by accident.
func TestEveryModerationReasonIsTruncated(t *testing.T) {
	long := strings.Repeat("é", 900)

	for _, tc := range []struct {
		name      string
		eventType event.Type
		reasonOf  func(event.Event) (string, bool)
		call      func(*ForumHandler, string)
	}{{
		name:      "rename",
		eventType: event.ForumTopicRenamed,
		reasonOf: func(e event.Event) (string, bool) {
			ev, ok := e.(*event.ForumTopicRenamedEvent)
			if !ok {
				return "", false
			}
			return ev.Reason, true
		},
		call: func(h *ForumHandler, reason string) {
			body := `{"title":"A new title","reason":"` + reason + `"}`
			req := withForumAuth(httptest.NewRequest(http.MethodPut,
				"/api/v1/forums/topics/100/title", strings.NewReader(body)), 1, staffPerms())
			req = withURLParam(req, "id", "100")
			w := httptest.NewRecorder()
			h.HandleRenameTopic(w, req)
			if w.Code != http.StatusOK {
				panic("rename failed: " + w.Body.String())
			}
		},
	}, {
		name:      "move",
		eventType: event.ForumTopicMoved,
		reasonOf: func(e event.Event) (string, bool) {
			ev, ok := e.(*event.ForumTopicMovedEvent)
			if !ok {
				return "", false
			}
			return ev.Reason, true
		},
		call: func(h *ForumHandler, reason string) {
			body := `{"forum_id":2,"reason":"` + reason + `"}`
			req := withForumAuth(httptest.NewRequest(http.MethodPost,
				"/api/v1/forums/topics/100/move", strings.NewReader(body)), 1, staffPerms())
			req = withURLParam(req, "id", "100")
			w := httptest.NewRecorder()
			h.HandleMoveTopic(w, req)
			if w.Code != http.StatusOK {
				panic("move failed: " + w.Body.String())
			}
		},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			d := newForumDeps()
			d.topics.topics[100] = &model.ForumTopic{ID: 100, ForumID: 1, UserID: 7, Title: "Original"}
			bus := event.NewInMemoryBus()

			var seen []string
			bus.Subscribe(tc.eventType, func(_ context.Context, e event.Event) error {
				if reason, ok := tc.reasonOf(e); ok {
					seen = append(seen, reason)
				}
				return nil
			})

			h := NewForumHandler(service.NewForumService(
				nil, d.categories, d.forums, d.topics, d.posts, d.users, d.groups, bus,
			))
			tc.call(h, long)

			if len(seen) == 0 {
				t.Fatal("no event was published, so this test proves nothing about the reason")
			}
			for _, reason := range seen {
				if n := len([]rune(reason)); n > maxReasonLength {
					t.Errorf("a %d-rune reason reached the activity log; the cap is %d",
						n, maxReasonLength)
				}
			}
		})
	}
}
