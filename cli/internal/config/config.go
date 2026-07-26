// Package config resolves which site the CLI talks to and with what credential.
//
// Two files are kept, deliberately separated:
//
//   - config.yaml      profile definitions (site URLs). Not secret; safe to commit
//     to a dotfiles repo or bake into a CI image.
//   - credentials.yaml tokens, one per profile, written 0600.
//
// Keeping them apart means "share my profiles" never means "share my tokens",
// and it lets a CI runner mount a config while supplying its token through the
// environment instead.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// DefaultProfileName is the profile used when none is selected.
const DefaultProfileName = "default"

// Environment variables that override resolved configuration.
const (
	EnvConfigDir = "TT_CONFIG_DIR"
	EnvProfile   = "TT_PROFILE"
	EnvURL       = "TT_URL"
	EnvToken     = "TT_TOKEN"
)

// ErrNoProfile reports that the requested profile is not defined.
var ErrNoProfile = errors.New("profile not defined")

// ErrNoURL reports that a site URL could not be resolved from any source.
var ErrNoURL = errors.New("no site URL configured")

// ErrTokenHostMismatch reports that a stored token was withheld because the
// effective URL is not the site the token belongs to.
var ErrTokenHostMismatch = errors.New("stored token belongs to a different site")

// Profile is a single named site the CLI can talk to.
type Profile struct {
	URL string `yaml:"url"`
}

// File is the on-disk config document (config.yaml).
type File struct {
	CurrentProfile string             `yaml:"current_profile,omitempty"`
	Profiles       map[string]Profile `yaml:"profiles,omitempty"`
}

// Dir returns the directory holding the CLI's configuration, honouring
// TT_CONFIG_DIR and then XDG_CONFIG_HOME before falling back to ~/.config.
func Dir() (string, error) {
	if dir := os.Getenv(EnvConfigDir); dir != "" {
		return dir, nil
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "go-torrent-trader"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locating home directory: %w", err)
	}
	return filepath.Join(home, ".config", "go-torrent-trader"), nil
}

// ConfigPath returns the path of the profile document.
func ConfigPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

// Load reads the profile document. A missing file is not an error — it yields an
// empty document, so a fresh install behaves the same as an empty one.
func Load() (*File, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &File{Profiles: map[string]Profile{}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var f File
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if f.Profiles == nil {
		f.Profiles = map[string]Profile{}
	}
	return &f, nil
}

// Save writes the profile document, creating the config directory if needed.
func (f *File) Save() error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	out, err := yaml.Marshal(f)
	if err != nil {
		return fmt.Errorf("encoding config: %w", err)
	}
	return writeFilePrivate(path, out)
}

// ProfileName resolves which profile an invocation is talking about.
//
// Every caller must go through this. Resolving the name in more than one place
// is how `tt auth set-token` came to ignore TT_PROFILE while everything else
// honoured it — the command reported success against "default" while the rest of
// the CLI was using the profile the environment had selected.
func ProfileName(f *File, override string) string {
	current := ""
	if f != nil {
		current = f.CurrentProfile
	}
	return firstNonEmpty(override, os.Getenv(EnvProfile), current, DefaultProfileName)
}

// Resolved is the effective configuration for one invocation.
type Resolved struct {
	// Profile is the name the settings came from. It is reported in errors so a
	// confusing 401 can be traced to the profile that produced it.
	Profile string
	URL     string
	// Token is the bearer credential, or empty when the command does not need
	// one. Commands requiring auth must reject empty rather than sending an
	// anonymous request — see client.ErrNoCredentials.
	Token string
}

// Override carries values supplied by flags, which win over everything else.
type Override struct {
	Profile string
	URL     string
	Token   string
}

// Resolve computes the effective configuration.
//
// Precedence is flag, then environment, then the profile in config.yaml, then
// credentials.yaml for the token. This ordering is what lets CI export TT_TOKEN
// over a checked-in config without editing files.
func Resolve(f *File, o Override) (Resolved, error) {
	name := ProfileName(f, o.Profile)

	profile, defined := f.Profiles[name]
	// An explicitly requested profile that does not exist is a mistake worth
	// reporting. An implicit fallback to "default" is not: a user who only ever
	// passes --url or exports TT_URL should never have to create a profile.
	if !defined && (o.Profile != "" || os.Getenv(EnvProfile) != "") {
		return Resolved{}, fmt.Errorf("%w: %q", ErrNoProfile, name)
	}

	profileURL := normalizeURL(profile.URL)
	url := normalizeURL(firstNonEmpty(o.URL, os.Getenv(EnvURL), profile.URL))
	if url == "" {
		return Resolved{}, fmt.Errorf("%w: set --url, export %s, or run 'tt profile set %s --url ...'", ErrNoURL, EnvURL, name)
	}

	// A token supplied for this invocation is used as-is: the caller said where
	// it should go.
	token := strings.TrimSpace(firstNonEmpty(o.Token, os.Getenv(EnvToken)))
	if token == "" {
		stored, err := LoadCredential(name)
		if err != nil {
			return Resolved{}, err
		}
		// A stored token belongs to the site it was stored for. Sending it
		// anywhere else — a typo in --url, a copy-pasted command, a hostile
		// value in TT_URL — hands the credential to whoever answers that name.
		if stored != "" && url != profileURL {
			return Resolved{}, fmt.Errorf(
				"%w: profile %q holds a token for %s, but this command targets %s. Pass --token or set %s to authenticate against it",
				ErrTokenHostMismatch, name, profileURL, url, EnvToken)
		}
		token = stored
	}

	return Resolved{Profile: name, URL: url, Token: token}, nil
}

// normalizeURL trims a trailing slash so that a profile stored with one and a
// flag passed without one compare equal.
func normalizeURL(u string) string {
	return strings.TrimRight(strings.TrimSpace(u), "/")
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
