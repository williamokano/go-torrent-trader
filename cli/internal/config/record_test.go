package config

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestStoreAndLoadCredentialRecord(t *testing.T) {
	isolate(t)

	want := Credential{
		Token:        "access-1",
		RefreshToken: "refresh-1",
		ExpiresAt:    time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC),
	}
	if err := StoreCredentialRecord("prod", want); err != nil {
		t.Fatalf("StoreCredentialRecord() error = %v", err)
	}

	got, err := LoadCredentialRecord("prod")
	if err != nil {
		t.Fatalf("LoadCredentialRecord() error = %v", err)
	}
	if got.Token != want.Token || got.RefreshToken != want.RefreshToken {
		t.Errorf("record = %+v, want %+v", got, want)
	}
	if !got.ExpiresAt.Equal(want.ExpiresAt) {
		t.Errorf("ExpiresAt = %v, want %v", got.ExpiresAt, want.ExpiresAt)
	}
}

// A file written by `tt auth set-token` has only a token. It must keep working
// unchanged — that is the shape a scoped API key will have too.
func TestATokenOnlyRecordStaysTokenOnly(t *testing.T) {
	isolate(t)

	if err := StoreCredential("prod", "just-a-key"); err != nil {
		t.Fatalf("StoreCredential() error = %v", err)
	}

	got, err := LoadCredentialRecord("prod")
	if err != nil {
		t.Fatalf("LoadCredentialRecord() error = %v", err)
	}
	if got.Token != "just-a-key" {
		t.Errorf("Token = %q, want just-a-key", got.Token)
	}
	if got.CanRefresh() {
		t.Error("a pasted key reports that it can refresh, which would cost a pointless round trip")
	}
	if !got.ExpiresAt.IsZero() {
		t.Errorf("ExpiresAt = %v, want zero for a pasted key", got.ExpiresAt)
	}
}

// set-token must replace a login's record wholesale, not leave the old refresh
// token behind attached to an unrelated new token.
func TestStoreCredentialClearsAPreviousRefreshToken(t *testing.T) {
	isolate(t)

	if err := StoreCredentialRecord("prod", Credential{
		Token:        "access-1",
		RefreshToken: "refresh-1",
		ExpiresAt:    time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("StoreCredentialRecord() error = %v", err)
	}
	if err := StoreCredential("prod", "pasted-key"); err != nil {
		t.Fatalf("StoreCredential() error = %v", err)
	}

	got, err := LoadCredentialRecord("prod")
	if err != nil {
		t.Fatalf("LoadCredentialRecord() error = %v", err)
	}
	if got.RefreshToken != "" {
		t.Errorf("RefreshToken = %q, want it cleared with the replaced credential", got.RefreshToken)
	}
	if !got.ExpiresAt.IsZero() {
		t.Errorf("ExpiresAt = %v, want it cleared", got.ExpiresAt)
	}
}

func TestCanRefresh(t *testing.T) {
	if (Credential{Token: "t"}).CanRefresh() {
		t.Error("a credential with no refresh token reports it can refresh")
	}
	if !(Credential{Token: "t", RefreshToken: "r"}).CanRefresh() {
		t.Error("a credential with a refresh token reports it cannot refresh")
	}
}

func TestExpiresWithin(t *testing.T) {
	skew := 30 * time.Second

	tests := []struct {
		name string
		in   Credential
		want bool
	}{
		{
			name: "unknown expiry is not treated as expired",
			in:   Credential{},
			want: false,
		},
		{
			name: "comfortably valid",
			in:   Credential{ExpiresAt: time.Now().Add(time.Hour)},
			want: false,
		},
		{
			name: "already past",
			in:   Credential{ExpiresAt: time.Now().Add(-time.Minute)},
			want: true,
		},
		{
			// Inside the skew: the request would die in flight, so renew first.
			name: "about to expire",
			in:   Credential{ExpiresAt: time.Now().Add(5 * time.Second)},
			want: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.in.ExpiresWithin(skew); got != tc.want {
				t.Errorf("ExpiresWithin() = %v, want %v", got, tc.want)
			}
		})
	}
}

// A zero expiry must never be read as "expired in 1970", which would refresh on
// every single invocation.
func TestZeroExpiryIsNotTreatedAsExpired(t *testing.T) {
	if (Credential{Token: "t", RefreshToken: "r"}).ExpiresWithin(time.Hour) {
		t.Error("a zero ExpiresAt was treated as expired")
	}
}

func TestResolveSiteUsesTheProfileURL(t *testing.T) {
	isolate(t)
	f := &File{Profiles: map[string]Profile{"prod": {URL: "https://tracker.example.com/"}}}

	got, err := ResolveSite(f, "prod", "")
	if err != nil {
		t.Fatalf("ResolveSite() error = %v", err)
	}
	if got.URL != "https://tracker.example.com" {
		t.Errorf("URL = %q, want the profile URL without a trailing slash", got.URL)
	}
	if got.Profile != "prod" {
		t.Errorf("Profile = %q, want prod", got.Profile)
	}
}

func TestResolveSitePrefersTheOverrideThenEnvironment(t *testing.T) {
	f := &File{Profiles: map[string]Profile{"prod": {URL: "https://from-profile.example.com"}}}

	t.Run("flag wins", func(t *testing.T) {
		isolate(t)
		t.Setenv(EnvURL, "https://from-env.example.com")
		got, err := ResolveSite(f, "prod", "https://from-flag.example.com")
		if err != nil {
			t.Fatalf("ResolveSite() error = %v", err)
		}
		if got.URL != "https://from-flag.example.com" {
			t.Errorf("URL = %q, want the flag", got.URL)
		}
	})

	t.Run("environment beats the profile", func(t *testing.T) {
		isolate(t)
		t.Setenv(EnvURL, "https://from-env.example.com")
		got, err := ResolveSite(f, "prod", "")
		if err != nil {
			t.Fatalf("ResolveSite() error = %v", err)
		}
		if got.URL != "https://from-env.example.com" {
			t.Errorf("URL = %q, want the environment", got.URL)
		}
	})
}

// Logging in to a profile that has no URL yet must say what to do, not fail
// obscurely on an empty request.
func TestResolveSiteRequiresAURL(t *testing.T) {
	isolate(t)
	f := &File{Profiles: map[string]Profile{}}

	_, err := ResolveSite(f, "prod", "")
	if !errors.Is(err, ErrNoURL) {
		t.Fatalf("ResolveSite() error = %v, want ErrNoURL", err)
	}
	if !strings.Contains(err.Error(), "tt profile set") {
		t.Errorf("error = %q, want it to say how to set a URL", err)
	}
}

// Resolve must report where the token came from: only a stored one may be
// refreshed and rewritten.
func TestResolveReportsWhetherTheTokenCameFromTheStore(t *testing.T) {
	f := &File{
		CurrentProfile: "prod",
		Profiles:       map[string]Profile{"prod": {URL: "https://tracker.example.com"}},
	}

	t.Run("stored", func(t *testing.T) {
		isolate(t)
		if err := StoreCredential("prod", "stored"); err != nil {
			t.Fatalf("StoreCredential() error = %v", err)
		}
		got, err := Resolve(f, Override{})
		if err != nil {
			t.Fatalf("Resolve() error = %v", err)
		}
		if !got.FromStore {
			t.Error("FromStore = false for a credential read from the file")
		}
	})

	t.Run("explicit flag", func(t *testing.T) {
		isolate(t)
		if err := StoreCredential("prod", "stored"); err != nil {
			t.Fatalf("StoreCredential() error = %v", err)
		}
		got, err := Resolve(f, Override{Token: "explicit"})
		if err != nil {
			t.Fatalf("Resolve() error = %v", err)
		}
		if got.FromStore {
			t.Error("FromStore = true for a caller-supplied token, which must not be rewritten")
		}
	})

	t.Run("environment", func(t *testing.T) {
		isolate(t)
		t.Setenv(EnvToken, "from-env")
		got, err := Resolve(f, Override{})
		if err != nil {
			t.Fatalf("Resolve() error = %v", err)
		}
		if got.FromStore {
			t.Error("FromStore = true for an environment token")
		}
	})

	t.Run("nothing stored", func(t *testing.T) {
		isolate(t)
		got, err := Resolve(f, Override{})
		if err != nil {
			t.Fatalf("Resolve() error = %v", err)
		}
		if got.FromStore {
			t.Error("FromStore = true when there is no credential at all")
		}
	})
}
