# Session Resume Document

## Current State (2026-03-06)

### Branch
- Working on: `feat/phase1-foundation`
- Latest commit: `5c2c527`
- Base: `main` (commit `66b3582`)
- PR not yet created (user doesn't have GitHub API access, will create manually)

### What's Done

**Infrastructure (all on main):**
- INFRA-1: Monorepo scaffolding + Taskfile
- INFRA-2: Docker Compose (Postgres 16, Redis 7, MinIO, Mailpit)
- INFRA-3: Multi-stage Dockerfiles + docker-compose.prod.yml
- INFRA-4: GitHub Actions CI (backend, frontend, migration-tool, release)
- INFRA-5: Dev workflow (`task dev` wires up everything)

**Backend (on feat/phase1-foundation):**
- BE-0.1: Project scaffolding, /healthz, graceful shutdown
- BE-0.2: Config system (env vars, validation, defaults)
- BE-0.3: 10 goose SQL migrations (groups, users, categories, torrents, peers, invites, forums, messages, chat, site tables)
- BE-0.4: Storage abstraction (FileStorage interface, local + S3/MinIO implementations)
- BE-0.6: Chi router, middleware (logger, CORS, recovery), JSON response helpers
- BE-10.1: Replaced custom bencode with `github.com/zeebo/bencode`

**Frontend (on feat/phase1-foundation):**
- FE-0.1: ESLint, Prettier, path aliases (@/)
- FE-0.2: Theme system (CSS tokens, ThemeProvider, light/dark/system)
- FE-0.3: React Router, RootLayout (header/nav/footer), placeholder pages, ProtectedRoute/AdminRoute

**Migration Tool (on feat/phase1-foundation):**
- MT-0.1: Cobra CLI with 5 subcommands (discover, validate, run, verify, rollback)

**BE-1.3: Password Recovery (on feat/password-recovery):**
- [x] Migration: `011_create_password_resets.sql`
- [x] In-memory `PasswordResetStore` with token hash storage
- [x] `SessionStore.DeleteByUserID` for session invalidation
- [x] `AuthService.ForgotPassword` — generic response, rate limit (3/hr), SHA-256 token hash, log reset URL
- [x] `AuthService.ResetPassword` — validate token, update Argon2id password, invalidate sessions
- [x] Handler endpoints: `POST /forgot-password`, `POST /reset-password`
- [x] Routes registered as public (no auth required)
- [x] OpenAPI spec updated with both endpoints + schemas
- [x] Service tests: token generation, rate limiting, reset success, expired/used/invalid tokens, weak password
- [x] Handler tests: generic 200 response, invalid body, invalid token
- [x] All tests pass, go build + go vet clean

### What's Next

Remaining Phase 1 tasks (dependencies met, ready to build):
- **BE-0.5**: Repository layer (interfaces, pgx, transaction support) — depends on BE-0.3 (done)
- **BE-0.7**: Background job system — use `asynq` or `river`, NOT custom
- **FE-0.4**: API client generation (needs backend OpenAPI spec from BE-0.6)

After Phase 1, move to Phase 2 (Core Features):
- BE-1.1/1.2: Auth (registration, login, sessions)
- BE-2.1: HTTP tracker announce
- BE-3.1: Torrent upload
- FE-0.5/0.6: Auth state, shared components
- FE-1.1/1.2/1.3: Home, login/signup, browse pages
- MT-0.2/0.3: Source + target DB connectors

### Key Decisions Made
- Use libraries over custom implementations (bencode, rate limiting, job queue, markdown editor, password hashing)
- UDP tracker (BE-2.5) is OK to build custom — it's core app logic
- Go version is 1.24.0 (dependencies require it, spec said 1.23 but that's outdated)
- golangci-lint v2 with action v7
- Taskfile auto-loads .env via `dotenv: ['.env']`
- Dev containers stay running on Ctrl+C (manual `task dev:down`)

### Known Issues / Notes
- PR needs to be created manually by user (no GitHub API access)
- Frontend ErrorBoundary.tsx exists but isn't wired into the app yet
- CORS middleware uses wildcard `*` origin — needs restricting before auth phase
- `task dev` requires `cp .env.example .env` first

### Workflow Rules (from memory)
- Every builder agent gets a devil's advocate reviewer
- Fix all reviewer findings before committing
- Verify everything works after changes (build, test, lint, docker)
- Prefer libraries over custom implementations
- All code must have tests
- Progress tracked in `docs/IMPLEMENTATION_TASKS.md` with [DONE] markers
