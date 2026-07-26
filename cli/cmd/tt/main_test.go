package main

import (
	"os"
	"testing"
)

// run() is the whole of main's logic. Asserting it maps a successful command to
// exit 0 and a failing one to a non-zero code is what makes the `run() int`
// pattern worth using, and it matches backend/cmd/server and migration-tool.
func TestRun(t *testing.T) {
	// Keep the test off the developer's real configuration.
	t.Setenv("TT_CONFIG_DIR", t.TempDir())

	tests := []struct {
		name    string
		args    []string
		wantOK  bool
		wantOut bool
	}{
		{name: "version succeeds", args: []string{"tt", "version"}, wantOK: true},
		{name: "an unknown command fails", args: []string{"tt", "no-such-command"}, wantOK: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			original := os.Args
			t.Cleanup(func() { os.Args = original })
			os.Args = tc.args

			code := run()
			if tc.wantOK && code != 0 {
				t.Errorf("run() = %d, want 0", code)
			}
			if !tc.wantOK && code == 0 {
				t.Error("run() = 0, want a non-zero exit code")
			}
		})
	}
}

// The default must be the placeholder, so a binary built without -ldflags is
// obviously not a release build.
func TestVersionDefaultsToDev(t *testing.T) {
	if version != "dev" {
		t.Errorf("version = %q, want dev for a plain go build", version)
	}
}
