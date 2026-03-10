# Architecture Decision Log — TorrentTrader 3.0 Go Port

This document records technology decisions made for the project. All decisions are final unless revisited explicitly.

---

## Backend

### 1. HTTP Router/Framework

**Decision:** Chi — lightweight, idiomatic, `net/http` compatible middleware.

### 2. ORM / Query Builder

**Decision:** Raw SQL with pgx driver — no ORM. Repositories write parameterized queries directly. Keeps full control over SQL and avoids ORM magic.

### 3. Database Migration Tool

**Decision:** goose — embeddable, supports Go migration functions, used throughout the project.

### 4. Authentication

**Decision:** Hybrid — short-lived JWT access tokens + Redis-backed sessions for revocation and persistence. `SessionStore` is an interface (memory for tests, Redis for production).

### 5. Background Job Processing

**Decision:** Asynq (Redis-based) — persistent queues, retries, scheduler with `ENABLE_SCHEDULER` env toggle, unique task dedup.

### 6. Search

**Decision:** PostgreSQL full-text search — tsvector with prefix matching (`:*` operator). No extra infrastructure needed at current scale.

### 7. File Storage

**Decision:** MinIO (S3-compatible) — self-hosted via Docker Compose in dev, S3-compatible API for production flexibility.

### 8. Real-time Communication

**Decision:** WebSocket — used for chat (shoutbox), real-time private messages, and future notifications. gorilla/websocket with write pump pattern (single writer goroutine per client).

### 9. API Documentation

**Decision:** None currently — no OpenAPI spec generation. Frontend API client is hand-written using openapi-fetch with typed routes.

### 10. Rate Limiting

**Decision:** In-memory (`golang.org/x/time/rate`) — per-instance, sufficient for single-node deployment. WebSocket has additional per-client rate limiting (10 msgs/10s).

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
