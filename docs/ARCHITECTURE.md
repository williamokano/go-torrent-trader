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
│   │   ├── api/               # Generated API client
│   │   ├── components/
│   │   ├── features/
│   │   │   ├── auth/
│   │   │   ├── torrents/
│   │   │   ├── forums/
│   │   │   ├── chat/
│   │   │   ├── messages/
│   │   │   ├── admin/
│   │   │   └── user/
│   │   ├── hooks/
│   │   ├── layouts/
│   │   ├── themes/
│   │   ├── routes/
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
- **API Contract**: The OpenAPI spec is the contract between backend and frontend. The frontend generates its API client from this spec — API calls are never hand-written.
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

- **Middleware**: Auth (JWT), logging, rate limiting, CORS. Applied at the router level.
- **Tracker**: BitTorrent announce/scrape handler. This is a separate HTTP handler from the REST API — it speaks the BitTorrent tracker protocol and is performance-critical.
- **Workers**: Background jobs for periodic tasks like cleanup (expired tokens, dead peers), stats aggregation, and email dispatch.

## Frontend Architecture

- **Feature-based structure**: Each feature module (`auth/`, `torrents/`, `forums/`, etc.) contains its own components, hooks, and API calls. This keeps related code colocated rather than scattered across `components/`, `hooks/`, and `api/` directories.
- **Theme system**: CSS variables managed by a `ThemeProvider`. Supports multiple themes (light, dark, classic TorrentTrader). User preference is persisted and applied on load.
- **Generated API client**: Auto-generated from the backend's OpenAPI spec. This eliminates hand-written fetch calls and keeps the frontend in sync with the API contract.
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
| `task generate` | Runs all code generation (OpenAPI client, sqlc, etc.) |

**Why not Bazel?** Bazel's benefits (hermetic builds, remote caching) aren't needed at this scale. For a 3-project monorepo, the overhead of maintaining Bazel BUILD files far outweighs the gains. Taskfile provides everything needed with minimal configuration.

## Docker Setup

- **Development** (`docker-compose.yml`): Runs infrastructure services only — PostgreSQL 16, Redis 7, MinIO (S3-compatible object storage for torrents/avatars), and Mailhog (email testing). The backend and frontend run on the host with hot reload (air and vite respectively) for fast iteration.
- **Production** (`docker-compose.prod.yml`): Multi-stage Dockerfiles for all 3 projects produce minimal images. The backend runs behind nginx, which also serves the frontend's static files. Redis handles session storage and caching.

## Conventions

- Go code follows the [standard project layout](https://github.com/golang-standards/project-layout) (`cmd/`, `internal/`)
- Frontend follows feature-based organization
- All configuration via environment variables (12-factor app)
- Structured logging: `slog` for Go, browser console for frontend dev
- OpenAPI spec is the source of truth for the API contract
- Database migrations are forward-only, numbered sequentially
