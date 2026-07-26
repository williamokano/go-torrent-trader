package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
)

func TestLoadCredentialMissingFileIsEmptyNotError(t *testing.T) {
	isolate(t)

	got, err := LoadCredential("prod")
	if err != nil {
		t.Fatalf("LoadCredential() on a fresh install error = %v", err)
	}
	if got != "" {
		t.Errorf("token = %q, want empty", got)
	}
}

func TestStoreThenLoadCredentialRoundTrips(t *testing.T) {
	isolate(t)

	if err := StoreCredential("prod", "token-abc"); err != nil {
		t.Fatalf("StoreCredential() error = %v", err)
	}

	got, err := LoadCredential("prod")
	if err != nil {
		t.Fatalf("LoadCredential() error = %v", err)
	}
	if got != "token-abc" {
		t.Errorf("token = %q, want token-abc", got)
	}
}

// Storing a second profile's token must not discard the first one.
func TestStoreCredentialPreservesOtherProfiles(t *testing.T) {
	isolate(t)

	if err := StoreCredential("prod", "prod-token"); err != nil {
		t.Fatalf("StoreCredential(prod) error = %v", err)
	}
	if err := StoreCredential("staging", "staging-token"); err != nil {
		t.Fatalf("StoreCredential(staging) error = %v", err)
	}

	for profile, want := range map[string]string{"prod": "prod-token", "staging": "staging-token"} {
		got, err := LoadCredential(profile)
		if err != nil {
			t.Fatalf("LoadCredential(%s) error = %v", profile, err)
		}
		if got != want {
			t.Errorf("%s token = %q, want %q", profile, got, want)
		}
	}
}

func TestStoreCredentialWritesOwnerOnlyMode(t *testing.T) {
	isolate(t)

	if err := StoreCredential("prod", "token-abc"); err != nil {
		t.Fatalf("StoreCredential() error = %v", err)
	}

	path, err := CredentialsPath()
	if err != nil {
		t.Fatalf("CredentialsPath() error = %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("credentials.yaml mode = %#o, want 0600", mode)
	}
}

// os.WriteFile does not change the mode of a file that already exists, so the
// write path uses temp-file-and-rename to guarantee 0600 every time.
func TestStoreCredentialAlwaysWritesOwnerOnlyMode(t *testing.T) {
	isolate(t)

	if err := StoreCredential("prod", "first"); err != nil {
		t.Fatalf("StoreCredential() error = %v", err)
	}
	if err := StoreCredential("prod", "second"); err != nil {
		t.Fatalf("second StoreCredential() error = %v", err)
	}

	path, err := CredentialsPath()
	if err != nil {
		t.Fatalf("CredentialsPath() error = %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("credentials.yaml mode = %#o after rewrite, want 0600", mode)
	}
}

// A write must refuse a too-open file rather than quietly tightening it. Silently
// self-healing would repair the symptom and leave the user believing their tokens
// had never been exposed — the file was world-readable, and they need to rotate.
func TestStoreCredentialRefusesAnAlreadyOpenFile(t *testing.T) {
	isolate(t)

	path, err := CredentialsPath()
	if err != nil {
		t.Fatalf("CredentialsPath() error = %v", err)
	}
	if err := os.MkdirAll(dirOf(t), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte("profiles:\n  other:\n    token: exposed\n"), 0o644); err != nil {
		t.Fatalf("seeding a 0644 credentials file: %v", err)
	}

	err = StoreCredential("prod", "token-abc")
	if !errors.Is(err, ErrCredentialsTooOpen) {
		t.Fatalf("StoreCredential() error = %v, want ErrCredentialsTooOpen", err)
	}
	// The message must tell the user the exposed tokens need rotating, not just
	// that a mode is wrong.
	if !strings.Contains(err.Error(), "compromised") {
		t.Errorf("error = %q, want it to say the tokens should be treated as compromised", err)
	}
}

// The config directory is the other half of the credential's protection: anyone
// who can write it can repoint a profile's URL and collect the token.
func TestWritesTightenAnOpenConfigDirectory(t *testing.T) {
	dir := isolate(t)
	if err := os.MkdirAll(dir, 0o777); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.Chmod(dir, 0o777); err != nil {
		t.Skipf("cannot create a 0777 directory: %v", err)
	}

	if err := StoreCredential("prod", "token-abc"); err != nil {
		t.Fatalf("StoreCredential() error = %v", err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if mode := info.Mode().Perm(); mode&0o077 != 0 {
		t.Errorf("config directory mode = %#o, want no group or other access", mode)
	}
}

// Two invocations racing must not lose a token while both report success.
func TestConcurrentStoreCredentialKeepsEveryToken(t *testing.T) {
	isolate(t)

	const n = 8
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = StoreCredential(fmt.Sprintf("p%d", i), fmt.Sprintf("token-%d", i))
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("StoreCredential(p%d) error = %v", i, err)
		}
	}
	// Every write reported success, so every token must actually be there.
	for i := 0; i < n; i++ {
		got, err := LoadCredential(fmt.Sprintf("p%d", i))
		if err != nil {
			t.Fatalf("LoadCredential(p%d) error = %v", i, err)
		}
		if want := fmt.Sprintf("token-%d", i); got != want {
			t.Errorf("p%d token = %q, want %q", i, got, want)
		}
	}
}

func TestStoredProfilesReportsOnlyProfilesHoldingTokens(t *testing.T) {
	isolate(t)

	if err := StoreCredential("prod", "token-abc"); err != nil {
		t.Fatalf("StoreCredential() error = %v", err)
	}

	stored, err := StoredProfiles()
	if err != nil {
		t.Fatalf("StoredProfiles() error = %v", err)
	}
	if !stored["prod"] {
		t.Error("prod is not reported as holding a token")
	}
	if stored["staging"] {
		t.Error("staging is reported as holding a token it never had")
	}
}

func TestHasCredential(t *testing.T) {
	isolate(t)

	if err := StoreCredential("prod", "token-abc"); err != nil {
		t.Fatalf("StoreCredential() error = %v", err)
	}

	has, err := HasCredential("prod")
	if err != nil {
		t.Fatalf("HasCredential(prod) error = %v", err)
	}
	if !has {
		t.Error("HasCredential(prod) = false, want true")
	}

	has, err = HasCredential("staging")
	if err != nil {
		t.Fatalf("HasCredential(staging) error = %v", err)
	}
	if has {
		t.Error("HasCredential(staging) = true, want false")
	}
}

// Reading a token out of a world-readable file is the failure the mode check
// exists to prevent, so it must be an error and not a warning.
func TestLoadCredentialRefusesAnOpenFile(t *testing.T) {
	isolate(t)

	if err := StoreCredential("prod", "token-abc"); err != nil {
		t.Fatalf("StoreCredential() error = %v", err)
	}
	path, err := CredentialsPath()
	if err != nil {
		t.Fatalf("CredentialsPath() error = %v", err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatalf("Chmod() error = %v", err)
	}

	_, err = LoadCredential("prod")
	if !errors.Is(err, ErrCredentialsTooOpen) {
		t.Fatalf("LoadCredential() error = %v, want ErrCredentialsTooOpen", err)
	}
}

func TestDeleteCredentialRemovesOnlyThatProfile(t *testing.T) {
	isolate(t)

	if err := StoreCredential("prod", "prod-token"); err != nil {
		t.Fatalf("StoreCredential(prod) error = %v", err)
	}
	if err := StoreCredential("staging", "staging-token"); err != nil {
		t.Fatalf("StoreCredential(staging) error = %v", err)
	}
	if err := DeleteCredential("prod"); err != nil {
		t.Fatalf("DeleteCredential() error = %v", err)
	}

	gone, err := LoadCredential("prod")
	if err != nil {
		t.Fatalf("LoadCredential(prod) error = %v", err)
	}
	if gone != "" {
		t.Errorf("prod token = %q, want empty after delete", gone)
	}

	kept, err := LoadCredential("staging")
	if err != nil {
		t.Fatalf("LoadCredential(staging) error = %v", err)
	}
	if kept != "staging-token" {
		t.Errorf("staging token = %q, want staging-token", kept)
	}
}

// `tt logout` and `tt profile remove` both delete tokens, and neither should
// fail because there was nothing to delete.
func TestDeleteCredentialIsIdempotent(t *testing.T) {
	isolate(t)

	if err := DeleteCredential("never-existed"); err != nil {
		t.Fatalf("first DeleteCredential() error = %v", err)
	}
	if err := DeleteCredential("never-existed"); err != nil {
		t.Fatalf("second DeleteCredential() error = %v", err)
	}
}

func dirOf(t *testing.T) string {
	t.Helper()
	d, err := Dir()
	if err != nil {
		t.Fatalf("Dir() error = %v", err)
	}
	return d
}
