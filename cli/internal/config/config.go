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
	if err := refuseIfOthersCanWrite(path); err != nil {
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

// ErrConfigWritable reports that config.yaml, or the directory holding it, can be
// modified by someone other than its owner.
var ErrConfigWritable = errors.New("config file is writable by other users")

// refuseIfOthersCanWrite rejects a profile document that group or others can
// write, and the directory that holds it.
//
// ensureConfigDir already tightens the directory, but only on the paths that
// write — so an operator who only ever runs `tt whoami` never triggers it, and
// that is precisely the case where it matters. Anyone who can write config.yaml
// can repoint a profile's URL at a host they control and collect the bearer token
// on the next command, which the credential file's own 0600 does nothing to
// prevent: the token is read correctly and then sent to the wrong place.
//
// Checked here rather than by binding each stored token to a URL recorded beside
// it, because the URL is not the only thing worth protecting in that file, and a
// writable config is a problem whether or not a token happens to be stored.
//
// Only the write bits are tested. Unlike credentials.yaml this file holds no
// secret, so being readable is not a finding — mirroring the credential check
// would reject the perfectly ordinary 0644.
func refuseIfOthersCanWrite(path string) error {
	if !modeChecksSupported {
		return nil
	}
	for _, target := range []string{filepath.Dir(path), path} {
		info, err := os.Stat(target)
		if errors.Is(err, os.ErrNotExist) {
			continue // a fresh install has neither; Save creates both at 0700/0600
		}
		if err != nil {
			return fmt.Errorf("inspecting %s: %w", target, err)
		}
		if mode := info.Mode().Perm(); mode&0o022 != 0 {
			return fmt.Errorf("%w: %s is %#o, so another user could repoint a profile at "+
				"a host they control and collect your token. Run 'chmod go-w %s'",
				ErrConfigWritable, target, mode, target)
		}
	}
	return nil
}

// Save writes the profile document, creating the config directory if needed.
//
// Locked for the same reason StoreCredential is: callers do load-mutate-save, and
// without a lock two concurrent `tt profile set` invocations both read the old
// document and the second rename wins — so a profile vanishes while both commands
// report success. That was fixed for tokens and left unfixed for profiles, which
// is the more likely race, since provisioning scripts add several profiles at once.
//
// The lock covers only this write. A caller that loaded before taking it can still
// overwrite a change made in between; making that impossible needs the whole
// load-mutate-save inside the lock, which is MutateSaved below.
func (f *File) Save() error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	if err := ensureConfigDir(filepath.Dir(path)); err != nil {
		return err
	}
	release, err := acquireLock(path + ".lock")
	if err != nil {
		return err
	}
	defer release()

	return f.write(path)
}

func (f *File) write(path string) error {
	out, err := yaml.Marshal(f)
	if err != nil {
		return fmt.Errorf("encoding config: %w", err)
	}
	return writeFilePrivate(path, out)
}

// MutateSaved applies a change to the profile document with the read and the
// write under one lock, so concurrent callers serialise instead of losing each
// other's edits.
//
// This is what `tt profile set` and `tt profile remove` need: Save alone still
// writes a document that was read before the lock was held.
//
// The mutator may fail, so a command that has to check the document before
// changing it — "does this profile exist?" — can do that check against the same
// read the write is based on, and nothing is written when it returns an error.
func MutateSaved(mutate func(*File) error) error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	if err := ensureConfigDir(filepath.Dir(path)); err != nil {
		return err
	}
	release, err := acquireLock(path + ".lock")
	if err != nil {
		return err
	}
	defer release()

	f, err := Load()
	if err != nil {
		return err
	}
	if err := mutate(f); err != nil {
		return err
	}
	return f.write(path)
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
	// FromStore reports that Token came from credentials.yaml rather than a flag
	// or the environment. Only a stored credential may be refreshed and
	// rewritten: a token the caller passed in is theirs, and replacing it with a
	// renewed session would be a surprise.
	FromStore bool
}

// ResolveSite resolves only the URL for a named profile.
//
// Commands that establish a credential — login, logout — need to know where to
// send the request but must not fail because no credential exists yet.
func ResolveSite(f *File, name, urlOverride string) (Resolved, error) {
	profile := f.Profiles[name]
	url := normalizeURL(firstNonEmpty(urlOverride, os.Getenv(EnvURL), profile.URL))
	if url == "" {
		return Resolved{}, fmt.Errorf("%w: set --url, export %s, or run 'tt profile set %s --url ...'", ErrNoURL, EnvURL, name)
	}
	return Resolved{Profile: name, URL: url}, nil
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
	fromStore := false
	if token == "" {
		stored, err := LoadCredential(name)
		if err != nil {
			return Resolved{}, err
		}
		fromStore = stored != ""
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

	return Resolved{Profile: name, URL: url, Token: token, FromStore: fromStore}, nil
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
