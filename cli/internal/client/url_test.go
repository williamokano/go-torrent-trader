package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A base URL is a site root. Concatenating a path onto one that carries a query
// or fragment silently sends every request to "/" — and a real site answers "/"
// with its SPA shell and a 200, so the only symptom is a confusing decode error.
func TestNewRejectsABaseURLThatIsNotASiteRoot(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		wantMsg string
	}{
		{name: "query string", baseURL: "https://tracker.example.com/?utm=x", wantMsg: "query"},
		{name: "bare question mark", baseURL: "https://tracker.example.com/?", wantMsg: "query"},
		{name: "fragment", baseURL: "https://tracker.example.com/#section", wantMsg: "fragment"},
		{name: "embedded credentials", baseURL: "https://user:pass@tracker.example.com", wantMsg: "credentials"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := New(tc.baseURL)
			if err == nil {
				t.Fatalf("New(%q) succeeded, want an error", tc.baseURL)
			}
			if !strings.Contains(err.Error(), tc.wantMsg) {
				t.Errorf("error = %q, want it to mention %q", err, tc.wantMsg)
			}
		})
	}
}

// A site served from a subdirectory is legitimate and must keep working.
func TestBaseURLWithAPathPrefixIsPreserved(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c, err := New(srv.URL + "/tracker")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := c.Get(context.Background(), "/api/v1/auth/me", nil); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if want := "/tracker/api/v1/auth/me"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}
}
