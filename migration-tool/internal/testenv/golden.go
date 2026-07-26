package testenv

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// update regenerates the golden files instead of comparing against them.
//
// It is a flag rather than an environment variable so that regenerating is
// something you do deliberately, on purpose, having read the diff — the point
// of a golden file is that a change to it turns up in review as text somebody
// has to agree with.
var update = flag.Bool("update", false, "rewrite golden files instead of comparing against them")

// AssertGolden compares got against the contents of testdata/<name>, failing
// with a readable diff if they differ. With -update it rewrites the file.
//
// The files exist to make a change visible. A count assertion says "42 rows
// migrated" and passes whether or not the 42 rows are right; a golden file puts
// the actual output in the pull request, where a wrong value has to be read and
// approved by somebody before it ships.
func AssertGolden(t *testing.T, name, got string) {
	t.Helper()

	path := filepath.Join("testdata", name)
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("creating %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil { //nolint:gosec // test fixture
			t.Fatalf("writing %s: %v", path, err)
		}
		t.Logf("wrote %s", path)
		return
	}

	want, err := os.ReadFile(path) //nolint:gosec // fixed path under testdata
	if err != nil {
		if os.IsNotExist(err) {
			t.Fatalf("%s does not exist. Run `go test ./... -update` and review what it writes.", path)
		}
		t.Fatalf("reading %s: %v", path, err)
	}

	if string(want) == got {
		return
	}
	t.Errorf("%s is out of date.\n\n%s\n\nIf the change is intended, run `go test ./... -update` and review the diff.",
		path, firstDifference(string(want), got))
}

// firstDifference renders the first differing line with a little context.
// Printing two multi-kilobyte documents in full makes the failure unreadable,
// which in practice means nobody reads it.
func firstDifference(want, got string) string {
	wantLines := strings.Split(want, "\n")
	gotLines := strings.Split(got, "\n")

	for i := 0; i < len(wantLines) || i < len(gotLines); i++ {
		w, g := lineAt(wantLines, i), lineAt(gotLines, i)
		if w == g {
			continue
		}

		var b strings.Builder
		fmt.Fprintf(&b, "first difference at line %d:\n", i+1)
		for c := max(0, i-3); c < i; c++ {
			fmt.Fprintf(&b, "  %s\n", lineAt(wantLines, c))
		}
		fmt.Fprintf(&b, "- want: %s\n", w)
		fmt.Fprintf(&b, "+ got:  %s\n", g)
		if len(wantLines) != len(gotLines) {
			fmt.Fprintf(&b, "\n(%d lines expected, %d produced)", len(wantLines), len(gotLines))
		}
		return b.String()
	}
	return "the files differ only in trailing whitespace"
}

func lineAt(lines []string, i int) string {
	if i < len(lines) {
		return lines[i]
	}
	return "<end of file>"
}
