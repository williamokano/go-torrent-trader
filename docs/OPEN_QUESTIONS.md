# Architecture Decision Log — TorrentTrader 3.0 Go Port

This document records technology decisions made for the project. All decisions are final unless revisited explicitly.

> **Accuracy pass, 2026-07-26.** Decisions 4 (authentication), 9 (API documentation) and
> 10 (rate limiting) described things the codebase never contained — a JWT, a generated
> spec, and a global rate-limit middleware respectively. Each has been corrected against
> the code, with the original wording and what actually shipped both recorded, so the
> correction is auditable rather than silent. A decision log that disagrees with the source
> is worse than no decision log: it gets cited.

---

## Backend

### 1. HTTP Router/Framework

**Decision:** Chi — lightweight, idiomatic, `net/http` compatible middleware.

### 2. ORM / Query Builder

**Decision:** Raw SQL with pgx driver — no ORM. Repositories write parameterized queries directly. Keeps full control over SQL and avoids ORM magic.

### 3. Database Migration Tool

**Decision:** goose — embeddable, supports Go migration functions, used throughout the project.

### 4. Authentication

**Decision:** Short-lived **opaque** access tokens + Redis-backed sessions for revocation
and persistence. `SessionStore` is an interface (memory for tests, Redis for production).

Access tokens are 64-character hex strings with a 1-hour TTL; refresh tokens are the same
shape with a 30-day TTL and rotate on use. The token carries no claims — every request
resolves it against Redis, which is what makes instant revocation possible.

**Corrected 2026-07-26:** this decision previously read "short-lived **JWT** access
tokens". No JWT was ever implemented — there is no JWT library in `go.mod` and no `jwt`
reference anywhere in the backend. The `JWT_SECRET` environment variable that this wording
produced was read by no code and has been removed from `.env.example`, `stack.env.example`,
`docker-compose.stack.yml` and the README's required-variables table. Opaque-plus-Redis is
the better fit for this project anyway: a stateless JWT cannot be revoked before it expires,
which a tracker with bans and quick-ban tooling actually needs.

### 5. Background Job Processing

**Decision:** Asynq (Redis-based) — persistent queues, retries, scheduler with `ENABLE_SCHEDULER` env toggle, unique task dedup.

### 6. Search

**Decision:** PostgreSQL full-text search — tsvector with prefix matching (`:*` operator). No extra infrastructure needed at current scale.

### 7. File Storage

**Decision:** MinIO (S3-compatible) — self-hosted via Docker Compose in dev, S3-compatible API for production flexibility.

### 8. Real-time Communication

**Decision:** WebSocket — used for chat (shoutbox), real-time private messages, and future notifications. gorilla/websocket with write pump pattern (single writer goroutine per client).

### 9. API Documentation

**Decision:** **Two OpenAPI documents, one hand-maintained source.**

- `backend/api/openapi.yaml` — the **full** spec. Every endpoint, including admin and
  staff routes. This is the source of truth and the file humans edit. The frontend
  generates its TypeScript types from it (`openapi-typescript` → `frontend/src/api/schema.d.ts`),
  which is why it must include the admin surface.
- `backend/api/openapi.public.yaml` — **generated** from the full spec by
  `cmd/openapi-public`, containing only what a member or third-party integrator may call.
  This is the document intended for publication.

Every operation in the full spec carries an `x-audience: public | internal` extension.
The generator drops `internal` operations, prunes the component schemas and security
schemes they alone referenced, and strips the extension from its output. A guard test in
`internal/handler` walks the real Chi router and fails the build when a registered route
is neither documented nor listed in an explicit, shrinking debt ledger — so a new endpoint
cannot be added without being classified.

**Why two documents rather than one.** The web UI is a convenience, not the only client:
members are welcome to drive the API directly or build their own tools against it. The
administrative surface is deliberately not advertised as a supported interface. The project
is open source and those routes are discoverable from the source — "findable in the code"
and "published as a contract" are different commitments, and only the second one implies
stability.

**Corrected 2026-07-26:** this decision previously read "None currently — no OpenAPI spec
generation", while `ARCHITECTURE.md` simultaneously claimed the spec was the source of truth
and that API calls were never hand-written. Both were wrong. A hand-written spec existed and
covered roughly 15% of the ~189 registered routes, and most frontend calls are raw `fetch`.
See `IMPLEMENTATION_TASKS.md` for the story that closes the remaining gap.

### 10. Rate Limiting

**Decision:** Applied per-surface rather than as one global middleware.

- **Login attempts** — capped in `service/auth.go` (5 failures per 15 minutes per IP).
- **WebSocket chat** — per-client limiting (10 messages / 10 seconds) in `handler/chat_ws.go`,
  plus the `chat_mutes` table and anti-spam settings for sustained abuse.
- **Connectors** — per-instance `rate_per_min` with burst coalescing.
- **Live feeds (SSE)** — a per-user concurrent stream cap.

**Corrected 2026-07-26:** this decision previously read "In-memory (`golang.org/x/time/rate`)
— per-instance". No such middleware exists: `internal/middleware/` contains only auth,
activity, CORS and logging, and `golang.org/x/time` is an indirect dependency with no
`rate.Limiter` usage. A general per-IP HTTP limiter remains **unbuilt**; see
`docs/FUTURE_WORK.md`. Anything relying on "requests are rate limited" as an existing
guarantee — notably any anti-abuse proposal — must treat it as work to do, not a given.

### 10a. Time and Timezones

**Decision:** The server is **UTC, always**. There is no site timezone setting and
no per-user timezone preference. Every timestamp is stored as `TIMESTAMPTZ` and
served in UTC; the browser formats it into the reader's local time, which is what
the frontend already does throughout.

Original TorrentTrader had both a site timezone and a per-user timezone. That
parity is **deliberately not ported**. For anything scheduled — a freeleech
window, a contest, a double-bonus weekend — the maintainer decides what "the
weekend" means for their community and configures the window in UTC. On a site
with a global membership there is no correct per-user answer, and making the
schedule depend on a user-controlled field opens an obvious abuse door: a member
could shift their own timezone to enter or extend a window.

Display stays local because that is a pure rendering concern with no accounting
consequence. Scheduling stays UTC because it has one.

---

## Frontend

### 11. State Management

**Decision:** React Context + hooks — `AuthProvider`, `ChatProvider`, `ThemeProvider`, `ToastProvider`. No external state management library.

### 12. CSS Approach

**Decision:** Plain CSS with CSS variables — theme system via `ThemeProvider`, CSS custom properties for theming. No Tailwind or CSS-in-JS.

### 13. Form Handling

**Decision:** Native controlled components — no form library. Simple forms don't justify the dependency.

### 14. UI Component Library

**Decision:** Built from scratch — custom components (`MarkdownRenderer`, `UsernameDisplay`, `ConfirmModal`, etc.). No component library dependency.

### 15. Rich Text Editor

**Decision:** Plain textarea with Markdown — consistent with the rest of the app. No BBCode, no WYSIWYG editor.

---

## Migration Tool

### 16. CLI Framework

**Decision:** Cobra — industry standard, used for the `migration-tool/` CLI.

### 17. Migration Strategy

**Decision:** Table-by-table with transformers — `source/` reads legacy MySQL, `transform/` converts data (BBCode→Markdown, schema mapping), `target/` writes to PostgreSQL.

---

## Infrastructure

### 18. Container Orchestration (Production)

**Decision:** Docker Compose + Portainer — `docker-compose.stack.yml` for single-node deployment. Kubernetes deferred until scaling demands it.

### 19. CI/CD Platform

**Decision:** GitHub Actions — CI runs lint, test, build for both backend and frontend. Release workflow triggered on tags.

### 20. Monitoring / Observability

**Decision:** Deferred — not yet implemented. Structured logging via Go's `slog` is in place.

### 21. Log Aggregation

**Decision:** stdout/stderr — 12-factor app compliant. No external log aggregation yet.
