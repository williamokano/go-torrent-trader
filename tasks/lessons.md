# Lessons

Mistakes worth not repeating, each one written down because it actually happened
here. Reviewed at the start of a session; added to after any correction.

Keep entries concrete — what went wrong, why it hid, and the rule that would have
caught it.

---

## An authorization gate must fail closed at the point of decision

`FeedAccessService` failed closed, and the docs said so — but both handlers waved
everyone through when the check was nil, and the only wiring point (`main.go`) is
excluded from coverage. The documented guarantee was not the system's behaviour, and
nothing would have gone red if a refactor had switched the privilege off entirely.

**Rule:** fail-closed belongs at every layer that can decide, not only the one that
computes. A dependency being absent must never mean "allow" — that is a bypass
waiting for a refactor. Write a test that constructs the handler with nothing wired
and asserts it refuses.

## Struct-writing inserts defeat column defaults

`UserRepo.Create` writes every column from the struct, so a new
`NOT NULL DEFAULT true` column read `false` for every row created through Go.
Registration would have given every *new* member no live feeds while every existing
one kept them — a split that surfaces weeks later as "works for me".

**Rule:** adding a defaulted column is two changes, the migration *and* every
constructor that builds the row. Test the default with a **raw** `INSERT`, because a
fixture that sets the field never exercises it.

## A frontend fixture is not evidence the backend sends that field

`AdminUserView` was missing `can_feed`, so the admin panel would have shown
"Live feeds: Suspended" for every user on the site, with a Restore button that
changed nothing. The frontend test passed because its `mockUser` fixture included
`can_feed: true` — it asserted a response shape the backend never produced.

**Rule:** when a field crosses the API boundary, assert it on the **backend**
view/serializer too, ideally that the marshaled JSON contains the key. A hand-written
mock proves only that the component renders what it is handed.

## Optional booleans in a write request revoke things

`GroupWriteRequest.CanFeed` started as a plain `bool`. A browser tab loaded before
the deploy, saving an unrelated group edit, would have sent no `can_feed` key — Go
reads that as `false` — and revoked the privilege for an entire class.

**Rule:** for a request type that replaces a whole object, any field an older client
might omit must be a pointer, where absent means "leave it as it was". This is why
`can_self_approve` was kept out of that request shape in the first place.

## Test doubles must fail the way production fails

`CategoryAncestorIDs` swallowed any error after the first hop and returned a
truncated chain with a `nil` error — which would have leaked an excluded category
into a public feed on a connection blip. It hid because the mock repository returned
a bare `errors.New("not found")`, indistinguishable from a real failure, while
production returns a wrapped `sql.ErrNoRows`.

**Rule:** a mock that returns a generic error where production returns a
distinguishable one hides exactly the branch that matters. Mirror the real error
values.

## Side effects on the HTTP handler miss every other caller

Revoking `can_feed` closed the member's open streams — but only from the admin form,
because that is where the call was. Warning escalation applies restrictions without
going near that handler, so an automatically revoked member kept watching.

**Rule:** if a state change has a consequence, hang the consequence off the event or
the service, not off one entry point. The existing chat kick has the same shape;
being consistent with it was being consistently wrong.

## Mutation-check any test that guards something important

Several tests in this work passed for the wrong reason: a racy drain, an assertion
derived from the same reflection walk it was checking, and a leader-election test
that succeeded re-entrantly because `database/sql` handed back the same pooled
connection.

**Rule:** for a test that is the only thing between a bug and production, break the
guard deliberately and watch it go red. It takes a minute and has caught a wrong test
more than once.

## Anchored edits: verify the anchor exists in the file you are editing

**Twice in one session** I inserted a section into `tasks/todo.md` from a git
worktree using an anchor string that existed only in the *main* working copy's
uncommitted version. `str.replace` matched nothing, wrote the file back unchanged,
and printed "ok" — a silent no-op noticed much later.

**Rule:** `assert anchor in s` before replacing, or grep the file first. A replace
that matches nothing must fail loudly, not succeed vacuously. This bites hardest in
worktrees, which check out *committed* content: anything uncommitted in the main
working directory is not there, so anchors taken from "what I remember writing" are
unreliable by construction.

## Clean up worktrees when the branch merges

Five agent worktrees from 2026-07-19 were still on disk on 2026-07-25, holding
branches whose work had long since squash-merged. Because squashing rewrites the
commit, `git merge-base --is-ancestor` reports them as unmerged, so `git worktree
list` read as though five pieces of work were in flight when none were.

**Rule:** remove the worktree in the same step that merges the PR. To check whether
an old branch is really outstanding, look for its change on `main` (or its story's
status in the backlog) rather than trusting ancestry after a squash merge.
