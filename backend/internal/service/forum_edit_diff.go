package service

import (
	"fmt"

	"github.com/sergi/go-diff/diffmatchpatch"
)

// newForumEditDMP returns a diffmatchpatch engine configured for exact
// reconstruction rather than diff-match-patch's usual fuzzy, best-effort
// application. Edits on a post are strictly linear (BE-9.23): a stored patch
// is always applied to the exact text it was computed against, never to
// independently-edited text, so there's nothing to fuzzily reconcile — and
// tolerating fuzz here would risk turning data corruption or a bug elsewhere
// into a silently wrong reconstruction instead of a loud error.
func newForumEditDMP() *diffmatchpatch.DiffMatchPatch {
	dmp := diffmatchpatch.New()
	dmp.MatchThreshold = 0
	dmp.PatchDeleteThreshold = 0
	return dmp
}

// computeReverseDiff returns a serialized patch that reconstructs oldBody
// when applied to newBody. Storing this (rather than a full oldBody
// snapshot) is what lets forum_post_edits avoid duplicating near-identical
// bodies across repeated small edits (BE-9.23): reconstructing history walks
// backward from the post's current body, applying one stored patch per edit
// instead of reading a full copy per edit.
func computeReverseDiff(newBody, oldBody string) string {
	dmp := newForumEditDMP()
	patches := dmp.PatchMake(newBody, oldBody)
	return dmp.PatchToText(patches)
}

// applyReverseDiff reconstructs the body immediately before an edit, given
// the body immediately after it (currentBody) and that edit's stored diff.
// Edits on a given post are strictly linear — one body, edits applied in
// server-received order — so currentBody is always an exact match for the
// patch's source text: no fuzzy matching, no merge conflicts, deterministic
// reconstruction every time. A patch that doesn't apply cleanly (corrupt
// diff, or currentBody isn't actually the version the diff was computed
// against) is a hard error rather than a best-effort/fuzzy result.
func applyReverseDiff(diff, currentBody string) (string, error) {
	dmp := newForumEditDMP()
	patches, err := dmp.PatchFromText(diff)
	if err != nil {
		return "", fmt.Errorf("parse stored diff: %w", err)
	}
	reconstructed, applied := dmp.PatchApply(patches, currentBody)
	for _, ok := range applied {
		if !ok {
			return "", fmt.Errorf("diff did not apply cleanly against the current body")
		}
	}
	return reconstructed, nil
}
