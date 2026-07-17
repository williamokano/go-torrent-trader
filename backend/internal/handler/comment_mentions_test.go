package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleCreateComment_IncludesMentionedUsernames(t *testing.T) {
	router, sessions := setupCommentRouter()

	registerUserAndGetToken(t, router, "carol", "carol@example.com", "password123")
	token := createSessionWithGroup(sessions, 1101, 5)

	body, _ := json.Marshal(map[string]string{"body": "great work @carol"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/torrents/1/comments", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	comment := resp["comment"].(map[string]interface{})
	mentioned, ok := comment["mentioned_usernames"].([]interface{})
	if !ok || len(mentioned) != 1 || mentioned[0] != "carol" {
		t.Errorf("mentioned_usernames = %v, want [carol]", comment["mentioned_usernames"])
	}
}

func TestHandleCreateComment_UnknownMentionOmitted(t *testing.T) {
	router, sessions := setupCommentRouter()
	token := createSessionWithGroup(sessions, 1102, 5)

	body, _ := json.Marshal(map[string]string{"body": "hey @totallyfakeuser"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/torrents/1/comments", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	comment := resp["comment"].(map[string]interface{})
	if mentioned, ok := comment["mentioned_usernames"].([]interface{}); ok && len(mentioned) != 0 {
		t.Errorf("mentioned_usernames = %v, want empty — the username doesn't exist", mentioned)
	}
}
