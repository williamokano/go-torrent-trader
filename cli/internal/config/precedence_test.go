package config

import (
	"errors"
	"strings"
	"testing"
)

// The documented precedence puts flag and environment above credentials.yaml.
// Neither existing case exercised both at once, so reversing the order in
// Resolve left the whole suite green — exactly the mutation-survivable gap
// tasks/lessons.md warns about. This is also the case CI depends on: an image
// bakes a credentials.yaml and the job exports TT_TOKEN for its own identity.
func TestResolveTokenPrecedenceWithAStoredCredentialPresent(t *testing.T) {
	file := &File{
		CurrentProfile: "prod",
		Profiles:       map[string]Profile{"prod": {URL: "https://a.example.com"}},
	}

	tests := []struct {
		name     string
		envToken string
		override Override
		want     string
	}{
		{name: "environment beats the stored credential", envToken: "env-token", want: "env-token"},
		{name: "flag beats the stored credential", override: Override{Token: "flag-token"}, want: "flag-token"},
		{name: "flag beats the environment and the stored credential", envToken: "env-token", override: Override{Token: "flag-token"}, want: "flag-token"},
		{name: "the stored credential is the fallback", want: "stored-token"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			isolate(t)
			if err := StoreCredential("prod", "stored-token"); err != nil {
				t.Fatalf("StoreCredential() error = %v", err)
			}
			t.Setenv(EnvToken, tc.envToken)

			got, err := Resolve(file, tc.override)
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}
			if got.Token != tc.want {
				t.Errorf("Token = %q, want %q", got.Token, tc.want)
			}
		})
	}
}

// A token pasted with whitespace, or read from a file with a trailing newline,
// would otherwise pass the empty-token guard and then produce an opaque
// "invalid header field value" from net/http.
func TestResolveTrimsTokenWhitespace(t *testing.T) {
	file := &File{
		CurrentProfile: "prod",
		Profiles:       map[string]Profile{"prod": {URL: "https://a.example.com"}},
	}

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "trailing newline", in: "token-abc\n", want: "token-abc"},
		{name: "surrounding spaces", in: "  token-abc  ", want: "token-abc"},
		{name: "whitespace only is no token at all", in: "   ", want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			isolate(t)
			t.Setenv(EnvToken, tc.in)

			got, err := Resolve(file, Override{})
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}
			if got.Token != tc.want {
				t.Errorf("Token = %q, want %q", got.Token, tc.want)
			}
		})
	}
}

// A stored token must not follow a redirected --url to an unrelated host.
func TestResolveWithholdsAStoredTokenFromAnotherHost(t *testing.T) {
	isolate(t)
	if err := StoreCredential("prod", "stored-token"); err != nil {
		t.Fatalf("StoreCredential() error = %v", err)
	}
	file := &File{
		CurrentProfile: "prod",
		Profiles:       map[string]Profile{"prod": {URL: "https://tracker.example.com"}},
	}

	_, err := Resolve(file, Override{URL: "https://attacker.example.net"})
	if !errors.Is(err, ErrTokenHostMismatch) {
		t.Fatalf("Resolve() error = %v, want ErrTokenHostMismatch", err)
	}
	for _, want := range []string{"tracker.example.com", "attacker.example.net", "--token"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

// With no stored token there is nothing to withhold, so overriding the URL is
// ordinary usage and must not be turned into a mismatch error.
func TestResolveAllowsAnotherHostWhenNothingIsStored(t *testing.T) {
	isolate(t)
	file := &File{
		CurrentProfile: "prod",
		Profiles:       map[string]Profile{"prod": {URL: "https://tracker.example.com"}},
	}

	got, err := Resolve(file, Override{URL: "https://other.example.net"})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got.URL != "https://other.example.net" {
		t.Errorf("URL = %q, want the override", got.URL)
	}
}

// An explicit token means the caller chose the destination.
func TestResolveSendsAnExplicitTokenAnywhere(t *testing.T) {
	isolate(t)
	if err := StoreCredential("prod", "stored-token"); err != nil {
		t.Fatalf("StoreCredential() error = %v", err)
	}
	file := &File{
		CurrentProfile: "prod",
		Profiles:       map[string]Profile{"prod": {URL: "https://tracker.example.com"}},
	}

	got, err := Resolve(file, Override{URL: "https://other.example.net", Token: "explicit"})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got.Token != "explicit" {
		t.Errorf("Token = %q, want the explicit token", got.Token)
	}
}

// ProfileName is the single resolution point; every caller must agree.
func TestProfileName(t *testing.T) {
	tests := []struct {
		name     string
		override string
		env      string
		current  string
		want     string
	}{
		{name: "override wins", override: "from-flag", env: "from-env", current: "cur", want: "from-flag"},
		{name: "environment beats current", env: "from-env", current: "cur", want: "from-env"},
		{name: "current is the fallback", current: "cur", want: "cur"},
		{name: "default when nothing is set", want: DefaultProfileName},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			isolate(t)
			t.Setenv(EnvProfile, tc.env)

			if got := ProfileName(&File{CurrentProfile: tc.current}, tc.override); got != tc.want {
				t.Errorf("ProfileName() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestProfileNameToleratesANilFile(t *testing.T) {
	isolate(t)
	if got := ProfileName(nil, ""); got != DefaultProfileName {
		t.Errorf("ProfileName(nil) = %q, want %q", got, DefaultProfileName)
	}
}
