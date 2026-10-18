package model

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// A ban that sets Enabled=false by hand leaves AwaitingConfirmation as it
// was, and a still-pending account could then confirm its email and switch
// itself back on. Disabling goes through User.Disable everywhere.
func TestAccountsAreOnlyDisabledThroughDisable(t *testing.T) {
	assign := regexp.MustCompile(`\.Enabled\s*=\s*false\b`)
	root := ".." // backend/internal

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		if path == filepath.Join(root, "model", "user.go") {
			return nil // Disable itself
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(src), "\n") {
			if assign.MatchString(line) {
				t.Errorf("%s:%d sets Enabled=false directly; call User.Disable() instead", path, i+1)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
}
