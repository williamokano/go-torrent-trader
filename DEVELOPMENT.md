# Development

Everything you need to work on TorrentTrader. If you only want to *run* a tracker,
the [README](README.md) and the [project site](https://williamokano.github.io/go-torrent-trader/)
are the right places.

## Prerequisites

| Tool | Version | Why |
|---|---|---|
| Go | 1.25+ | backend and migration tool |
| Node.js | 22+ | frontend |
| Docker | any recent | Postgres, Redis, MinIO, Mailpit — and the repository tests |
| [Task](https://taskfile.dev/installation/) | 3+ | build orchestration |

Docker is not optional for the full test suite: the Postgres repository tests spin a
real database with testcontainers and apply the real migrations to it. Use
`go test -short` to skip them when Docker is unavailable.

## Getting started

```bash
git clone https://github.com/williamokano/go-torrent-trader.git
cd go-torrent-trader
cp .env.example .env

task tools              # golangci-lint, air
task frontend:install
task dev:up             # Postgres, Redis, MinIO, Mailpit
task dev                # backend + frontend, both hot-reloading
```

| Service | URL |
|---|---|
| Frontend | http://localhost:5173 |
| Backend | http://localhost:8080 |
| Mailpit | http://localhost:8025 |
| MinIO console | http://localhost:9001 |

`task dev:reset` tears the infrastructure down including volumes, for a clean slate.
`task --list` shows everything else.

## Layout

```
backend/          Go API server and tracker
  api/            OpenAPI contract — see below
  cmd/server/     entry point
  internal/
    handler/      HTTP layer: parsing, responses, routing
    service/      business logic, validation, orchestration
    repository/   SQL
    model/        domain types shared across layers
    connector/    outbound announcement framework
    event/        in-process event bus and types
    listener/     event handlers (services publish, listeners react)
    worker/       asynq background jobs
  migrations/     goose, forward-only, numbered

frontend/         React 19 + Vite + TypeScript
  src/pages/      screens (the convention — see caveat below)
  src/components/ shared UI
  src/lib/        providers and hooks
  src/api/        generated types + openapi-fetch client

migration-tool/   standalone Cobra CLI, its own Go module
website/          the public GitHub Pages site
docs/             architecture, decisions, and history
```

**Frontend caveat:** `src/features/` exists but only contains `auth/`. The app is
page-based in practice. Add screens under `pages/` rather than half-adopting both
conventions.

## Architecture in one paragraph

The backend is layered — `handler → service → repository` — with `model` shared
across all three. Handlers do HTTP and nothing else; services own business logic and
cross-entity orchestration; repositories own SQL. The tracker's announce/scrape
handler is deliberately separate from the REST API because it is performance
critical. Services **publish** events; all reaction logic lives in `listener/`. The
event bus is in-process and **synchronous**, so anything expensive in a listener
needs a goroutine or a queued job.

Full detail, including why each choice was made, is in
[`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) and
[`docs/OPEN_QUESTIONS.md`](docs/OPEN_QUESTIONS.md).

## The API contract

`backend/api/openapi.yaml` is the **full** spec — every endpoint, including admin —
and it is hand-maintained. `backend/api/openapi.public.yaml` is **generated** from it
and contains only the member-facing surface; that is the document published to
third parties.

Every operation carries `x-audience: public | internal`. When you add a route:

1. Document it in `openapi.yaml` with an `x-audience` marker.
2. Run `task generate` — this regenerates the public spec and the frontend types.
3. Commit both.

A guard test walks the real router and fails the build if a route is neither
documented nor listed in the shrinking debt ledger at
`backend/internal/handler/openapi_undocumented.go`. **Never add a new route to that
ledger** — it exists to record pre-existing debt and may only get smaller.

Classification is per-operation, not per-path, because several staff-only endpoints
live on member-looking paths where authorization happens in the service layer.

## Testing

```bash
task test                # everything
task backend:coverage    # coverage against the CI floor, plus an HTML report
```

Tests are mandatory. Handler, service and repository layers each want their own
suite. CI gates on **80% overall** backend coverage (`COVERAGE_FLOOR` in
`.github/workflows/backend.yml`), with `cmd/server`, `internal/testutil` and the
spec generator excluded from the denominator. The floor ratchets up as coverage
improves — raise it when you raise coverage, and never lower it to turn a build
green.

[`tasks/lessons.md`](tasks/lessons.md) is worth reading before you write tests here.
It records real failures from this codebase: authorization that failed open when a
dependency was nil, a mock that failed differently from production and hid a data
leak, and a struct-writing insert that silently defeated a column default.

## Before you push

```bash
cd backend  && go build ./... && go test ./... && go vet ./... && golangci-lint run
cd frontend && npm run build && npm test && npm run lint && npm run format:check
```

`errcheck` is the most common lint failure — every error-returning call must be
handled or explicitly discarded with `_ =`.

## Database migrations

goose, forward-only, numbered sequentially. **A merged migration is immutable** —
fix problems with a new one. Use `IF NOT EXISTS` / `IF EXISTS` so a partial run can
converge. PL/pgSQL needs `-- +goose StatementBegin` / `StatementEnd`.

Read the header comment on migration `039` before writing your first one; it
documents a fresh-install break caused by `CREATE TABLE IF NOT EXISTS` silently
skipping, and the convergence pattern that fixes it.

Because the repository tests apply the real migrations to a real database, **a
migration that cannot apply to a clean database fails the build.** That is by
design.

## How work is tracked

**[GitHub Issues](https://github.com/williamokano/go-torrent-trader/issues) is the
source of truth.** Milestones group workstreams; parent issues with sub-issues act
as epics. Work starts from an issue, and the pull request closes it with `Closes #N`.

Do not record work in Markdown. `docs/IMPLEMENTATION_TASKS.md`,
`docs/PROPOSED_FEATURES.md` and `docs/FUTURE_WORK.md` are frozen history from before
this rule. Every real item in them has been migrated to an issue; what remains is the
build log of the port and the reasoning behind the proposals, which is worth reading
and not worth updating.

Task state in a document drifts across branches and merges, and this project has been
bitten by exactly that: a story sat marked done over three unimplemented criteria and
an endpoint with no authorization check, and two other stories were marked
"deferred" for work that had quietly shipped elsewhere months earlier.

## Conventions

- **Ship backend and frontend together.** An endpoint without its UI is not a
  feature. Once the CLI exists, this becomes backend + frontend + CLI.
- **Update the website** when you ship something a user would notice — see
  `website/`. A feature nobody knows about may as well not exist.
- **Branch from an issue**, named `feat/`, `fix/`, `refactor/`, `chore/`, `docs/` or
  `test/`.
- **Commit messages explain why**, not what.
- **No hardcoded secrets.** Validate every external input. Parameterized queries
  only. Sanitize user input before logging.
- **Frontend fetches must use `${getConfig().API_URL}`** — a relative URL hits the
  dev server, not the backend.
- **When a document disagrees with the code, the code wins** — and fix the document
  in the same pull request.

## Reference documents

These describe what the system *is* and *why*. They stay current.

| Document | What it is for |
|---|---|
| [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) | layering, boundaries, conventions |
| [`docs/OPEN_QUESTIONS.md`](docs/OPEN_QUESTIONS.md) | the decision log, with corrections |
| [`docs/NOT_PORTING.md`](docs/NOT_PORTING.md) | what was deliberately dropped, and why |
| [`docs/FULL_FEATURE_DOCUMENTATION.md`](docs/FULL_FEATURE_DOCUMENTATION.md) | the original TorrentTrader spec |
| [`docs/NOTIFICATION_CONNECTORS.md`](docs/NOTIFICATION_CONNECTORS.md) | connector framework design |
| [`docs/TRACKER_MODS.md`](docs/TRACKER_MODS.md) | classic tracker mods, mapped to this codebase |
| [`tasks/lessons.md`](tasks/lessons.md) | mistakes already made once, written as rules |

Check `NOT_PORTING.md` before adding something that feels missing. It may have been
declined on purpose, with reasoning worth reading first.
