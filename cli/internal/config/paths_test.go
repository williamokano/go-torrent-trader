package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDirFallsBackToHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv(EnvConfigDir, "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", home)

	got, err := Dir()
	if err != nil {
		t.Fatalf("Dir() error = %v", err)
	}
	if want := filepath.Join(home, ".config", "go-torrent-trader"); got != want {
		t.Errorf("Dir() = %q, want %q", got, want)
	}
}

func TestLoadReportsMalformedYAML(t *testing.T) {
	dir := isolate(t)
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("profiles: [this is not a map\n"), 0o600); err != nil {
		t.Fatalf("seeding a broken config: %v", err)
	}

	_, err := Load()
	if err == nil {
		t.Fatal("Load() succeeded on malformed YAML, want an error")
	}
	// The path has to be in the message; "parsing failed" alone leaves the user
	// hunting for which file.
	if !strings.Contains(err.Error(), "config.yaml") {
		t.Errorf("error = %q, want it to name the file", err)
	}
}

func TestLoadCredentialReportsMalformedYAML(t *testing.T) {
	dir := isolate(t)
	if err := os.WriteFile(filepath.Join(dir, "credentials.yaml"), []byte("profiles: [broken\n"), 0o600); err != nil {
		t.Fatalf("seeding broken credentials: %v", err)
	}

	if _, err := LoadCredential("prod"); err == nil {
		t.Fatal("LoadCredential() succeeded on malformed YAML, want an error")
	}
}

func TestStoreCredentialReportsMalformedExistingFile(t *testing.T) {
	dir := isolate(t)
	if err := os.WriteFile(filepath.Join(dir, "credentials.yaml"), []byte("profiles: [broken\n"), 0o600); err != nil {
		t.Fatalf("seeding broken credentials: %v", err)
	}

	// Overwriting blindly would discard whatever the file really held, so a parse
	// failure has to stop the write.
	if err := StoreCredential("prod", "token"); err == nil {
		t.Fatal("StoreCredential() succeeded over a malformed file, want an error")
	}
}

// A read-only config directory is the realistic version of "the write failed",
// and it must surface rather than being swallowed.
func TestSaveReportsAnUnwritableDirectory(t *testing.T) {
	dir := isolate(t)
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Skipf("cannot make the directory read-only: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	f := &File{Profiles: map[string]Profile{"a": {URL: "https://a.example.com"}}}
	if err := f.Save(); err == nil {
		t.Error("Save() succeeded into a read-only directory, want an error")
	}
}

func TestStoreCredentialReportsAnUnwritableDirectory(t *testing.T) {
	dir := isolate(t)
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Skipf("cannot make the directory read-only: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	if err := StoreCredential("prod", "token"); err == nil {
		t.Error("StoreCredential() succeeded into a read-only directory, want an error")
	}
}

func TestConfigAndCredentialsPathsAreDistinct(t *testing.T) {
	isolate(t)

	cfg, err := ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath() error = %v", err)
	}
	creds, err := CredentialsPath()
	if err != nil {
		t.Fatalf("CredentialsPath() error = %v", err)
	}
	if cfg == creds {
		t.Fatal("config and credentials share a path; secrets would travel with shared config")
	}
}
