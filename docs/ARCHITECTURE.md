# Architecture

This document describes the architecture of the TorrentTrader 3.0 port — a modern rewrite of the classic PHP-based private tracker, rebuilt as a Go backend with a React frontend.

## Monorepo Structure

The project is organized as a monorepo containing three independent projects:

```
go-torrent-trader/
├── backend/                    # Go API server
│   ├── cmd/server/main.go
│   ├── internal/
│   │   ├── config/
│   │   ├── handler/
│   │   ├── middleware/
│   │   ├── model/
│   │   ├── repository/
│   │   ├── service/
│   │   ├── tracker/
│   │   └── worker/
│   ├── migrations/
│   ├── go.mod
│   └── Dockerfile
├── frontend/                   # React SPA
│   ├── src/
│   │   ├── api/               # Generated types + openapi-fetch client
│   │   ├── components/
│   │   ├── features/          # only auth/ today — see note below
│   │   ├── pages/             # where most screens actually live
│   │   ├── lib/               # providers and hooks (Auth, Chat, Theme, Toast)
│   │   ├── layouts/
│   │   ├── types/
│   │   ├── utils/
│   │   └── App.tsx
│   ├── package.json
│   ├── vite.config.ts
│   └── Dockerfile
├── migration-tool/             # Standalone Go CLI
│   ├── cmd/migrate/main.go
│   ├── internal/
│   │   ├── source/
│   │   ├── target/
│   │   ├── transform/
│   │   └── verify/
│   ├── go.mod
│   └── Dockerfile
├── docs/
├── .github/workflows/
├── docker-compose.yml
├── docker-compose.prod.yml
├── Taskfile.yml
├── .gitignore
├── .env.example
└── README.md
```

**Why a monorepo?** A single repository allows atomic changes across projects (e.g., updating an API endpoint and its frontend consumer in one commit), shared CI configuration, and simplified code review. Each project still builds and deploys independently — the monorepo is an organizational choice, not a coupling mechanism.

## Project Boundaries

- **Shared Nothing**: Each project has its own `go.mod` or `package.json`. There are no shared Go packages between `backend/` and `migration-tool/`. This keeps dependency trees independent and avoids accidental coupling.
- **API Contract**: `backend/api/openapi.yaml` is the hand-maintained contract. The frontend generates **TypeScript types** from it (`openapi-typescript` → `src/api/schema.d.ts`) and a typed `openapi-fetch` client wraps them. A second document, `backend/api/openapi.public.yaml`, is generated from the first and contains only the member-facing surface — that is the one intended for publication. See decision 9 in `OPEN_QUESTIONS.md`.
  - **Honest caveat:** most frontend calls are still raw `fetch` against string URLs rather than the typed client, so the types are advisory in much of the app. Narrowing that gap is tracked in `IMPLEMENTATION_TASKS.md`; until it closes, do not assume a route change breaks the frontend build.
- **Database**: The backend owns the schema and migrations. The migration-tool reads from the source database (MySQL, the legacy TorrentTrader DB) and writes to the target database (PostgreSQL) but uses its own connection logic, completely independent of the backend's data access layer.

## Backend Architecture

The backend follows a **layered architecture**: `handler → service → repository`.

| Layer | Responsibility |
|---|---|
| **Handlers** | HTTP layer — request parsing, response formatting, route definitions |
| **Services** | Business logic, validation, cross-entity orchestration |
| **Repositories** | Data access, SQL queries, database interaction |
| **Models** | Domain types shared across layers |

Additional components:

- **Middleware**: Auth (opaque Bearer token resolved against a Redis session), request logging, activity tracking, CORS. Applied at the router level. **There is no general rate-limiting middleware** — limiting is per-surface (login attempts, WebSocket messages, connector sends, SSE stream count). See decision 10.
- **Tracker**: BitTorrent announce/scrape handler. This is a separate HTTP handler from the REST API — it speaks the BitTorrent tracker protocol and is performance-critical.
- **Workers**: Background jobs for periodic tasks like cleanup (expired tokens, dead peers), stats aggregation, and email dispatch.

## Frontend Architecture

- **Page-based structure, with one feature module**: the app is organized around `pages/` (plus shared `components/` and providers in `lib/`). `features/` exists but contains only `auth/`. The feature-based layout below was the original intent and was never carried through — treat `pages/` as the convention when adding screens, or migrate deliberately rather than half-adopting both.
- **Theme system**: CSS variables managed by a `ThemeProvider`. Multi-theme switching (FE-7.1–7.3) is deferred — see `docs/FUTURE_WORK.md`.
- **Generated API types**: `openapi-typescript` produces `src/api/schema.d.ts` from the backend spec, consumed through `openapi-fetch`. Types only — there are no request interceptors, and auth headers are passed per call.
- **Routing**: Config-based routes with layout wrappers and auth guards. Protected routes redirect to login; admin routes check role permissions.

## Migration Tool Architecture

The migration tool converts a legacy TorrentTrader 3.x MySQL database into the new PostgreSQL schema. It follows a **pipeline architecture**:

```
Source Reader → Transformer → Target Writer
```

- Each table/entity type has its own transformer that handles schema differences, data cleaning, and type conversions.
- **Resumable**: The tool checkpoints after each entity type completes, so a failed migration can be restarted without re-processing already-migrated data.
- **Verification**: A separate post-migration phase that compares record counts, validates referential integrity, and spot-checks data correctness.

## Build Tooling

**[Taskfile](https://taskfile.dev)** is the task runner, chosen over Make for its YAML syntax, cross-platform support, built-in dependency tracking, and watch mode.

Key tasks:

| Command | Scope |
|---|---|
| `task build` / `task test` / `task lint` | All projects |
| `task backend:build` / `task frontend:build` / `task migration-tool:build` | Per-project |
| `task dev` | Starts docker-compose + hot reload for backend (air) + frontend (vite) |
| `task docker:build` | Builds all Docker images |
| `task generate` | Regenerates the public OpenAPI spec from the full one, then the frontend's TypeScript types from the full spec |

**Why not Bazel?** Bazel's benefits (hermetic builds, remote caching) aren't needed at this scale. For a 3-project monorepo, the overhead of maintaining Bazel BUILD files far outweighs the gains. Taskfile provides everything needed with minimal configuration.

## Docker Setup

- **Development** (`docker-compose.yml`): Runs infrastructure services only — PostgreSQL 16, Redis 7, MinIO (S3-compatible object storage; **`.torrent` files only** — avatars are external URLs, there is no upload endpoint), and Mailpit (email testing). The backend and frontend run on the host with hot reload (air and vite respectively) for fast iteration.
- **Production** (`docker-compose.prod.yml`): Multi-stage Dockerfiles for all 3 projects produce minimal images. The backend runs behind nginx, which also serves the frontend's static files. Redis handles session storage and caching.

## Conventions

- Go code follows the [standard project layout](https://github.com/golang-standards/project-layout) (`cmd/`, `internal/`)
- Frontend is page-based (`pages/`), with shared components and providers in `components/` and `lib/`
- All configuration via environment variables (12-factor app)
- Structured logging: `slog` for Go, browser console for frontend dev
- `backend/api/openapi.yaml` is the hand-maintained API contract; **a new route must be documented and classified `x-audience: public | internal` in the same PR that adds it**, and a guard test enforces this
- Database migrations are forward-only, numbered sequentially

> **Reading this document critically.** As of 2026-07-26 it described a JWT, a generated
> API client, a `sqlc` step, Mailhog, and a feature-based frontend — none of which exist.
> Those claims are corrected above. When this file and the code disagree, the code wins;
> fix the file in the same PR rather than working around it.
