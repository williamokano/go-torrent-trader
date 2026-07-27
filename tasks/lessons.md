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

## `git checkout --` restores from the index, not from what you were just editing

Mid-review I mutated a file to prove a test would catch the bug, then undid the
mutation with `git checkout -- <file>`. The index held a snapshot staged *before*
the fix, so the "undo" silently reverted the fix as well — the escaping and
length-bound work vanished, and the tests went green again because the mutation
and the fix were removed together. It only surfaced because the restored file was
echoed back.

**Rule:** to undo a deliberate mutation, restore from a copy taken immediately
before it (`cp file /tmp/…` then `cp` back), never from git. `git checkout --`
means "make this match the index", which is only the same thing when the index is
current — and during a staged pre-push review it usually is not.

## Release a lock with `defer`, never with a trailing call

The first cut of the settings cache was `Lock()` … work … `Unlock()`. Operator
correction: always `Lock(); defer Unlock()`, so an early return or a panic in the
middle cannot leave the mutex held and wedge every later reader.

The one place this needs care is a read-then-write path — an `RWMutex` is not
reentrant, so a deferred `RUnlock` still in scope when the same goroutine reaches
`Lock()` deadlocks. The fix is not to drop the defer but to give each critical
section its own small method (`cacheLookup` / `cacheStore`), which is clearer
anyway.

**Rule:** every `Lock`/`RLock` is followed by `defer Unlock`/`RUnlock` on the
next line. If that would hold the lock too long, extract the critical section
into its own function rather than unlocking by hand.

## Clean up worktrees when the branch merges

Five agent worktrees from 2026-07-19 were still on disk on 2026-07-25, holding
branches whose work had long since squash-merged. Because squashing rewrites the
commit, `git merge-base --is-ancestor` reports them as unmerged, so `git worktree
list` read as though five pieces of work were in flight when none were.

**Rule:** remove the worktree in the same step that merges the PR. To check whether
an old branch is really outstanding, look for its change on `main` (or its story's
status in the backlog) rather than trusting ancestry after a squash merge.

## A generated artifact that CI never regenerates will be hand-edited

`frontend/src/api/schema.d.ts` carries the header "auto-generated by
openapi-typescript. Do not make direct changes to the file." Two fields —
`category_image_url` and `email_confirmation_required` — were nonetheless present in
it and in the backend's responses, but had never existed in `backend/api/openapi.yaml`.
Somebody added them where the type error was, not where the truth lives.

Nothing caught it because CI never ran `task generate`. The first person to run that
sanctioned command would have silently deleted both fields and broken the build, and
the diff would have looked like the generator was at fault.

**Rule:** if a generated file is checked in, CI must regenerate it and fail on a diff.
Until it does, treat the "do not edit" header as decorative and assume the file has
been edited. When a generated type is missing a field, fix the *source* — and check
whether the artifact was ever regenerated from it.

## A decision log that describes code nobody wrote is worse than none

`OPEN_QUESTIONS.md` recorded "short-lived **JWT** access tokens" (decision 4) and an
in-memory `golang.org/x/time/rate` middleware (decision 10). Neither existed: there is
no JWT library in `go.mod`, no `jwt` string in the backend, and `internal/middleware/`
has no limiter. The JWT wording had propagated into a `JWT_SECRET` variable documented
as **required** in the README, both env examples and the Portainer stack — a mandatory
production secret that nothing reads.

The cost is not the stale sentence, it is that the sentence gets *cited*. A proposal
for anti-abuse work was building on "requests are already rate limited", which was
never true.

**Rule:** a decision records what was *built*, not what was *chosen* — when the
implementation diverges, amend the decision in the same PR and say what actually
shipped. When citing a doc as the basis for a design, grep for the thing it promises
before you rely on it. And a claim that some config is required is a testable claim:
if no code reads the variable, it is not required, it is a trap.

## "Marked DONE" is not evidence a criterion shipped

`BE-3.9: Reseed Request` was `[DONE]`. Three of its five acceptance criteria were
never implemented: the 24-hour rate limit (the real constraint is a UNIQUE row, so one
request per user per torrent *forever*), the PM fan-out to everyone who completed the
torrent, and the background job. Worse, its "0 seeders only" rule lives entirely in
the React component — the endpoint itself accepts a POST against any torrent.

A status marker is a claim by whoever last touched the story, and it decays silently
because nothing re-checks it.

**Rule:** when a story's behaviour matters to what you are building, verify the
criterion against the code, not the marker. When you ship a partial story, put the
scope in the status (`[DONE — X shipped; Y deferred]`) and annotate the unshipped
bullets — the backlog's job is to be an honest history, and a bare `[DONE]` over
unshipped work is how a security gap hides for months.

## A test whose expectation is hand-typed proves nothing

Documenting the forum endpoints (#238) meant pinning `map[string]interface{}` response
builders to their OpenAPI schemas, since nothing else connected them. Three of the four
assertions compared the schema against a value the *production code* produced. The
fourth compared it against a hand-typed list of field names:

```go
want := []string{"created_at", "edited_by", "id", "new_body", "old_body", ...}
```

It passed. The schema was missing `reconstruction_failed` — a field `ListPostEdits`
sets and the frontend already renders — because the list was written from the same
reading of the struct, in the same sitting, as the schema it was checking. Both sides
were the same memory, so they agreed. Its comment even claimed the test would force a
future serialised field into the spec; it could not, and it had already failed to do so
for a field that existed at the time.

The fix was ten lines of reflection over the struct's json tags. Reverting the schema
to the state that shipped then failed the test.

**Rule:** an assertion is only worth what its expectation is derived from. If the
expected value is typed by hand from the same source you are validating, the test
encodes your belief rather than the system's behaviour — and it will be green for
exactly the bug you were trying to prevent. Derive the expectation from the code
(reflection, a fixture the production path builds, a real call) or don't write the
assertion. This applies well beyond schemas: any hardcoded list of "the fields/routes/
cases that exist" is a snapshot of an assumption.

Corollary, from the same PR: mutation-test the guard by reverting the bug it was
written to catch. If that does not fail, the guard is decoration.
## A doc that describes a security property is a claim to verify, not repeat

Building the CLI against `openapi.yaml`, two descriptions turned out to be fiction.
`ratio` documented an `Infinity` that JSON cannot encode — the API sends a `-1`
sentinel, so a client trusting the description renders a negative ratio. Worse,
`passkey` claimed to be *"Masked passkey (first 4 + last 4 chars visible)"* and is
not masked at all: `buildOwnerProfile` returns it in the clear.

The second one nearly shipped a leak. The CLI's own comment repeated the spec
("passkey arrives masked"), a test fixture was hand-written with a *pre-masked*
value, and the test passed — so nothing in the loop ever saw the real thing. A
fixture that encodes the assumption cannot falsify it. This is the same shape as
"a frontend fixture is not evidence the backend sends that field", one layer up:
the fixture was evidence for a *claim about the data's content*, not just its shape.

**Rule:** when a doc says a value is masked, redacted, hashed, filtered or scoped,
go read the code that produces it before you rely on it — especially before
deciding a field is safe to print or log. Build the fixture from what the producing
function actually returns. And treat "is this a credential?" as a question about
the value's power, not its name: the passkey is the tracker download credential in
every announce URL, which is what made an unmasked field a real problem rather than
a cosmetic one.

## Resolve a setting in one place, or the second place will disagree

`tt auth set-token` resolved its target profile itself instead of going through the
shared resolver, and so honoured `--profile` but silently ignored `TT_PROFILE`.
Everything else in the CLI honoured both. The result: a CI job exporting
`TT_PROFILE=staging` stored its token against `default`, reported success, and then
failed every later command with "no token" — while `clear-token` cheerfully deleted
a credential from a profile the user was not targeting and reported that as success
too.

It hid because the test that covered it called the shared isolation helper, which
clears `TT_PROFILE` — the exact variable that breaks it.

**Rule:** precedence chains (flag → env → config → default) belong in one function
that every caller uses; a second implementation is a divergence waiting to happen,
and the destructive command is where it will bite. When a test helper clears
environment variables for isolation, make sure some test sets each one back — an
isolation helper that neutralises the input under test turns the suite green for
the wrong reason.

## A guard that cannot be reached is not a guard — check the framework's order

The CLI documented its exit codes as an interface: `2` for "bad flag, bad
argument, unknown command". `tt profile lst` exited **0**, printed help, and a
cron wrapper written as `tt profile lst || alert` never fired. The most likely
operator error there is was reported as success.

Two cobra details, each individually reasonable, combined into the hole:

- `Command.Find` only consults `legacyArgs` when `Args` is nil, and `legacyArgs`
  errors for an unknown subcommand of the **root** while returning nil for an
  unknown subcommand of anything else. So grouping commands accepted anything.
- `Command.execute` returns `flag.ErrHelp` for a command that is not `Runnable()`
  **before** it calls `ValidateArgs`.

The first fix — setting `Args` on the grouping commands — made it *worse*: it
moved validation out of `Find` into `execute`, where the not-Runnable check
short-circuited first, so the root's previously-working error disappeared too and
`tt bogus` went from exit 1 to exit 0. The fix only worked once the grouping
commands also got a `RunE`, which is what makes `ValidateArgs` reachable at all.

**Rule:** when you add a validation hook a framework calls, verify *where in the
framework's order it runs* before believing it fires — read the dispatch path, or
assert against the built binary's exit code rather than against the hook. A
handler installed on a branch that is never taken looks exactly like a handler
that works. Related: the same PR's `mode&0o077` credential check can never pass on
Windows, where Go synthesises the permission bits — a check whose condition is
unsatisfiable on a platform you ship a binary for is the same failure wearing a
different hat.

**Corollary on scope:** the reviewer also flagged that a base URL with a path was
accepted, producing `/api/v1/auth/me/api/v1/auth/me`. Rejecting paths turned
`TestBaseURLWithAPathPrefixIsPreserved` red — a site served from a subdirectory is
a supported deployment. A finding that is real is not automatically a finding you
should act on; run the suite before believing a tightening is free.
## A guard test that catches a bug by hanging is barely a guard test

Mutation-testing the CLI's 401 retry, changing its `if` to a `for` did not fail the
suite — it hung it. `TestDoRetriesAtMostOnce` and `TestDoSurfacesThe401WhenRefreshFails`
both called `Do` directly with `context.Background()`, so an infinite retry never
returned and the whole package sat there until something killed it. That cost a
ten-minute mutation run, and worse, it killed the script mid-mutation and left the
deliberate defect applied to `client.go` — a hang does not clean up after itself the
way a failure does.

In CI the symptom would have been a job timing out with no failing test named, which
reads like flakiness or a slow runner rather than the loop it actually is. Note that
one of the two mutants could not have been caught by a context deadline either: when
the refresh call itself errored, the loop spun without issuing a request, so nothing
was waiting on the context.

**Rule:** a test whose subject could loop, block or deadlock must bound its own
runtime — run the call in a goroutine and fail on a `time.After` — so the regression
reports as a named failure instead of a stalled job. Bounding the *context* is not
enough; bound the *call*. And a mutation harness needs a per-mutation timeout and a
restore that runs even when the run is killed, or a timeout leaves your working tree
holding a deliberate bug.

## Revocation that needs a live credential cannot revoke a dead one

`tt auth logout` calls the site so the session is really gone, then deletes the local
file. But the server finds a session *from its access token* and gives up when that
lookup misses, and the access token lives one hour while the refresh token lives
thirty days. So logging out the morning after logging in revoked nothing, answered
401, and still deleted the only copy of a refresh token that stayed valid for a
month — precisely the "credential nobody can see and nobody revoked" the command was
written to prevent. It was invisible in tests because every test logged out
immediately after logging in, when the token was still fresh.

**Rule:** when a command's job is to destroy a credential, test it in the state the
credential is normally in when someone reaches for that command — expired, stale, a
day old — not the state the previous line of the test just created. And check the
lifetimes: if the thing being revoked outlives the thing authorising the revocation,
there is a window where revocation silently does nothing. Deleting the local copy is
what makes it unrecoverable, so a client that cleans up after itself needs the server
call to have actually worked.

## A statement the server may ignore is not a fix until you watch it work

Writing the self-referencing tables (#240, shipped in #248) began as `SET CONSTRAINTS
ALL DEFERRED` at the top of the transaction. It is the answer in every article about
the problem, it reads correctly, it compiled, and it passed every test in the package.

It does nothing here. PostgreSQL defers a constraint only when the constraint was
declared `DEFERRABLE`, and nothing in `backend/migrations` is —
`grep -ric deferrable backend/migrations/*.sql` totals zero. Against a `NOT DEFERRABLE`
constraint the statement is *accepted and ignored*: no error, no warning, no notice,
no behaviour change. The suite stayed green because no test put a child row in an
earlier batch than its parent, so nothing ever asked the deferral to do anything. A
probe against a real server is what showed it, and the real fix is a different shape
entirely — write the self-referencing column as NULL, backfill it once every row
exists.

The trap is that the failure is *silent by design*. A wrong query errors; a directive
the server declines to honour looks exactly like one it honoured.

**Rule:** when a fix is a directive *to* the database rather than a change to what you
send it — `SET CONSTRAINTS`, `SET LOCAL`, a session GUC, an isolation level, an index
hint — the server decides whether to honour it, and "accepted and ignored" is a normal
outcome, not an exotic one. Verify the precondition in the schema rather than in the
documentation, and prove the fix by building the case it exists to fix and watching
that case fail without it. This is the mutation-check rule aimed at a specific blind
spot: a test suite that never constructs the failing scenario cannot tell a working
fix from an inert one.
