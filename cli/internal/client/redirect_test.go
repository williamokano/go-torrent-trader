package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// net/http strips the Authorization header across a redirect only when the target
// *hostname* changes — it ignores the scheme and the port. Both gaps leak a live
// bearer token without anything hostile being involved, so the client refuses
// cross-origin redirects outright.
//
// Each subtest asserts on what the second server actually received, because the
// question is not "did we set a policy" but "did the token arrive somewhere it
// should not have".
func TestARedirectNeverCarriesTheTokenToAnotherOrigin(t *testing.T) {
	const token = "SECRET-TOKEN"

	t.Run("a different port on the same host", func(t *testing.T) {
		var leaked string
		second := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			leaked = r.Header.Get("Authorization")
		}))
		defer second.Close()

		first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, second.URL+"/api/v1/auth/me", http.StatusFound)
		}))
		defer first.Close()

		c, err := NewAuthenticated(first.URL, token)
		if err != nil {
			t.Fatalf("NewAuthenticated: %v", err)
		}
		err = c.Get(context.Background(), "/api/v1/auth/me", nil)

		if leaked != "" {
			t.Errorf("the redirect target received %q — any co-tenant process "+
				"listening on another port of the site's host collects a live token", leaked)
		}
		if !errors.Is(err, ErrRedirect) {
			t.Errorf("err = %v, want ErrRedirect", err)
		}
	})

	t.Run("an https to http downgrade on the same host", func(t *testing.T) {
		var leaked string
		plaintext := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			leaked = r.Header.Get("Authorization")
		}))
		defer plaintext.Close()

		tls := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, plaintext.URL+"/api/v1/auth/me", http.StatusFound)
		}))
		defer tls.Close()

		c, err := NewAuthenticated(tls.URL, token, WithHTTPClient(tls.Client()))
		if err != nil {
			t.Fatalf("NewAuthenticated: %v", err)
		}
		err = c.Get(context.Background(), "/api/v1/auth/me", nil)

		// This is what a reverse proxy missing X-Forwarded-Proto produces, so it
		// needs no attacker at all — just a routine misconfiguration.
		if leaked != "" {
			t.Errorf("the plaintext server received %q — the token went over the wire "+
				"in the clear", leaked)
		}
		if !errors.Is(err, ErrRedirect) {
			t.Errorf("err = %v, want ErrRedirect", err)
		}
	})

	// WithHTTPClient must not be able to reinstate the default policy, since every
	// test and any future caller supplying a client would silently opt out.
	t.Run("an injected http client cannot opt out", func(t *testing.T) {
		var leaked string
		second := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			leaked = r.Header.Get("Authorization")
		}))
		defer second.Close()

		first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, second.URL+"/api/v1/auth/me", http.StatusFound)
		}))
		defer first.Close()

		c, err := NewAuthenticated(first.URL, token, WithHTTPClient(&http.Client{}))
		if err != nil {
			t.Fatalf("NewAuthenticated: %v", err)
		}
		_ = c.Get(context.Background(), "/api/v1/auth/me", nil)

		if leaked != "" {
			t.Errorf("an injected client followed the redirect and leaked %q", leaked)
		}
	})
}

// The refusal must not be reported as a network failure. A cron wrapper written
// against the documented exit codes retries a network error; the site here is up
// and answering, so retrying never succeeds and never pages anyone either.
func TestARefusedRedirectIsNotANetworkError(t *testing.T) {
	second := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer second.Close()
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, second.URL, http.StatusFound)
	}))
	defer first.Close()

	c, err := New(first.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	err = c.Get(context.Background(), "/api/v1/auth/me", nil)
	if !errors.Is(err, ErrRedirect) {
		t.Fatalf("err = %v, want ErrRedirect", err)
	}
	if !strings.Contains(err.Error(), "different origin") {
		t.Errorf("error = %q, want it to say which origins were involved", err)
	}
}

// A same-origin redirect is ordinary — a trailing-slash normalisation, or moving
// an endpoint — and must still be followed, or the refusal above would be
// indistinguishable from breaking redirects altogether.
func TestASameOriginRedirectIsStillFollowed(t *testing.T) {
	var reached string
	var token string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/auth/me" {
			http.Redirect(w, r, "/api/v1/auth/whoami", http.StatusFound)
			return
		}
		reached = r.URL.Path
		token = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c, err := NewAuthenticated(srv.URL, "SECRET-TOKEN")
	if err != nil {
		t.Fatalf("NewAuthenticated: %v", err)
	}
	if err := c.Get(context.Background(), "/api/v1/auth/me", nil); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if reached != "/api/v1/auth/whoami" {
		t.Errorf("reached %q, want the redirect to have been followed", reached)
	}
	if token != "Bearer SECRET-TOKEN" {
		t.Errorf("Authorization = %q, want the token to survive a same-origin redirect", token)
	}
}

// https://host and https://host:443 are one origin, not two, or a site that
// redirects to its own explicit port would be refused for no reason.
func TestSameOriginTreatsADefaultPortAsEqual(t *testing.T) {
	for _, tc := range []struct {
		name  string
		a, b  string
		equal bool
	}{
		{name: "https default port spelled out", a: "https://site", b: "https://site:443", equal: true},
		{name: "http default port spelled out", a: "http://site", b: "http://site:80", equal: true},
		{name: "hostname case differs", a: "https://Site", b: "https://site", equal: true},
		{name: "different port", a: "https://site", b: "https://site:8443", equal: false},
		{name: "scheme downgrade", a: "https://site", b: "http://site", equal: false},
		{name: "different host", a: "https://site", b: "https://other", equal: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := mustParse(t, tc.a)
			b := mustParse(t, tc.b)
			if got := sameOrigin(a, b); got != tc.equal {
				t.Errorf("sameOrigin(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.equal)
			}
		})
	}
}

func mustParse(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parsing %q: %v", raw, err)
	}
	return u
}
