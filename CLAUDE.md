# go-torrent-trader Development Guidelines

## Agent Development Flow

```
Receive task → Create branch → Write spec → Implement → Push → Pipeline validates → Merge
```

### 1. Receive Task

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
cd /Users/williamokano/Workspace/Personal/go-torrent-trader
git worktree remove ../go-torrent-trader-<branch-name>
```

**Branch naming:**
- `feat/add-forum-search` — new feature
- `fix/null-pointer-in-router` — bug fix
- `refactor/extract-common-metrics` — refactoring
- `chore/update-dependencies` — maintenance
- `docs/update-guardrails` — documentation

**Worktree conflict warning:** Parallel worktrees WILL conflict on shared files like `main.go`, `router.go`, and repository interfaces. When running multiple tracks in parallel, merge Track A first, then rebase Track B before pushing. Never push without running tests after a rebase.

### 3. Write Spec (for non-trivial tasks)

Before implementing, document the approach:
- What changes are needed and why
- Which files will be modified
- What tests will be added
- Any architectural decisions

### 4. Implement

- **Tests are mandatory** — every feature or fix must include tests
- **Coverage >= 80%** — CI gates at 80%. New code must not decrease overall coverage
- **Mark the story as DONE in `docs/IMPLEMENTATION_TASKS.md`** — every PR must update the backlog
- **Update affected stories** — when implementation reveals new insights, update related stories in the same PR
- **Continuously refine the backlog** — findings during implementation feed back into upcoming stories

### 5. Pre-Push Checklist

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

### 6. Push & Create PR

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
- `tasks/todo.md` — session resume context (not the source of truth — use IMPLEMENTATION_TASKS.md)

## Project Structure

- `backend/` — Go 1.24, Chi router, goose migrations, pgx, minio-go
- `frontend/` — React 19, Vite 6, TypeScript 5.7, React Router 7
- `migration-tool/` — Go 1.23, Cobra CLI (TorrentTrader data migration)
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
