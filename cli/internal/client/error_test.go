package client

import (
	"net/http"
	"strings"
	"testing"
)

// The rendered message is what the user reads, so each shape of APIError has to
// produce something meaningful rather than an empty string.
func TestAPIErrorMessages(t *testing.T) {
	tests := []struct {
		name  string
		err   APIError
		wants []string
	}{
		{
			name:  "code and message",
			err:   APIError{StatusCode: 403, Code: "forbidden", Message: "administrator access required"},
			wants: []string{"administrator access required", "403", "forbidden"},
		},
		{
			name:  "message only",
			err:   APIError{StatusCode: 502, Message: "502 Bad Gateway"},
			wants: []string{"502 Bad Gateway", "502"},
		},
		{
			name:  "neither, so fall back to the status text",
			err:   APIError{StatusCode: http.StatusUnauthorized},
			wants: []string{"401", "Unauthorized"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.err.Error()
			if got == "" {
				t.Fatal("Error() returned an empty string")
			}
			for _, want := range tc.wants {
				if !strings.Contains(got, want) {
					t.Errorf("Error() = %q, want it to contain %q", got, want)
				}
			}
		})
	}
}

func TestWithHTTPClientReplacesTheTransport(t *testing.T) {
	custom := &http.Client{}
	c, err := New("https://tracker.example.com", WithHTTPClient(custom))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if c.http != custom {
		t.Error("WithHTTPClient did not install the supplied client")
	}
}

func TestNewDefaultsTheUserAgentAndTimeout(t *testing.T) {
	c, err := New("https://tracker.example.com")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if c.userAgent == "" {
		t.Error("userAgent is empty; CLI traffic would be indistinguishable in access logs")
	}
	if c.http.Timeout != DefaultTimeout {
		t.Errorf("timeout = %v, want %v", c.http.Timeout, DefaultTimeout)
	}
}
