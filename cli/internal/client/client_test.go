package client

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewRejectsUnusableURLs(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
	}{
		{name: "empty", baseURL: ""},
		{name: "no scheme", baseURL: "tracker.example.com"},
		{name: "wrong scheme", baseURL: "ftp://tracker.example.com"},
		{name: "no host", baseURL: "https://"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := New(tc.baseURL); err == nil {
				t.Errorf("New(%q) succeeded, want an error", tc.baseURL)
			}
		})
	}
}

// A missing credential must stop the request from being built at all. Sending it
// anonymously would return public data for some endpoints, turning a
// configuration mistake into a quietly wrong answer.
func TestNewAuthenticatedRefusesAnEmptyToken(t *testing.T) {
	_, err := NewAuthenticated("https://tracker.example.com", "")
	if !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("NewAuthenticated() error = %v, want ErrNoCredentials", err)
	}
}

func TestDoSendsBearerTokenAndDecodesResponse(t *testing.T) {
	var gotAuth, gotAccept, gotAgent, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotAccept = r.Header.Get("Accept")
		gotAgent = r.Header.Get("User-Agent")
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"user":{"username":"alice"}}`))
	}))
	defer srv.Close()

	c, err := NewAuthenticated(srv.URL, "token-abc", WithUserAgent("tt/test"))
	if err != nil {
		t.Fatalf("NewAuthenticated() error = %v", err)
	}

	var out struct {
		User struct {
			Username string `json:"username"`
		} `json:"user"`
	}
	if err := c.Get(context.Background(), "/api/v1/auth/me", &out); err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if gotAuth != "Bearer token-abc" {
		t.Errorf("Authorization = %q, want Bearer token-abc", gotAuth)
	}
	if gotAccept != "application/json" {
		t.Errorf("Accept = %q, want application/json", gotAccept)
	}
	if gotAgent != "tt/test" {
		t.Errorf("User-Agent = %q, want tt/test", gotAgent)
	}
	if gotPath != "/api/v1/auth/me" {
		t.Errorf("path = %q, want /api/v1/auth/me", gotPath)
	}
	if out.User.Username != "alice" {
		t.Errorf("username = %q, want alice", out.User.Username)
	}
}

// An anonymous client must not invent an Authorization header, or a public
// endpoint would start behaving as though someone were signed in.
func TestDoOmitsAuthorizationWhenAnonymous(t *testing.T) {
	var hadAuth bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, hadAuth = r.Header["Authorization"]
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c, err := New(srv.URL)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := c.Get(context.Background(), "/api/v1/health", nil); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if hadAuth {
		t.Error("anonymous client sent an Authorization header")
	}
}

func TestDoEncodesRequestBody(t *testing.T) {
	var gotBody, gotType, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// io.ReadAll, not a single Read: one Read may return fewer bytes and the
		// assertion would then be checking a zero-padded prefix.
		buf, _ := io.ReadAll(r.Body)
		gotBody = string(buf)
		gotType = r.Header.Get("Content-Type")
		gotMethod = r.Method
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c, err := New(srv.URL)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	body := map[string]string{"name": "value"}
	if err := c.Do(context.Background(), http.MethodPost, "/api/v1/thing", body, nil); err != nil {
		t.Fatalf("Do() error = %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotType)
	}
	if gotBody != `{"name":"value"}` {
		t.Errorf("body = %q, want {\"name\":\"value\"}", gotBody)
	}
}

func TestDoMapsAPIErrorEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"code":"forbidden","message":"administrator access required"}}`))
	}))
	defer srv.Close()

	c, err := New(srv.URL)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	err = c.Get(context.Background(), "/api/v1/admin/users", nil)

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("Get() error = %v (%T), want *APIError", err, err)
	}
	if apiErr.StatusCode != http.StatusForbidden {
		t.Errorf("StatusCode = %d, want 403", apiErr.StatusCode)
	}
	if apiErr.Code != "forbidden" {
		t.Errorf("Code = %q, want forbidden", apiErr.Code)
	}
	if apiErr.Message != "administrator access required" {
		t.Errorf("Message = %q, want the API message", apiErr.Message)
	}
	// The rendered error is what the user actually reads.
	if !strings.Contains(apiErr.Error(), "administrator access required") {
		t.Errorf("Error() = %q, want it to contain the API message", apiErr.Error())
	}
}

// A wrong URL usually lands on a proxy that answers with HTML. Quoting a bounded
// prefix is what tells the user they are not talking to the API at all.
func TestDoQuotesANonEnvelopeErrorBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("<html><body>502 Bad Gateway</body></html>"))
	}))
	defer srv.Close()

	c, err := New(srv.URL)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	err = c.Get(context.Background(), "/api/v1/auth/me", nil)

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("Get() error = %v (%T), want *APIError", err, err)
	}
	if !strings.Contains(apiErr.Message, "502 Bad Gateway") {
		t.Errorf("Message = %q, want the raw body quoted", apiErr.Message)
	}
}

func TestDoTruncatesAHugeErrorBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(strings.Repeat("x", 10_000)))
	}))
	defer srv.Close()

	c, err := New(srv.URL)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	err = c.Get(context.Background(), "/api/v1/auth/me", nil)

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("Get() error = %v (%T), want *APIError", err, err)
	}
	if len(apiErr.Message) > maxErrorBodyBytes+len("...") {
		t.Errorf("Message length = %d, want it bounded near %d", len(apiErr.Message), maxErrorBodyBytes)
	}
	if !strings.HasSuffix(apiErr.Message, "...") {
		t.Errorf("Message = %q, want a truncation marker", apiErr.Message[:min(64, len(apiErr.Message))])
	}
}

// An empty body with a failing status must still produce a usable message rather
// than an empty one.
func TestDoHandlesAnEmptyErrorBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c, err := New(srv.URL)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	err = c.Get(context.Background(), "/api/v1/auth/me", nil)

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("Get() error = %v (%T), want *APIError", err, err)
	}
	if !strings.Contains(apiErr.Error(), "401") {
		t.Errorf("Error() = %q, want the status code", apiErr.Error())
	}
}

func TestDoReportsADecodeFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()

	c, err := New(srv.URL)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	var out struct {
		Field string `json:"field"`
	}
	err = c.Get(context.Background(), "/api/v1/auth/me", &out)
	if err == nil {
		t.Fatal("Get() succeeded on a non-JSON 200, want an error")
	}
	if !strings.Contains(err.Error(), "decoding") {
		t.Errorf("error = %q, want it to say decoding failed", err)
	}
}

func TestDoHonoursContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c, err := New(srv.URL)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := c.Get(ctx, "/api/v1/auth/me", nil); !errors.Is(err, context.Canceled) {
		t.Errorf("Get() error = %v, want context.Canceled", err)
	}
}

// A trailing slash on the configured URL must not produce a doubled path.
func TestNewTrimsATrailingSlash(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c, err := New(srv.URL + "/")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := c.Get(context.Background(), "/api/v1/health", nil); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if gotPath != "/api/v1/health" {
		t.Errorf("path = %q, want /api/v1/health", gotPath)
	}
}
