package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

// The credential file being 0600 is not enough on its own. Anyone who can write
// config.yaml repoints a profile at a host they control, and the next command
// reads the token correctly and sends it to them — so the read path has to refuse
// a writable config, not just the write path.
//
// Asserted for both the file and the directory holding it, since replacing a file
// only needs write access to its directory.
func TestLoadRefusesAConfigOtherUsersCanWrite(t *testing.T) {
	if !modeChecksSupported {
		t.Skip("permission bits are not meaningful on this platform")
	}

	for _, tc := range []struct {
		name string
		mode os.FileMode
		dir  bool
	}{
		{name: "group-writable file", mode: 0o660},
		{name: "world-writable file", mode: 0o666},
		{name: "group-writable directory", mode: 0o770, dir: true},
		{name: "world-writable directory", mode: 0o777, dir: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := isolate(t)
			path := filepath.Join(dir, "config.yaml")
			if err := os.WriteFile(path, []byte("profiles:\n  prod:\n    url: https://real.example\n"), 0o600); err != nil {
				t.Fatalf("seeding config: %v", err)
			}
			target := path
			if tc.dir {
				target = dir
			}
			if err := os.Chmod(target, tc.mode); err != nil {
				t.Fatalf("chmod: %v", err)
			}
			t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

			_, err := Load()
			if !errors.Is(err, ErrConfigWritable) {
				t.Fatalf("Load() error = %v, want ErrConfigWritable", err)
			}
			if got := err.Error(); !containsAll(got, "chmod go-w", target) {
				t.Errorf("error = %q, want it to name %q and say how to fix it", got, target)
			}
		})
	}
}

// A config only its owner can write is the ordinary case and must keep working —
// including 0644, which is not a finding: the file holds no secret, so being
// readable is fine and only being *writable* by others is the problem. Mirroring
// the credential check here would reject a perfectly normal install.
func TestLoadAcceptsAReadableButNotWritableConfig(t *testing.T) {
	if !modeChecksSupported {
		t.Skip("permission bits are not meaningful on this platform")
	}
	dir := isolate(t)
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("profiles:\n  prod:\n    url: https://real.example\n"), 0o644); err != nil {
		t.Fatalf("seeding config: %v", err)
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatalf("chmod dir: %v", err)
	}

	f, err := Load()
	if err != nil {
		t.Fatalf("Load() on a world-readable config error = %v, want success", err)
	}
	if f.Profiles["prod"].URL != "https://real.example" {
		t.Errorf("profiles = %+v, want the document to have been read", f.Profiles)
	}
}

// The same lost-update race that was fixed for tokens applied to profiles, and
// profiles are the likelier victim: a provisioning script adds several at once.
// Every one of them reported success while all but one vanished.
func TestConcurrentProfileWritesKeepEveryProfile(t *testing.T) {
	isolate(t)

	const n = 8
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := fmt.Sprintf("p%d", i)
			errs[i] = MutateSaved(func(f *File) error {
				f.Profiles[name] = Profile{URL: fmt.Sprintf("https://%s.example", name)}
				return nil
			})
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("MutateSaved(p%d) error = %v", i, err)
		}
	}

	f, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("p%d", i)
		if _, ok := f.Profiles[name]; !ok {
			t.Errorf("profile %q is missing — a concurrent write lost it while "+
				"reporting success (kept %d of %d)", name, len(f.Profiles), n)
		}
	}
}

// A mutator that fails must leave the document untouched, or a rejected
// `tt profile use` would still have rewritten the file.
func TestMutateSavedWritesNothingWhenTheMutatorFails(t *testing.T) {
	isolate(t)
	if err := MutateSaved(func(f *File) error {
		f.Profiles["keep"] = Profile{URL: "https://keep.example"}
		return nil
	}); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	sentinel := errors.New("no")
	err := MutateSaved(func(f *File) error {
		f.Profiles["gone"] = Profile{URL: "https://gone.example"}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("MutateSaved() error = %v, want the mutator's error", err)
	}

	f, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if _, ok := f.Profiles["gone"]; ok {
		t.Error("a failed mutation was written anyway")
	}
	if _, ok := f.Profiles["keep"]; !ok {
		t.Error("a failed mutation destroyed the existing document")
	}
}

// A lock file left behind by a killed process must not brick credential writes
// forever. Without this, one SIGKILL mid-write means every later `tt auth`
// command fails permanently — on a container or a CI image, with nobody able to
// run the rm the error suggests.
func TestAStaleLockIsBrokenRatherThanBrickingTheCLI(t *testing.T) {
	isolate(t)
	path, err := CredentialsPath()
	if err != nil {
		t.Fatalf("CredentialsPath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	lock := path + ".lock"
	if err := os.WriteFile(lock, nil, 0o600); err != nil {
		t.Fatalf("seeding lock: %v", err)
	}
	// Older than any real critical section, which is a read, a marshal and a rename.
	old := time.Now().Add(-staleLockAge - time.Minute)
	if err := os.Chtimes(lock, old, old); err != nil {
		t.Fatalf("ageing lock: %v", err)
	}

	if err := StoreCredential("prod", "token-abc"); err != nil {
		t.Fatalf("StoreCredential() over a stale lock error = %v, want it to break the lock", err)
	}
	got, err := LoadCredential("prod")
	if err != nil {
		t.Fatalf("LoadCredential() error = %v", err)
	}
	if got != "token-abc" {
		t.Errorf("token = %q, want it stored", got)
	}
}

// A fresh lock is a live holder, so it must still be respected — otherwise the
// staleness rule above would have removed the locking it was added to keep.
func TestAFreshLockIsStillRespected(t *testing.T) {
	isolate(t)
	path, err := CredentialsPath()
	if err != nil {
		t.Fatalf("CredentialsPath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path+".lock", nil, 0o600); err != nil {
		t.Fatalf("seeding lock: %v", err)
	}

	err = StoreCredential("prod", "token-abc")
	if !errors.Is(err, ErrLocked) {
		t.Fatalf("StoreCredential() error = %v, want ErrLocked", err)
	}
}

// Windows synthesizes the permission bits Go reports (0666 writable, 0444
// read-only), so every mode check would reject files it had just written and
// os.Chmod cannot fix it. The release workflow builds a windows binary, so the
// gate has to exist rather than being assumed.
func TestModeChecksAreDisabledOnlyOnWindows(t *testing.T) {
	if runtime.GOOS == "windows" && modeChecksSupported {
		t.Error("modeChecksSupported is true on Windows, where tt would refuse every " +
			"credential file it writes and advise a chmod the platform cannot perform")
	}
	if runtime.GOOS != "windows" && !modeChecksSupported {
		t.Errorf("modeChecksSupported is false on %s, silently disabling a real "+
			"security control", runtime.GOOS)
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		found := false
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
