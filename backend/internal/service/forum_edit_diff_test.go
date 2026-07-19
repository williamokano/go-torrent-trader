package service

import (
	"strings"
	"testing"
)

// A reverse diff must round-trip: applying it to the new body reconstructs
// the exact old body it was computed against. This is the property
// BE-9.23's whole reconstruction chain depends on.
func TestComputeAndApplyReverseDiff_RoundTrip(t *testing.T) {
	cases := []struct {
		name    string
		oldBody string
		newBody string
	}{
		{"single word change", "the quick brown fox", "the slow brown fox"},
		{"append", "hello", "hello world"},
		{"prepend", "world", "hello world"},
		{"delete", "hello world", "hello"},
		{"multiline edit", "line one\nline two\nline three", "line one\nline TWO\nline three\nline four"},
		{"unicode", "café ☕ 日本語", "café latte ☕ 日本語です"},
		{"emoji", "hello \U0001F600", "hello \U0001F600\U0001F44D"},
		{"complete replacement", "aaaaaaaaaa", "bbbbbbbbbb"},
		{"old empty", "", "new content"},
		{"new empty", "old content", ""},
		{"long body small edit", strings.Repeat("lorem ipsum dolor sit amet ", 200) + "END", strings.Repeat("lorem ipsum dolor sit amet ", 200) + "TAIL"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			diff := computeReverseDiff(tc.newBody, tc.oldBody)
			got, err := applyReverseDiff(diff, tc.newBody)
			if err != nil {
				t.Fatalf("applyReverseDiff: %v", err)
			}
			if got != tc.oldBody {
				t.Errorf("reconstructed = %q, want %q", got, tc.oldBody)
			}
		})
	}
}

// A chain of edits must be reconstructible by repeatedly applying each
// step's reverse diff, walking backward exactly like ListPostEdits does.
func TestApplyReverseDiff_ChainedWalkBackward(t *testing.T) {
	versions := []string{"v1", "v1 and v2", "v1 and v2 and v3", "final version v4"}

	// diffs[i] reconstructs versions[i] from versions[i+1].
	diffs := make([]string, len(versions)-1)
	for i := 0; i < len(versions)-1; i++ {
		diffs[i] = computeReverseDiff(versions[i+1], versions[i])
	}

	current := versions[len(versions)-1]
	for i := len(diffs) - 1; i >= 0; i-- {
		prev, err := applyReverseDiff(diffs[i], current)
		if err != nil {
			t.Fatalf("step %d: applyReverseDiff: %v", i, err)
		}
		if prev != versions[i] {
			t.Fatalf("step %d: reconstructed %q, want %q", i, prev, versions[i])
		}
		current = prev
	}
}

func TestApplyReverseDiff_InvalidDiffText(t *testing.T) {
	if _, err := applyReverseDiff("this is not valid patch text", "some body"); err == nil {
		t.Error("expected an error for unparseable diff text")
	}
}

// A diff computed against one source text must not silently misapply when
// handed a different current body — the caller (ListPostEdits) relies on
// getting an explicit error rather than a corrupted reconstruction.
func TestApplyReverseDiff_MismatchedCurrentBody(t *testing.T) {
	diff := computeReverseDiff("the new body", "the old body")
	if _, err := applyReverseDiff(diff, "a completely unrelated body that shares nothing"); err == nil {
		t.Error("expected an error when the diff doesn't cleanly apply to the given body")
	}
}

func TestComputeReverseDiff_NoOpForIdenticalBodies(t *testing.T) {
	diff := computeReverseDiff("same body", "same body")
	got, err := applyReverseDiff(diff, "same body")
	if err != nil {
		t.Fatalf("applyReverseDiff: %v", err)
	}
	if got != "same body" {
		t.Errorf("got %q, want %q", got, "same body")
	}
}
