package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// isolate points the config package at a scratch directory so tests never read
// or write the developer's real configuration.
func isolate(t *testing.T) string {
	t.Helper()
	dir := privateTempDir(t)
	t.Setenv(EnvConfigDir, dir)
	// Resolve consults these; clear them so a developer's exported values do not
	// leak into the expectations below.
	t.Setenv(EnvProfile, "")
	t.Setenv(EnvURL, "")
	t.Setenv(EnvToken, "")
	return dir
}

// privateTempDir is t.TempDir() at the mode a real install has.
//
// t.TempDir() inherits the umask, so on a developer or CI box with umask 002 it
// is 0775 — group-writable. A real config directory is 0700, because
// ensureConfigDir creates it with an explicit MkdirAll(0700). Load refuses a
// group-writable directory, so leaving the test fixture loose would either fail
// every test or, worse, tempt someone to weaken the check to match a mode no
// install actually has.
func privateTempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("tightening temp config dir: %v", err)
	}
	return dir
}

func TestDirPrefersConfigDirEnv(t *testing.T) {
	t.Setenv(EnvConfigDir, "/explicit")
	t.Setenv("XDG_CONFIG_HOME", "/xdg")

	got, err := Dir()
	if err != nil {
		t.Fatalf("Dir() error = %v", err)
	}
	if got != "/explicit" {
		t.Errorf("Dir() = %q, want /explicit", got)
	}
}

func TestDirFallsBackToXDG(t *testing.T) {
	t.Setenv(EnvConfigDir, "")
	t.Setenv("XDG_CONFIG_HOME", "/xdg")

	got, err := Dir()
	if err != nil {
		t.Fatalf("Dir() error = %v", err)
	}
	if want := filepath.Join("/xdg", "go-torrent-trader"); got != want {
		t.Errorf("Dir() = %q, want %q", got, want)
	}
}

func TestLoadMissingFileIsEmptyNotError(t *testing.T) {
	isolate(t)

	f, err := Load()
	if err != nil {
		t.Fatalf("Load() on a fresh install error = %v", err)
	}
	if len(f.Profiles) != 0 {
		t.Errorf("Profiles = %v, want empty", f.Profiles)
	}
	// A nil map would panic on the first assignment in `tt profile set`.
	if f.Profiles == nil {
		t.Error("Profiles is nil, want an initialised map")
	}
}

func TestSaveThenLoadRoundTrips(t *testing.T) {
	isolate(t)

	f := &File{
		CurrentProfile: "prod",
		Profiles: map[string]Profile{
			"prod": {URL: "https://tracker.example.com"},
		},
	}
	if err := f.Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.CurrentProfile != "prod" {
		t.Errorf("CurrentProfile = %q, want prod", got.CurrentProfile)
	}
	if got.Profiles["prod"].URL != "https://tracker.example.com" {
		t.Errorf("prod URL = %q, want https://tracker.example.com", got.Profiles["prod"].URL)
	}
}

func TestSaveWritesOwnerOnlyMode(t *testing.T) {
	isolate(t)

	f := &File{Profiles: map[string]Profile{"a": {URL: "https://a.example.com"}}}
	if err := f.Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	path, err := ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath() error = %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if mode := info.Mode().Perm(); mode&0o077 != 0 {
		t.Errorf("config.yaml mode = %#o, want no group or other access", mode)
	}
}

func TestResolvePrecedence(t *testing.T) {
	file := &File{
		CurrentProfile: "prod",
		Profiles: map[string]Profile{
			"prod":  {URL: "https://from-profile.example.com"},
			"other": {URL: "https://other.example.com"},
		},
	}

	tests := []struct {
		name      string
		env       map[string]string
		override  Override
		wantURL   string
		wantToken string
		wantName  string
	}{
		{
			name:     "profile file supplies the URL",
			wantURL:  "https://from-profile.example.com",
			wantName: "prod",
		},
		{
			name:     "environment beats the profile file",
			env:      map[string]string{EnvURL: "https://from-env.example.com"},
			wantURL:  "https://from-env.example.com",
			wantName: "prod",
		},
		{
			name:     "flag beats the environment",
			env:      map[string]string{EnvURL: "https://from-env.example.com"},
			override: Override{URL: "https://from-flag.example.com"},
			wantURL:  "https://from-flag.example.com",
			wantName: "prod",
		},
		{
			name:      "token comes from the environment",
			env:       map[string]string{EnvToken: "env-token"},
			wantURL:   "https://from-profile.example.com",
			wantToken: "env-token",
			wantName:  "prod",
		},
		{
			name:      "flag token beats the environment token",
			env:       map[string]string{EnvToken: "env-token"},
			override:  Override{Token: "flag-token"},
			wantURL:   "https://from-profile.example.com",
			wantToken: "flag-token",
			wantName:  "prod",
		},
		{
			name:     "profile flag selects a different profile",
			override: Override{Profile: "other"},
			wantURL:  "https://other.example.com",
			wantName: "other",
		},
		{
			name:     "profile environment variable selects a profile",
			env:      map[string]string{EnvProfile: "other"},
			wantURL:  "https://other.example.com",
			wantName: "other",
		},
		{
			name:     "a trailing slash is trimmed so paths do not double up",
			override: Override{URL: "https://slash.example.com/"},
			wantURL:  "https://slash.example.com",
			wantName: "prod",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			isolate(t)
			for k, v := range tc.env {
				t.Setenv(k, v)
			}

			got, err := Resolve(file, tc.override)
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}
			if got.URL != tc.wantURL {
				t.Errorf("URL = %q, want %q", got.URL, tc.wantURL)
			}
			if got.Token != tc.wantToken {
				t.Errorf("Token = %q, want %q", got.Token, tc.wantToken)
			}
			if got.Profile != tc.wantName {
				t.Errorf("Profile = %q, want %q", got.Profile, tc.wantName)
			}
		})
	}
}

func TestResolveRejectsExplicitUnknownProfile(t *testing.T) {
	isolate(t)
	file := &File{Profiles: map[string]Profile{}}

	if _, err := Resolve(file, Override{Profile: "ghost"}); !errors.Is(err, ErrNoProfile) {
		t.Errorf("Resolve() error = %v, want ErrNoProfile", err)
	}
}

func TestResolveRejectsUnknownProfileFromEnvironment(t *testing.T) {
	isolate(t)
	t.Setenv(EnvProfile, "ghost")
	file := &File{Profiles: map[string]Profile{}}

	if _, err := Resolve(file, Override{}); !errors.Is(err, ErrNoProfile) {
		t.Errorf("Resolve() error = %v, want ErrNoProfile", err)
	}
}

// An implicit fallback to "default" must not demand that the profile exist —
// otherwise `tt --url https://... whoami` would refuse to run on a fresh install.
func TestResolveAllowsImplicitDefaultWithOnlyAURLFlag(t *testing.T) {
	isolate(t)
	file := &File{Profiles: map[string]Profile{}}

	got, err := Resolve(file, Override{URL: "https://only-flag.example.com"})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got.Profile != DefaultProfileName {
		t.Errorf("Profile = %q, want %q", got.Profile, DefaultProfileName)
	}
	if got.URL != "https://only-flag.example.com" {
		t.Errorf("URL = %q, want the flag value", got.URL)
	}
}

func TestResolveRequiresAURL(t *testing.T) {
	isolate(t)
	file := &File{Profiles: map[string]Profile{}}

	_, err := Resolve(file, Override{})
	if !errors.Is(err, ErrNoURL) {
		t.Fatalf("Resolve() error = %v, want ErrNoURL", err)
	}
	// The message has to say what to do about it; a bare "no URL" sends the user
	// looking for a config file they have never seen.
	for _, want := range []string{"--url", EnvURL, "tt profile set"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

// Resolve must fall back to the stored credential when neither flag nor
// environment supplies one, or `tt profile set-token` would have no effect.
func TestResolveFallsBackToStoredCredential(t *testing.T) {
	isolate(t)
	if err := StoreCredential("prod", "stored-token"); err != nil {
		t.Fatalf("StoreCredential() error = %v", err)
	}
	file := &File{
		CurrentProfile: "prod",
		Profiles:       map[string]Profile{"prod": {URL: "https://a.example.com"}},
	}

	got, err := Resolve(file, Override{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got.Token != "stored-token" {
		t.Errorf("Token = %q, want stored-token", got.Token)
	}
}
