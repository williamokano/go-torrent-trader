# Contributing

TorrentTrader is open source and free of charge. **You do not have to contribute
anything to use it.** Fork it, change it, run your own version, never speak to us —
that is a completely legitimate way to use this project and it is the reason it is
public.

If you *do* want to send something back, this page tells you what that takes. The bar
is about the code being correct and maintainable, not about who you are.

## Ways to help that are not code

All of these are genuinely useful, and none of them require Go:

- **Report a bug.** What you did, what happened, what you expected. A tracker running
  in the wild finds things a test suite never will.
- **Say what confused you.** If a setting name, an error message or a page of these
  docs made no sense, that is a defect. Documentation bugs are real bugs.
- **Run it and tell us what broke.** Especially on a real deployment with real
  members — that is a test environment nobody here has.
- **Improve the docs or the site.** Same review bar, much smaller diff.

## Before you write code

**Start from an issue.** Open one if it does not exist. This is not bureaucracy — it
is how you find out whether something was already decided, already attempted, or
deliberately dropped. `docs/NOT_PORTING.md` records features that were declined on
purpose, with reasoning; check it before building something that seems obviously
missing.

For anything non-trivial, say what you plan to do in the issue before writing it.
Nobody enjoys being told their finished branch took the wrong approach.

Then read **[DEVELOPMENT.md](DEVELOPMENT.md)** for setup, architecture and the
project's conventions.

## The checklist

Every one of these must be true before a pull request is approved. They apply to
everyone equally.

### Correctness

- [ ] **Tests are included.** Every fix and every feature. A bug fix should have a
      test that fails without the fix — that is what stops it coming back.
- [ ] **The whole suite passes locally**, not just your new tests. Run
      `go test ./...` in `backend/` and `npm test` in `frontend/`. Adding a
      dependency to a function under test can break fixtures elsewhere.
- [ ] **CI is green.** Every job, no exceptions. See the note on first-time
      contributors below — a PR showing *no* checks is not a passing PR.
- [ ] **Coverage did not go down.** CI gates backend coverage at 80% overall. Check
      with `task backend:coverage`.
- [ ] **Lint is clean** — `golangci-lint run`, `npm run lint`,
      `npm run format:check`. `errcheck` is the usual offender: every error-returning
      call is handled or explicitly discarded with `_ =`.

### Shape

- [ ] **The PR closes an issue** — `Closes #N` in the description.
- [ ] **Backend and frontend ship together.** An endpoint with no UI is not a
      feature, and neither is a UI with no endpoint.
- [ ] **New endpoints are documented** in `backend/api/openapi.yaml` with an
      `x-audience: public | internal` marker, and `task generate` has been run. A
      guard test fails the build otherwise. Never add a route to the undocumented
      debt ledger — that list may only shrink.
- [ ] **User-visible changes update `website/`.** A new capability goes in the
      feature list, a new setting goes in the configuration table.
- [ ] **Errors use sentinel values with real status codes.** A rejection the caller
      can act on must not surface as a 500.
- [ ] **Migrations are forward-only and additive.** A migration that has been merged
      is immutable — fix it with a new one.

### Conduct of the change

- [ ] **The commit message explains why**, not what. The diff already says what.
- [ ] **No secrets, no credentials, no `.env` file.**
- [ ] **External input is validated**, queries are parameterized, and user input is
      sanitized before it reaches a log.
- [ ] **The change is as small as it can be.** An unrelated refactor bundled into a
      fix makes both harder to review and harder to revert.

## What reviewers actually look for

Beyond the checklist, in rough order of how often it comes up:

**Does it fail closed?** Authorization must refuse when a dependency is missing, not
allow. This project has been bitten by a nil check that meant "permit". A test that
constructs the thing with nothing wired and asserts refusal is worth writing.

**Does the test double fail the way production fails?** A mock returning a different
error type than the real implementation has hidden a data leak here before.

**Does the side effect hang off the right layer?** Consequences belong on the service
or the event, not on one HTTP handler — otherwise every other caller skips them.

**Is it consistent with what is already there?** Matching the surrounding pattern
beats a better pattern applied in one place.

`tasks/lessons.md` is the accumulated version of this list — each entry is a real
mistake made in this repository, written as a rule. It is short, and reading it will
save you a review round.

## After you open a pull request

**If this is your first contribution, CI will not run until a maintainer approves
it.** That is a GitHub default for fork pull requests, not a judgement. It means your
PR may show *no checks at all* rather than a green tick — so run the suite locally
first, because an empty check list is not evidence of anything.

Review may ask for changes. That is normal and not a rejection. If a request does not
make sense, say so — being wrong in review is cheaper than being wrong in `main`.

## What will not be accepted

- **Changes with no tests**, on anything that is not purely cosmetic.
- **Broad reformatting or "cleanup"** bundled with a functional change.
- **Features that were deliberately dropped** — check `docs/NOT_PORTING.md` first.
  If you disagree with one of those decisions, argue it in an issue; several have
  been reversed that way.
- **Anything that lowers the coverage floor** to make a build pass.
- **AI-generated changes nobody ran.** Using a model to write code is fine and
  nobody will ask. Opening a PR without executing the result is not, and it is
  usually obvious because the test suite fails on the first run.

## On money

**There is no bounty programme and no budget attached to any issue.** Nothing here is
paid work, and an open issue is not an offer of employment. Contributions are made
because you want the software to be better.

## Licensing

The project is [MIT licensed](LICENSE), and contributions are accepted under the same
terms. Opening a pull request means you are happy for your work to be distributed
under MIT — including by people who fork it, change it, and never tell you.

You keep the copyright on what you write. The licence line names "William Okano and
the TorrentTrader contributors", which includes you the moment your first change is
merged.

---

Thank you for reading this far. Most projects this size have no contributing guide at
all, and the fact you are looking for one is a good sign.
