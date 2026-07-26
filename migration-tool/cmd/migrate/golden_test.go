package main

import (
	"regexp"
	"strings"
	"testing"

	"github.com/williamokano/go-torrent-trader/migration-tool/internal/testenv"
)

// What the operator actually reads, captured as text and checked into the
// repository.
//
// Counts and integrity checks cannot see a plausible-but-wrong value: a mapping
// that quietly starts sending passkeys somewhere else still produces the right
// number of entries. These files put the output itself in the pull request, so
// changing what an operator is told requires somebody to read the change and
// agree with it.
//
// Regenerate with `go test ./cmd/migrate -update`, then read the diff.

// containerPort matches the random port testcontainers publishes, which changes
// on every run and is not something the golden files should be sensitive to.
var containerPort = regexp.MustCompile(`(127\.0\.0\.1|localhost|\[::1\]):\d+`)

func normalize(out string) string {
	out = containerPort.ReplaceAllString(out, "$1:3306")
	return strings.ReplaceAll(out, "\r\n", "\n")
}

// The mapping is the deliverable of this whole command. Every rule, every
// comment, and every column of the corpus appears in it.
func TestMappingGoldenFile(t *testing.T) {
	noDatabaseConfigured(t)

	out, err := execute(t, "mapping", "--source", legacyDSN(t), "--out", "-")
	if err != nil {
		t.Fatalf("mapping: %v\n%s", err, out)
	}

	// execute captures stdout and stderr together; the summary goes to stderr
	// and is asserted separately below.
	yaml := out
	if idx := strings.Index(out, "\n\n15 tables mapped"); idx >= 0 {
		yaml = out[:idx+1]
	}
	testenv.AssertGolden(t, "mapping.golden.yaml", normalize(yaml))
}

// The validate report is the other thing an operator reads, and the one they
// read under time pressure.
func TestValidateGoldenFile(t *testing.T) {
	noDatabaseConfigured(t)

	out, err := execute(t, "validate", "--source", legacyDSN(t))
	if err != nil {
		t.Fatalf("validate: %v\n%s", err, out)
	}
	testenv.AssertGolden(t, "validate.golden.txt", normalize(out))
}

// The corpus is built to be nasty, and the golden files are only worth having
// if it stays that way. This asserts the nastiness is still reaching the
// output rather than having been quietly tidied away.
func TestGoldenOutputStillCoversTheHardCases(t *testing.T) {
	noDatabaseConfigured(t)

	out, err := execute(t, "mapping", "--source", legacyDSN(t), "--out", "-")
	if err != nil {
		t.Fatalf("mapping: %v", err)
	}

	cases := []struct{ what, substring string }{
		{"the mod-added columns", "seedbonus"},
		{"the mod-added table", "bonus_log"},
		{"latin1 text", "charset: latin1"},
		{"the passkey rule", "target: passkey"},
		{"the info_hash transform", "hex_to_bytea"},
		{"the forum write level", "min_post_level"},
		{"drafts and templates", "saved_messages"},
	}
	for _, c := range cases {
		if !strings.Contains(out, c.substring) {
			t.Errorf("the corpus no longer exercises %s (looked for %q)", c.what, c.substring)
		}
	}
}
