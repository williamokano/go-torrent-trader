package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

// Positive content assertion, not just key presence: a purely negative-
// assertion suite previously shipped a real bug in the sanitize schema for
// comment/forum mentions (see docs/IMPLEMENTATION_TASKS.md, BE-8.20/FE-5.17)
// — asserting the actual resolved username is what would have caught it.
func TestSendMessageIncludesMentionedUsernames(t *testing.T) {
	messages := newStubMessageRepo()
	users := newStubUserRepo(
		&model.User{ID: 9, Username: "receiver"},
		&model.User{ID: 11, Username: "alice"},
	)
	h := newMessageHandler(messages, users)

	body := `{"receiver_id":9,"subject":"hi","body":"hey @alice, check this out"}`
	req := authed(httptest.NewRequest(http.MethodPost, "/api/v1/messages", strings.NewReader(body)), 7, 1)
	w := httptest.NewRecorder()
	h.HandleSendMessage(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
	}

	msg := decodeBody(t, w)["message"].(map[string]interface{})
	mentioned, ok := msg["mentioned_usernames"].([]interface{})
	if !ok {
		t.Fatalf("response has no mentioned_usernames array: %v", msg)
	}
	if len(mentioned) != 1 || mentioned[0] != "alice" {
		t.Errorf("mentioned_usernames = %v, want [\"alice\"]", mentioned)
	}
}

func TestSendMessageDropsUnresolvedMentionInResponse(t *testing.T) {
	messages := newStubMessageRepo()
	users := newStubUserRepo(&model.User{ID: 9, Username: "receiver"})
	h := newMessageHandler(messages, users)

	body := `{"receiver_id":9,"subject":"hi","body":"hey @nosuchuser"}`
	req := authed(httptest.NewRequest(http.MethodPost, "/api/v1/messages", strings.NewReader(body)), 7, 1)
	w := httptest.NewRecorder()
	h.HandleSendMessage(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
	}

	msg := decodeBody(t, w)["message"].(map[string]interface{})
	mentioned, _ := msg["mentioned_usernames"].([]interface{})
	if len(mentioned) != 0 {
		t.Errorf("mentioned_usernames = %v, want empty — the username doesn't exist", mentioned)
	}
}
