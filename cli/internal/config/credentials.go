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
	Profiles map[string]credential `yaml:"profiles,omitempty"`
}

type credential struct {
	Token string `yaml:"token"`
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
	f, _, err := loadCredentialFile()
	if err != nil {
		return "", err
	}
	return f.Profiles[profile].Token, nil
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
		f.Profiles[profile] = credential{Token: token}
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
		time.Sleep(delay)
	}
	// A stale lock left by a killed process needs a human, and saying so beats
	// silently overwriting whatever the other process was in the middle of.
	return nil, fmt.Errorf("%w: remove %s if no other tt process is running", ErrLocked, path)
}

func loadCredentialFile() (*credentialFile, string, error) {
	path, err := CredentialsPath()
	if err != nil {
		return nil, "", err
	}
	f := &credentialFile{Profiles: map[string]credential{}}

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
	if mode := info.Mode().Perm(); mode&0o077 != 0 {
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
		f.Profiles = map[string]credential{}
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
	if mode := info.Mode().Perm(); mode&0o077 != 0 {
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
