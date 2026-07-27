package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// credentialFile is the on-disk credential document (credentials.yaml).
//
// The stored value is an opaque bearer token. The CLI deliberately does not
// know whether it is a session access token or a scoped API key (#170) — it
// forwards it and lets the server decide. That keeps this file unchanged when
// scoped tokens land.
type credentialFile struct {
	Profiles map[string]Credential `yaml:"profiles,omitempty"`
}

// Credential is what is stored for one profile.
//
// Token alone is the whole record for a key pasted in with `tt auth set-token`
// — including a scoped API key once #170 lands, which never needs refreshing.
// The other two fields are only populated by `tt auth login`, so a file written
// by an older version stays readable and a key-only profile stays key-only.
type Credential struct {
	Token string `yaml:"token"`
	// RefreshToken renews Token without re-entering a password. Empty when the
	// credential did not come from a login.
	RefreshToken string `yaml:"refresh_token,omitempty"`
	// ExpiresAt is when Token stops working, used to renew before spending a
	// request discovering it. Zero means unknown, which is treated as "might
	// still be good" — the 401 path is the backstop.
	ExpiresAt time.Time `yaml:"expires_at,omitempty"`
}

// CanRefresh reports whether this credential can renew itself.
func (c Credential) CanRefresh() bool { return c.RefreshToken != "" }

// ExpiresWithin reports whether the access token is already past, or close to,
// its expiry. The skew avoids a request that is certain to 401 in flight.
func (c Credential) ExpiresWithin(skew time.Duration) bool {
	if c.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().Add(skew).After(c.ExpiresAt)
}

// ErrCredentialsTooOpen reports that the credential file is readable by users
// other than its owner.
var ErrCredentialsTooOpen = errors.New("credentials file has overly permissive mode")

// ErrLocked reports that another tt process holds the credential lock.
var ErrLocked = errors.New("another tt process is writing credentials")

// CredentialsPath returns the path of the credential document.
func CredentialsPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "credentials.yaml"), nil
}

// LoadCredential returns the stored token for a profile, or empty when none is
// stored. A missing file is not an error.
//
// It refuses to read a file that group or others can access. A token silently
// read out of a world-readable file is the failure this check exists to prevent,
// and ssh sets the precedent that refusing is kinder than warning.
func LoadCredential(profile string) (string, error) {
	c, err := LoadCredentialRecord(profile)
	if err != nil {
		return "", err
	}
	return c.Token, nil
}

// LoadCredentialRecord returns the full stored credential for a profile, or a
// zero Credential when none is stored.
func LoadCredentialRecord(profile string) (Credential, error) {
	f, _, err := loadCredentialFile()
	if err != nil {
		return Credential{}, err
	}
	return f.Profiles[profile], nil
}

// StoreCredentialRecord writes a full credential for a profile.
// LoadCredentialRecordSynced reads a credential after any in-flight write has
// finished, by taking the same lock the writers hold.
//
// The plain read is fine for the ordinary case, where nothing else is running. This
// exists for the moment after a refresh was refused: another tt invocation has very
// likely just won the race and is mid-write, and reading immediately would see the
// old pair and conclude — wrongly — that the refresh token is simply dead. Waiting on
// the lock is deterministic where polling for the file to change is not.
//
// A contended or stale lock falls back to an unsynchronised read rather than
// failing: the caller is recovering from an error already, and refusing to look is
// strictly worse than looking at a possibly-stale copy.
func LoadCredentialRecordSynced(profile string) (Credential, error) {
	path, err := CredentialsPath()
	if err != nil {
		return Credential{}, err
	}
	release, lockErr := acquireLock(path + ".lock")
	if lockErr != nil {
		return LoadCredentialRecord(profile)
	}
	defer release()

	return LoadCredentialRecord(profile)
}

func StoreCredentialRecord(profile string, c Credential) error {
	return withCredentialLock(func(f *credentialFile) {
		f.Profiles[profile] = c
	})
}

// HasCredential reports whether a token is stored for a profile, without
// returning it.
func HasCredential(profile string) (bool, error) {
	token, err := LoadCredential(profile)
	return token != "", err
}

// StoredProfiles returns the set of profiles holding a token, reading the
// credential file once. Callers listing every profile use this instead of
// LoadCredential per name.
func StoredProfiles() (map[string]bool, error) {
	f, _, err := loadCredentialFile()
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(f.Profiles))
	for name, c := range f.Profiles {
		out[name] = c.Token != ""
	}
	return out, nil
}

// StoreCredential writes the token for a profile, preserving other profiles'
// tokens and leaving the file mode at 0600.
func StoreCredential(profile, token string) error {
	return withCredentialLock(func(f *credentialFile) {
		f.Profiles[profile] = Credential{Token: token}
	})
}

// DeleteCredential removes the token for a profile. Removing a token that is not
// there succeeds, so revoking twice is safe.
func DeleteCredential(profile string) error {
	return withCredentialLock(func(f *credentialFile) {
		delete(f.Profiles, profile)
	})
}

// withCredentialLock runs a mutation against the credential file under an
// exclusive lock.
//
// The read-modify-write is not atomic on its own: two `tt auth set-token`
// invocations racing each other both read the old file and the second rename
// wins, so the first token is lost while both commands report success. The lock
// makes concurrent provisioning either succeed or fail loudly.
func withCredentialLock(mutate func(*credentialFile)) error {
	path, err := CredentialsPath()
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

	f, _, err := loadCredentialFile()
	if err != nil {
		return err
	}
	mutate(f)

	out, err := yaml.Marshal(f)
	if err != nil {
		return fmt.Errorf("encoding credentials: %w", err)
	}
	return writeFilePrivate(path, out)
}

// acquireLock takes an exclusive lock by creating a file with O_EXCL, retrying
// briefly so two commands started together do not simply fail.
func acquireLock(path string) (func(), error) {
	const (
		attempts = 50
		delay    = 20 * time.Millisecond
	)
	for i := 0; i < attempts; i++ {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			_ = f.Close()
			return func() { _ = os.Remove(path) }, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("acquiring credential lock: %w", err)
		}
		// A lock older than any real critical section can only be debris from a
		// killed process. Without this, one SIGKILL mid-write leaves every later
		// `tt auth` command failing forever — on a CI image or a container, with
		// nobody able to run the rm the error suggests. The whole guarded section
		// is a read, a marshal and a rename, so a lock this old is not contended.
		if stale, err := lockIsStale(path); err == nil && stale {
			// Best effort: if another process wins the race to remove it, the
			// next attempt simply takes the lock normally.
			_ = os.Remove(path)
			continue
		}
		time.Sleep(delay)
	}
	return nil, fmt.Errorf("%w: remove %s if no other tt process is running", ErrLocked, path)
}

// staleLockAge is how old a lock file must be to count as abandoned. Generous
// next to the microseconds the guarded write takes, so a live holder is never
// mistaken for a dead one, and short next to how long an operator would spend
// working out why tt stopped storing tokens.
const staleLockAge = 30 * time.Second

func lockIsStale(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		// Gone already, so the next O_EXCL attempt decides.
		return false, err
	}
	return time.Since(info.ModTime()) > staleLockAge, nil
}

func loadCredentialFile() (*credentialFile, string, error) {
	path, err := CredentialsPath()
	if err != nil {
		return nil, "", err
	}
	f := &credentialFile{Profiles: map[string]Credential{}}

	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return f, path, nil
	}
	if err != nil {
		return nil, "", fmt.Errorf("inspecting %s: %w", path, err)
	}
	// Checked on every path that touches the file, not only on read: a write
	// that quietly tightened the mode would repair the symptom and leave the
	// user believing tokens were never exposed.
	if mode := info.Mode().Perm(); modeChecksSupported && mode&0o077 != 0 {
		return nil, "", fmt.Errorf("%w: %s is %#o and its tokens should be treated as compromised. Rotate them, then run 'chmod 600 %s'",
			ErrCredentialsTooOpen, path, mode, path)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("reading %s: %w", path, err)
	}
	if err := yaml.Unmarshal(raw, f); err != nil {
		return nil, "", fmt.Errorf("parsing %s: %w", path, err)
	}
	if f.Profiles == nil {
		f.Profiles = map[string]Credential{}
	}
	return f, path, nil
}

// ensureConfigDir creates the config directory and tightens its mode.
//
// MkdirAll does not chmod a directory that already exists, so a config
// directory that is group- or world-writable stays that way. That matters even
// though the credential file itself is 0600: anyone who can write the directory
// can rewrite config.yaml to point a profile at their own host and collect the
// token on the next command.
func ensureConfigDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("inspecting config directory: %w", err)
	}
	if mode := info.Mode().Perm(); modeChecksSupported && mode&0o077 != 0 {
		if err := os.Chmod(dir, 0o700); err != nil {
			return fmt.Errorf("tightening config directory %s from %#o: %w", dir, mode, err)
		}
	}
	return nil
}

// writeFilePrivate writes data to path with mode 0600, atomically.
//
// os.WriteFile does not change the mode of a file that already exists, so a
// credentials.yaml left at 0644 by anything else would keep that mode forever.
// Writing a fresh 0600 temp file and renaming over the target fixes the mode
// every time and never leaves a half-written file behind.
func writeFilePrivate(path string, data []byte) error {
	if err := ensureConfigDir(filepath.Dir(path)); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), ".tt-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file next to %s: %w", path, err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("securing temp file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("writing temp file: %w", err)
	}
	// Rename is atomic but not durable. Without the sync a crash can publish an
	// empty file, which for credentials.yaml means losing every stored token.
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("flushing temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temp file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}
