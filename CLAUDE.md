# go-torrent-trader Development Guidelines

## Where work lives: GitHub Issues, not Markdown

**Decided 2026-07-26. This overrides any instruction elsewhere in this repository
that tells you to record work in a document.**

GitHub Issues is the source of truth for everything to be done — features, bugs,
chores, proposals, spikes. Do **not** add new items to `docs/IMPLEMENTATION_TASKS.md`,
`docs/PROPOSED_FEATURES.md`, `docs/FUTURE_WORK.md` or `tasks/todo.md`. Open an issue
and reference it from the pull request (`Closes #N`).

**Why.** Task state in Markdown drifts, silently, and this project has paid for it.
A story sat marked `[DONE]` over three acceptance criteria that were never
implemented and an endpoint shipped with no authorization check. Two of the most-cited
architecture decisions described a JWT and a rate limiter that no one ever wrote, and
one of them had spawned a "required" production secret that nothing reads. None of
that survives in an issue tracker: an issue has one state, it closes from the PR that
does the work, and a merge cannot quietly rewrite it.

**Reference documents stay as files, and stay authoritative.** They describe what the
system *is* and *why*, not what to do next. Read these before working in an unfamiliar
area — they are listed again under Key References with what each is for:

- `docs/ARCHITECTURE.md` — layering, boundaries, conventions
- `docs/OPEN_QUESTIONS.md` — the decision log
- `docs/NOT_PORTING.md` — what was deliberately dropped, and why. Check it before
  "adding" something; it may have been declined on purpose
- `docs/FULL_FEATURE_DOCUMENTATION.md` — the original TorrentTrader spec
- `tasks/lessons.md` — mistakes already made once, written as rules

**The drain is complete.** Every real item from `docs/IMPLEMENTATION_TASKS.md`,
`docs/PROPOSED_FEATURES.md` and `docs/FUTURE_WORK.md` now exists as an issue. Those
files are kept only as history — the build log of the port and the reasoning behind
the proposals. Read them for context and cite them freely; never append to them, and
never treat a status marker in them as current. They can be deleted whenever they
stop being useful.

## Agent Development Flow

```
Receive task → Create branch → Evaluate alternatives → Agree on approach → Implement → Review & Challenge → Push → Pipeline validates → Merge
```

### 1. Receive Task

- **Work starts from a GitHub issue.** If there isn't one, open one first — that is
  where scope, acceptance criteria and discussion live
- Understand requirements, acceptance criteria, and scope
- Check Pre-Task Checks (cross-repo deps, manual steps, code dwarfing, duplicates)

### 2. Create Branch (using worktrees)

Always use git worktrees so the main working directory stays on `main`:

```bash
# From the main repo directory (always on main branch)
git pull origin main
git worktree add ../go-torrent-trader-<branch-name> -b <type>/<description>
cd ../go-torrent-trader-<branch-name>
```

Clean up after merge:
```bash
cd <repo-root>   # e.g. ~/Workspace/Personal/go-torrent-trader
git worktree remove ../go-torrent-trader-<branch-name>
```

**Branch naming:**
- `feat/add-forum-search` — new feature
- `fix/null-pointer-in-router` — bug fix
- `refactor/extract-common-metrics` — refactoring
- `chore/update-dependencies` — maintenance
- `docs/update-guardrails` — documentation

**Worktree conflict warning:** Parallel worktrees WILL conflict on shared files like `main.go`, `router.go`, and repository interfaces. When running multiple tracks in parallel, merge Track A first, then rebase Track B before pushing. Never push without running tests after a rebase.

### 3. Evaluate Alternatives (for non-trivial tasks)

Before implementing, a team of agents must consider alternative solutions for the problem. Each agent proposes a different approach, evaluating trade-offs (complexity, performance, maintainability, consistency with existing patterns). The team must discuss and agree on one approach before any code is written. Document the chosen approach and why alternatives were rejected.

The spec should cover:
- What changes are needed and why
- Which files will be modified
- What tests will be added
- Any architectural decisions
- Alternatives considered and why they were rejected

### 4. Implement

- **Features ship in BE+FE pairs** — every feature must include both backend and frontend in the same PR. A backend endpoint without its corresponding UI (or vice versa) is not considered complete. No half-shipped features.
- **Every new endpoint is documented and classified in the same PR** — add it to `backend/api/openapi.yaml` with an `x-audience: public | internal` marker, then run `task generate` to refresh `backend/api/openapi.public.yaml` and the frontend types. The public document is what third-party integrators build against; the full one includes admin routes and is not published as a contract. A guard test walks the real router and fails the build on an unclassified route, so this is enforced, not merely expected. Never add a route to the undocumented-debt ledger — that list may only shrink.
- **Every user-visible feature updates the public site in the same PR** — `website/` is the project's front door, published to GitHub Pages. If you shipped something a member or an operator would notice, it belongs there: a new capability goes in the feature list on `index.html`, a new setting goes in the table on `configure.html`, anything that changes how the site is deployed goes in `install.html`. A feature nobody knows about may as well not exist, and a site that lags the software is worse than no site because people plan around it. Match the existing voice: plain description of what the thing does, no marketing adjectives, and keep the open-source framing — free of charge, forking encouraged, contributions appreciated.
- **Tests are mandatory** — every feature or fix must include tests
- **Coverage** — 80% is the target; **CI currently gates at the `COVERAGE_FLOOR` in `.github/workflows/backend.yml` (80%)**. Run `task backend:coverage` to check against the floor locally and get a line-by-line HTML report. `cmd/server` (bootstrap) and `internal/testutil` (test helpers) are excluded from the denominator via `COVERAGE_EXCLUDE`. The floor ratchets up as coverage improves — raise it when you raise coverage, and never lower it to turn a red build green. New code must not decrease overall coverage
- **Repository tests need Docker** — `internal/repository/postgres` spins a real Postgres via testcontainers (`TestMain` in `main_test.go`), applies the real goose migrations to it, and tears it down. This means **a migration that cannot apply to a clean database fails the build** — which is how the broken 039 was caught. Use `go test -short` to skip the container when Docker is unavailable
- **Close the issue from the PR** — put `Closes #N` in the PR description so the work and its record land together. Do not mark anything DONE in `docs/IMPLEMENTATION_TASKS.md`; that file is historical
- **Update affected issues** — when implementation reveals something that changes another issue's scope, comment on it in the same session rather than trusting memory
- **File what you find** — a gap discovered mid-implementation becomes a new issue, not a TODO comment or a line in a document

### 5. Post-Implementation Review

After implementation is complete but **before pushing**, launch two agents in parallel:

- **Devil's Advocate Agent** — challenges the solution. Questions design decisions, identifies edge cases that weren't handled, proposes scenarios where the implementation could break, and suggests simpler alternatives. The implementer must address every challenge or explain why the current approach is correct.
- **Code Reviewer Agent** — reviews the actual code for bugs, security issues, performance problems, missing error handling, test coverage gaps, inconsistencies with existing patterns, and adherence to project conventions. Produces a list of findings that must be resolved before pushing.

Both agents run in parallel against the implementation. Findings are triaged by severity:

- **Critical / High / Medium** — must be fixed before pushing. No exceptions.
- **Minor / Nice-to-have** — triage individually. If the fix is a quick win (< 5 minutes, small change), do it now. If it requires significant changes or touches many files, add it as a refined story to `docs/IMPLEMENTATION_TASKS.md` with a clear description and link to the original review finding. Do not block the PR for minor items that are better handled as follow-up work.

### 6. Pre-Push Checklist

Run ALL checks before pushing:

```bash
# Backend
cd backend && go build ./... && go test ./... && go vet ./... && golangci-lint run

# Frontend
cd frontend && npm run build && npm test && npm run lint && npm run format:check
```

**If you changed:**
- Taskfile → verify with `task --list` and run affected tasks
- Dockerfiles → `docker build` must succeed
- CI config → validate YAML syntax, check paths/triggers

**golangci-lint notes:**
- Install: `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest`
- CI uses `golangci-lint-action@v7` (v6 doesn't support v2)
- `errcheck` is the most common failure — every `.Close()`, `.Create()`, or error-returning call must be checked or explicitly discarded with `_ =`

### 7. Push & Create PR

```bash
git push -u origin <branch-name>
```

Create a PR targeting `main`. Check CI status after push and fix failures before moving on.

## Commit Conventions

Write commit messages that explain the **why**, not just the **what**.

**Prefixes:** `feat:`, `fix:`, `refactor:`, `chore:`, `docs:`, `test:`

```
feat: add forum search with full-text matching

PostgreSQL tsvector search on posts and topics enables users to find
past discussions without scrolling through pages of threads.
```

## Security Rules

- **No hardcoded secrets** — use environment variables or 1Password references
- **Validate all external inputs** — request bodies, query params, headers
- **No SQL injection** — use parameterized queries only
- **No log injection** — sanitize user input before logging
- **Never commit `.env` files** — only `.env.example` with placeholders

## Database Migrations (goose)

- **Never edit merged migrations** — once pushed, treat as immutable. Fix issues with a new migration
- **PL/pgSQL requires** `-- +goose StatementBegin` / `-- +goose StatementEnd`
- **Use `IF NOT EXISTS` / `IF EXISTS`** — makes migrations resilient to partial runs
- **Check for numbering collisions** after rebasing parallel branches — renumber if needed
- **Stale versions from branch testing** — when switching branches, goose_db_version may have orphaned entries. Delete stale rows before restarting

## Key References

Read these before starting work on an unfamiliar area:

- `docs/ARCHITECTURE.md` — layered architecture, project boundaries, backend/frontend/migration-tool design
- `docs/IMPLEMENTATION_TASKS.md` — **living backlog** — mark tasks DONE here when completing work
- `docs/FULL_FEATURE_DOCUMENTATION.md` — original TorrentTrader feature specs (porting reference)
- `docs/OPEN_QUESTIONS.md` — architecture decision log (all decisions finalized)
- `docs/NOT_PORTING.md` — original features deliberately dropped, with rationale. Check before "adding" something: it may have been declined already
- `docs/PROPOSED_FEATURES.md` — proposed but **not yet specified** features, with their open questions. Upstream of the backlog: an item moves into IMPLEMENTATION_TASKS.md as a story once its questions are answered, and is deleted here
- `docs/FUTURE_WORK.md` — deferred stories, with what remains
- `docs/TRACKER_MODS.md` — classic private-tracker mods mapped to where each would land in this codebase
- `docs/NOTIFICATION_CONNECTORS.md` — connector framework design (BE-10)
- `docs/EXTENSIBILITY.md` — exploratory notes on a plugin story
- `tasks/todo.md` — session resume context (not the source of truth — use IMPLEMENTATION_TASKS.md)
- `tasks/lessons.md` — **read at session start.** Mistakes already made once, written as rules. Cheaper than rediscovering them

**When a doc disagrees with the code, the code wins — and fix the doc in that same PR.**
A stale doc is not harmless: it gets cited in review and it shapes the next design. The
2026-07-26 alignment pass found a decision log describing a JWT that never existed, a
required deployment secret nothing read, and a backlog story marked DONE whose endpoint
shipped unguarded.

## Project Structure

- `backend/` — Go 1.25, Chi router, goose migrations, pgx, minio-go. API contract in `backend/api/`
- `frontend/` — React 19, Vite 6, TypeScript 5.7, React Router 7
- `migration-tool/` — Go 1.23, Cobra CLI (TorrentTrader data migration)
- `docs/` — architecture, decision log, backlog, and the porting reference (see Key References)
- `tasks/` — `todo.md` session context and `lessons.md` accumulated rules
- `Taskfile.yml` — build orchestration (use `task --list` to see available tasks)

## Key Conventions

- Backend uses `run() int` pattern in main for testability
- Frontend uses `@/` path alias for imports
- All config via environment variables (`.env.example` with placeholders)
- Sessions stored in Redis; dev infra via Docker Compose
- Event system: `event/` (bus + types) → `listener/` (handlers) → `service/` (publishers only)
- Prefer libraries over custom implementations (e.g., zeebo/bencode, not custom)
- Frontend fetch must use `${getConfig().API_URL}` — never relative URLs (they hit the dev server, not backend)
- ESLint flat config — CI may be stricter than local (e.g., `react-hooks/set-state-in-effect`). Always run `npm run lint` before pushing
