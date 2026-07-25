# Reimplementation Task Breakdown (Monorepo)

> Each story is independently implementable and testable.
> Stories are ordered within each epic by dependency (build bottom-up).
> Estimates are T-shirt sizes: S (1-2 days), M (3-5 days), L (1-2 weeks).
>
> Stories are organized by project area: Infrastructure, Backend, Frontend, Migration Tool.
> See `ARCHITECTURE.md` for monorepo structure and conventions.
> See `NOT_PORTING.md` for features explicitly excluded.

## Development Standards

### Test Coverage
- **Minimum 80% coverage** per package is required. CI gates on this threshold.
- All new code must ship with tests. No exceptions — if it's not tested, it doesn't ship.
- New PRs must not decrease overall coverage.
- Backend: `go test -coverprofile=coverage.out ./...` — check with `go tool cover -func=coverage.out`
- Frontend: `npm test -- --coverage` — check summary output
- Handler, service, and repository layers must all have dedicated test suites.

### Code Quality
- All code must pass linting before merge: `golangci-lint run` (backend), `npm run lint` (frontend)
- Activity log messages must be self-contained: include WHO (actor username), WHAT (target name, not IDs), and the ACTION. Never leak sensitive data (PM content, passwords, IPs, emails).
- Migrations that have been merged to main are immutable — fix issues with a new migration.
- Features ship in BE+FE pairs — both backend and frontend must be included for a feature to be considered complete.

---

## Phase Overview

| Phase | Focus | Ships |
|-------|-------|-------|
| 1 | Foundation | Monorepo scaffolding, dev environment, backend foundation, frontend foundation, migration CLI scaffold |
| 2 | Core Features | Auth, tracker, torrent management, public frontend pages, data transformers |
| 3 | Community | Forum, chat, PMs, invites, user pages, real-time features |
| 4 | Admin & Polish | Admin panel, moderation, migration verification, admin frontend |
| 5 | Advanced | UDP tracker, additional themes, static pages, polish |

---

## Infrastructure Epics (INFRA-)

### INFRA-1: Monorepo Scaffolding + Taskfile [S] [DONE]
**As a** developer
**I want** a monorepo structure with build orchestration
**So that** all three projects are organized, buildable, and testable from a single repo

**Acceptance Criteria:**
- Directory structure: `backend/`, `frontend/`, `migration-tool/`, `docs/`
- `Taskfile.yml` with tasks: `build`, `test`, `lint`, `dev`, `docker:build`, `generate`
- Per-project tasks: `task backend:build`, `task frontend:build`, `task migration-tool:build`
- `.gitignore` covering Go, Node.js, Docker, IDE files
- `README.md` with quickstart instructions
- `.env.example` with all required config vars (placeholder values only)

### INFRA-2: Docker Compose Dev Environment [S] [DONE]
**As a** developer
**I want** a local dev environment with all dependencies
**So that** I can develop without installing services manually

**Acceptance Criteria:**
- `docker-compose.yml` with: PostgreSQL 16, Redis 7, MinIO (S3-compatible), Mailhog (SMTP)
- Health checks on all services
- Named volumes for data persistence
- Port mappings documented in `.env.example`
- `task dev:up` and `task dev:down` tasks
- Backend and frontend run on host (not in containers) for hot reload

### INFRA-3: Dockerfiles (Multi-Stage) [S] [DONE]
**As a** developer
**I want** production-ready Docker images for all projects
**So that** deployments are reproducible and images are small

**Acceptance Criteria:**
- Backend: multi-stage (build with Go, run with distroless/alpine), < 50MB
- Frontend: multi-stage (build with Node, serve with nginx), < 30MB
- Migration Tool: multi-stage (build with Go, run with distroless/alpine), < 50MB
- `docker-compose.prod.yml` for production-like local testing
- All images tagged with git SHA

### INFRA-4: GitHub Actions CI [M] [DONE]
**As a** developer
**I want** CI pipelines that validate all projects on every push
**So that** broken code doesn't reach main

**Acceptance Criteria:**
- Separate workflow files: `backend.yml`, `frontend.yml`, `migration-tool.yml`, `release.yml`
- Path-based triggers (backend workflow only runs on `backend/**` changes)
- Pipeline order: lint -> test -> build
- Go module caching, node_modules caching
- Backend: `golangci-lint`, `go test -race`, `go build`
- Frontend: `eslint`, `tsc --noEmit`, `vitest`, `vite build`
- Migration Tool: `golangci-lint`, `go test -race`, `go build`
- Release workflow: build Docker images, push to registry on tag

### INFRA-5: Dev Workflow [S] [DONE]
**As a** developer
**I want** hot reload and code generation in development
**So that** I get fast feedback loops

**Acceptance Criteria:**
- Backend hot reload with `air` (rebuild on Go file changes)
- Frontend hot reload with Vite HMR
- `task generate` runs all code generation (OpenAPI client, sqlc, etc.)
- `task dev` starts docker-compose + backend + frontend with hot reload
- Pre-commit hooks: lint + format check (optional, via lefthook or similar)

---

## Backend Epics (BE-)

### Epic BE-0: Foundation

#### BE-0.1: Project Scaffolding [S] [DONE]
**As a** developer
**I want** a Go project with module structure and dev tooling
**So that** I have a working development environment from day one

**Acceptance Criteria:**
- `backend/cmd/server/main.go` entry point
- `backend/go.mod` with module path
- `task backend:run` starts the application
- `task backend:test` runs tests
- Hot reload with air
- Linter configured (golangci-lint)

#### BE-0.2: Configuration System [S] [DONE]
**As a** developer
**I want** a typed configuration loaded from environment variables with validation
**So that** all settings are centralized, documented, and fail fast on misconfiguration

**Acceptance Criteria:**
- Struct-based config with env tags
- Validation on startup (required fields, value ranges)
- Sensible defaults matching the reference implementation
- Covers: site info, DB, Redis, SMTP, tracker settings, feature flags
- No config stored in database (unlike original) - use env vars only

#### BE-0.3: Database Schema & Migrations [M] [DONE]
**As a** developer
**I want** a migration system with the initial schema using PostgreSQL with proper foreign keys, indexes, and constraints
**So that** the data model is correct, enforced, and versioned

**Acceptance Criteria:**
- Migration tool integrated (golang-migrate, goose, or atlas)
- All tables from reference created with:
  - Proper foreign keys (not implicit like original)
  - UUID or BIGINT primary keys
  - `created_at` / `updated_at` timestamps on all tables
  - ENUM types or check constraints for status fields
- Junction table `user_invites` replaces space-separated invitees column
- `password_scheme` column on users table
- Seed data for: groups, categories, countries, languages, default admin user

#### BE-0.4: Storage Abstraction Layer [M] [DONE]
**As a** developer
**I want** a file storage interface that supports local disk and S3-compatible backends
**So that** the application can run as multiple instances behind a load balancer

**Acceptance Criteria:**
- `FileStorage` interface with methods: `Put(ctx, key, reader)`, `Get(ctx, key) reader`, `Delete(ctx, key)`, `Exists(ctx, key) bool`, `URL(ctx, key) string`
- Local disk implementation (for development)
- S3-compatible implementation (MinIO, AWS S3, Backblaze B2, etc.)
- Used for: .torrent files, NFO files, torrent images, database backups
- Configurable via env: `STORAGE_TYPE=local|s3`, `S3_ENDPOINT`, `S3_BUCKET`, `S3_ACCESS_KEY`, `S3_SECRET_KEY`
- Bucket/prefix organization: `torrents/{id}.torrent`, `nfo/{id}.nfo`, `images/{id}_{n}.{ext}`
- No file paths hardcoded anywhere in business logic

#### BE-0.5: Repository Layer [M] [DONE]
**As a** developer
**I want** a repository pattern with interfaces for all data access
**So that** business logic is decoupled from database implementation

**Acceptance Criteria:**
- Interface per aggregate (UserRepo, TorrentRepo, PeerRepo, ForumRepo, etc.)
- PostgreSQL implementations using sqlx or pgx
- Context-aware (accept `context.Context`)
- Transaction support (begin/commit/rollback helper)
- Query builder or raw SQL (no ORM magic)

#### BE-0.6: HTTP Router & Middleware Stack [S] [DONE]
**As a** developer
**I want** an HTTP router with common middleware
**So that** all endpoints share consistent auth, logging, and error handling

**Acceptance Criteria:**
- Router: chi, echo, or gin
- Middleware: request logging, panic recovery, CORS, request ID
- Rate limiter middleware (per-IP, configurable) — **use a library** (e.g., `tollbooth`, `ulule/limiter`), do NOT implement from scratch
- Auth middleware that extracts Bearer token, validates session, sets user in context
- All endpoints return JSON (except announce/scrape which return bencode)
- Error response helper: `{ "error": { "code": "...", "message": "..." } }`
- Health check endpoint (`GET /healthz`)
- OpenAPI/Swagger spec generation (swag or oapi-codegen)

#### BE-0.7: Background Job System [S] [DONE]
**As a** developer
**I want** a background job processor for async tasks
**So that** request handlers don't block on slow operations

**Acceptance Criteria:**
- Job queue backed by Redis or Postgres — **use `asynq` or `river`**, do NOT build a custom queue
- Jobs: send email, connectivity check, cleanup, stats recalculation
- Retry with backoff on failure
- Logging per job execution

---

### Epic BE-1: Authentication & User Management

#### BE-1.1: User Registration [M] [DONE]
**As a** visitor
**I want** to create an account with username, email, and password
**So that** I can access the tracker

**Acceptance Criteria:**
- Username validation: 3-20 chars, alphanumeric + underscore
- Password: minimum 8 chars, hashed with Argon2id — use `golang.org/x/crypto/argon2`, do NOT implement custom hashing
- Email: valid format, unique, not in banned domains list
- First registered user gets Administrator role
- Email confirmation flow (if enabled): generates token, sends email, confirms on click
- Admin approval mode (if enabled): account stays pending until admin approves
- Configurable: open registration, invite-only, or closed
- On success: auto-login (returns access_token + refresh_token per BE-1.2)

#### BE-1.2: Login & Multi-Device Session Management [M] [DONE — core auth, sessions/API keys deferred]
**As a** registered user
**I want** to log in from multiple devices simultaneously
**So that** I can use the tracker from my browser, TUI client, phone, and automations at the same time

**Acceptance Criteria:**
- `POST /api/v1/auth/login` -> returns `{ access_token, refresh_token, expires_at }`
- Verify password against stored hash (support legacy migration: SHA1, wrapped SHA1, Argon2)
- On successful legacy verify: re-hash with Argon2 transparently
- **Multi-device support**:
  - Each login creates an independent session (NOT one session per user)
  - Sessions stored in Redis: `session:{token}` -> `{user_id, device_name, ip, created_at, last_active}`
  - Optional `device_name` parameter on login (e.g. "Firefox", "TUI", "Upload Bot")
  - `GET /api/v1/auth/sessions` - list all active sessions for current user
  - `DELETE /api/v1/auth/sessions/{id}` - revoke a specific session (remote logout)
  - `DELETE /api/v1/auth/sessions` - revoke all sessions except current (panic button)
- **Token design**:
  - Access token: opaque, 64-char hex, short-lived (1 hour)
  - Refresh token: opaque, 64-char hex, long-lived (30 days)
  - `POST /api/v1/auth/refresh` -> issue new access token using refresh token
  - Refresh token rotation: old refresh token invalidated on use
- **API key support** (for automations/bots):
  - `POST /api/v1/auth/api-keys` - create named API key (no expiry, manually revoked)
  - API keys are Bearer tokens like access tokens but don't expire
  - Scoped permissions: read-only, upload, full (user chooses on creation)
  - `GET /api/v1/auth/api-keys` - list keys (shows name, created, last used, never shows secret)
  - `DELETE /api/v1/auth/api-keys/{id}` - revoke key
- Logout: `POST /api/v1/auth/logout` invalidates current session only
- Ban check: reject login if user disabled or IP banned
- Rate limit: max 5 failed attempts per 15 minutes per IP
- No IP binding (sessions work across networks)

#### BE-1.2.2: Persistent Session Store (Redis) [M] [DONE]
**As a** site operator
**I want** sessions to survive backend restarts
**So that** users don't get logged out when the server is updated

**Acceptance Criteria:**
- Define a `SessionStore` interface abstracting session CRUD (Get, GetByRefresh, Create, Delete, DeleteByUserID, DeleteByUserIDExcept)
- Implement Redis-backed session store using the existing Redis config (REDIS_URL)
- Migrate from current in-memory map to the Redis implementation
- Sessions persist across backend restarts
- Configurable token TTLs via env vars: `ACCESS_TOKEN_TTL` (default 1h), `REFRESH_TOKEN_TTL` (default 30d)
- Keep the in-memory implementation available (for testing or simple deployments without Redis)
- Factory function selects implementation based on config (e.g., `SESSION_STORE=memory|redis`, default redis)

> **Note:** Current in-memory session store loses all sessions on backend restart. This is the root cause of users being logged out during development and deployments.

#### BE-1.2.3: Move Test-Only Implementations Out of Domain Code [S] [DONE]
**As a** developer
**I want** test utilities separated from domain code
**So that** the service package contains only interfaces and business logic, not test doubles

**Acceptance Criteria:**
- Create `backend/internal/testutil/` package for shared test helpers
- Move `MemorySessionStore` from `service/session.go` to `testutil/`
- Move `MemoryPasswordResetStore` from `service/password_reset_store.go` to `testutil/`
- Move `NoopSender` from `service/email.go` to `testutil/`
- Service package only defines interfaces — no concrete test implementations
- Update all 40+ test files across `service/`, `handler/`, `cmd/server/` to import from `testutil/`
- All tests still pass

> **Note:** Domain code should never know about testing. All dependencies must be injected. Memory implementations exist purely for test DI and should not live alongside production interfaces.

#### BE-1.2.1: Email Confirmation Flow [M] [DONE]
**As a** site operator
**I want** new users to confirm their email address before accessing the tracker
**So that** fake/spam accounts are prevented

**Acceptance Criteria:**
- Configurable via env: `REGISTRATION_EMAIL_CONFIRM=true|false` (default false)
- On register (when enabled): account created with `enabled=false`, confirmation token generated
- `POST /api/v1/auth/register` returns 201 but with `"email_confirmation_required": true` instead of tokens
- Confirmation email sent via SMTP (use background job from BE-0.7) with link containing token
- `GET /api/v1/auth/confirm-email?token=...` validates token, sets `enabled=true`, redirects to login
- Token: cryptographically random, single-use, stored hashed in DB, expires in 24 hours
- Login rejects users with `enabled=false` with clear error message ("Please confirm your email")
- Resend confirmation: `POST /api/v1/auth/resend-confirmation` (rate limited: 1 per 5 minutes)
- Frontend: confirmation pending page, resend button, success redirect
- When `REGISTRATION_EMAIL_CONFIRM=false`, current behavior preserved (auto-login on register)

> **Note:** This was deferred from BE-1.1. Depends on SMTP being wired (Mailpit for dev).

#### BE-1.3: Password Recovery [S] [DONE]
**As a** user who forgot their password
**I want** to reset it via email
**So that** I can regain access

**Acceptance Criteria:**
- Request reset: enter email, receive link with time-limited token (1 hour)
- Token: cryptographically random, single-use, stored hashed in DB
- Reset: validate token, set new password (Argon2), invalidate all existing sessions
- Rate limit: max 3 reset requests per hour per email
- Generic response ("if this email exists...") to prevent enumeration

#### BE-1.4: User Profile & Settings [M] [DONE]
**As a** logged-in user
**I want** to view and edit my profile settings
**So that** I can customize my experience

**Acceptance Criteria:**
- View profile: username, join date, ratio, uploaded/downloaded, class, avatar, bio
- Edit: email (triggers re-confirmation), avatar URL, bio, signature, timezone, language, theme
- Privacy setting: public / limited / private (controls what others see)
- Passkey management: view current, regenerate (with grace period for old key)
- Accept PMs toggle
- Change password (requires current password)

#### BE-1.5: User Roles & Permissions [S] [DONE — RBAC with group permissions, RequireAuth/RequireAdmin/RequireStaff/RequireCapability middleware]
**As an** admin
**I want** a role-based permission system
**So that** different user classes have different capabilities

**Acceptance Criteria:**
- Roles stored in DB with granular permissions (same 18 fields as original)
- Default roles: Member, Power User, VIP, Uploader, Moderator, Super Mod, Admin
- Middleware helper: `RequirePermission("edit_torrents")`
- Role checked on every protected action
- Admin can create/edit/delete roles (except cannot delete role with active users)

#### BE-1.6: IP & Email Bans [S] [DONE]
**As an** admin
**I want** to ban IP ranges and email domains
**So that** I can block abusive users

**Acceptance Criteria:**
- IP bans: single IP or CIDR range, IPv4 and IPv6
- Email bans: full address or domain wildcard
- Checked on: registration, login
- Admin CRUD for both ban types
- Audit log: who banned, when, reason

#### BE-1.7: User Warnings & Auto-Ban [M] [DONE]
**As an** admin
**I want** to warn users for rule violations with automatic escalation
**So that** moderation is consistent and partially automated

**Acceptance Criteria:**
- Manual warning: reason, expiry date, type
- Warning notification: PM sent to user
- Auto-warning: triggered by cleanup job when ratio < threshold AND downloaded > minimum
- Auto-removal: when ratio improves above threshold
- Auto-ban: if ratio not improved within warning period
- Warning history visible to staff on user profile

#### BE-1.8: Staff Page & Member List [S] [DONE]
**As a** user
**I want** to see the staff team and browse member list
**So that** I know who to contact and can find other users

**Acceptance Criteria:**
- Staff page: shows users whose role has staff_page=true, grouped by role
- Online/offline indicator (configurable threshold, default 5 min)
- Member list: paginated, searchable by username, filterable by role
- Respects privacy settings (strong privacy hides from non-staff)

---

### Epic BE-2: Tracker (Announce & Scrape)

#### BE-2.1: HTTP Announce Endpoint [L] [DONE]
**As a** BitTorrent client
**I want** to announce my presence to the tracker and receive a peer list
**So that** I can participate in the swarm

**Acceptance Criteria:**
- `GET /announce?passkey=&info_hash=&peer_id=&port=&uploaded=&downloaded=&left=&event=`
- Passkey authentication: validate against users table
- Info hash validation: 20-byte binary or 40-char hex
- Peer ID validation: exactly 20 bytes
- Port validation: 1-65535, blacklist check (DC++, Kazaa, eMule, Gnutella, WinMX ports)
- Client agent check against banned agents list
- Torrent lookup: must exist and not be banned
- Event handling:
  - **started/empty**: upsert peer record, adjust seeder/leecher counts
  - **stopped**: delete peer record, adjust counts
  - **completed**: increment times_completed, log in completed table
- Stats delta: calculate upload/download since last announce, update user totals
- Freeleech: if torrent is freeleech, don't count download against user
- Peer list: return up to N peers (configurable, default 50) using random offset (NOT ORDER BY RAND)
- Compact response format (BEP 23): default to compact=1
- Dict response format: support compact=0 fallback
- Response: bencoded dict with interval, min_interval, complete, incomplete, peers
- Error responses: bencoded `failure reason`
- All DB operations within a transaction

#### BE-2.2: Connection Limits [S] [DONE]
**As a** tracker operator
**I want** to limit concurrent connections per user
**So that** account sharing is deterred

**Acceptance Criteria:**
- Max concurrent leeching slots per user (configurable, default 1)
- Max concurrent seeding slots per user (configurable, default 3)
- Checked for new peers only (not existing peers re-announcing)
- Error response with clear message when limit exceeded

#### BE-2.3: Wait Time System [S] [DONE]
**As a** tracker operator
**I want** users with poor ratios to wait before downloading new torrents
**So that** there's incentive to maintain good ratio

**Acceptance Criteria:**
- Configurable tier system (ratio threshold, GB threshold, wait hours)
- Only applies to configurable set of user roles
- Only applies to leechers on torrents they haven't announced for before
- Wait time calculated from torrent added time
- Clear error message with remaining wait time
- Exempt: seeders, high-ratio users, privileged roles

#### BE-2.4: HTTP Scrape Endpoint [S] [DONE]
**As a** BitTorrent client
**I want** to scrape torrent statistics without announcing
**So that** I can display swarm info in my client

**Acceptance Criteria:**
- `GET /scrape?info_hash=` (supports multiple info_hash params)
- Returns bencoded dict with per-torrent: complete, incomplete, downloaded, name
- No authentication required (or optional passkey)
- Rate limited per IP

#### BE-2.5: UDP Tracker Protocol [L] [DEFERRED — moved to docs/FUTURE_WORK.md]

#### BE-2.6: Peer Cleanup Job [S] [DONE]
**As a** tracker operator
**I want** stale peers automatically removed
**So that** seeder/leecher counts stay accurate

**Acceptance Criteria:**
- Background job runs every N minutes (configurable, default 10)
- Delete peers with last_action older than announce_interval * 1.5
- Recalculate seeder/leecher counts from actual peer records
- Update torrent visibility: hide torrents with no peers beyond dead threshold
- Log stats: peers removed, torrents hidden

#### BE-2.7: Cheating Detection [M] [DONE]
**As a** tracker operator
**I want** basic cheating detection
**So that** users can't fake upload/download stats

**Acceptance Criteria:**
- Detect impossible upload speed (e.g., > 100 MB/s sustained)
- Detect upload reported but no peers downloading
- Detect download reported but left didn't decrease proportionally
- Flag suspicious announces in a log table (don't auto-ban)
- Admin view: list flagged users with evidence
- Configurable thresholds

---

### Epic BE-3: Torrent Management

#### BE-3.1: Upload Torrent [L] [DONE]
**As an** uploader
**I want** to upload a .torrent file with metadata
**So that** others can download it

**Acceptance Criteria:**
- Parse .torrent file: extract info_hash, name, size, file list, announce URLs
- Rewrite announce URL to tracker's own URL (strip external announces for local torrents)
- Duplicate detection: reject if info_hash already exists
- Required fields: torrent file, category
- Optional fields: name (defaults from torrent), description (Markdown), language, images (up to 2), NFO file, anonymous flag
- Image validation: max size, allowed types (JPEG, PNG, WEBP, GIF), verify with image decoder
- NFO validation: max 65KB, text file
- Store .torrent file via `FileStorage` interface (BE-0.4) - works with local disk or S3
- External torrent support: if announce URL doesn't match tracker, mark as external
- Permission check: can_upload, optional uploader-only mode (min role)

#### BE-3.2: Download Torrent File [S] [DONE]
**As a** user
**I want** to download the .torrent file with my passkey embedded
**So that** my client can connect to the tracker authenticated

**Acceptance Criteria:**
- `GET /download/{id}`
- Requires authentication
- Permission check: can_download
- Read .torrent file, BDecode, replace announce URL with user's passkey URL, BEncode
- Remove announce-list (multi-tracker) for local torrents
- External torrents: serve unmodified
- Increment hit counter
- Response: `Content-Type: application/x-bittorrent`, `Content-Disposition: attachment`
- Generate passkey on-the-fly if user doesn't have one

#### BE-3.3: Browse & List Torrents [M] [DONE]
**As a** user
**I want** to browse available torrents with filtering and sorting
**So that** I can find content to download

**Acceptance Criteria:**
- `GET /torrents?cat=&sort=&order=&page=`
- Filters: category, parent category, multiple categories
- Sorting: name, added, size, seeders, leechers, completed, comments
- Pagination: configurable page size (default 25)
- Only show visible=true, banned=false
- Response includes: name, category, size, seeders, leechers, completed, added, comments count, rating, freeleech flag, uploader (or "Anonymous")
- Respect user privacy settings for uploader name

#### BE-3.4: Torrent Detail Page [M] [DONE — NFO + peer list in detail response, browse filters for date/seeders/uploader]
**As a** user
**I want** to see full details about a torrent
**So that** I can decide whether to download it

**Acceptance Criteria:**
- `GET /torrents/{id}`
- Full metadata: name, description, category, language, size, info_hash, uploader, dates, all stats
- File list with sizes
- Peer list (local torrents): IP hidden, shows uploaded/downloaded/ratio/client/connectable
- Comments: paginated, with user info
- Rating: current average and vote count
- NFO content (if present)
- Download link
- Health indicator (based on seeder/leecher ratio)
- External torrents: show external tracker stats
- Banned torrents: only visible to staff

#### BE-3.5: Search Torrents [M] [DONE]
**As a** user
**I want** to search torrents by keyword and filters
**So that** I can find specific content

**Acceptance Criteria:**
- `GET /torrents/search?q=&cat=&lang=&alive=&freeleech=`
- Full-text search on torrent name (Postgres `tsvector` or trigram index)
- All filters from browse endpoint
- Additional filters: alive only, dead only, freeleech only, local only, external only
- Paginated results
- Minimum query length: 2 characters

#### BE-3.6: Edit & Delete Torrent [S] [DONE]
**As a** torrent owner or moderator
**I want** to edit or delete a torrent
**So that** I can fix mistakes or remove bad content

**Acceptance Criteria:**
- Edit: name, description, category, language, visible, anonymous, images, NFO
- Staff-only edits: banned, freeleech
- Permission: owner OR edit_torrents role
- Delete: requires reason, removes DB record + files
- If deleter != owner: send PM to owner with reason
- All actions logged

#### BE-3.7: Comments & Ratings [M] [DONE — PR pending merge]
**As a** user
**I want** to comment on and rate torrents
**So that** I can share feedback

**Acceptance Criteria:**
- Add comment: requires login, non-empty body (Markdown)
- Edit comment: author or moderator
- Delete comment: moderator only
- Rating: 1-5 scale, one vote per user per torrent
- Average rating displayed when >= 2 votes
- Comments paginated (default 20/page)

#### BE-3.8: Reporting System [S] [DONE]
**As a** user
**I want** to report rule-breaking content
**So that** moderators can take action

**Acceptance Criteria:**
- Reportable: torrents, users, comments, forum posts
- Requires reason
- One report per user per item (prevent spam)
- Admin view: list reports, filter by type/status, mark as resolved
- Resolved reports keep history

#### BE-3.9: Reseed Request [S] [DONE]
**As a** user
**I want** to request a reseed for a dead torrent
**So that** it becomes downloadable again

**Acceptance Criteria:**
- Only for local, non-banned torrents with 0 seeders
- Rate limit: one request per torrent per 24h per user (server-side, NOT cookie)
- Sends PM to all users who completed the torrent + torrent owner
- Queued as background job (could be hundreds of PMs)

#### BE-3.10: RSS Feed [S] [DONE]
**As a** user
**I want** an RSS feed of new torrents
**So that** I can auto-download with my client

**Acceptance Criteria:**
- `GET /rss?cat=&passkey=`
- RSS 2.0 or Atom format
- Filters: category, language
- Passkey auth for download links
- Last 50 torrents
- Each item: title, link (details or download), size, category, seeders/leechers, date

#### BE-3.11: Categories & Languages [S] [DONE — categories CRUD + hierarchical dropdowns]
**As an** admin
**I want** to manage torrent categories and languages
**So that** content is organized

**Acceptance Criteria:**
- Categories: hierarchical (parent -> child), with image/icon
- CRUD for categories (admin only)
- CRUD for torrent languages (admin only)
- Reorder support
- Prevent delete if torrents exist in category (or force reassign)

#### BE-3.12: @Mention Search Endpoint [S] [DONE — backend endpoint reused; `@` typeahead wired into the shared MarkdownEditor]
**As a** frontend developer
**I want** a user search endpoint for @mention autocomplete
**So that** users can be mentioned in comments and forum posts

**Acceptance Criteria:**
- ~~`GET /api/v1/users/search?q=prefix`~~ — superseded, see below
- Returns matching users (id, username, avatar) limited to 10 results
- Prefix match on username
- Requires authentication
- Used by frontend for typeahead in comment/post editors

**Status (audited 2026-07-14) — rescoped: the backend half already exists.**
- `GET /api/v1/users?search=<q>&per_page=<n>` (`MemberHandler.HandleList`, auth-required) already does username search with pagination, and the PM compose screen (`frontend/src/pages/MessagesPage.tsx`) uses it for typeahead today. A second dedicated `/users/search` endpoint would duplicate it — **do not build it.**
- Mention *notifications* also already work: `backend/internal/listener/notification.go` parses `@username` via `mentionRegex` and emits `forum_mention`.
- REMAINING WORK IS FRONTEND ONLY — extract the typeahead from `MessagesPage` into a reusable component and wire it into the forum post and comment editors, which have no mention autocomplete. FE-0.7 shipped `frontend/src/components/MarkdownEditor.tsx` (now used by comments, forum posts, PMs, torrent descriptions and news), so the `@` typeahead should hook into that single component and every compose surface gets it at once.
- **DONE (2026-07-15):** the `@` typeahead now lives in `MarkdownEditor` — an `@` after a word boundary opens a debounced (250ms) `GET /api/v1/users?search=&per_page=8` lookup whose trigger detection mirrors the backend `mentionRegex`, so the inserted token has the exact shape the backend recognises. Keyboard-navigable (arrows / Enter / Tab / Escape) with combobox ARIA, closes on blur and in preview. Every compose surface (comments, forum new-topic + reply, PMs, torrent edit, upload, admin news) gets the typeahead at once. Covered by new tests in `MarkdownEditor.test.tsx`.
- **Notification-coverage caveat:** an inserted `@mention` only produces a `forum_mention` *notification* on forum new-topic and reply — those are the only writes that fire `ForumPostCreated`, the sole event whose listener runs `mentionRegex`. On torrent comments, forum post edits, torrent descriptions, upload and news the typeahead is currently an insertion/spelling aid only. Extending notifications is tracked as **BE-5.10** (PMs are deliberately excluded — notifying a non-participant would leak a private message).

#### BE-3.13: Rich Torrent Metadata [M] [DONE — category-driven JSONB metadata schema]
**As an** uploader
**I want** to add detailed metadata to my uploads
**So that** torrents are well-categorized by category-specific attributes

**Chosen approach (shipped):** data-driven, not hardcoded fields. An admin defines the
extra fields **on a category**; those definitions drive the upload/edit UI and are validated
server-side. Values persist as JSONB, avoiding a column-per-attribute.

- `categories.metadata_schema JSONB` — the category's own field definitions (array). Migration `065`.
- `torrents.metadata JSONB` — the submitted field values (object). Migration `065`.
- **Field types:** `text` (max length / regex), `number` (min/max/integer), `select`,
  `multiselect` (options + max items), `boolean`. See `backend/internal/metadata`.
- **Inheritance:** a category's *effective* schema = its ancestors' fields merged with its own
  (child overrides parent by key, ancestor order preserved). Define shared fields (codec, quality)
  on a parent "Video" category; Movies/TV inherit and add `year` / `season`+`episode`.
- **Resolve endpoint:** `GET /api/v1/categories/{id}/metadata-schema` returns the effective fields
  so external clients and the upload/edit UI render dynamically. The torrent detail response also
  embeds the effective schema alongside the stored values.
- **Validation** on upload + edit rejects unknown keys, enforces required/type/constraints, and
  stores a canonical object. Editing a torrent's category validates provided metadata against the
  new category; changing category without resending metadata leaves stored values untouched.
- **Frontend:** admin `MetadataSchemaEditor` (define fields per category), shared `MetadataFields`
  (dynamic upload/edit inputs), and a read-only "Details" section on the torrent detail page.

**Deliberately out of scope (spun out below):** external/API auto-detection of metadata, and
browse/search filtering by metadata fields.

> **MT-1.2 note:** the JSONB `metadata` column is migration-compatible — the torrent migration can
> map legacy attributes into a category's schema without schema changes.

#### BE-3.13a: Filter Browse/Search by Metadata Fields [M] [DONE — JSONB filters on browse/search + category-aware filter UI]
**As a** user
**I want** to filter torrents by category metadata (e.g. `year=2024`, `codec=x265`)
**So that** I can find content by specific attributes

**Chosen approach (shipped):** `GET /api/v1/torrents` (the same endpoint serves search via
`?search=`) accepts `meta_<key>` equality params plus `meta_<key>__gte` / `meta_<key>__lte`
numeric ranges. Because a raw query value is an untyped string, the handler resolves the selected
category's *effective* schema (`service.ResolveCategorySchema`) and coerces each value to its
field's type via `metadata.CoerceFilterValue` — so metadata filters **require a `cat`**. An
unknown field, a missing category, or a range on a non-numeric field is a 400.

- Equality predicates are merged into a single JSONB containment check (`t.metadata @> $n::jsonb`),
  index-backed by a **GIN index on `torrents.metadata`** (migration `066`). Multiselect equality
  matches array containment.
- Numeric ranges compare `(t.metadata ->> key)::numeric`, guarded by `jsonb_typeof(... ) = 'number'`
  so a drifted non-numeric value can never abort the cast for the whole query.
- Threaded through `repository.MetadataFilter` / `ListTorrentsOptions.MetadataFilters` and the
  `torrent.go` WHERE builder.
- **Frontend:** `MetadataFilterControls` renders per-field controls (select/multiselect → dropdown,
  boolean → Any/Yes/No, number → Min/Max) when a category is selected on the browse page; filter
  state lives in the URL. Free-text fields are intentionally omitted (exact-match containment isn't
  useful for prose; still filterable via the API). Changing category clears stale filters.
- Follow-up to **BE-3.13**, which shipped define + validate + display only.

#### BE-3.13b: Auto-Detect Metadata from Torrent Name [S] [DONE — client-side name parsing pre-fills upload fields]
**As an** uploader
**I want** metadata pre-filled by parsing the torrent name
**So that** I don't type year/resolution/codec by hand

**Chosen approach (shipped):** a **frontend-only** aid (`detectMetadataFromName` in
`utils/metadata.ts`). It parses scene-style names like `Movie.2024.1080p.BluRay.x265.DTS-HD` for
year/resolution/source/codec/audio and pre-fills the matching category-schema fields on the upload
form — only keys the schema defines, and select/multiselect values only when they match a defined
option (adapting to the admin's spelling). Purely additive: it fills empty fields only, the user
can still edit, and the manual fields remain the source of truth. **No backend change** — it's a UI
convenience over the existing upload endpoint, which still validates on submit.

#### BE-3.13c: Category Edit as a Page + Metadata Fields Table [S] [DONE — admin category editor moved to a page]
**As an** admin
**I want** to edit a category on a dedicated page with its custom fields shown as a table
**So that** the (now larger) category form isn't cramped inside a modal

**Chosen approach (shipped):** a **frontend-only** refactor. Now that a category carries a metadata
schema (BE-3.13), the create/edit form outgrew a modal. Add/Edit Category now navigate to a
dedicated page (`/admin/categories/new`, `/admin/categories/:id/edit` → `AdminCategoryEditPage`);
the list page keeps only the table + delete. Custom fields render as a **table**
(`MetadataFieldsTable`: Label · Key · Type · Required · Details); **Add Field** opens a modal
(`MetadataFieldModal`) that saves each field as a row, and each row's **Edit** reopens the modal
pre-filled. The old inline `MetadataSchemaEditor` is retired. There's no single-category admin GET,
so the edit page reuses the existing list endpoint for both the category and its parent options.
**No backend change** — same `POST`/`PUT /api/v1/admin/categories` payload, still validated server-side.

#### BE-3.13d: Category Tree View + Inherited Metadata Display [S] [DONE — hierarchical admin list + read-only inherited fields]
**As an** admin
**I want** to see categories as a hierarchy and, when editing a sub-category, see the fields it inherits
**So that** I can manage deep category trees and avoid redefining fields a parent already provides

**Chosen approach (shipped):** a **frontend-only** feature over the existing endpoints.
- **Tree list:** the admin categories page renders an inline, indented, expand/collapse tree
  (`utils/categoryTree.ts` builds/flattens the forest; siblings ordered by sort_order then name;
  orphans surface as roots; cycle-safe). Each row keeps Edit/Delete and gains **Add sub**
  (→ `/admin/categories/new?parent=<id>`, which pre-selects the parent). Default fully expanded.
- **Multi-level parents:** the edit page's Parent dropdown now offers *any* category (shown indented),
  excluding the category itself and its descendants to prevent cycles — so hierarchies can go many
  levels deep (the backend already had no depth restriction).
- **Inherited metadata:** when a parent is selected, the edit page fetches that parent's *effective*
  schema (`GET /categories/{id}/metadata-schema`, which already merges the whole ancestor chain) and
  shows those fields in a read-only **Inherited Fields** table above the editable own-fields table,
  with required fields marked. Updates live when the parent changes.

**No backend change** — the category `parent_id`, the admin list, and the metadata-schema resolve
endpoint already supported arbitrary depth and full-chain inheritance. Reorder (drag / up-down) is a
deliberate follow-up.

#### BE-3.13e: Drag-and-Drop Category Reorder [M] [DONE — atomic batch reorder endpoint + tree DnD]
**As an** admin
**I want** to drag categories to reorder and re-nest them in the tree
**So that** I can arrange the hierarchy visually instead of editing sort_order / parent by hand

**Chosen approach (shipped):** a proper BE+FE feature — reordering the whole tree should be atomic,
not a series of per-row PUTs.
- **Backend:** new `PUT /api/v1/admin/categories/reorder` accepting `{items:[{id, parent_id,
  sort_order}]}`. The service validates the resulting hierarchy (no category is its own ancestor,
  no cycle, every referenced parent is in the request) and the repo applies all placements in a
  single transaction (`CategoryRepo.Reorder` via `CategoryPlacement`), so an invalid parent rolls
  the whole batch back. Invalid input → 400 `invalid_category`.
- **Frontend:** native HTML5 drag-and-drop on the tree rows (no new dependency). A pure
  `utils/categoryReorder.ts` (`computeReorder` / `dropZoneFromOffset` / `isInvalidDrop`) does the
  placement math — top/bottom bands reorder as a sibling before/after the target, the middle band
  nests inside it — and emits gap-free placements for the whole forest. Drops onto the dragged
  node's own subtree are blocked (cycle-safe on the client too). The move is applied optimistically
  and reverts (refetch + toast) if the request fails.

Completes the reorder follow-up noted under BE-3.13d.

#### BE-3.13f: Metadata Issues Report [M] [DONE — torrents missing required fields, per-user and site-wide]
**As a** user (my uploads) / admin (all uploads)
**I want** to see which torrents are missing a metadata field their category now requires
**So that** I can fix data left stale when a required field was added after upload

**Context:** metadata fields live on the category, and adding one never rewrites existing torrents.
A field made *required* after torrents exist leaves those rows silently non-compliant — they're only
forced to fill it on their next edit. This report surfaces them so they can be fixed proactively.

**Chosen approach (shipped):** BE+FE.
- **Backend:** `GET /api/v1/torrents/metadata-issues?scope=mine|all`. Non-admins are always scoped to
  their own uploads; `scope=all` is admin-only (403 otherwise). A new `MetadataAuditService` resolves
  each category's *effective* schema (so inherited required fields count), and for the categories that
  actually have required fields queries torrents missing them via a `jsonb_exists`-based repo method
  (`TorrentRepo.ListMissingRequiredMetadata`). Implemented behind a narrow one-method interface, so it
  didn't ripple across the torrent-repository mocks.
- **Frontend:** `MetadataIssuesPage` at `/metadata-issues` — a table of Torrent · Category ·
  (Uploader, for the admin all-view) · Missing fields · **Fix** (link to the torrent's edit page).
  Admins get an All-uploaders / Only-mine toggle; regular users see only their own. Linked from the
  Torrents menu (all users) and the admin sidebar.

Follow-up to the metadata schema work (BE-3.13); addresses the "no backfill/audit for newly-required
fields" gap.

#### BE-3.14: Show Uploader in Torrent Browse List [S] [DONE]
**As a** user
**I want** to see who uploaded each torrent in the browse/list views
**So that** I can identify trusted uploaders

**Acceptance Criteria:**
- Torrent list API response includes `uploader_name` (or "Anonymous" if `anonymous=true`)
- Browse page table shows uploader column with link to profile
- Home page latest torrents table shows uploader
- Today's Torrents and Need Seed pages show uploader
- Respect anonymous flag — never reveal uploader identity for anonymous uploads
- Staff can optionally see the real uploader even for anonymous torrents (future enhancement)

#### BE-3.15: Category Images [S] [DONE]
**As a** site operator
**I want** categories to have associated images/icons
**So that** torrent listings are visually identifiable by category

**Acceptance Criteria:**
- Add `image_url` nullable column to categories table (migration)
- Admin category CRUD supports setting an image URL (or uploading to S3)
- Categories API returns `image_url` field
- Torrent browse/list views show category image next to category name
- Torrent detail page shows category image in the breadcrumb
- If no image is set, display a styled placeholder icon (generic file icon or first letter of category name)
- Frontend: `CategoryIcon` component reusable across browse, detail, home page

#### BE-3.16: User Torrent Activity on Profile [M] [DONE]
**As a** user
**I want** to see my torrent activity on my profile page
**So that** I can track what I've uploaded, downloaded, and am currently transferring

**Acceptance Criteria:**
- **Public (visible to everyone):**
  - List of torrents uploaded by the user (paginated, respects anonymous flag)
- **Private (visible only to profile owner and staff):**
  - Torrents currently seeding (active peers where `seeder=true`)
  - Torrents currently leeching (active peers where `seeder=false`)
  - Download history: completed torrents with upload/download amounts per torrent
- Backend endpoints:
  - `GET /api/v1/users/{id}/torrents` — uploaded torrents (public, filtered by anonymous)
  - `GET /api/v1/users/{id}/activity` — seeding/leeching/history (owner + staff only)
- Activity response includes per-torrent stats: torrent name, uploaded bytes, downloaded bytes, ratio, seeder status, last announce
- Frontend: tabs or sections on profile page (Uploads | Seeding | Leeching | History)
- Staff view: can see any user's full activity (for moderation/cheating detection)

---

### Epic BE-4: Invitation System

#### BE-4.1: Send & Redeem Invites [M] [DONE — token-based invites, registration mode, invite tracking]
**As a** user with available invites
**I want** to invite someone by email
**So that** they can join the tracker

**Acceptance Criteria:**
- Configurable registration mode via env: `REGISTRATION_MODE=open|invite|closed` (default open)
- When `invite`: registration requires a valid invite token, signup form shows invite code field
- When `closed`: registration disabled entirely, returns 403
- When `open`: current behavior (anyone can register)
- Requires invites > 0 and total users < max
- Validate email: format, not banned, not already registered
- Create invite record in `user_invites` table (not a dummy user like original)
- Store: inviter_id, email, token (random), created_at, expires_at, status
- Send email with signup link containing token
- Signup with invite: token validates, email pre-filled, account confirmed immediately
- Decrement inviter's invite count on send
- Expire unused invites after configurable period (default 7 days)

#### BE-4.2: Auto-Invite Distribution [S] [DONE]
**As a** tracker operator
**I want** invites distributed automatically based on user activity
**So that** active users can grow the community

**Context:** The original acceptance criteria's wording ("downloaded GB range", "max invites per role") was ambiguous; the design below is the clarified, concrete spec that shipped. It mirrors the auto class-promotion engine (BE-8.13, admin page FE-5.11/5.12) almost exactly: a background job evaluates users against per-group configurable rules, an admin FE edits the rule set, and a Run-now button forces an off-cycle run.

**Delivered:**
- **Schema** — migration 056 (`invite_distribution_rules`): `group_id` PK/FK, `min_ratio`, `min_downloaded_bytes`, `max_downloaded_bytes` (0 = unbounded, mirrors `promotion_rules`), `max_invites` (a per-user ceiling; 0 deliberately means "grant nothing" until an admin configures a real cap — the safe default for a freshly added rule row). Plus `invite_distribution_runs` for run bookkeeping, mirroring `promotion_runs`. New site settings `invite_distribution_enabled` (default off) and `invite_distribution_interval_days` (default 7).
- **Eligibility per group**: `ratio >= min_ratio` AND `downloaded_bytes` within `[min_downloaded_bytes, max_downloaded_bytes]`. **Grant**: +1 invite per cycle, only while the user's current `Invites` balance is below the group's `max_invites` — a per-user ceiling, not a shared pool.
- **`can_invite`-restriction interaction (design decision):** a user whose invite privilege is suspended does **not** receive auto-grants, even if otherwise eligible. This mirrors how outstanding invite tokens are already voided while `can_invite` is false (BE-8.15/8.16) — the restriction means "no invite activity for this user," not just "no manual invite creation." Granting anyway would let the balance quietly bank rewards during a punitive suspension, available in full the moment it's lifted.
- **Backend:** `model.InviteDistributionRule` / `model.UserInviteState`; `repository.InviteDistributionRepository` (`internal/repository/postgres/invite_distribution.go`); `service.InviteDistributionService` (`Run(ctx, force)`, `ListRules`/`UpsertRule`/`DeleteRule`) mirroring `PromotionService`'s shape; `worker/invite_distribution.go` (asynq task, registered daily at `30 5 * * *` — offset from promotion's `0 5 * * *`); `handler/invite_distribution.go` admin CRUD + run-now endpoints under `/api/v1/admin/invite-distribution/rules[/{group_id}]` and `/run`.
- **Reused, not duplicated:** the per-user uploaded/downloaded/tenure query is shared between `PromotionRepository.LadderMetrics` and `InviteDistributionRepository.GroupMetrics` via a new `queryGroupMetrics` helper (`internal/repository/postgres/user_metrics.go`) — one query for that shape, not two.
- **New `InviteAutoGrantedEvent`** → activity log listener, one entry per grant: "System granted an invite to `{username}` via auto-distribution" (mirrors `InviteRevokedEvent`'s pattern from BE-8.15).
- **Bundled consistency fix, extended to full closure:** `UserRepo.AdjustInvites(ctx, userID, delta)` — an atomic `UPDATE users SET invites = invites + $1`, mirroring how `bonus_points` is adjusted (BE-8.14) — is now the only path that increments/decrements the invite balance; `InviteService.CreateInvite`'s decrement was rewired onto it (closing the same stale-read clobber race BE-8.16 closed for the privilege flags, but for `invites`). Going further than the minimum ask: `UserRepo.Update` now excludes `invites` entirely (mirroring `bonus_points` and the four privilege flags), and a new `UserRepo.SetInvites(ctx, userID, invites)` (an absolute, non-delta set) replaces `AdminService`'s prior read-modify-write-through-`Update` for the admin "set invites to N" edit — so *no* invites write path is a stale full-row `Update` anymore, not just the two this story added. `NewInviteService`/`NewInviteDistributionService`/`NewAdminService` take narrow local interfaces (`inviteAdjustRepository`, `inviteSetRepository`) for this, following the `privilegeFlagRepository` pattern from BE-8.16 so the broader `UserRepository`-implementing mocks elsewhere are untouched.
- **Tests:** repository (postgres, via testcontainers: rules CRUD, `GroupMetrics`, `UserInviteStates`, run bookkeeping, migration 056 applies cleanly), service (disabled/interval/force gating, eligible/ineligible ratio/downloaded-range/ceiling/zero-ceiling/suspended-privilege cases, staff-group and invalid-range rejection, error propagation for every repo call), worker (task construction + nil-deps skip), handler (admin-gated CRUD + run, 403 for non-admins, invalid-range 400), listener (activity log message, username-resolution fallback). Plus `TestUserRepoAdjustInvitesDoesNotLoseConcurrentWrite` / `TestUserRepoSetInvitesIsAbsolute` / `TestUserRepoUpdateDoesNotWriteInvites` (postgres), mirroring `TestUserRepoUpdate_DoesNotClobberPrivilegeFlags` from BE-8.16 for the same race shape applied to `invites`.
- **Known follow-up (not blocking):** none of the above closes every conceivable admin-vs-engine race elsewhere in the codebase — e.g. two admins racing `SetInvites` with different absolute targets is a plain last-write-wins on the same column, which is expected for an explicit "set to N" action (the same way `BonusRepo.SetPoints` behaves), not a bug. Separately (code-review finding, Low): `AdjustInvites` has no floor guard, so two concurrent `CreateInvite` calls against a balance of 1 can both pass the stale `Invites <= 0` pre-check and both decrement, landing the balance at `-1` instead of the pre-existing behavior's lost-update-at-`0` — benign (the `<= 0` gate still blocks further creates, and admin `SetInvites`/auto-grants recover it) but the clean fix is a conditional decrement mirroring `BonusRepo`'s guarded spend path (`UPDATE ... SET invites = invites - 1 WHERE id = $1 AND invites > 0`, mapping zero-rows-affected to `ErrNoInvitesRemaining`).

*(Frontend: see FE-5.16.)*

---

### Epic BE-5: Forum

#### BE-5.1: Forum Structure & Browsing [M] [DONE]
**As a** user
**I want** to browse forum categories and topics
**So that** I can participate in discussions

**Architecture:** phpBB-style two-level structure. Forum Categories are display groups, Forums are where topics live. No sub-forums.

**Schema:**
- `forum_categories` (id, name, sort_order, created_at) — display-only groupings
- `forums` (id, category_id FK, name, description, sort_order, topic_count, post_count, last_post_id, min_group_level, created_at) — denormalized counts for performance
- `forum_topics` (id, forum_id FK, user_id FK, title, pinned, locked, post_count, view_count, last_post_id, last_post_at, created_at, updated_at)
- `forum_posts` (id, topic_id FK, user_id FK, body (Markdown), reply_to_post_id, edited_at, edited_by, created_at)

**Access Control:**
- `can_forum` flag on users table — global privilege flag (same pattern as can_download/can_upload/can_chat). Integrates with existing restriction system (admin can suspend, tiered escalation can auto-restrict). Added via migration.
- `min_group_level` on forums table — controls which groups can see/post in specific forums. Matches TorrentTrader's class-based access.
- Access check: user has `can_forum=true` AND user's group level >= forum's `min_group_level`.

**Acceptance Criteria:**
- Forum Categories → Forums → Topics → Posts hierarchy (two-level, no sub-forums)
- Forum index: list all forum categories with their forums, showing topic_count, post_count, last post info per forum
- Forum view: list topics (paginated, 20/page), pinned first, then by last_post_at
- Topic view: list posts (paginated, 20/page), flat display sorted by date, with user info per post
- Read tracking: track last read post per topic per user
- Unread indicators on forums and topics
- Permission: `can_forum` user flag + `min_group_level` per forum
- View count per topic (increment on view)
- "Mark all read" bulk action

#### BE-5.2: Create Topics & Post Replies [M] [DONE]
**As a** user
**I want** to create topics and reply to them
**So that** I can participate in discussions

**Acceptance Criteria:**
- Create topic: title (max 200 chars) + body (Markdown)
- **Posts are flat** (not threaded trees), sorted by date
- `reply_to_post_id` is for quoting context only — does NOT create a tree structure
  - Response includes quoted snippet of referenced post (auto-generated)
  - Frontend renders flat with inline quote block, not indented threading
- Denormalized count updates: increment forum.post_count, forum.topic_count (on new topic), topic.post_count; update forum.last_post_id, topic.last_post_id, topic.last_post_at
- Permission: `can_forum` user flag + `min_group_level` per forum
- Updates topic's last_post_id and last_post_at
- Users without `can_forum` cannot post
- **@mention support**: `@username` in post body
  - Auto-linked to user profile in rendered output
  - Triggers notification (see BE-5.6)

#### BE-5.3: Edit & Delete Posts [S] [DONE]
**As a** post author or moderator
**I want** to edit or delete posts
**So that** content can be corrected or removed

**Acceptance Criteria:**
- Edit: author or moderator can edit; tracks `edited_at` and `edited_by` fields on forum_posts
- Edit history: store previous version (at least last revision, for mod review)
- Delete post: moderator permission required, cannot delete if only post in topic (delete the topic instead)
- Delete topic: moderator permission required, deletes all posts and read tracking; decrements forum.topic_count and forum.post_count
- Author can edit own posts; moderators can edit/delete any post

#### BE-5.4: Moderation Tools [S] [DONE]
**As a** moderator
**I want** to lock, pin, move, and rename topics
**So that** I can keep the forum organized

**Acceptance Criteria:**
- Lock/unlock: sets `locked` flag on forum_topics, prevents new replies
- Pin/unpin: sets `pinned` flag on forum_topics, pins to top of forum list
- Move: change topic's forum_id (validates destination exists, updates denormalized counts on both source and destination forums)
- Rename: change topic title
- Delete topic: removes topic and all posts, updates denormalized counts
- All moderation actions require `can_forum` + moderator/admin role
- Access check includes `min_group_level` on destination forum for move operations

#### BE-5.5: Forum Search [S] [DONE]
**As a** user
**I want** to search forum posts
**So that** I can find past discussions

**Acceptance Criteria:**
- Full-text search on forum_posts.body AND forum_topics.title using PostgreSQL tsvector (same pattern as torrent search with `:*` prefix matching)
- Results: post snippet with keyword highlighting, topic title, forum name, author, date
- Paginated (50 results per page)
- Respects forum access: only returns results from forums where user's group level >= `min_group_level`
- Filter by: forum, author, date range

#### BE-5.6: Notification Infrastructure [M] [DONE — notifications table, NotificationService, NotificationRepo, WebSocket push, preference system]
**As a** developer
**I want** a unified notification system with event bus and delivery engine
**So that** all notification triggers share common infrastructure

**Acceptance Criteria:**
- `notifications` table: id, user_id, type, data (JSONB), read, created_at
- Event bus: publish/subscribe pattern for notification triggers
- Delivery engine: processes events, creates notification records, dispatches to channels
- `GET /api/v1/notifications` - list user's notifications (paginated)
- `PATCH /api/v1/notifications/{id}` - mark read
- `PATCH /api/v1/notifications/read-all` - mark all read
- Unread count endpoint or response header
- **Notification types** (extensible):
  - `forum_reply` - someone replied to your post
  - `forum_mention` - someone @mentioned you in a forum post
  - `forum_topic_reply` - new post in a subscribed topic
  - `chat_mention` - someone @mentioned you in chat
  - `pm_received` - new private message
  - `torrent_comment` - someone commented on your torrent
  - `system` - system notifications (warnings, ratio alerts, etc.)

#### BE-5.7: Forum Notification Triggers [S] [DONE — ForumPostCreated/ForumTopicCreated events, forum_reply/forum_mention/topic_reply notifications via listener]
**As a** user
**I want** to be notified when someone replies to my post or mentions me
**So that** I don't miss relevant discussions

**Acceptance Criteria:**
- Trigger on: reply to my post (reply_to_post_id points to my post)
- Trigger on: @mention in any forum post
- Trigger on: new post in a topic I'm subscribed to
- Auto-subscribe on: topic creation, posting in topic (configurable per user)
- Uses notification infrastructure from BE-5.6

#### BE-5.8: Subscription Management API [S] [DONE — topic_subscriptions table, subscribe/unsubscribe/check endpoints, auto-subscribe on topic creation and posting, notification_preferences table]
**As a** user
**I want** to subscribe/unsubscribe to topics and forums
**So that** I control which discussions I follow

**Acceptance Criteria:**
- `POST /api/v1/forum/topics/{id}/subscribe` - watch a topic
- `DELETE /api/v1/forum/topics/{id}/subscribe` - unwatch
- Auto-subscribe on: topic creation, posting in topic (configurable per user)
- Mute: override subscription for specific noisy topics
- **User preferences** (in user_settings):
  - Per-type toggle: enable/disable each notification type
  - Delivery method per type: in-app only, in-app + email, off

#### BE-5.9: Notification Delivery [M] [DONE — in-app notifications via DB, WebSocket push via ChatHub.SendToUser. Email digest and batching deferred to follow-up]
**As a** user
**I want** notifications delivered via multiple channels
**So that** I receive them however I prefer

**Acceptance Criteria:**
- In-app: stored in notifications table, returned via API
- Email digest: configurable (immediate, daily summary, off)
- WebSocket push: if user is connected, push notification events in real-time
- Batching: group multiple notifications of same type (e.g., "5 new replies in topic X")
- Email sent as background job (via BE-0.7)

#### BE-5.10: Unified @Mention Notification System [M] [DONE — forum + comments; other surfaces deferred]
**As a** user
**I want** to be notified when someone @mentions me in a forum post or a comment
**So that** the mention typeahead's promise holds beyond just forum posts

**Context:** BE-3.12 shipped the `@mention` typeahead into the shared `MarkdownEditor`, so it appears on every compose surface — but originally `mentionRegex` only ran in the `ForumPostCreated` listener, shipping the whole post body on the event to do it. Surfaced by the BE-3.12 devil's-advocate review.

**DONE (2026-07-15):** replaced the forum-only path with a **unified mention system**. Mentions are parsed at publish time (`internal/mention.Extract`), and each domain publishes a generic `UserMentionedEvent` (via the shared `service.publishMention` helper) carrying only the extracted usernames, a `source` tag, a context title, and a structured `Link` (source-specific ids + page) — never the body (so `Body` was removed from `ForumPostCreatedEvent`). One listener merges that into a single `mention` notification (replacing `forum_mention`); the **frontend** builds the deep-link from the structured data (`notificationDisplay.ts`), so routes stay frontend-owned. Wired for **forum new-topic + reply** and **torrent comments**.

The link carries the item's **page** (computed from its position: forum `topic.PostCount+1`, comment total count, over the per-surface page size) so it lands on the mention, not page 1 — the forum page already reads `?page=` and `CommentsSection` now initializes its page from the URL. A shared `useScrollToHashAnchor` hook (frontend/src/lib) scrolls to the `#post-`/`#comment-` anchor once the page loads. Migration `051` renames existing `forum_mention` preference rows to `mention` so opt-outs survive the rename.

**Behavior change:** mentions are now a separate event, so a user both replied-to and @mentioned in one post receives two distinct notifications (a reply and a mention), where before dedup collapsed them to one. Intentional (confirmed) — they are distinct facts.

**Remaining (still TODO under this story):**
- **Forum post edit** — `EditPost` fires no event; needs new-vs-old mention diffing to avoid re-notifying on every edit.
- **Global chat** — no deep-link target yet (needs a message-anchored history view); chat mentions stay typeahead-only until then.
- Torrent descriptions / news bodies — lower priority; decide per surface.
- Deep-link page is best-effort across post/comment deletions (position drifts); acceptable — the anchor still lands on the right page.
- **Exclude PMs** — permanently; notifying a mentioned non-participant would leak a private message.

---

### Epic BE-6: Real-Time Chat (Shoutbox Replacement)

#### BE-6.1: WebSocket Chat [M] [DONE — WS hub with write pump, rate limiting, session revalidation, origin check]
**As a** user
**I want** a real-time chat on the homepage
**So that** I can communicate with other users instantly

**Acceptance Criteria:**
- WebSocket endpoint: `ws:///ws/chat`
- Authentication: validate Bearer token (access token or API key) on connect handshake
- Send message: broadcast to all connected users
- Receive message: real-time push (no polling)
- Message format: `{ id, user: {id, username, role}, message, timestamp }`
- Duplicate prevention: reject identical message from same user within 10 seconds
- Persist messages to DB (last N messages for history)
- On connect: send last 50 messages as backfill
- Markdown formatting support
- **@mention support**: `@username` in message body
  - Triggers notification via BE-5.6 notification system (type: `chat_mention`)
  - If mentioned user is connected: highlighted message in their chat stream
  - If mentioned user is offline: in-app notification + optional email
- **Scalability**: if running multiple instances, use Redis pub/sub to broadcast messages across instances

#### BE-6.2: Chat Moderation [S] [DONE]
**As a** moderator
**I want** to delete chat messages
**So that** I can remove inappropriate content

**Acceptance Criteria:**
- Delete by message ID: author or edit_users permission
- Broadcast deletion to all connected clients (message disappears in real-time)
- Logged with moderator info

#### BE-6.3: Chat History [S] [DONE]
**As a** user
**I want** to view older chat messages
**So that** I can catch up on conversations I missed

**Acceptance Criteria:**
- `GET /chat/history?page=&per_page=`
- Paginated (100 per page)
- Requires login

---

### Epic BE-7: Private Messaging

#### BE-7.1: Send & Receive Messages [M] [DONE — inbox/outbox/compose, autocomplete, reply with parent_id, unread badge]
**As a** user
**I want** to send private messages to other users
**So that** I can communicate privately

**Acceptance Criteria:**
- Compose: select recipient, subject, body (Markdown)
- Recipient validation: exists, enabled, confirmed, accepts PMs (or sender is staff)
- Inbox: received messages, paginated, sortable by date/sender/subject
- Outbox: sent messages (if save copy enabled)
- Read/unread tracking
- Mark as read: on view or bulk action
- Delete: soft delete per side (sender/receiver independent)
- Unread count in navigation/header

#### BE-7.2: Drafts & Templates [S] [DONE — saved_messages table with a kind discriminator (draft/template), /messages/drafts + /messages/templates REST endpoints, compose-form Save Draft/Save as Template/list/load/delete UI]
**As a** user
**I want** to save message drafts and templates
**So that** I can compose messages later or reuse common messages

**Acceptance Criteria:**
- Save draft: stores incomplete message for later editing
- Save template: stores reusable message pattern
- Load template into compose form
- List/delete drafts and templates

> **Known follow-up (not blocking, Devil's Advocate finding, rated High by the reviewer):** `Update` has no optimistic-concurrency check — two tabs/devices saving the same draft is last-write-wins with no conflict signal. Deliberately not fixed here: a real fix needs a version/`updated_at` token round-tripped through the PUT request, a new 409 response, and frontend handling for it — a scope increase for an `[S]` story, not a quick win. Judged acceptable to ship without it because the blast radius is narrow (a user's own draft/template, never shared or cross-user) and "last write wins" is the standard, expected behavior for exactly this kind of autosave resource in comparable products (Gmail, Outlook, Notion). If multi-tab draft editing becomes a real complaint, revisit with a `updated_at`-based conditional update (`SavedMessageRepo.Update` already carries `AND user_id = $N` in its WHERE clause — extending it to `AND updated_at = $N+1` and mapping the resulting `sql.ErrNoRows` to a 409 is the whole change). **Resolved by BE-7.5.**

#### BE-7.5: Optimistic Concurrency on Saved Drafts/Templates [S] [DONE — integer `version` column, conditional `UPDATE ... WHERE id = $1 AND version = $2`, 409 with the current row on conflict, frontend "Load latest version" action]
**As a** user
**I want** a stale save to a draft/template I have open in two tabs (or two devices) to fail loudly
**So that** I never silently lose edits to whichever save landed second

**Acceptance Criteria:**
- `saved_messages` gains a `version INTEGER NOT NULL DEFAULT 1` column (migration 064), incremented by exactly 1 on every successful update — a monotonic counter, not a UUID/hash token, since the only concurrent writers are a single user's own sessions
- The client sends back the version it last saw on every `PUT /messages/drafts/{id}` and `PUT /messages/templates/{id}`; `SavedMessageRepo.Update` does a conditional `UPDATE ... WHERE id = $1 AND user_id = $2 AND version = $3`
- Zero rows affected because the version moved on (not because the row is missing/not owned) returns a clean `409` — not the generic `500` a bare `sql.ErrNoRows` would've produced — with the saved message's current server-side state (subject/body/version) attached under the same `draft`/`template` key a 200 response uses, so the frontend can react without a second request
- Frontend compose form (`MessagesPage`) tracks the version returned by every save/get/load and round-trips it on the next PUT; a 409 shows a clear error plus a "Load latest version" action that replaces the compose form with the server's current content — the user's in-progress edits are never silently discarded, and never silently overwritten either
- Regression tests: two concurrent updates to the same saved message (real goroutines against Postgres, not just sequential calls) — exactly one wins, the other gets a conflict, and the stored row ends up exactly as the winner left it

> **Origin:** Devil's Advocate follow-up to BE-7.2, deferred there as a deliberate scope cut for an `[S]` story (see the note above). Chose an integer version counter over an `updated_at`-based conditional update (BE-7.2's own suggested approach) — same mechanism, but a counter that increments by exactly 1 is simpler to reason about and compare than a timestamp, and sidesteps any concern about clock/precision granularity.

#### BE-7.3: PM Notifications [S] [DONE — handled by notification system BE-5.6-5.9, pm_received type + WS push + unread badge]

#### BE-7.4: Real-Time PM Notification via WebSocket [S] [DONE]
**As a** user
**I want** to see my unread message count update in real time
**So that** I know immediately when I receive a new message

**Acceptance Criteria:**
- Piggyback on the chat WebSocket connection (BE-6.1) — no separate connection
- When a PM is sent, publish a `MessageSent` event
- A listener checks if the receiver is connected to the chat WebSocket hub
- If connected, broadcast `{"type":"pm_notification","unread_count":N}` to that user's connection only (not all clients)
- Frontend chat WebSocket handler updates the header unread badge on receiving this message type
- Eliminates the 30-second polling interval for unread count when WebSocket is active
- Graceful fallback: if WebSocket is not connected, continue polling

> **Depends on:** BE-6.1 (WebSocket chat hub must be merged first)
> **Note:** The `MessageSentEvent` and unread count API already exist. This task only adds the WebSocket push layer.

---

### Epic BE-8: Admin Panel

#### BE-8.1: Admin Dashboard & Site Settings [M] [DONE]
**As an** admin
**I want** a control panel to manage site settings
**So that** I can configure the tracker

**Acceptance Criteria:**
- Requires control_panel permission
- Dashboard: user count, torrent count, peer count, traffic stats
- Site settings: all configuration values (site name, URL, feature flags, limits, etc.)
- Settings stored in DB (overrides env vars for runtime-configurable values)

#### BE-8.2: User Management [M] [DONE]
**As an** admin
**I want** to search, view, edit, and moderate users
**So that** I can manage the community

**Acceptance Criteria:**
- [x] Search: by username, email
- [ ] Search: by IP, role, status
- [ ] View: full profile with all fields, stats, invite history, warning history, mod notes
- [x] Edit: role (group), enabled, warned
- [ ] Edit: title, uploaded/downloaded, avatar, signature, passkey reset
- [ ] Promote/demote (cannot promote above own level)
- [ ] Delete account with reason (logged)
- [ ] Warning management (add/remove/view)
- [ ] Mod notes (staff-only field)
- [x] Invalidate sessions when disabling user (tracked as future enhancement)

#### BE-8.3: Torrent & Content Moderation [S] [DONE]
**As an** admin
**I want** to manage torrents and content
**So that** I can maintain site quality

**Acceptance Criteria:**
- [ ] Search torrents by name, info_hash
- [ ] Ban/unban torrents
- [ ] Toggle freeleech per torrent
- [x] View/manage all reports (filter by status, enriched with reporter/torrent names)
- [x] Resolve reports
- [ ] Resolve with action (warn uploader, delete torrent, ban user)
- [ ] Bulk actions: ban multiple, delete multiple
- [ ] View all freeleech torrents, all banned torrents

#### BE-8.4: News Management [S] [DONE]
**As an** admin
**I want** to post and manage site news
**So that** I can communicate with users

**Acceptance Criteria:**
- CRUD for news articles (Markdown)
- News displayed on homepage
- Comments on news (same system as torrent comments)
- Delete news also deletes associated comments

#### BE-8.5: Forum Administration [DONE]
**As an** admin
**I want** to manage forum structure
**So that** the forum stays organized

**Acceptance Criteria:**
- CRUD for `forum_categories` (name, sort_order)
- CRUD for `forums` (name, description, category_id, sort_order, min_group_level)
- Set `min_group_level` per forum to control which user groups can access
- Reorder forums and categories via sort_order field
- Toggle user `can_forum` flag via admin user management (integrates with existing privilege flags)

#### BE-8.6: Logs & Monitoring [S] [DONE — activity log with filtering + pagination, public at /log]
**As an** admin
**I want** to view system logs and activity
**So that** I can audit actions and troubleshoot issues

**Acceptance Criteria:**
- Activity log: all admin actions, searchable, paginated
- Current peers: list all active peers with stats
- Online users: who's currently active
- SQL/application error log (or integrate with external logging)

#### BE-8.7: Database Backup [S] [DONE]
**As an** admin
**I want** to create and download database backups
**So that** I have disaster recovery capability

**Acceptance Criteria:**
- Trigger backup via admin panel ✅
- Creates pg_dump compressed file ✅
- Download backup file ✅
- List/delete old backups ✅
- Optional: scheduled backups via cron job ✅

**Implementation:**
- Admin-only endpoints (`RequireAdmin`): `POST /api/v1/admin/backups` (create), `GET /api/v1/admin/backups` (list),
  `GET /api/v1/admin/backups/{name}/download`, `DELETE /api/v1/admin/backups/{name}`.
- Admin UI at `/admin/backups` (`AdminBackupsPage`): create button, table with size/age, download and delete (confirm modal).
- `service.BackupService` shells out to `pg_dump --format=custom --compress=9` via `exec.CommandContext` with an
  argument slice — **no shell**. Credentials reach pg_dump through libpq env vars (`PGPASSWORD`, `PGHOST`, …), never
  argv, so they cannot be read from the process list. Dumps write to `<name>.partial` and are renamed on success;
  stale partials are cleared at startup.
- Backups are files, not rows: the backup directory is the source of truth. Names are server-generated
  (`backup-<UTC timestamp>-<8 hex>.dump`) and `ValidateBackupName` rejects anything else — path separators, `..`,
  absolute paths — at both the handler and the service. `resolveExisting` additionally requires a **regular file**
  (`Lstat`), so a symlink planted in the backup directory cannot be used to read or delete a file elsewhere.
- Only one dump runs at a time per process (409 `backup_in_progress`; the scheduled job stands down instead of
  retrying). The dump is detached from the caller's context (`context.WithoutCancel`) so closing the browser tab does
  not kill it, but is bounded by `BACKUP_TIMEOUT`. Dump files are chmod 0600.
- Config: `BACKUP_DIR`, `PG_DUMP_PATH`, `BACKUP_TIMEOUT` (30m), `BACKUP_RETENTION` (0 = keep all),
  `BACKUP_SCHEDULE_CRON` (empty = off). Scheduled backups run as the asynq task `backup:database`.
- `backup_created` / `backup_deleted` / `backup_downloaded` events feed the activity log. Downloads are audited
  because a dump is the entire database; retention prunes and scheduled backups are attributed to `system`.
- A misconfigured backup directory, or a `DATABASE_URL` in libpq keyword form (which pgx accepts but pg_dump cannot be
  driven from here), **disables backups with a warning** rather than failing server startup.
- The backend image now ships `postgresql16-client`; keep it in step with the Postgres server major version.

**Known limitation:** the single-dump guard is per-process. With several replicas sharing the backup volume each can
run its own dump — wasteful but safe, since every dump writes to a uniquely named file and startup only sweeps
`*.partial` files older than `BACKUP_TIMEOUT`, never one a peer may still be writing.

#### BE-8.8: Admin Password & Passkey Reset [S] [DONE]
**As an** admin or staff member
**I want** to reset a user's password or passkey
**So that** I can help users who are locked out or protect accounts with suspected leaked credentials

**Acceptance Criteria:**
- `PUT /api/v1/admin/users/{id}/reset-password` — sets a new password, hashes with Argon2id
- Option to auto-generate a random password or accept an admin-provided one
- Invalidates all existing sessions for the user after password reset
- Email notification sent to the user with the new password (or a reset link)
- `PUT /api/v1/admin/users/{id}/reset-passkey` — regenerates passkey (32-char hex)
- Invalidates existing .torrent files (user must re-download with new passkey)
- Both actions logged to activity log with actor info
- Frontend: buttons in the admin user edit panel
- Staff can reset passwords for users at or below their group level (cannot reset admin passwords unless admin)

#### BE-8.9: Per-User Privilege Restrictions [M] [DONE]
**As a** staff member
**I want** to restrict specific privileges for a user without fully disabling their account
**So that** I can apply proportional consequences for rule violations

**Acceptance Criteria:**
- New restriction flags on users table: `can_download`, `can_upload`, `can_chat` (all default `true`) *(FE-5.15 later added `can_invite` as a fourth restriction type — suspending it blocks invite creation with 403 `invite_restricted`)*
- Migration adds columns with `DEFAULT true` so existing users are unaffected
- `PUT /api/v1/admin/users/{id}/restrictions` — set restriction flags `{can_download, can_upload, can_chat}` with optional `reason` and `expires_at`
- Restrictions are checked in the relevant handlers:
  - `can_download=false` → download .torrent file returns 403 with "Your download privileges have been suspended"
  - `can_upload=false` → upload torrent returns 403 with "Your upload privileges have been suspended"
  - `can_chat=false` → WebSocket chat rejects messages with "Your chat privileges have been suspended"
- Restrictions table: `user_restrictions` (id, user_id, restriction_type, reason, issued_by, expires_at, created_at) for audit history
- Maintenance job auto-removes expired restrictions
- User profile shows active restrictions to the user (so they know why they can't download/upload/chat)
- Admin user detail shows restriction history
- Restriction issued/removed events logged to activity log
- Frontend: restriction controls in admin user edit panel, checkboxes + optional reason/expiry

#### BE-8.10: Tiered Warning Escalation [S] [DONE]
**As a** site operator
**I want** optional automatic escalation based on warning count
**So that** moderation consequences are consistent and predictable

**Acceptance Criteria:**
- Configurable via site settings (all optional, disabled by default):
  - `warning_escalation_enabled` — master toggle (default `false`)
  - `warning_count_restrict` — number of active warnings before privilege restriction (default 2)
  - `warning_count_ban` — number of active warnings before account ban (default 3)
  - `warning_restrict_type` — which privilege to restrict: `download`, `upload`, `chat`, or `all` (default `download`)
  - `warning_restrict_days` — duration of the restriction in days (default 7)
- When a new manual warning is issued, the job checks the user's active warning count:
  - If count >= `warning_count_restrict` and < `warning_count_ban`: apply privilege restriction (requires BE-8.9)
  - If count >= `warning_count_ban`: disable account
- Escalation is logged to activity log with the trigger reason
- Staff can always override (lift warnings, re-enable accounts, remove restrictions)
- When disabled (`warning_escalation_enabled=false`), warnings remain purely informational (current behavior)

#### BE-8.11: Quick Ban Action [S] [DONE]
**As a** staff member
**I want** a one-click ban action that combines account disable + warning + optional IP ban
**So that** I can swiftly handle severe violations without multiple steps

**Acceptance Criteria:**
- `POST /api/v1/admin/users/{id}/ban` — accepts `{reason, ban_ip?, ban_email?, duration_days?}`
- In a single transaction:
  - Sets `enabled=false` on the user
  - Creates a warning record (type `manual`, status `escalated`) with the reason
  - Optionally creates an IP ban for the user's last known IP (if `ban_ip=true`)
  - Optionally creates an email ban for the user's email domain (if `ban_email=true`)
  - Invalidates all user sessions
  - Sends a PM to the user with the ban reason (before disabling)
- If `duration_days` is set, the ban is temporary — maintenance job re-enables after expiry
- Activity log records the ban with all details (IP ban, email ban, duration)
- Frontend: "Ban User" button in admin user management with a modal for reason, checkboxes for IP/email ban, optional duration
- Separate from the existing warning lift/escalate flow — this is a direct moderation action

---

#### BE-8.12: Group Management (CRUD) [M] [DONE]
**As an** administrator
**I want** to create, edit, and delete permission groups from the admin panel
**So that** I can manage colors, levels, and capability flags without editing the database

**Context:** Groups were read-only — seeded by migration 001, listed via `GET /admin/groups`, with no way to mutate them. This adds full CRUD with safety guards.

**Acceptance Criteria (met):**
- `POST/PUT/DELETE /api/v1/admin/groups[/{id}]` (admin only), alongside the existing list endpoint.
- New `GroupWriteRepository` (Create/Update/Delete/CountMembers) kept separate from the read-only `GroupRepository` so the ~10 existing read-only mocks are untouched; `NewGroupRepo` now returns the concrete type to satisfy both.
- Validation: name/slug required, slug format + auto-derivation from name, hex color, level 0–1000; name/slug uniqueness (case-insensitive) with a clean 409.
- Safety guards: cannot delete the admin group (id 1) or default registration group (id 5) — both hardcoded in `auth.go`; cannot delete a group that still has members (409); cannot strip `is_admin` from the admin group (lockout prevention).
- Frontend: `AdminGroupsPage` upgraded from a read-only matrix to full CRUD — "New Group" + per-row Edit/Delete, modal form with name/slug/level, color picker, and the eight capability checkboxes; server-side guard messages surfaced via toast.
- Tests: repository (CRUD + CountMembers + missing-row), service (validation, conflicts, all guards, not-found, writer-unavailable), full-router handler (authz + status-code mapping), and frontend (list/create/delete/confirm-decline).

> **Related follow-up:** per-user privilege editing + invite capability (`can_invite` is group-scoped) — see docs/FUTURE_WORK.md → "Editable User Privileges + Invite Capability". Managing `can_invite` per group is now possible through this page.

---

#### BE-8.13: Auto Class Promotion / Demotion Engine [L] [DONE]
**As a** site operator
**I want** users to move up and down a class ladder automatically on merit
**So that** classes reflect current contribution without manual staff work — and so invite auto-distribution has a class signal to build on

**Context:** Requested as the foundation for auto invite distribution: invites are class-gated, so a merit ladder must exist first. Groups had a `level` and permission flags but no automatic movement.

**Acceptance Criteria (met):**
- `promotion_rules` table (per-class thresholds: min ratio, uploaded bytes, account age, uploaded torrent count, seed hours) + `promotion_runs` audit/last-run table + settings `promotion_enabled` (off by default), `promotion_interval_days` (cycle), `promotion_seed_window_days`. Migration 053.
- **The ladder** is exactly the groups that have a rule, ordered by level. Staff groups (`is_admin`/`is_moderator`) are rejected as rungs, and the engine additionally refuses to ever move a user *into* a staff class — auto-promotion can never reach staff.
- **±1 rung per run**, single-threshold (as chosen): promote if the user meets the next rung's rule, else demote if they no longer meet their current rung's; never below the floor, never above the top. Qualifying for +N rungs takes N runs.
- **Metrics**: ratio/uploaded/age/torrent-count from the users+torrents tables; **seed hours estimated from the `announce_events` log** over the configurable window (gap-sum with a per-gap cap so offline stretches aren't billed as seeding).
- **Worker task** `promotion:run` on a daily cron, gated by the enable flag and the elapsed interval; the engine no-ops between cycles. Manual `POST /admin/promotion/run` trigger.
- **Admin config**: `GET/PUT/DELETE /admin/promotion/rules[/{groupId}]`, and a `Class Promotion` admin page (per-class threshold editing, add/remove rungs, Run now). Enable/cycle/window live in Site Settings.
- Tests: repository (rules CRUD, ladder metrics, the seed-hours SQL incl. gap capping, run bookkeeping), service (one-step promote/demote, floor/top guards, staff exclusion, seed-hours gate, disabled/interval skips, rule validation), full-router handler (authz + status mapping), and frontend (ladder list, staff exclusion, add-to-ladder, run-now).

> **Enables (follow-up):** auto invite distribution — the capped weekly drip can now gate on class/level. See docs/FUTURE_WORK.md → "Auto Invite Distribution".

---

#### BE-8.14: Bonus Points Foundation [M] [DONE]
**As a** site operator
**I want** a points economy — earned by seeding, spendable in a store, adjustable by admins
**So that** sustained seeding is rewarded and perks (invites, upload credit) have a merit price

**Delivered (migration 054):**
- `users.bonus_points` balance + **append-only `bonus_transactions` ledger** — every seeding award, purchase, and admin adjustment writes a row (reason + ref), so history is always reconstructible.
- **Clobber-proofing:** `bonus_points` is readable via `userColumns` but deliberately excluded from `UserRepo.Create/Update`; all writes are atomic statements in `BonusRepo` (award: `+= delta`; purchase: conditional `-= price WHERE bonus_points >= price` in one tx with the reward + ledger; admin set: `SELECT ... FOR UPDATE`).
- **`BonusSource` interface** (Name doubles as ledger reason) with the basic **hourly seeding snapshot** source: rate × currently-seeding torrents (distinct, from peers), rate configurable via `bonus_points_per_seeding_torrent`. Worker task `bonus:award` at `30 * * * *`; master switch `bonus_enabled` (off by default).
- **Store** (`bonus_store_items` seeded): `invite-1` (5000 pts → 1 invite) and `upload-5gib` (3000 pts → 5 GiB) purchasable; `freeleech_ticket` + `double_upload` seeded **disabled** as catalogue-only kinds — server rejects their purchase and hides them from the listing until effects exist.
- Endpoints: `GET /api/v1/store/items` (200 always; `{enabled,items}`), `POST /api/v1/store/purchase/{itemId}` (403 disabled / 404 unknown / 409 unavailable-or-insufficient).
- **Admin**: `AdminUpdateUserRequest.bonus_points` sets an absolute balance via the bonus repo (negative rejected), recorded as `admin_adjust` with the actor as ref.
- Tests at every layer, incl. the race-safety case: an unaffordable purchase fails and changes nothing.

#### (paired frontend: see FE-5.14)

---

### Epic BE-9: Cleanup & Maintenance Jobs

#### BE-9.1: Scheduled Cleanup Job [M] [DONE]
**As a** tracker operator
**I want** automated maintenance tasks
**So that** the system stays healthy without manual intervention

**Acceptance Criteria:**
- Runs on configurable interval (default 10 minutes)
- Tasks:
  - Remove stale peers (last_action > announce_interval * 1.5)
  - Recalculate torrent seeder/leecher counts from peers table
  - Hide dead torrents (no peers > configurable threshold)
  - Delete expired pending registrations
  - Remove expired invite tokens
  - Prune old log entries (> configurable retention)
  - Deactivate expired warnings
- Each task independently toggleable
- Execution logged with stats (rows affected)

#### BE-9.2: Ratio Warning Automation [S] [DONE]
**As a** tracker operator
**I want** automatic ratio warnings and bans
**So that** ratio enforcement is consistent

**Acceptance Criteria:**
- Configurable: min ratio, min downloaded GB, warning period
- Auto-warn: users below ratio with enough download to judge
- Auto-remove: warnings where ratio improved
- Auto-ban: users who didn't improve within warning period
- PM sent for each action (via background job)

#### BE-9.3: Cache Site Stats Query [S] [DONE]
**As a** tracker operator
**I want** the site stats query to be cached
**So that** the footer polling from every client doesn't hammer the database

**Acceptance Criteria:**
- Stats endpoint (`/api/v1/stats`) returns cached results
- Cache backend abstracted via interface (Redis or in-memory)
- Short TTL (15-30 seconds) — stats are near-real-time, not stale
- Cache populated on first request or by scheduler
- Fallback to direct query if cache unavailable

#### BE-9.4: Real-Time Stats via SSE or WebSocket [M] [DEFERRED — moved to docs/FUTURE_WORK.md]
**Decision (2026-07-15):** keep polling. Footer stats are low-stakes and already served from the Redis `StatsCache`, so polling reads a cache, not the DB. A dedicated WebSocket isn't worth a second hub + per-client goroutines for numbers nobody watches second-by-second; if fresher data is ever wanted, lower the `StatsCache` TTL (polling faster than the TTL just returns the same cached value). SSE remains the tasteful upgrade if it ever becomes a real concern — see `docs/FUTURE_WORK.md`.

#### BE-9.5: Backfill Torrent File Lists [S] [REMOVED — no legacy data to backfill; if needed, handle during MT-1.2 torrent migration]

#### BE-9.6: Increase Test Coverage to 80% [DONE]
**As a** developer
**I want** comprehensive test coverage across all packages
**So that** regressions are caught early and code quality is maintained

**Acceptance Criteria:**
- Minimum 80% test coverage per package (handler, service, repository, worker, middleware)
- CI gates on coverage threshold — build fails if coverage drops below the floor (see BE-9.18; the floor ratchets rather than sitting at 80, which would red-line every PR until the target is reached)
- Current low-coverage packages to prioritize:
  - `handler` — add tests for dashboard, admin, chat, news, warning, user activity handlers
  - `worker` — add tests for maintenance, ratio warning, cleanup jobs
  - `repository/postgres` — add integration tests or improve mock coverage
  - `config` — test validation and edge cases
  - `database` — test connection and migration error handling

**Status (2026-07-15): DONE — overall 42.1% → 80.4% (floor 80).**

| Package | Coverage | Note |
|---|---:|---|
| `repository/postgres` | **92.5%** | target met (BE-9.17, then deepened) |
| `handler` | **~88%** | httptest + real-router authz walk (BE-9.16, BE-9.11 fixes) |
| `listener` | ~86% | notification listener covered (BE-9.16) |
| `service` | ~73% | the largest remaining block (816 uncovered) |
| `worker` | ~34% | ratio-warning helpers covered; the handlers need a warning-repo stub |
| `middleware` | ~63% | |

- The remaining ~0.5 point to 80% sits in `internal/service` and `internal/worker`, both of which need service-layer test harnesses rather than more handler tests.
- `internal/handler` includes a **route-authorization walk** (`route_authz_test.go`): it enumerates the real router with `chi.Walk` and asserts every `/admin/**` route rejects an anonymous caller (401) and an ordinary member (403). This catches a privilege-escalation regression that the direct-handler tests structurally cannot, and it covers new routes automatically. Verified by moving the backup routes out of the `RequireAdmin` group and watching it fail.
- `chat_ws.go` is covered (81%) by a real-connection harness (BE-9.16 follow-up) rather than being excluded.
- Raise `COVERAGE_FLOOR` after each increment. Never lower it to turn a red build green.
- All new code must ship with tests above the threshold
- Add `go test -coverprofile` to CI with coverage check step

> **Note:** This is a tech-debt task. Should be addressed incrementally — each new PR must not decrease coverage, and dedicated test sprints can bring existing packages up to the threshold.

#### BE-9.7: Forum Post Soft-Delete & Edit History [M] [DONE]
**As a** moderator
**I want** deleted posts to be soft-deleted and edits to be tracked
**So that** moderation actions are reversible and edit abuse is prevented

**Acceptance Criteria:**
- Add `deleted_at` timestamp to `forum_posts` (soft-delete instead of hard delete)
- Add `forum_post_edits` table tracking edit history (post_id, old_body, edited_by, edited_at)
- Soft-deleted posts show "This post was deleted" placeholder in topic view
- Staff can view deleted post content and restore posts
- Edit history viewable by staff (shows diffs or previous versions)
- Handle dangling `reply_to_post_id` references gracefully (show "deleted post" instead of broken ref)

> **Origin:** Review finding from BE-5.3 — hard delete has no audit trail, no undo, and creates dangling references. Edit-then-delete abuse vector exists without history.

#### BE-9.23: Forum Post Edit History via Diff Storage [M] [DONE]
**As a** developer
**I want** post edits to reconstruct history from diffs instead of storing a full body snapshot per edit
**So that** repeated small edits to the same post don't multiply storage with near-duplicate full-text copies

**Acceptance Criteria:**
- Editable by: the post's author, or a staff member acting on behalf of another user
- Not editable when: the topic/post is locked, or the editor no longer has permission to edit/post (mirror the existing create-post capability checks)
- `forum_post_edits` stores a diff against the previous version instead of a full `old_body` snapshot — `forum_posts.body` is always the latest version; history is reconstructed by walking edits backward from latest, applying each diff in reverse
- No merge-conflict handling needed: edits on a given post are strictly linear (one body, edits applied in server-received order), so diff reconstruction is always deterministic
- Each edit record stores who actually made that edit (the real editor — author or staff — not just the post's original author)
- When a staff member edits on behalf of someone else: an optional "reason" field on that edit, staff-only visible (never shown to the post's author or other regular users viewing edit history)
- Existing edit-history viewer (BE-9.7) keeps working unchanged from the viewer's perspective — reconstructed diffs render the same "previous version" view it already shows staff

> **Origin:** Follow-up to BE-9.7, which stores a full `old_body` snapshot per edit — requested to avoid unbounded storage growth from repeated small edits to the same post. Deferred until after BE-8.21/FE-5.18 (mention backfill + PM mention linking).

#### BE-9.25: Optimistic Concurrency Control on Forum Post Edits [S] [DONE]
**As a** developer
**I want** `EditPost` to detect when a post's body changed underneath a stale read
**So that** two overlapping edits to the same post can't produce a diff chain that no longer matches the post's actual history

**Acceptance Criteria:**
- `EditPost` currently reads the post, computes a diff against that read, then writes the edit row and the new body as two independent, non-transactional statements (documented in a comment on `ForumService.EditPost`) — a second edit racing in between can commit against the same stale body
- Detect the race (e.g. a conditional update — `UPDATE ... WHERE id = $1 AND body = $2` — or a transaction with `SELECT ... FOR UPDATE`) and surface a clear conflict (e.g. HTTP 409) instead of silently landing a diff computed against a body that's no longer current
- Add a regression test that exercises two concurrent `EditPost` calls against the same post and asserts one succeeds and the other is rejected as a conflict, not silently corrupted

> **Origin:** Devil's Advocate review of BE-9.23 (diff-storage forum post edit history). `EditPost` was already non-transactional before that story (see the comment on it explaining why `CreateEdit`/`Update` are independent writes), so this race isn't new — but BE-9.23 changed its failure mode: under the old full-snapshot storage, a lost-update race produced silently-inconsistent-but-still-readable history; under diff storage, the same race can produce a stored diff that no longer matches the body it's later applied against. BE-9.23 addresses that specific regression by having `ListPostEdits` degrade gracefully (flag the unreconstructable edit and everything older than it as `reconstruction_failed` instead of failing the whole request), which closes the "one bad row 500s the entire staff view" failure mode. It does not close the race itself — that's this story, and the reason it's scoped separately: fixing it properly needs a transaction/locking change to `EditPost`'s write path (touching a pattern shared with `CreatePost`/`DeletePost`), which is more invasive than a single-PR follow-up alongside the storage-format change.

#### BE-9.26: Batch "Mark Topic Read" Endpoint for Grouped Notifications [S] [DONE]
**As a** user
**I want** "Mark all read" on a collapsed topic_reply group to resolve in one request
**So that** marking a heavily-replied topic read doesn't fire one PUT per underlying notification

**Acceptance Criteria:**
- A dedicated batch endpoint marks every unread notification underlying a `topic_reply` group as read via one set-based `UPDATE`, scoped to exactly that user + topic + type
- The endpoint only ever touches notifications that actually belong to the targeted group — it must not leak into other topics, other users, or other notification types
- Calling it again after the group is already fully read is a no-op, not an error (idempotent)
- The frontend's "Mark all read" on a group replaces its N-parallel-PUTs loop with a single call to the new endpoint, keeping the existing single unread-count refetch afterward (the refetch race that used to exist there was already fixed by BE-9.14)

> **Origin:** Follow-up to BE-9.14 (Notification Batching), whose review explicitly accepted-but-documented the N-individual-PUTs "mark all read" as safe-but-inefficient for a topic with a very large number of replies, and named a dedicated batch endpoint as the candidate follow-up.

**Delivered:**
- New route `PUT /api/v1/notifications/groups/{key}/read`, where `key` is exactly the `NotificationGroup.Key` value BE-9.14 already returns in the `/grouped` response (`"topic_reply:<topic_id>"`) — reused as-is rather than inventing a second grouping identity. Shaped as a `PUT .../read` to stay consistent with the existing single-notification `PUT /{id}/read` and `PUT /read-all` endpoints rather than introducing a `POST /mark-read`-style path.
- `repository.NotificationRepository` gained `MarkTopicReplyGroupRead(ctx, userID, topicID) (int64, error)`. The Postgres implementation is one set-based `UPDATE notifications SET is_read = TRUE WHERE user_id = $1 AND type = 'topic_reply' AND is_read = FALSE AND data->>'topic_id' = $3`, returning rows affected. `topic_id` is compared as text rather than cast to `bigint` specifically so a row with a malformed/missing `topic_id` is silently excluded from the match instead of aborting the whole `UPDATE` with a cast error — the same graceful-degradation posture BE-9.14's read-time grouping already has for the same field.
- `service.NotificationService.MarkGroupRead(ctx, userID, key)` parses `key` via the new exported `ParseTopicReplyGroupKey` (shares the `topic_reply:` prefix constant with `groupKeyFor`, so the two can't drift apart) and delegates to the repository. A key that isn't a `topic_reply:<id>` group — i.e. a `"single:<id>"` singleton, which the UI never routes through this endpoint since a `count <= 1` group renders as a plain notification with the existing single "Mark read" button — returns `ErrGroupNotBatchable`, surfaced by the handler as HTTP 400.
- Response is `200 {"marked": <count>}` (unlike the single mark-read endpoint's `204`) so the count is available if a future caller wants it; today the frontend only uses success/failure, applies the same optimistic local update BE-9.14 already had for the group, and then does its one pre-existing `GET /unread-count` refetch — no new refetch race introduced.
- Idempotency and leak-scoping both fall directly out of the `WHERE` clause (`user_id` + `type = 'topic_reply'` + `is_read = FALSE` + exact `topic_id` match): a second call matches zero rows and returns `200 {"marked": 0}` rather than erroring; a different topic, a different user, or a different notification type (even one like `forum_reply` that also carries a `topic_id` in its data) is untouched by construction, not by a runtime check.
- **Deliberate: the batch UPDATE is a live match, not a snapshot of the IDs the frontend last fetched.** A `topic_reply` that lands in this topic between the caller's `/grouped` fetch and the "mark all read" click is swept into the same `UPDATE` and marked read too, even though the user never saw it rendered. This is what the acceptance criteria asked for (match on "same topic, same type, unread" at call time, not a passed-in ID list) and mirrors the semantics `MarkAllRead` already has for the whole inbox — "mark all read" means "catch this up to now," the same way Slack/Gmail thread-level read actions work. An ID-pinned alternative was considered and rejected: it would require the frontend to ship a notification-ID list with every call, reintroducing exactly the per-item state this set-based design exists to avoid.
- Frontend: `NotificationsPage.handleMarkGroupRead` now makes one `fetch` to `PUT /notifications/groups/{key}/read` instead of `Promise.all`-ing the single-notification endpoint over every unread ID in the group, mirroring `handleMarkRead`'s shape (one request, then the existing single unread-count refetch on success). `markNotificationRead` is unchanged and still backs the single-notification "Mark read" buttons (singletons and expanded-group children).
- Tests: `repository/postgres/notification_test.go` (real Postgres via testcontainers — leak scoping across user/topic/type, idempotency on a second call, a malformed-`topic_id` sibling row is excluded rather than erroring the whole query, and a table-driven comparison proving the batch `UPDATE`'s end state matches what looping `MarkRead` per notification would have produced), `service/notification_group_test.go` (`ParseTopicReplyGroupKey` table test including an int64-overflow suffix, `MarkGroupRead` marks only the target group / is idempotent / rejects non-batchable keys), `handler/notification_group_test.go` (auth required, happy path delegates with the parsed topic ID, singleton/malformed keys → 400, store failure → 500, idempotent zero-marked response), `NotificationsPage.test.tsx` (one batch request replaces the per-notification PUT loop, failure path shows a toast and skips the unread-count refetch, existing single-refetch-per-action test still holds).
- **Review:** Devil's Advocate + Code Reviewer agents ran in parallel per this repo's process. No Critical/High findings from either. Devil's Advocate's one Medium (the live-match-vs-snapshot semantics above) was addressed by documenting it as an intentional design choice rather than changed, since changing it would contradict the story's own acceptance criteria. Minor findings addressed: the in-memory test double's `MarkTopicReplyGroupRead` now string-compares `topic_id` (mirroring Postgres's `->>` text-extraction semantics) instead of only matching a JSON-number `topic_id` and silently missing a JSON-string one; added the overflow test case above. Minor findings left as-is with rationale: the `200`-with-body vs. sibling endpoints' `204` response shape (intentional, documented above); no in-flight/double-click guard on the frontend button (both the batch endpoint and the optimistic local update are idempotent, so a double-click is harmless, and no sibling mark-read button in this file has this guard either); a generic "mark read matching a filter" repository method instead of a `topic_reply`-specific one (deferred — YAGNI until a second batchable group type exists).

#### BE-9.8: Forum Moderation Reason & Hierarchy [S] [DONE]
**As a** staff member
**I want** moderation actions to require a reason and respect role hierarchy
**So that** actions are accountable and lower-rank staff can't override higher-rank decisions

**Acceptance Criteria:**
- All moderation endpoints accept optional `reason` string parameter
- Reason stored in activity log events (already published via event bus)
- Moderators cannot act on topics in admin-only forums or on admin-created content
- Topic owners can lock/delete their own topics (within time window, e.g., 30 minutes)

> **Origin:** Review finding from BE-5.4 — no reason field (unlike warnings/bans), flat moderator hierarchy, zero topic-owner powers.

#### BE-9.9: Extract Shared Search Utilities [S] [DONE]
**As a** developer
**I want** search query building and tsvector utilities shared across features
**So that** torrent search and forum search don't duplicate code

**Acceptance Criteria:**
- Extract `buildPrefixQuery` from `repository/postgres/torrent.go` into shared `repository/postgres/search.go`
- Forum search uses the shared function (remove `buildForumPrefixQuery` duplicate)
- Support Unicode characters (use `unicode.IsLetter` instead of `[a-zA-Z0-9]` filter)
- Add `ts_headline` support for generating search result snippets server-side
- Add unit tests for the shared function (edge cases: CJK, emoji, special chars, very long input)

> **Origin:** Review findings from BE-5.5 — DRY violation between torrent and forum search, Unicode stripped, full post body sent instead of snippet.

#### BE-9.10: Forum Integration Tests for Transactional Paths [S] [DONE]
**As a** developer
**I want** integration tests covering the transactional code paths in forum services
**So that** the actual production SQL (not just mock fallbacks) is tested

**Acceptance Criteria:**
- Test `DeletePost` transactional path (counter decrements, last_post recalculation)
- Test `MoveTopic` transactional path (both forums' counts updated atomically)
- Test `DeleteTopic` transactional path (cascade + forum counter recalculation)
- Use test database or `sqlmock` to exercise the `if s.db != nil` branches
- Verify rollback behavior on partial failures

> **Origin:** Review finding across BE-5.3/5.4 — all service tests use `db=nil` (mock fallback), actual transactional SQL is untested.

#### BE-9.11: Hide Delete Button on First Post & Deep-Link Search Results [DONE]
**As a** user
**I want** the UI to not show delete on the opening post, and search to link to the exact post
**So that** I don't get confusing errors and can find search matches directly

**Acceptance Criteria:**
- Backend includes `is_first_post` flag in post response (or frontend checks `post.id === posts[0].id` on first page)
- Delete button hidden on first post in topic view
- Forum search results deep-link to the specific post: `/forums/topics/{id}?page=X#post-{postId}`
- Post anchors added to topic view page (`id="post-{id}"` on each post element)

> **Origin:** Review findings from BE-5.3 and BE-5.5 — delete button shown on first post causes confusing error, search results link to topic page 1 even if match is on page 5.

---

#### BE-9.12: Forum FK ON DELETE RESTRICT & Atomic Delete Checks [S] [DONE]
**As a** developer
**I want** forum category and forum deletes to be safe against race conditions
**So that** concurrent admin operations cannot accidentally cascade-delete data

**Acceptance Criteria:**
- New migration changes `forums.category_id` FK from `ON DELETE CASCADE` to `ON DELETE RESTRICT`
- New migration changes `forum_topics.forum_id` FK from `ON DELETE CASCADE` to `ON DELETE RESTRICT`
- `AdminDeleteCategory` and `AdminDeleteForum` wrapped in transactions (count + delete atomic)
- Category/forum delete with ConfirmModal instead of `window.confirm` on frontend

> **Origin:** Review findings from BE-8.5 — TOCTOU race on check-then-delete with CASCADE FKs, plus `window.confirm` inconsistency.

#### BE-9.13: Notification Email Digest [S] [DONE]
**As a** user
**I want** email digests of my unread notifications
**So that** I don't miss important activity when I'm not on the site

**Acceptance Criteria:**
- Asynq periodic task sends daily/weekly digest emails
- User preference for digest frequency (off, daily, weekly)
- Email summarizes unread notifications since last digest

> **Origin:** Deferred from BE-5.9 — in-app + WS push covers the immediate need.
>
> **Implementation notes:** Digest frequency lives in a new `notification_digest_preferences`
> table (migration 061) rather than folding into `notification_preferences` — that table is
> keyed on `(user_id, notification_type)` for per-type booleans, while digest frequency is a
> single per-user tri-state value that also needs its own `last_digest_sent_at` cursor, which
> has no natural home on a per-type row. `NotificationDigestService.Run` is scheduled once
> daily at `0 6 * * *` (an hour after promotion/invite distribution, to avoid contending with
> them) and gates each recipient independently by elapsed time since their last send (24h for
> daily, 7d for weekly) rather than a fixed calendar day — a missed scheduler run self-heals on
> the next tick instead of skipping a user's weekly digest. Emails reuse the existing
> asynq `TaskEnqueuer` → `email:send` pipeline (same as auth emails), summarizing unread
> notifications grouped by type since the last digest. Frontend control added to the
> Notifications page's Preferences tab (off/daily/weekly select).

#### BE-9.14: Notification Batching [S] [DONE]
**As a** user
**I want** grouped notifications like "5 new replies in topic X"
**So that** my notification list isn't flooded by active threads

**Acceptance Criteria:**
- Collapse multiple `topic_reply` notifications from the same topic into a single entry
- Show count and last few actors
- Expand on click to see individual notifications

> **Origin:** Deferred from BE-5.9.

**Delivered:**
- **Design: read-time grouping**, computed entirely at query time with zero changes to the write path. Chosen over write-time collapsing (a persisted, incrementally-updated row) because it keeps `Create` and the event listeners completely untouched — important given BE-9.13 (notification email digest) was an open, unmerged PR touching the same core files (`model/notification.go`, `service/notification.go`, `repository/postgres/notification.go`, `handler/notification.go`) at the same time. The trade-off: grouping considers a bounded window of a user's most recent notifications (`groupWindow = 200`, fetched via up to 2 calls to the existing `List` method — each of which is itself a `COUNT` + a `SELECT`, so up to 4 SQL statements, re-run on every page of the grouped view since nothing is cached across requests) rather than the full history. Acceptable because `DeleteOld` already purges read notifications after 90 days, so real users stay well under the cap; a user who exceeds it simply won't see their oldest entries folded into a group (still fully visible, uncollapsed, on the plain `GET /notifications` endpoint). Follow-up if this ever matters: a SQL-level `GROUP BY` aggregate query. A second accepted consequence of computing groups fresh per request: under concurrent writes, a single new reply can move a whole topic from the bottom of a page to the top of page 1 (not just shift rows by one, as the flat list would) — inherent to any live, recency-sorted grouped view (same class of behavior as Slack/Gmail threading), not a bug.
- **One non-additive change, called out explicitly per this story's rebase-safety plan:** `repository/postgres/notification.go`'s `List` query gained an `id DESC` tiebreaker (`ORDER BY created_at DESC, id DESC`, was `ORDER BY created_at DESC` alone). Two notifications can share a `created_at` (e.g. `topic_reply` fan-out to several subscribers), and grouping's actor-recency-ordering and "representative data" pick depend on `List` returning a strict, stable order across the two calls `fetchGroupWindow` makes. This is a 1-line diff on a shared file — flagged here so whoever rebases second can see at a glance it's an ordering fix, not a behavior change to what `List` returns.
- `List` is plain OFFSET/LIMIT, so a notification created between the window's two page fetches can shift every later row down by one and get re-served on the next page. `fetchGroupWindow` dedupes by ID as it accumulates so a race like that can only ever under-count a page (an item briefly absent until the next fetch), never inflate a group's count or duplicate an entry in the expand-on-click list — covered by `TestListGrouped_DedupesRowsRepeatedAcrossPagesByOffsetDrift`.
- New, additive-only backend files (no existing notification file was modified beyond the one-line route registration and the one-line `ORDER BY` tiebreaker above): `model/notification_group.go` (`NotificationGroup` type), `service/notification_group.go` (`NotificationService.ListGrouped`, built on the existing `List` method — no repository interface changes), `handler/notification_group.go` (`HandleListNotificationsGrouped`). New route: `GET /api/v1/notifications/grouped` (page/per_page/unread_only, same params as the existing list endpoint — but note `total` here counts *groups within the window*, not raw rows, since the two endpoints paginate fundamentally different things), registered in `router.go`.
- Grouping key is `type + topic_id` for `topic_reply` (extracted from the notification's existing JSON `data`, one `json.Unmarshal` per notification into a combined `{topic_id, actor_username}` struct rather than two); every other type passes through as its own singleton group (`count == 1`) so the API/UI has one uniform shape either way. Each group carries `count`, `unread` (true if *any* underlying notification is unread), `last_actors` (up to 3 distinct actors, most-recent-first), and the full underlying `notifications` array (used directly for expand-on-click — no second API call needed). A group's representative `data`/`latest_created_at` is set once, at creation, from the first (i.e. newest, given the ordering guarantee above) notification seen for that key — simpler and more robust than a running "is this newer" comparison.
- Unread-count badges and the digest email's "count of individual unread notifications" are unaffected by design: `CountUnread`/`UnreadCount` were never touched and keep counting raw rows regardless of how the list UI groups them for display.
- Frontend: `NotificationsPage` now fetches `/notifications/grouped` instead of the flat list; `notificationDisplay.ts` gained additive `NotificationGroup`, `groupMessage`, `groupLink` exports (existing `Notification`/`notificationLink`/`notificationMessage` untouched). Collapsed entries show `"N replies in \"<topic>\" from <actors>"` (only says "new replies" while the group actually has an unread member) with an expand/collapse toggle revealing the individual notifications (each with its own working "Mark read" + deep link, identical to the pre-grouping single-item UI). A group's "Mark all read" loops the existing per-notification endpoint over just that group's still-unread entries (no new batch endpoint — see accepted trade-off below) but refreshes the unread-count badge exactly once after all of them settle, not once per notification, which would otherwise race N GETs against each other for a single click. Only the group summary's type+message sits inside its `<Link>`; the count badge and action buttons are rendered as siblings so a `<button>` never nests inside an `<a>`. Marking a whole group read can empty an entire page in one click (more likely than the old one-row-at-a-time flow), so the page fetch now snaps back to the last valid page instead of rendering an empty one when the requested page no longer exists.
- **Accepted trade-off, not implemented:** a dedicated `PUT /notifications/topic/{id}/read`-style batch endpoint would collapse a group's "mark all read" into one request instead of N parallel single-notification PUTs. Out of scope for an "S" story and the frontend fix above (single refetch, partial-failure toast) makes the current N-PUT approach safe, just not maximally efficient for a topic with a very large number of replies (bounded by `groupWindow=200` in the worst case). Candidate follow-up if a topic ever realistically gets that large.
  - **Update (BE-9.26):** implemented below — `PUT /api/v1/notifications/groups/{key}/read`.
- Tests: `service/notification_group_test.go` (collapsing, actor cap/order, unread-flag derivation, pagination over groups, unread-only filter, window-cap boundary, a group straddling the window boundary gets truncated to its newest members rather than dropped, an unrelated type interleaved mid-topic doesn't split the group, malformed `topic_id` falls back to a singleton, offset-drift dedup), `handler/notification_group_test.go` (auth, collapsing via HTTP, store-failure passthrough, non-`topic_reply` types stay singleton), `notificationDisplay.test.ts` and `NotificationsPage.test.tsx` (message/link formatting, "new" vs. plain wording, collapsed rendering, expand/collapse, group mark-all-read, single unread-count refetch per group action, page-clamp on a vanished page).
- **Review:** Devil's Advocate + Code Reviewer agents ran in parallel per this repo's process; both independently flagged the unread-count refetch race and the missing `ORDER BY` tiebreaker as the top issues (both fixed above), plus several minor coverage gaps and a nested-interactive-element a11y issue (all fixed). No Critical/High findings were left unaddressed.

#### BE-9.15: Notification Cleanup in Maintenance Worker [DONE]
**As a** developer
**I want** old read notifications to be automatically purged
**So that** the notifications table doesn't grow unboundedly

**Acceptance Criteria:**
- Wire `NotificationRepository.DeleteOld()` into the periodic maintenance worker
- Configurable retention period (default 90 days for read notifications)

> **Origin:** Review finding from BE-5.6 implementation — DeleteOld exists but is not called.

**Implementation:** `DeleteOld` (which already scoped its delete to `is_read = TRUE`) is now called as step 5 of the maintenance job. Retention is `NOTIFICATION_RETENTION` (default `2160h` / 90 days). A non-positive retention **disables** the purge rather than setting the cutoff to now — otherwise a misconfigured `0` would delete every read notification.

#### BE-9.16: Notification Listener & Handler Test Coverage [DONE]
**As a** developer
**I want** tests for the notification listener and HTTP handlers
**So that** event-to-notification mapping and API responses are verified

**Acceptance Criteria:**
- Listener tests: event-to-notification mapping, dedup, self-notify skip, @mention parsing
- Handler tests: HTTP status codes, auth checks, pagination, error mapping
- Meet 80% coverage gate

> **Origin:** Review finding from BE-5.6 implementation.

#### BE-9.17: Repository Integration Test Harness (testcontainers) [DONE]
**As a** developer
**I want** the repository layer tested against a real PostgreSQL
**So that** wrong-but-valid SQL and unappliable migrations are caught before release

**Acceptance Criteria:**
- `TestMain` in `internal/repository/postgres` starts a throwaway Postgres 16 container (matching docker-compose) and applies the **real goose migrations** to it — DONE
- `go test -short` skips the container for Docker-less environments — DONE
- Revive the pre-existing `user`/`peer`/`torrent` integration tests, which skipped unless `TEST_DATABASE_URL` was set and so had **never once run** — DONE
- Repository coverage: 1.9% → **80.7%** — DONE

> **Found by this harness on its first run:** migration `039_create_forums.sql` could **never be applied to a clean database**. 007 already creates the forum tables, so 039's `CREATE TABLE IF NOT EXISTS` silently skipped and the next statement indexed a `category_id` column that was never added. Every fresh install — including any new production deploy — failed to bootstrap. Fixed by editing 039 in place to `ALTER` rather than `CREATE`; a follow-up migration could not repair it because goose stops *at* 039 and never reaches a later file. Because the harness applies the real migrations on every CI run, a migration that cannot apply to a clean database now fails the build.

#### BE-9.18: CI Quality Gates (coverage floor, gofmt, prettier) [DONE]
**As a** developer
**I want** the quality rules in CLAUDE.md to actually be enforced by CI
**So that** documented standards cannot silently drift

**Acceptance Criteria:**
- Backend coverage floor enforced in `.github/workflows/backend.yml` via `COVERAGE_FLOOR`, ratcheting upward — DONE
- `cmd/server` (bootstrap) and `internal/testutil` (test helpers) excluded from the coverage denominator via `COVERAGE_EXCLUDE` — DONE
- `gofmt` enforced through the `formatters` block in `.golangci.yml` (the lint job already ran; nothing was checking format) — DONE
- `npm run format:check` added to the frontend lint job — DONE
- `task backend:coverage` reproduces the CI gate locally and emits an HTML report — DONE

> **Origin:** Audit finding. Three rules in CLAUDE.md were enforced by nothing and had all drifted: `format:check` failed on 18 frontend files, `gofmt` on 49 backend files, and the claimed "CI gates at 80%" coverage gate **did not exist** — `backend.yml` wrote `coverage.out` and discarded it while real coverage sat at 42%. A standard nobody enforces is worse than no standard: it buys false confidence.

---

#### BE-9.19: Memoize the Toast Context Value [DONE]
**As a** developer
**I want** `useToast()` to return a stable identity
**So that** data-fetching effects don't refire on every toast

**Acceptance Criteria:**
- `ToastProvider` wraps its context value in `useMemo`, with `success`/`error`/`info`/`warning`/`removeToast` wrapped in `useCallback` (`frontend/src/components/toast/Toast.tsx:65`)
- The `// eslint-disable-next-line react-hooks/exhaustive-deps` escape hatches and the `}, []` fetch callbacks across the admin pages are reverted to honest dependency arrays
- A test asserts a failing list fetch produces exactly one request (no refetch loop)

> **Origin:** Review finding from BE-8.7. `ToastProvider` passes an object literal as its context value, so `useToast()` returns a fresh identity on every provider render — and the provider re-renders on every toast add *and* auto-dismiss. Any `useCallback(fetch, [toast])` feeding a `useEffect` therefore refires on each toast; on the error path that is self-sustaining (fetch fails → error toast → new `toast` → refetch → fails …), measured at ~324 requests in 500ms. Every admin page dodges this today by omitting `toast` from the dependency array, which silences the symptom, carries an ESLint warning, and leaves the trap armed for the next page. Fixing the provider fixes all of them at once — deliberately deferred out of BE-8.7 because it touches a shared component that parallel branches also modify.

#### BE-9.20: Bump Deprecated GitHub Actions off Node 20 [S] [DONE]
**As a** maintainer
**I want** the CI/release workflows to stop relying on Node.js 20 actions
**So that** builds keep working once GitHub removes Node 20 from runners

**Acceptance Criteria:**
- Update the actions that still target Node 20 (surfaced during the v0.11.0 release build as "Node.js 20 is deprecated … forced to run on Node.js 24"): `actions/setup-go@v5`, `actions/upload-artifact@v4`, `actions/download-artifact@v4`, and any others the warnings name — DONE
- Bump to versions that run on Node 24 (or the current supported major) across `release.yml`, `backend.yml`, `frontend.yml`, `migration-tool.yml` — DONE (`frontend.yml` did not reference any of the flagged actions; only `actions/setup-node@v5` and `actions/checkout@v5`, both already current, so it required no edits)
- Re-run a release (or a dry build) and confirm the deprecation warnings are gone — **follow-up**, requires an actual Actions run after merge; not verifiable locally

> **Origin:** Non-blocking deprecation warnings logged during the v0.11.0 release. GitHub is phasing out Node 20 on runners; currently the actions are force-run on Node 24, so nothing is broken yet — this is housekeeping before the forced-fallback is removed. See https://github.blog/changelog/2025-09-19-deprecation-of-node-20-on-github-actions-runners/.

> **Delivered:** Bumped to the current major-tag release of each action (matching the repo's existing major-only pinning style): `actions/setup-go@v5` → `@v7` (`backend.yml` ×3, `migration-tool.yml` ×3, `release.yml` ×1), `actions/upload-artifact@v4` → `@v7` (`release.yml` ×2), `actions/download-artifact@v4` → `@v8` (`release.yml` ×1). Checked each action's intermediate-major release notes for breaking changes relevant to this repo's usage: `download-artifact` v5's single-artifact-by-ID path nesting change doesn't apply (this workflow downloads via `pattern: archive-*` with `merge-multiple: true`, called out in the v5 migration guide as needing no action); `upload-artifact` v7's direct-upload mode only activates with `archive: false`, unused here. No `retention-days` params are present in these workflows. `actions/checkout@v5` and `node-version: '22'` (the app's own Node runtime, unrelated to the deprecated-action warning) were left untouched per scope.

---

#### BE-9.21: Append-Only Announce Event Log + Retention Config [M] [DONE]
**As a** site operator
**I want** every tracker announce recorded in an immutable log with configurable retention
**So that** time-windowed and per-user questions (monthly upload totals, bonus reconciliation, GDPR/LGPD data export) remain answerable after the fact

**Context:** The peers table is overwritten each announce and `transfer_history` keeps one overwritten row per (user, torrent). Both destroy the per-announce deltas at write time, so a cumulative counter can never be decomposed into "how much in June". Capturing the raw stream is a one-way door — logs can be discarded later via retention, but events never written cannot be backfilled.

**Acceptance Criteria (met):**
- `announce_events` table (migration 052): user/torrent/peer_id/ip/port/event, client-reported totals, and three deltas (`uploaded_delta`, `downloaded_delta`, freeleech-adjusted `counted_downloaded_delta`). `ON DELETE SET NULL` on torrent so history survives torrent deletion; `ON DELETE CASCADE` on user so account deletion purges it (GDPR erasure).
- `model.AnnounceEvent`, `AnnounceEventRepository` (`Create` + `ListByUser`), Postgres impl.
- Best-effort write hook in `TrackerService.Announce` — logging failure never breaks an announce; gated by `announce_log_enabled` (default on, capture-on).
- `announce_log_retention_days` setting (default 90) — **advisory only, no automatic deletion job yet** (deliberate: retention housekeeping is a follow-up).
- Frontend `SETTING_DEFINITIONS` gained labels/descriptions for the announce-log settings **and the previously-unlabelled cheat-detection and wait-time settings** (they had rendered as raw keys).
- Tests: repository (create/list/pagination/torrent-deletion), tracker hook (default-on capture, disabled flag, delta capture), and an AdminSettingsPage render test.

> **Follow-ups (not built here):** (1) a retention cleanup job that honours `announce_log_retention_days` (+ month partitioning and a nightly rollup into per-user/period aggregates for scale); (2) a GDPR/LGPD data-export endpoint over `ListByUser`; (3) bonus-points / monthly-campaign features that consume the log. See docs/FUTURE_WORK.md → "Announce Event Log — Consumers & Retention".

---

### Epic BE-10: Protocol Support

#### BE-10.1: BEncode Library [S] [DONE — replaced with github.com/zeebo/bencode]
**As a** developer
**I want** bencode encoding/decoding for Go structs
**So that** tracker endpoints speak the BitTorrent protocol

**Resolution:** Using `github.com/zeebo/bencode` instead of a custom implementation. Building a bencode library is out of scope — well-tested libraries exist.

> **Note on API design**: This project is API-first by design. ALL features are JSON REST
> endpoints under `/api/v1/...`. The only non-JSON endpoints are `/announce` and `/scrape`
> (bencode) and `/ws/chat` (WebSocket). Every story implicitly exposes its functionality
> as JSON API endpoints. The OpenAPI spec is auto-generated from route definitions (BE-0.6).
> This means any client (web SPA, TUI, mobile browser, automation scripts, Upload-Assistant
> integrations) can consume the full API with Bearer token auth (BE-1.2).

---

## Frontend Epics (FE-)

### Epic FE-0: Frontend Foundation [L]

#### FE-0.1: React + Vite + TypeScript Setup [S] [DONE]
**As a** frontend developer
**I want** a modern React project with TypeScript and fast dev tooling
**So that** I can build the UI efficiently

**Acceptance Criteria:**
- `frontend/` directory with Vite + React + TypeScript
- ESLint + Prettier configured
- Path aliases (`@/components`, `@/features`, etc.)
- `task frontend:dev` starts dev server with HMR
- `task frontend:build` produces production bundle
- `task frontend:test` runs vitest

#### FE-0.2: Theme System [M] [DONE]
**As a** user
**I want** to switch between light and dark themes
**So that** I can use the site comfortably in any lighting

**Acceptance Criteria:**
- CSS custom properties for all theme tokens (colors, spacing, typography)
- `ThemeProvider` React context
- Default theme (light) and dark theme
- Theme preference persisted in localStorage and synced to user settings API
- `prefers-color-scheme` media query respected as default
- Theme tokens documented for future theme creation

#### FE-0.3: Routing + Layout [M] [DONE]
**As a** user
**I want** consistent navigation and page layout
**So that** I can move around the site easily

**Acceptance Criteria:**
- React Router with route configuration
- Layout components: header (nav, user menu, notifications), footer, sidebar
- Responsive breakpoints (mobile, tablet, desktop)
- Protected route wrapper (redirects to login if unauthenticated)
- Admin route wrapper (redirects if not admin)
- Loading states and error boundaries per route

#### FE-0.4: API Client Generation [S] [DONE]
**As a** frontend developer
**I want** a type-safe API client generated from the OpenAPI spec
**So that** I never hand-write API calls or guess response types

**Acceptance Criteria:**
- Auto-generate TypeScript API client from backend's OpenAPI spec
- `task generate:api-client` regenerates on spec changes
- Axios or fetch-based, with interceptors for auth token injection and refresh
- All endpoints fully typed (request params, body, response)

#### FE-0.5: Auth State Management [M] [DONE]
**As a** user
**I want** to log in, stay logged in, and be redirected appropriately
**So that** authentication is seamless

**Acceptance Criteria:**
- Auth context with: user, isAuthenticated, login(), logout(), isLoading
- Token storage in memory (access token) + httpOnly cookie or localStorage (refresh token)
- Automatic token refresh on 401 response
- Login page with form validation
- Redirect to intended page after login
- Logout clears state and redirects to home

#### FE-0.6: Shared Components Library [L] [DONE — scoped: Form, Toast, Modal]
**As a** frontend developer
**I want** reusable UI components
**So that** pages are consistent and development is faster

**Acceptance Criteria:**
- `DataTable`: sortable columns, pagination, row selection, loading state
- `Pagination`: page numbers, prev/next, configurable page size
- `Form` components: Input, Select, Textarea, Checkbox, Radio, with validation integration
- `Modal`: accessible dialog with overlay, close on escape/outside click
- `Toast` notifications: success, error, info, auto-dismiss
- `MarkdownRenderer`: shared component for rendering Markdown content — see FE-0.7
- `MarkdownEditor`: toolbar with common formatting, preview toggle — shipped in FE-0.7 as a plain textarea + toolbar (no editor framework), per FE-0.7's "lightweight" acceptance criterion
- `Avatar`: user avatar with fallback to initials
- `Badge`: role badges, status indicators
- All components theme-aware (use CSS custom properties)

#### FE-0.7: Markdown Rendering System [DONE]
**As a** user
**I want** rich text formatting in descriptions, comments, forum posts, PMs, news, and chat
**So that** content is readable and expressive

**Acceptance Criteria:**
- Shared `MarkdownRenderer` component using `react-markdown` + `remark-gfm` + `rehype-raw`
- Supports standard Markdown: headings, bold, italic, strikethrough, links, images, code blocks, blockquotes, tables, lists, horizontal rules
- Supports GFM extensions: tables, task lists, strikethrough, autolinks
- Spoiler support via custom remark plugin: `!!spoiler text!!` renders as a click-to-reveal `<details>/<summary>` element
- Safe by default: sanitize HTML via `rehype-sanitize` to prevent XSS — only allow safe tags (`<details>`, `<summary>`, `<br>`, `<hr>`, `<img>` with restricted src)
- No color/font-size syntax — keep it clean and consistent
- Theme-aware: code blocks, blockquotes, tables styled with CSS custom properties
- `MarkdownEditor` component: textarea with toolbar buttons (bold, italic, link, image, code, quote, spoiler, list) that insert Markdown syntax at cursor position, plus a preview toggle that renders via `MarkdownRenderer`
- Used across: torrent descriptions, comments, forum posts, PMs, news articles, chat messages
- Formatting reference page (FE-6.3) updated to show Markdown syntax instead of BBCode
- Lightweight: no heavy editor frameworks — just a textarea with helpers + the rendering library

**Status (completed 2026-07-14):**
- `MarkdownRenderer` (`frontend/src/components/MarkdownRenderer.tsx`) — `react-markdown` + `remark-gfm` + `rehype-raw` + `rehype-sanitize`, theme-aware styling (`markdown.css`).
- `!!spoiler!!` remark plugin (`frontend/src/components/remarkSpoiler.ts`) — emits `<details><summary>Spoiler</summary>…</details>`. Matches across sibling nodes (`!!**Ending**: he dies!!`), applies CommonMark-style flanking rules so `Wow!! Amazing!!` is not a spoiler, and never fires inside code spans/blocks. A paragraph containing a spoiler renders as a `<div>` because `<details>` may not nest inside a `<p>`.
- `MarkdownEditor` (`frontend/src/components/MarkdownEditor.tsx`) — plain textarea + toolbar (bold, italic, link, image, code, quote, spoiler, list) inserting syntax at the cursor, with a Write/Preview toggle rendering through `MarkdownRenderer`. No editor framework.
- Surfaces wired: torrent descriptions (detail view + upload/edit forms), comments (display + compose + edit), PMs (detail + compose), forum topics/posts (new topic, reply, edit — display already existed), news (admin compose — display already existed), chat + shoutbox (inline mode).
- Chat/shoutbox use `MarkdownRenderer inline`: rendered in a `<span>` with block elements (headings, tables, images, blockquotes) unwrapped to their text, so a one-line message cannot break the layout. Inline emphasis, code, links and spoilers work.
- Sanitization unchanged and enforced: only `<details>`/`<summary>` added to the default schema; no `dangerouslySetInnerHTML` anywhere. XSS payloads (`<script>`, `onerror=`, `javascript:` URLs, `<iframe>`) are covered by tests on the renderer, editor preview, comments, PMs, torrent descriptions and chat.
- FE-6.3's formatting reference (`frontend/src/pages/FormattingPage.tsx`) already documented `!!hidden text!!`; the syntax it advertises now actually works.
- NFO fields stay a plain textarea on purpose — NFOs are pre-formatted ASCII art and are rendered by `NfoViewer`, not Markdown.

> **Note:** No BBCode support. The original TorrentTrader used BBCode; this reimplementation standardizes on Markdown. Legacy content is converted during migration (MT-1.5).

---

### Epic FE-1: Public Pages [L]

#### FE-1.1: Home/Dashboard [M] [DONE]
**As a** user
**I want** a homepage with site activity at a glance
**So that** I can see what's new

**Acceptance Criteria:**
- News feed (latest news articles)
- Latest torrents list
- Site stats summary (users, torrents, peers, traffic)
- Shoutbox widget (embedded chat from FE-4.1)
- Responsive layout

#### FE-1.2: Login, Signup, Password Recovery Pages [M] [DONE — login + signup, recovery deferred]
**As a** visitor
**I want** to create an account, log in, or recover my password
**So that** I can access the tracker

**Acceptance Criteria:**
- Login form with username/email + password
- Signup form with all required fields, client-side validation
- CAPTCHA support (optional, configurable)
- Email verification flow page
- Password recovery: enter email, confirmation message, reset form
- Error handling with clear user messages

#### FE-1.3: Torrent Browse + Search [L] [DONE]
**As a** user
**I want** to browse and search torrents with filters
**So that** I can find content to download

**Acceptance Criteria:**
- Category filter (hierarchical, collapsible)
- Sort by: name, date, size, seeders, leechers, completed
- Pagination with configurable page size
- Health indicators: green (well-seeded), yellow (few seeders), red (dead)
- Freeleech badge
- Search bar with instant filter
- URL-persisted filters (shareable search URLs)

#### FE-1.4: Torrent Detail Page [L] [DONE — detail page + download button]
**As a** user
**I want** to see everything about a torrent
**So that** I can decide whether to download

**Acceptance Criteria:**
- Description (rendered Markdown)
- File list (collapsible tree for multi-file torrents)
- Peer list with stats (seeders/leechers, upload speed, client)
- Comments section with pagination and reply
- Rating widget (1-5 stars, click to rate)
- Download button (generates .torrent with passkey)
- NFO viewer (if available)
- Report button
- Reseed request button (if dead)
- Edit button (if owner or moderator)

#### FE-1.5: Today's Torrents, Need Seed, Completed Views [S] [DONE]
**As a** user
**I want** quick-access filtered views
**So that** I can find torrents matching specific criteria

**Acceptance Criteria:**
- Today's torrents: uploaded in last 24h
- Need seed: torrents with 0 seeders
- Completed: user's download history
- Reuses torrent list component with pre-set filters

**Delivered:**
- The history-tab table + pagination on `UserProfilePage` extracted into a reusable `ActivityHistoryTable` component (`frontend/src/components/ActivityHistoryTable.tsx`); the `ActivityItem` type moved to `frontend/src/types/activity.ts`.
- New standalone `/completed` page (`CompletedPage.tsx`) for the logged-in user, reusing `ActivityHistoryTable` against the existing `GET /api/v1/users/{id}/activity?tab=history` endpoint from BE-3.16 — no new backend work needed. Wired into the "Torrents" nav dropdown alongside Today/Need Seed, and gated by `ProtectedRoute`.

> **Follow-up (minor, from code review):** `ActivityHistoryTable`'s Prev/Next pagination control is a near-duplicate of the one still inline in `UserProfilePage.tsx` (used by its Uploads/Seeding/Leeching tabs). Worth factoring into one shared component if another consumer shows up; not worth the churn for two call sites today.

#### FE-1.6: RSS Feed Builder Page [S] [DONE]
**As a** user
**I want** to configure a personal RSS feed
**So that** I can auto-download torrents matching my preferences

**Acceptance Criteria:**
- Category multi-select
- Language filter
- Shows preview of matching torrents
- Generates RSS URL with passkey (copy to clipboard)
- Warning about sharing passkey

---

### Epic FE-2: User Pages [L]

#### FE-2.1: User Control Panel [M] [DONE]
**As a** user
**I want** to manage my profile and settings
**So that** I can customize my experience

**Acceptance Criteria:**
- Profile edit: avatar upload, bio, signature (Markdown editor)
- Settings: theme, timezone, privacy level, notification preferences
- Password change (requires current password)
- Email change (triggers re-verification)
- Active sessions list with revoke button
- API keys management (create, list, revoke)

#### FE-2.2: User Profile Page [M] [DONE — group name, seeding/leeching counts, recent uploads, invited-by link]
**As a** user
**I want** to view other users' profiles
**So that** I can see their stats and activity

**Acceptance Criteria:**
- Public info: username, join date, role, avatar, bio
- Stats: uploaded, downloaded, ratio, seeding/leeching count
- Recent uploads (if not anonymous)
- Respects privacy settings
- Staff view: additional info (IP, email, warnings, mod notes)
- Send PM button, report button

#### FE-2.3: Torrent Upload Page [M] [DONE — drag-drop upload, category select, anonymous toggle, validation, tests]
**As an** uploader
**I want** to upload a torrent with metadata
**So that** it's available for download

**Acceptance Criteria:**
- .torrent file drag-and-drop or file picker
- Auto-fill name from torrent file
- Category select (hierarchical)
- Language select
- Description editor (Markdown with preview)
- Image upload (up to 2)
- NFO file upload
- Anonymous upload checkbox
- Client-side validation before submit

#### FE-2.4: Torrent Edit Page [S] [DONE]
**As a** torrent owner or moderator
**I want** to edit a torrent's metadata
**So that** I can fix mistakes

**Acceptance Criteria:**
- Same form as upload but pre-filled
- Staff-only fields: banned, freeleech
- Save and cancel buttons
- Shows audit info (who last edited, when)

#### FE-2.5: Private Messages [M] [DONE — inbox/outbox/compose tabs, autocomplete, reply, unread badge in header, URL-driven navigation]
**As a** user
**I want** to send and receive private messages
**So that** I can communicate with others privately

**Acceptance Criteria:**
- Inbox with unread indicators, pagination, sorting
- Outbox (sent messages)
- Compose form with user search/autocomplete for recipient
- Message view with reply button
- Delete (per-side)
- Drafts list
- Templates list (load into compose)

#### FE-2.6: Invite System Page [S] [DONE]
**As a** user
**I want** to manage my invites
**So that** I can invite friends to the tracker

**Acceptance Criteria:**
- Available invite count
- Send invite form (email)
- Invite history (sent invites with status: pending, accepted, expired)
- Invite tree (who invited whom, if enabled)

#### FE-2.7: Member List + Staff Page [S] [DONE]
**As a** user
**I want** to browse members and see staff
**So that** I can find users and know who to contact

**Acceptance Criteria:**
- Member list: paginated, searchable by username, filterable by role
- Staff page: grouped by role, online/offline indicator
- Click through to user profile

#### FE-2.8: Report Dialog [S] [DONE — ReportModal component, torrent detail integration, duplicate detection, admin resolve]
**As a** user
**I want** to report content easily
**So that** moderators can handle rule violations

**Acceptance Criteria:**
- Reusable modal component (used for torrents, comments, users, forum posts)
- Reason text field (required)
- Confirmation on submit
- Rate limit feedback if too many reports

#### FE-2.9: NFO Viewer [S] [DONE — monospace pre-formatted viewer on torrent detail page]
**As a** user
**I want** to view NFO files properly
**So that** I can read release information

**Acceptance Criteria:**
- Monospace font rendering
- ANSI art support (CP437 character set)
- Contained within a scrollable, fixed-width container
- Optional raw text download

---

### Epic FE-3: Forum [L]

#### FE-3.1: Forum Index [M] [DONE]
**As a** user
**I want** to browse forum categories and forums
**So that** I can find discussions

**Architecture:** Displays the two-level structure: forum_categories as display groups, forums listed under each category. No sub-forums.

**Acceptance Criteria:**
- Forum categories displayed as sections, each containing its forums
- Per-forum: topic_count, post_count, last post info (from denormalized fields)
- Forums hidden if user's group level < forum's `min_group_level`
- Unread indicators (bold for forums with new posts)
- "Mark all read" button
- Responsive layout

#### FE-3.2: Topic List [M] [DONE]
**As a** user
**I want** to browse topics in a forum
**So that** I can find interesting discussions

**Acceptance Criteria:**
- Pinned topics shown at top (using `pinned` flag from forum_topics)
- Status icons: locked, pinned, hot (many replies)
- Sorting: last_post_at (default), creation date, post_count
- Pagination (20/page)
- Unread indicator per topic
- New topic button (hidden if user lacks `can_forum` or group level < forum's `min_group_level`)

#### FE-3.3: Topic View [L] [DONE]
**As a** user
**I want** to read and participate in a topic
**So that** I can discuss with other users

**Architecture:** Posts are flat (not threaded), sorted by date. `reply_to_post_id` renders as an inline quote block, not as tree indentation.

**Acceptance Criteria:**
- Posts displayed flat, sorted by created_at, with user info sidebar (avatar, role, join date, post count)
- Quoting: click "quote" to insert quoted text into reply editor (sets `reply_to_post_id`)
- Reply editor: Markdown with toolbar and preview (consistent with rest of app — no BBCode)
- @mention autocomplete in editor
- Pagination for long topics (20 posts/page)
- Quote context: if post has `reply_to_post_id`, show inline quote block referencing the original post
- Edit/delete buttons (for own posts or moderators)
- Subscribe/unsubscribe toggle

#### FE-3.4: Forum Search [S] [DONE]
**As a** user
**I want** to search forum content
**So that** I can find past discussions

**Acceptance Criteria:**
- Full-text search by keyword (backed by PostgreSQL tsvector, same pattern as torrent search)
- Filter by forum, author, date range
- Results show post snippet with highlighted keywords, topic title, forum name
- Click through to post in topic (deep link to correct page/post)
- Only shows results from forums the user has access to (group level >= min_group_level)

#### FE-3.5: Forum Moderation Tools [M] [DONE]
**As a** moderator
**I want** to manage topics and posts
**So that** the forum stays organized

**Acceptance Criteria:**
- Lock/unlock topic (toggles `locked` flag)
- Pin/unpin topic (toggles `pinned` flag)
- Move topic to different forum (dropdown of available forums, respects `min_group_level`)
- Delete topic (with confirmation, updates denormalized counts)
- Delete individual posts (with confirmation)
- All actions available inline (not separate admin page)
- Actions only visible to users with moderator/admin role + `can_forum`

---

### Epic FE-4: Real-Time Features [M]

#### FE-4.1: WebSocket Chat/Shoutbox [M] [DONE — ChatSocket singleton, side chat + home page shoutbox, shared context, auto-reconnect]
**As a** user
**I want** a real-time chat widget
**So that** I can communicate with other users instantly

**Acceptance Criteria:**
- WebSocket connection with auto-reconnect
- Message list with auto-scroll
- Send message input with Enter to send
- Markdown rendering in messages
- @mention autocomplete
- User role badges next to names
- Moderator: delete message button
- Embeddable as widget (homepage) or full page
- Connection status indicator

#### FE-4.2: Notification System [M] [DONE — bell icon + /notifications page with All/Unread/Preferences tabs, WS push, mark read, per-type toggles]

#### FE-4.3: Online Users Indicator [S] [DONE — online count in footer via stats API, last_access > 15min]
**As a** user
**I want** to see who's online
**So that** I know if the community is active

**Acceptance Criteria:**
- Online users count in footer or sidebar
- Optional: list of online usernames (respects privacy settings)
- Updates periodically (polling or WebSocket)

#### BE-9.22 / FE-4.4: Gate Footer Site Stats Behind Authentication [S] [BUG] [DONE]
**As a** tracker operator
**I want** site-wide stats (online users, totals, peers/seeders/leechers) hidden from anonymous visitors
**So that** membership/activity numbers aren't leaked to the public on a private tracker with no anonymous browsing (docs/NOT_PORTING.md §9)

**Context:** `/api/v1/stats` and the footer that renders it (BE-9.3/FE-4.3) were registered as fully public with no auth gate on either side — an oversight, not an intentional guest-access feature.

**Delivered:**
- `GET /api/v1/stats` moved from the router's public-endpoints section into the `authMiddleware`-gated block (same `r.Route("/x", func(r){ authMiddleware(r); ... })` idiom as `/activity-logs`, `/store`); anonymous callers now get 401. Handler logic (`HandleStats`) unchanged. OpenAPI spec updated to declare `bearerAuth` + a 401 response.
- Frontend: `RootLayout`'s stats-polling effect now gates on `isAuthenticated`, sends `Authorization: Bearer <token>` (read fresh on every 60s tick, not hoisted, so a mid-session token refresh is picked up), and the footer `<p className="footer__stats">` block itself is gated on `isAuthenticated` too (not just on `siteStats` being non-null) — `clearInterval` on effect cleanup doesn't cancel an in-flight request, so this is what prevents a response landing just after logout from flashing stale numbers.
- Tests: `route_authz_test.go` — `fullRouterDeps` now wires a `StatsCache` (sqlmock + miniredis, matching `stats_test.go`'s technique); added `/api/v1/stats` to the member-reachability check and a dedicated `TestStatsRouteRejectsAnonymous`. `RootLayout.test.tsx` — `@/api` mock made configurable (mirrors `AuthContext.test.tsx`), the old "renders footer with stats placeholder" assertion flipped to absence-for-anonymous, new test asserts presence once authenticated.

---

### Epic FE-5: Admin Panel [L] [DONE — all FE-5.x child stories complete]

#### FE-5.0: Admin Panel Foundation [S] [DONE]
**As an** admin
**I want** the admin panel scaffolding in place
**So that** admin features can be built on a solid foundation

**Acceptance Criteria:**
- [x] Backend exposes `permissions` in `/auth/me` response (loaded from user's group)
- [x] Frontend derives `isAdmin`/`isStaff` from server-provided permissions (removed hardcoded group ID)
- [x] `AdminLayout` component with sidebar navigation (Users, Reports, Groups)
- [x] Admin routes wired under `/admin` with `AdminRoute` guard
- [x] Conditional "Admin" link in header for admin users
- [x] Backend admin route group (`/api/v1/admin/*`) with `RequireAuth + RequireAdmin`
- [x] Groups list API (`GET /admin/groups`) and groups page with permission matrix (now full CRUD — see FE-5.10 / BE-8.12)

#### FE-5.1: Admin Dashboard [M] [DONE]
**As an** admin
**I want** an overview of site health and activity
**So that** I can quickly assess the system

**Acceptance Criteria:**
- Stats cards: total users, torrents, peers, traffic (24h)
- Recent activity feed (registrations, uploads, reports)
- Quick action buttons (create news, manage reports)
- System health indicators (DB connection, Redis, storage)

#### FE-5.2: User Management [L] [DONE]
**As an** admin
**I want** to search, view, and moderate users
**So that** I can manage the community

**Acceptance Criteria:**
- [x] Search/filter: username, email, group, enabled status
- [x] Paginated table with user data
- [x] Edit user modal: group, enabled, warned
- [ ] Search/filter: IP, ratio range
- [ ] User detail view: all profile data, stats, invite history, warnings, mod notes
- [ ] Edit user: title, stats override, avatar
- [ ] Actions: warn, ban, promote/demote, reset passkey, delete
- [ ] Mod notes: add/view staff-only notes
- [ ] Bulk actions: select multiple users for group changes

#### FE-5.3: Content Moderation [M] [DONE]
**As an** admin
**I want** to manage torrents and review reports
**So that** site content stays clean

**Acceptance Criteria:**
- [x] Reports list with status filter (all/pending/resolved)
- [x] Reporter and torrent name displayed (enriched from backend JOINs)
- [x] Resolve report button
- [ ] Reports queue filter by type (torrent/comment/user/forum)
- [ ] Report detail: reported content, reporter, reason, actions taken
- [ ] Resolve report with action (dismiss, warn, ban, delete content)
- [ ] Torrent moderation: search, ban/unban, toggle freeleech
- [ ] Bulk torrent actions

#### FE-5.4: News Management [S] [DONE]
**As an** admin
**I want** to create and manage site news
**So that** I can communicate with users

**Acceptance Criteria:**
- News list with edit/delete buttons
- Create/edit form with Markdown editor and preview
- Publish/unpublish toggle

#### FE-5.5: Category & Language Management [S] [DONE — categories CRUD + hierarchical display]
**As an** admin
**I want** to manage torrent categories and languages
**So that** content organization evolves with the community

**Acceptance Criteria:**
- Category list with drag-to-reorder
- Create/edit category: name, parent, image/icon
- Language list with CRUD
- Warning before deleting category with existing torrents

#### FE-5.6: Site Settings Editor [M] [DONE — AdminSettingsPage with editable table, type-aware inputs]
**As an** admin
**I want** to configure site settings from the admin panel
**So that** I don't need to edit environment variables for runtime settings

**Acceptance Criteria:**
- Settings grouped by category (general, tracker, registration, etc.)
- Form inputs appropriate to setting type (text, number, boolean, select)
- Save with validation
- Indication of which settings require restart vs. take effect immediately

#### FE-5.7: Logs Viewer [S] [DONE — ActivityLogPage with event type filter, pagination, currently public at /log]
**As an** admin
**I want** to browse system and audit logs
**So that** I can troubleshoot issues and review actions

**Acceptance Criteria:**
- Filterable by: action type, user, date range
- Paginated results
- Log detail view
- Export/download option

#### FE-5.8: IP/Email Bans Management [S] [DONE]
**As an** admin
**I want** to manage IP and email bans
**So that** I can block abusive users

**Acceptance Criteria:**
- Ban list with search and filter
- Add ban: IP (single or CIDR), email (address or domain wildcard)
- Remove ban
- Shows who created the ban and when

#### FE-5.9: Cheat Detection Dashboard [M] [DONE — admin page with flag type/status filters, dismiss action, pagination]
**As an** admin
**I want** to review flagged users and ratio anomalies
**So that** I can investigate potential cheating

**Acceptance Criteria:**
- Flagged users list with evidence summary
- Click through to user detail with suspicious announce logs
- Actions: dismiss flag, warn user, ban user
- Ratio anomaly charts (upload/download over time)

#### FE-5.10: Group Management (CRUD) [M] [DONE — editable AdminGroupsPage: create/edit/delete, color picker, capability checkboxes]
**As an** administrator
**I want** to manage permission groups from the admin panel
**So that** I can adjust colors, levels, and capabilities without touching the database

**Acceptance Criteria (met):**
- "New Group" action and per-row Edit/Delete on the existing groups matrix.
- Modal form: name, slug (auto-derived if blank), level, hex color with a native color picker, and the eight capability checkboxes.
- Wired to `POST/PUT/DELETE /api/v1/admin/groups[/{id}]`; server-side guard rejections (protected built-in, group has members, name/slug taken) are surfaced via toast. Pairs with BE-8.12.

#### FE-5.11: Class Promotion Admin Page [M] [DONE — per-class threshold editing, add/remove rungs, Run now]
**As an** administrator
**I want** to configure the auto class-promotion ladder from the admin panel
**So that** I can decide the merit thresholds for each class without touching the database

**Acceptance Criteria (met):**
- `Class Promotion` nav entry + `/admin/promotion` route.
- Lists non-staff classes ordered by level; each ladder class has inline threshold inputs (ratio, uploaded bytes, age, torrent count, seed hours) with Save/Remove; non-ladder classes get "Add to ladder". Staff groups are excluded.
- "Run now" triggers `POST /admin/promotion/run` and reports promoted/demoted (or the skip reason). Enable/cycle/window are edited in Site Settings. Pairs with BE-8.13.

#### FE-5.12: Admin UI Component Layer + Promotion Batch Editing [M] [DONE]
**As an** administrator
**I want** a consistent, modern admin UI where changes batch into one save
**So that** managing classes and rules isn't tedious and buttons behave predictably

**Context:** First pass of the Class Promotion page saved each rule individually and added rungs one at a time; buttons were bespoke inline styles across admin pages.

**Delivered:**
- Shared `Button` component (primary/secondary/ghost/danger, sizes, loading, focus-visible ring) + additive elevation tokens (`--shadow-sm/md`, `--space-2xs/2xl`, accent-contrast) and a shared `admin-ui.css` (page header, panel, data table, sticky save bar) — the reusable admin surface layer.
- **Class Promotion** rebuilt: every non-staff class in one table with an "On ladder" toggle and inline thresholds; edits and add/remove batch into a single **Save changes** (diffed to PUT/DELETE) via a sticky bar; monospace tabular numerics; upload threshold entered in GiB. "Run now" reports results with clearer copy.
- **Groups** page adopts the same header/panel/table + `Button`.
- Backend fix: the manual **Run now** (`POST /admin/promotion/run`) now forces evaluation, bypassing the interval so freshly-changed rules apply immediately (still respects the master enable flag).

> **Follow-up:** roll the `Button` + `admin-ui` primitives across the remaining admin pages (user management, etc.).

#### FE-5.13: User Management List Revamp [S] [DONE]
**As an** administrator
**I want** the user list to match the modernized admin surface
**So that** browsing, filtering, and scanning member accounts is cleaner

**Delivered:**
- `AdminUsersPage` rebuilt on the shared primitives: `admin-page-header`, a filter toolbar (search + group + status), `admin-panel` + `admin-table`, outline pill status badges (Active / Warned / Disabled) colored by the token palette, and monospace tabular transfer figures. Retired the bespoke `admin-users.css`.
- Added a page test (list, status badges, empty state).

> **Follow-up:** `AdminUserDetailPage` (single-user editing, ~1000 lines) — adopt `Button` + `admin-panel` for its actions and sections next. *(Done in FE-5.15.)*

#### FE-5.15: Admin Surface Rollout + Invite Restriction [M] [DONE]
**As an** administrator
**I want** every admin page on the shared surface layer, and the ability to suspend a member's invite privilege
**So that** the panel looks and behaves like one product, and invite abuse can be dealt with like any other privilege

**Delivered:**
- **Backend — `invite` restriction type:** `users.can_invite` column (migration 055, default `true`), `RestrictionTypeInvite`, restriction service/handler support (`can_invite` in `PUT /admin/users/{id}/restrictions`), exposed in admin user views and public profiles. `CreateInvite` refuses with `403 invite_restricted` when suspended (invite count untouched). Registration sets `can_invite=true`.
- **`AdminUserDetailPage` rebuilt** on the shared primitives: page header with status badges + ban action, a monospace key-figure strip (uploaded / downloaded / ratio / invites / active warnings), stacked panels (Edit profile, Privileges, Recent uploads, Staff notes), and outline badges. The Privileges panel now shows all four privileges (download / upload / chat / **invite**) with Allowed/Suspended state, per-privilege Restore, a suspend form (reason + optional expiry), and the restriction history table.
- **Remaining admin pages adopted the shared layer** (`admin-page-header`, `admin-toolbar`, `admin-panel`, `admin-table`, outline `admin-badge` pills, mono `admin-num` figures, `admin-btn` family): Dashboard, Torrents, Bans, Warnings, Reports, Chat Mutes, Cheat Flags, News, Forums, Categories, Backups, Site Settings. Per-page CSS trimmed to genuinely page-specific rules; `admin-ui.css` gained panel sections/titles, stat strip, empty states, and the button family.
- Tests: invite restriction covered in service/handler tests (backend) and page tests (frontend detail page: privilege grid, suspend checkbox, PUT body, history table).
- Review hardening (PR #102 devil's-advocate findings): escalation's `warning_restrict_type = "all"` now includes invite and the settings UI offers an "Invite" option; outstanding invite tokens are **voided while the inviter's privilege is suspended** (`ValidateInvite`/`RedeemInvite` -> 410 `voided`, valid again if the restriction lifts before expiry); "Restore" heals a drifted flag even with no active restriction rows (`SyncUserFlag`); past `expires_at` is rejected with 400.

#### FE-5.16: Invite Distribution Admin Page [S] [DONE]
**As an** administrator
**I want** to configure and run the auto invite distribution engine from the admin panel
**So that** I can tune eligibility per class and force an off-cycle run without touching the database

**Delivered (pairs with BE-4.2):**
- New `/admin/invite-distribution` page (`AdminInviteDistributionPage`), a direct port of the Class Promotion admin page's pattern: one row per non-staff class with a toggle ("Eligible") plus min ratio, min/max downloaded (entered in GiB, converted to/from bytes), and max invites; edits batch into a single "Save changes" bar (PUT per changed class, DELETE when toggled off); a "Run now" button posts to the run endpoint and reports the granted count or the skip reason (e.g. "Invite distribution is off — enable it in Site Settings to run"). Reuses the shared `admin-page-header`/`admin-panel`/`admin-table`/`admin-savebar` primitives and `Button` from `@/components/form` — no new bespoke CSS.
- Nav entry ("Invite Distribution") added to `AdminLayout` next to Class Promotion; route registered in `router.tsx` under the existing `AdminRoute`/`AdminLayout` guard, so no separate permission wiring was needed.
- `AdminSettingsPage`: labelled `SETTING_DEFINITIONS` entries for `invite_distribution_enabled` and `invite_distribution_interval_days`, placed next to the `promotion_enabled`/`promotion_interval_days` entries they mirror (unlabelled settings render as raw keys — the defect flagged in BE-9.21).
- Tests: `AdminInviteDistributionPage.test.tsx` mirrors `AdminPromotionPage.test.tsx` — lists non-staff classes, disables thresholds until a class is toggled on, batches a toggle+edit into one save (PUT ×2), removes a class via DELETE, runs the engine on demand, and surfaces the disabled-skip reason.

#### BE-8.15: Admin View + Revoke of a User's Outstanding Invites [S] [DONE]
**As a** staff member
**I want** to see and permanently revoke a user's unredeemed invite tokens from their admin detail page
**So that** invite abuse can be cut off outright, not just paused

**Context:** PR #102 voids outstanding tokens *while* the inviter's `can_invite` is suspended, but tokens spring back if the restriction is lifted early and staff cannot see them at all. From the PR #102 devil's-advocate review.

**Delivered:**
- `GET /api/v1/admin/users/{id}/invites` — lists the user's invites (reuses `InviteService.ListMyInvites`, which already takes an arbitrary `userID`); same `{token, status, invitee, expires_at}` shape as the self-service endpoint
- `DELETE /api/v1/admin/invites/{id}` — `InviteService.RevokeInvite` hard-deletes an unredeemed invite (404 if not found, 409 `already_used` if already redeemed — redeemed invites are never deletable, the invitee already has an account); publishes `InviteRevokedEvent` to the activity log ("`{actor}` revoked an invite issued by `{inviter}`")
- Repository layer gained `InviteRepository.GetByID` / `.Delete`
- Outstanding-invites panel on `AdminUserDetailPage`: status badges (Pending/Redeemed/Expired/Voided), invitee name, expiry, and a Revoke button (with confirm modal) on pending and voided invites
- Tests: repository (postgres, via testcontainers), service (`RevokeInvite` success/not-found/rejects-redeemed), handler (list + revoke, including a 403 check that the routes are admin-gated), listener (activity log message), and frontend page tests (empty state, status badges, revoke flow asserting the DELETE call)
- **Review hardening (code-review + devil's-advocate on this story):**
  - **TOCTOU race in `RevokeInvite`:** the original check-then-delete (`GetByID` → check `Redeemed` → `Delete(id)`) let a redemption landing in that window get silently deleted by an unconditional `DELETE FROM invites WHERE id=$1`. `InviteRepo.Delete` now guards the same way `Redeem` already does — `WHERE id=$1 AND used_by_id IS NULL` — so the delete itself is atomic against a concurrent redemption; `RevokeInvite` maps a post-check `sql.ErrNoRows` from `Delete` to `ErrInviteAlreadyUsed`. Closed by a deterministic test (`TestInviteService_RevokeInvite_ClosesConcurrentRedeemRace`) that injects the interleaving via a `GetByID` hook — proving the DB-level guard, not the in-memory check, is what stops it — plus a real-Postgres test (`TestInviteRepoDeleteRefusesRedeemed`).
  - **"Pending" badge ignored PR #102's own invite-voiding:** suspending a user's `can_invite` already blocks their outstanding tokens at `ValidateInvite`, but the admin panel (and the member's own Invites page) kept showing them as live "Pending" tokens. `model.Invite` gained an enrichment-only `Voided` field; `ListMyInvites` sets it when the inviter's `CanInvite` is false and the invite is unredeemed; `inviteResponse` reports `status: "voided"`. Both `AdminUserDetailPage` and `InvitesPage` render a distinct Voided badge (Revoke stays available on the admin panel, since deleting a voided row is still a valid cleanup action; the member's copy-code/copy-link buttons correctly disappear since `=== "pending"` no longer matches).

#### BE-8.16: Root-Cause the Privilege-Flag Drift Race [M] [DONE]
**As a** site operator
**I want** privilege flags (`can_download/upload/chat/invite`) to never drift from the restriction rows
**So that** enforcement and the admin UI always agree

**Context:** Enforcement reads the per-user flag, but several flows do full-row `users.Update` after a stale read (login `LastLogin` write, `CreateInvite` decrement, admin profile save). A restriction applied inside such a window is silently overwritten: the history table says Suspended while the flag says Allowed. Pre-existing for download/upload/chat; surfaced by the PR #102 review. PR #102 added `SyncUserFlag` as a band-aid on the restore path only.

**Delivered:** replaced full-row writes with a targeted `UPDATE users SET ...` statement on the one hot path that legitimately changes these flags, mirroring how `bonus_points` was clobber-proofed in BE-8.14 — chosen over making `HasActiveByType` the enforcement source of truth because it closes the race everywhere `Update` is called (present and future) rather than only at today's enforcement call sites, and needs no extra DB round trip per request.
- `UserRepo.Update` (`internal/repository/postgres/user.go`) no longer writes `can_download`, `can_upload`, `can_chat`, or `can_invite` at all — structurally identical to how `bonus_points` is excluded. `can_forum` stays in `Update`; nothing outside `Create` ever mutates it, so it isn't part of the race.
- New `UserRepo.SetPrivilegeFlag(ctx, userID, restrictionType, value)` does a single-column `UPDATE`, mapping the (already-validated) restriction type to its column via a closed switch — the only path that may change those four columns.
- `RestrictionService.ApplyRestriction` and `.restoreUserFlagIfNone` (used by lift, expiry resolution, and `SyncUserFlag`) call `SetPrivilegeFlag` directly instead of read-mutate-write; `restoreUserFlagIfNone` no longer fetches the user at all. The dead `setUserFlag` mutator was removed.
- `NewUserRepo` now returns the concrete `*postgres.UserRepo` (was the `repository.UserRepository` interface) so callers that need `SetPrivilegeFlag` — kept off the widely-implemented `UserRepository` interface via a local `privilegeFlagRepository` interface in `service/restriction.go`, so the ~16 other mocks implementing `UserRepository` elsewhere in the test suite are untouched — get it without a type assertion; it still satisfies `UserRepository` wherever that's expected.
- Regression test `TestUserRepoUpdate_DoesNotClobberPrivilegeFlags` (postgres, via testcontainers) reproduces the exact race — a stale read, a concurrent `SetPrivilegeFlag`, then the stale flow's `Update` landing — and asserts the flag survives. Plus `TestUserRepoSetPrivilegeFlag` (all four types, both directions), not-found and unknown-type cases, and an entry in the `TestRepositoriesPropagateDBErrors` table.

#### BE-8.17: Admin User Edit History + Unit-Aware Size Editing [M] [DONE]
**As a** staff member
**I want** every admin edit of a user profile recorded (old value, new value, who, when) and byte fields editable in human units
**So that** economy-moving changes (uploaded/downloaded/invites) are auditable and don't require byte arithmetic at terabyte scale

**Delivered:**
- Migration 057: append-only `user_edit_history` (`user_id` FK CASCADE, `changed_by` FK SET NULL so history outlives a deleted admin, `field`, `old_value`, `new_value`, `created_at`; `(user_id, created_at DESC)` index)
- `repository.UserEditHistoryRepository` (`Record` batch insert in one tx, `ListByUser` newest-first with limit/offset + total) + postgres impl and testcontainers tests (record/list, pagination, empty no-op, deleted-admin NULLs)
- `AdminService.UpdateUser` diffs every field it can change (username, email, avatar, title, info, group — recorded by *name*, not ID — uploaded, downloaded, enabled, warned, donor, parked, invites, bonus_points) into audit rows; only actual changes are recorded, and each batch is written **after** the write that persists it succeeds (full-row `Update`, `SetInvites`, `SetPoints` each flush their own entries — a failed later write can't record unpersisted changes, and old values are captured before the write to dodge aliasing). Recording is best-effort: a failed audit insert is logged at ERROR, never rolls back the user update. Nil repo (tests/partial wiring) skips recording.
- `GET /api/v1/admin/users/{id}/edit-history` (`?limit` ≤200 default 50, `?offset`) → `{entries, total, limit, offset}` with `changed_by_username` joined; admin-gated (covered by the route-walking authz test) + handler tests (records-then-lists round trip, 404, 400, 403)
- **`ByteSizeInput`** shared form component (`@/components/form`): amount input + unit select (B/KB/MB/GB/TB, 1024-based to match `formatBytes`); canonical bytes state — unit switches only convert the *display*, so opening and saving a form never silently rounds a stored value; exact `= N bytes` hint for non-byte units. Replaces the raw "Uploaded (bytes)" / "Downloaded (bytes)" inputs on `AdminUserDetailPage`.
- Edit-history panel on `AdminUserDetailPage`: ledger table (Field, From → To, Changed by, When) with uploaded/downloaded humanized (raw bytes kept in `title`), booleans as Yes/No, group by name, deleted admins rendered muted; "Show older edits (N more)" pagination
- Form-rhythm fix (the "fields almost touching" complaint): shared `.form-stack` primitive in `form.css` gives any form body consistent vertical gaps (adopted by the edit-profile form and the suspend-privileges section); textarea got `min-height` + vertical resize; inputs got a soft accent focus ring — all token-based, site-wide
- Tests: service (records changes incl. group names, skips no-ops, survives record failure, works without repo, list + not-found), repository, handler, `ByteSizeInput` (9 cases incl. display-only unit conversion, parent-value re-derive, empty-field protection, safe-integer clamp), page tests (unit-converted save payload, dirty-fields-only body, no-op save skips the request, history render + humanized values, empty/error states, load-more dedupe)
- **Review hardening (code-review + devil's-advocate on this story):**
  - **Dirty-fields-only saves (the critical find):** the form used to re-send every field as an absolute value on save, so uploaded/downloaded accrued by announces between page-open and save were silently reverted — and the revert landed in the shiny new audit trail as a phantom admin edit. `handleSaveProfile` now diffs against the last-fetched user and sends only changed fields (the API was already pointer-based/partial); a no-op save makes no request at all. Same protection for invites and bonus_points, which also accrue concurrently (auto-grants, seeding awards).
  - **Audit rows survive their subject:** `user_id` deliberately has no FK — the cleanup worker hard-deletes `enabled=false` accounts, and CASCADE would have let deleting an account destroy the evidence of edits made to it (repo test proves survival). `changed_by` keeps SET NULL **plus a write-time `changed_by_username` snapshot**, so attribution survives the acting admin's deletion too (reads prefer the live username via COALESCE). Added the `changed_by` index the SET NULL resolution needs.
  - `ByteSizeInput`: a transiently emptied field no longer zeroes the stored value (blur restores the display from canonical bytes; an explicit "0" still zeroes); amounts clamp at `Number.MAX_SAFE_INTEGER`; the exact-bytes hint is `aria-describedby`-linked.
  - History panel: load-more dedupes by id (offset pagination over a newest-first list re-serves rows when a fresh edit shifts offsets); fetch failures render a distinct "Couldn't load the edit history" + Retry instead of masquerading as "No edits recorded yet"; when a byte change is smaller than the 2-decimal display (5 GB at 50 TB scale), From/To fall back to exact byte counts so a real change never renders as "50.00 TB → 50.00 TB"; Yes/No mapping applies only to known boolean fields (a title literally equal to "true" stays "true").
  - Timestamps in the new endpoint are `.UTC()`-normalized before the `Z`-suffixed format; `?limit` clamps to 200 instead of resetting to 50; reused `derefString` instead of adding a twin helper.
  - **Accepted trade-offs (documented, not bugs):** audit recording is best-effort (logged at ERROR, never rolls back the already-committed update) — full tx integration is BE-8.18; `bonus_points` is intentionally double-recorded (bonus ledger `admin_adjust` = balance source of truth; edit history = admin-edit view); the invites `old_value` snapshot can be stale under a concurrent auto-grant (the set itself is race-safe).

#### BE-8.18: Harden the Admin Stats Write Path + Audit Durability [M] [DONE]
**As a** site operator
**I want** uploaded/downloaded excluded from full-row `users.Update` and audit rows written atomically with the update
**So that** no stale full-row write can clobber announce-accrued stats and the audit trail cannot silently lose entries

**Context:** from the BE-8.17 devil's-advocate review. The frontend now only sends dirty fields, which closes the minutes-long page-open-to-save window, but a millisecond server-side window remains: `UserRepo.Update` still writes `uploaded`/`downloaded` from a value read at request start, racing `IncrementStats` from announces — the same shape BE-8.16 closed for privilege flags and invites. Scope: (1) exclude uploaded/downloaded from `Update`, add a targeted `SetStats` used only by the admin edit path; (2) write `user_edit_history` rows in the same transaction as the write they describe (repo gains tx-aware variants or the update+audit composes via `repository.WithTx`); (3) optional: keyset pagination (`before_id`) for the edit-history endpoint to replace the dedupe-on-append mitigation.

**Delivered:**
- `UserRepo.Update` (`internal/repository/postgres/user.go`) now also excludes `uploaded`/`downloaded`, joining the four privilege flags, `invites`, and `bonus_points` already excluded. A new `UserRepo.SetStats(ctx, userID, uploaded, downloaded *int64, entries)` is the only path that may change them; `uploaded`/`downloaded` are independently optional (`nil` leaves a counter alone via `COALESCE` rather than writing back a value read at request start), so an admin edit that only dirties one of the two can't reopen the same clobber window for the other.
- Went past the letter of "write audit rows in the same transaction as the write they describe" to close it for every admin-edit write, not just stats — matching BE-8.17's own accepted-trade-off note ("full tx integration is BE-8.18", stated without a stats-only qualifier). Four methods now each commit their write and its `user_edit_history` rows in one transaction: `UserRepo.UpdateWithHistory` (the diffed profile fields — username, email, avatar, title, info, group, enabled, warned, donor, parked), `UserRepo.SetStats`, `UserRepo.SetInvites` (now takes `entries`), and `BonusRepository.SetPoints` (now takes `entries`, alongside the balance write and `admin_adjust` ledger row it already wrote transactionally in BE-8.17). A shared unexported `insertEditHistoryTx` helper (`internal/repository/postgres/user_edit_history.go`) backs both `UserEditHistoryRepo.Record`'s own transaction and these four callers' existing ones, so the insert logic has one source of truth.
- `AdminService.UpdateUser` (`internal/service/admin.go`) no longer calls `UserEditHistoryRepository.Record` at all: it builds each write's audit batch before calling the corresponding atomic method, so a failed audit insert now rolls back the write it describes — instead of BE-8.17's best-effort behavior, where the update had already committed and a failed audit insert was only logged at ERROR. `UserEditHistoryRepository`/`SetEditHistoryRepo` remain wired for the read side (`ListUserEditHistory`); the now-dead best-effort `recordEditHistory` helper was removed.
- `adminUserWriteRepository` (local to `admin.go`, renamed from `inviteSetRepository` — it outgrew that name once `SetStats`/`UpdateWithHistory` joined `SetInvites`) follows the `privilegeFlagRepository`/BE-8.16 pattern so the broader `UserRepository`-implementing mocks elsewhere are untouched.
- **Deferred (marked optional in the story):** part 3, keyset pagination (`before_id`) for the edit-history endpoint. The offset-based endpoint and its load-more dedupe-by-id mitigation from BE-8.17 are unchanged; still a reasonable follow-up if the dedupe mitigation ever proves insufficient in practice.
- No frontend changes: the admin edit-history/profile-save API contract (request/response JSON shapes) is unchanged — this story is a server-side write-path hardening only.
- **Review hardening (code-review + devil's-advocate on this story):**
  - **Event-publishing reorder (the substantive find):** `UpdateWithHistory`, `SetStats`, `SetInvites`, and `BonusRepo.SetPoints` are four independent transactions — a deliberate scope boundary, not an oversight (the story asks for audit rows atomic with *the write they describe*, not one whole-request transaction spanning unrelated columns; collapsing all four into one transaction was rejected as a much larger change — see below). The devil's-advocate review found a real consequence of that boundary: state-change events (ban/unban/warn/unwarn/group-change) were published only after *all four* writes succeeded, so a ban that durably committed via `UpdateWithHistory` could end up with no `UserBannedEvent` (no ban PM, no downstream listeners) if a later, unrelated write — e.g. `SetStats` — then failed. Fixed by moving event publishing to right after `UpdateWithHistory` commits, since none of those five events depend on the later stats/invites/bonus writes. `TestAdminUpdateUser_LaterStageFailureLeavesEarlierWriteCommitted` (service) proves the fixed shape end-to-end: `SetStats` fails, the ban and its audit row are still durable, and `UserBannedEvent` still fires.
  - **Repo-level tx-aware methods vs. service-layer `repository.WithTx`:** the story named both as acceptable; this went with the former. Composing everything through `repository.WithTx` in the service layer (the pattern `forum.go`/`torrent.go` use) would mean duplicating `UpdateUser`'s extensive field-diffing logic across two parallel implementations — a transactional-SQL path and a mock/interface path for unit tests without a real `*sql.DB` — for a method with roughly a dozen diffed fields plus group/invite/bonus resolution. The four small, self-contained repo methods added here follow the existing `BonusRepo.SetPoints`/`PurchaseItem` precedent (one method, one transaction, no service-side duplication) and keep `UpdateUser` single-implementation.
  - Test-mock precision: `mockUserRepo` (service) had one shared `writeErr` that always tripped on `UpdateWithHistory` first (it runs before `SetStats`/`SetInvites`), so the original failure test could not actually prove a *later* stage's failure behaves correctly — split into independent `updateErr`/`statsErr`/`invitesErr` fields; `TestAdminUpdateUser_UpdateWithHistoryFailureAbortsWholeUpdate` and the new `_LaterStageFailureLeavesEarlierWriteCommitted` now each target one stage.
  - **Accepted trade-offs (documented, not bugs):** the four admin-edit writes remain independent transactions (see above) — an earlier write's commit is never rolled back by a later write's failure; the admin sees an error, and only the failed stage's field is left unchanged. Stats' `old_value` audit snapshot can be stale under a concurrent announce between the initial `GetByID` and the `SetStats` call (the set itself, via `COALESCE`, is race-safe) — the same accepted trade-off BE-8.17 documented for invites, now extended to stats. `BonusRepo.SetPoints`'s audit entry can, in the same rare window, describe a `bonus_points` change the ledger didn't apply (a concurrent write makes the request's target equal the live balance, so `delta` becomes 0 and no ledger row is written, but the entry built from the request-start value is still inserted) — an extension of BE-8.17's accepted bonus_points double-record trade-off.

**Tests:** repository (postgres, via testcontainers) — `TestUserRepoUpdateDoesNotWriteStats` / `TestUserRepoUpdate_DoesNotClobberStats` mirror the BE-8.16/BE-8.17 privilege-flag/invites regression tests for the same race shape applied to stats (stale read, concurrent `IncrementStats`, stale `Update` lands, stats survive); `TestUserRepoSetStats` and `TestUserRepoSetStats_PartialUpdateDoesNotClobberConcurrentAccrual` cover the absolute set and the `COALESCE` partial-update semantics under a concurrent accrual. One atomicity pair per write path proves the transaction is real (not just "no error"), by making the audit insert fail on a `changed_by` FK violation (a nonexistent admin id) and asserting the primary write was rolled back too: `TestUserRepoUpdateWithHistory_RecordsAtomically` / `_AuditFailureRollsBackWrite`, `TestUserRepoSetStats_AuditFailureRollsBackWrite`, `TestUserRepoSetInvites_AuditFailureRollsBackWrite`, `TestBonusRepo_SetPoints_RecordsHistoryAtomically` / `_AuditFailureRollsBackWrite`. Service-level `AdminService.UpdateUser` tests updated for the new write shape; BE-8.17's `TestAdminUpdateUser_RecordFailureDoesNotFailUpdate` (which asserted the old best-effort behavior) was replaced with `TestAdminUpdateUser_WriteFailureAbortsWholeUpdate`, which asserts the opposite by design.

#### BE-8.19: Cleanup Worker Hard-Deletes Banned Accounts [S] [BUG] [DONE]
**As a** site operator
**I want** the expired-registration cleanup to only delete accounts that were never activated
**So that** banning a user doesn't quietly erase their account a week later

**Context:** found while verifying audit retention for BE-8.17. Step 4 of the cleanup worker (`internal/worker/handlers.go`) deletes `users WHERE enabled = false AND created_at < NOW() - 7 days` (excluding only pending unexpired email confirmations). Bans set `enabled = false` — so any banned user whose *registration* is older than 7 days matches and is hard-deleted on the next cleanup run, cascading away their torrents/notes/warnings and defeating the ban record itself. The filter needs a "never activated" signal (e.g. `last_access IS NULL`, or an explicit registration-pending state) instead of bare `enabled = false`.

**Delivered:**
- An explicit `activated_at` column (migration 060), not `last_access`: `last_access` is only touched by `ActivityTracker` middleware on a *subsequent* authenticated request (debounced 5 minutes), so a user banned immediately after registering or right after their first login — before any follow-up request — would still read as "never activated" under `last_access IS NULL` and reproduce the same bug in a narrower window (caught by the devil's-advocate review before merge). `model.User.ActivatedAt` is instead stamped exactly once, with no follow-up request required: at registration when no email confirmation is needed (`AuthService.Register`), at email confirmation (`ConfirmEmail`), or at first login (`Login`) — whichever happens first — and is never touched by either ban path (`AdminService.UpdateUser` / `QuickBanUser`, both plain read-modify-write of the whole row).
- Cleanup query (`internal/worker/handlers.go` step 4) changed to `enabled = false AND activated_at IS NULL AND created_at < NOW() - 7 days AND NOT EXISTS (pending unexpired confirmation)`.
- Migration 060 backfills `activated_at = COALESCE(last_login, last_access)` for existing rows so already-banned-but-previously-active accounts are protected immediately on deploy, without waiting for a login that a banned account will never make.
- `internal/repository/postgres/user.go`: `activated_at` added to `userColumns`/`scanUser`/`Create`/`Update` (full-row `Update` carries the in-memory value forward, same pattern as `last_access` — never independently mutated by a ban).
- Tests: repo-level round-trip (`TestUserRepoActivatedAtRoundTrips`), service-level coverage that each of the three producer paths sets (or correctly withholds) `ActivatedAt` (`auth_test.go`, `email_confirmation_test.go`), and a worker-level regression test against a real Postgres (new `internal/worker/main_test.go` testcontainers harness, mirroring `internal/repository/postgres` and `cmd/backfill-mentions`) proving a user banned after a prior login *and* a user banned immediately after a no-confirmation registration both survive a cleanup run, while a genuinely abandoned unconfirmed registration is still deleted — mirrors `TestUserRepoUpdate_DoesNotClobberPrivilegeFlags` (BE-8.16). Verified non-vacuous by reverting the query and confirming the test fails.

#### BE-8.20 / FE-5.17: Username Profile Route + Resolved @Mention Linking [L] [DONE]
**As a** member
**I want** profile URLs built from usernames instead of numeric IDs, and `@mentions` in comments/forum posts to link to the mentioned person
**So that** profile links are shareable/guessable, and a mention is something you can click through to, not inert text

**Delivered — Part A, username-based profile route:**
- `GET /api/v1/users/{username}` (was `{id}`): `UserService.GetProfile` now resolves via the existing `GetByUsername` instead of `GetByID`; `HandleGetProfile` reads the raw path segment with no numeric parsing. Clean break, no numeric-ID fallback — usernames can legally be all-numeric (`^[a-zA-Z0-9_]{3,20}$`), so a dual-resolution endpoint would risk a username colliding with someone else's ID. No persisted server-side link ever embedded the old numeric form, so this is a pure live-computed-link migration; the accepted trade-off is that (unlike the old ID) a username is mutable and unreserved after a rename, so a link can 404 or later resolve to a different person.
- Frontend: `/user/:id` → `/user/:username`; `UserProfilePage` fetches by username and — since the numeric-ID-only `torrents`/`activity` sub-endpoints deliberately weren't touched — switched to building those two URLs from the already-fetched `profile.id` instead of the raw route param (mirroring how the warnings fetch already did this). `UsernameDisplay` (18 call sites) and 5 direct link sites now build `/user/{username}`. Six call sites that pass a synthesized fallback (`comment.username ?? "User #123"`) when the real username is missing now pass `noLink` too, so a garbage link (`/user/User #123`) can never be built from that fallback string.

**Delivered — Part B, resolved @mention linking:**
- Migration 058: `mentioned_usernames JSONB NOT NULL DEFAULT '[]'::jsonb` on `torrent_comments` and `forum_posts` (JSONB, not a native array — no precedent for `TEXT[]` anywhere in the schema, and this is the codebase's established pattern for structured per-row data). New `UserRepository.GetByUsernames` (`WHERE username = ANY($1)`, one round trip, native pgx v5 array encoding — not `pq.Array()`, which is `lib/pq`-only).
- `ResolveMentionedUsernames` (new, in `internal/service/mention.go`, alongside — not replacing — the existing `publishMention`/notification pipeline): extracts `@usernames` with the same regex the notification listener already uses, dedupes, batch-resolves, returns only the canonical usernames of tokens that are real users. Wired into `CommentService.CreateComment`/`UpdateComment` and `ForumService.CreateTopic`/`CreatePost`/`EditPost` — edits re-resolve so a mention added later still becomes linked, but edits never call `publishMention`, so editing doesn't spam a new notification (accepted gap: a user mentioned for the first time via an edit gets no notification for it, only the link once someone revisits the thread).
- Response wiring (`commentResponse`, `postResponse`) exposes `mentioned_usernames`; a soft-deleted forum post redacts it in the exact same branch that already blanks `body` for non-staff viewers, so who a hidden post mentioned can't leak alongside what it said.
- Frontend `remarkMention` plugin (mirrors the structural pattern of the existing `remarkSpoiler`, simpler since mdast already has a native `link` node type): per-text-node regex splice using the *identical* left-boundary rule as the backend (`(?:^|[\s(])@(\w+)`), so an email-shaped `foo@bar` is never linkified even when `bar` is a real, validly-mentioned username elsewhere in the same content. Only tokens present in the resolved `mentionedUsernames` set (passed as a new `MarkdownRenderer` prop) become links to `/user/{username}`; `rehype-sanitize`'s schema is extended (by appending to the *existing* `className` allow-list entry for `<a>`, not adding a second one — `hast-util-sanitize`'s `findDefinition` only honors the first match per attribute name) so a `.mention` class can style it distinctly. Wired through `CommentsSection` and `ForumTopicViewPage`.
- **Scope, deliberately**: comments + forum posts only for v1. PMs are not just "unwired" but arguably a different problem — they're strictly 1:1 in this app, so a mention notification inside one would almost always duplicate the "new message" notification the sole recipient already gets, and the one case where it'd add information (mentioning a third party outside the thread) isn't representable in the current message model. News is deferred as genuinely net-new wiring (no existing resolution call site).
- Tests: repository (`GetByUsernames`, mentioned_usernames round-tripping through Create/Update/GetByID/List for both tables, via testcontainers), service (`ResolveMentionedUsernames` incl. dedup/unknown-token/email-boundary/error-propagation, the five call sites setting it, the edit-does-not-notify split), handler (response field, the soft-delete redaction case), frontend (`remarkMention` via `MarkdownRenderer.test.tsx` — valid/unresolved/omitted/email-boundary/multi-mention/start-of-string/code-span/inline-mode cases — plus `CommentsSection`/`ForumTopicViewPage` prop-threading).
- **Caught in review:** the `className` schema fix above was a real bug caught by the test suite itself — the first attempt appended a *second* `['className', 'mention']` tuple to the sanitize schema, which silently never took effect because `hast-util-sanitize` only consults the first entry per attribute name (the existing footnote-backref entry). Positive-assertion tests (does the link have the class), not just negative ones (does an invalid token stay plain), are what caught it — a purely negative-assertion suite would have shipped this passing.

#### BE-8.21 / FE-5.18: Backfill Historical @Mentions + Resolve @Mentions in PM Bodies (Links Only) [M] [BUG] [DONE]
**As a** tracker operator
**I want** existing comments/posts to get working @mention links retroactively, and PM bodies to resolve @mentions too
**So that** the mention feature doesn't silently only work on content created after it shipped, and a mention in a PM is something you can click through to like everywhere else

**Context:** found immediately after BE-8.20/FE-5.17 shipped in v0.15.0 — the operator tested it live and found mentions in pre-existing posts/comments rendered as plain text while a brand-new post worked. Root cause: migration 058 added `mentioned_usernames` with `DEFAULT '[]'::jsonb` but never backfilled existing rows — the column is only populated on create/edit, so anything written before the feature existed is stuck at `[]` forever. Separately, the operator asked for PM body mentions to link too; BE-8.20's PM exclusion was specifically about not duplicating the "new message" notification, which doesn't block link-only rendering.

**Delivered:**
- `backend/cmd/backfill-mentions`: one-off tool (`go run ./cmd/backfill-mentions [--dry-run]`) that keyset-paginates `torrent_comments`, `forum_posts`, and `messages`, reruns the real `service.ResolveMentionedUsernames` against each row's body, and writes back via a narrow `UPDATE <table> SET mentioned_usernames = $1 WHERE id = $2` only — deliberately not `CommentRepo.Update`/`ForumPostRepo.Update`, both of which also stamp `updated_at`/`edited_at`+`edited_by` and would have made every backfilled row incorrectly show up as "(edited)". Reprocesses unconditionally (mentioned_usernames=[] is indistinguishable between "processed, no mentions" and "never touched", and resolution is a pure function of body, so re-running is always safe/idempotent — demonstrated by its own test suite). `MarshalMentionedUsernames`/`ScanMentionedUsernames` (`internal/repository/postgres/mentioned_usernames.go`) exported so the tool shares the exact encoding, not a hand-rolled copy. Never touches the event bus — no retroactive "you were mentioned" notifications from historical content.
- Migration 059 extends `mentioned_usernames` to `messages`. `MessageService.SendMessage` now calls `ResolveMentionedUsernames` and persists the result — links only, exactly as before: no `publishMention` call anywhere in the message flow, guarded by a dedicated regression test (`TestSendMessage_MentionDoesNotPublishUserMentionedEvent`) so a future copy-paste from `CreateComment` can't silently reopen the notification-duplication problem this was originally excluded to avoid.
- Frontend: `MessagesPage`'s one `MarkdownRenderer` call for the opened message body now receives `mentionedUsernames={selectedMessage.mentioned_usernames}` — the only frontend change needed, since `MarkdownRenderer`/`remarkMention` were already generic.
- **Explicitly out of scope, confirmed correct as-is, no change made:** forum topic titles never resolve mentions (`CreateTopic` only extracts from the first post's body) and the PM subject field has no autocomplete (plain `<input>`, never wired to `MarkdownEditor`) — both were already the desired behavior.
- Tests: repository (`mentioned_usernames_repo_test.go` gains a messages round-trip + malformed-JSON-degrades case, mirroring the existing comment/forum ones), service (mention resolution + the no-notification regression guard), handler (`stubUserRepo.GetByUsernames` was a hardcoded no-op that would have silently no-op'd any mention test built on it — fixed to filter by username like the other mock user repos — plus a positive content-assertion test, not just key presence), frontend (`MessagesPage.test.tsx` mention-link-renders / unresolved-stays-plain, mirroring `CommentsSection.test.tsx`), and three `backend/cmd/backfill-mentions` integration tests via the same testcontainers pattern (resolves across all three tables while leaving `updated_at`/`edited_at` untouched; `--dry-run` writes nothing; schema check passes post-migration).

#### BE-8.22: Torrent Submission Moderation [L]
**As a** tracker operator
**I want** uploaded torrents to be reviewed and approved before they go public
**So that** low-quality, mislabeled, or malicious uploads never reach members, and trusted uploaders can self-approve

Ships in three staged PRs (a/b/c), each backend+frontend complete.

##### BE-8.22a: Status, gating, queue, claim, approve [DONE]
- Migration 067 adds `moderation_status` (`pending`/`approved`/`rejected`, default pending, CHECK), `assigned_moderator_id`, `approved_by`, `approved_at` to `torrents` (+ indexes); **existing rows backfilled to `approved`**. Migration 068 seeds `moderation_enabled=true` and `moderation_public_visibility=false`.
- New uploads are `pending` unless `moderation_enabled=false` (then auto-approved, no human approver). Enforcement at every gate: the public list filter adds `moderation_status='approved'` (browse/home/today/needseed/completed/RSS/search + `ListByUploader`); `GetByIDForViewer` 404s a pending/rejected torrent for non-uploader/non-staff (unless `moderation_public_visibility` reveals *pending* — never rejected); `DownloadTorrent` 403s the `.torrent` for non-uploader/non-staff; and the **tracker announce** rejects an unapproved torrent for anyone but the uploader/staff (`ErrTorrentNotApproved`) — even a hand-crafted announce can't seed it.
- `TorrentModerationRepository` (separate interface, mirrors `GroupWriteRepository` so the many read-only torrent mocks are untouched): `ClaimModeration` (staff can steal a stale claim), `UnclaimModeration`, `ApproveTorrent` (records approver + timestamp), `RejectTorrent`, `ListModerationQueue` (status + all/mine/unassigned filters, FIFO). Approve authorization lives in the service (staff in this PR; extended to self-approving Uploaders in 8.22c) so `POST /api/v1/torrents/{id}/moderation/approve` needs no route move later; claim/unclaim/reject/queue sit under `/api/v1/admin/moderation/*` (`RequireStaff`).
- Frontend: a Moderation panel on the torrent detail page (status, assigned moderator, Claim/Approve/Reject for staff, "Approved by X" when approved, pending banner + download-gate messaging); `/admin/moderation` queue page with All/Unassigned/Mine filters + Claim, linked from a staff-visible nav item; two toggles on the Site Settings page.
- Tests: repo (claim/approve/reject/unclaim, queue filters + pagination, public-list exclusion), service (upload default, viewer gate incl. public-visibility + rejected-stays-hidden, download gate, approve/claim/reject authz, unavailable), tracker (pending announce: uploader/staff pass, others rejected), handler (approve staff/non-staff/unauth, claim, reject, queue), frontend (queue page, moderation panel, approved-by line). Coverage ≥ floor.

##### BE-8.22b: Moderation message thread + notifications [DONE]
- Migration 069 adds `torrent_moderation_messages` (torrent_id FK ON DELETE CASCADE, author_id, body, created_at; index on `(torrent_id, created_at)`).
- `TorrentModerationMessageRepository` (Create / ListByTorrent / CountByTorrent); the moderation-queue query now populates `message_count` in one grouped query over the page (`fillMessageCounts`), so the queue and panel show "N messages" without a per-row round trip.
- `TorrentService.ListModerationMessages` / `PostModerationMessage` — readable and postable by staff **and the uploader** only. Posting publishes `TorrentModerationMessagePostedEvent` carrying the uploader + assigned-moderator ids; a notification listener creates a `moderation_message` notification for each, minus the actor (never self-notified). New `model.NotifModerationMessage` added to `AllNotificationTypes` so it appears in preferences.
- Endpoints `GET|POST /api/v1/torrents/{id}/moderation/messages` (auth; service authorizes).
- Frontend: a Discussion thread (list + composer) inside the torrent detail Moderation panel; `moderation_message` wired into `notificationDisplay` (links to the torrent) and the notification-preferences label.
- Tests: repo (message CRUD + count, queue `message_count` population), service (post/list authz for staff/uploader/stranger, empty body, event recipients), listener (notify uploader/moderator, skip actor, no-moderator case), handler (post/list authz), frontend (thread render + send, notification mapping). Coverage ≥ floor.

##### BE-8.22c: Uploader self-approval role [DONE]
- Migration 070 adds a `can_self_approve` capability flag to `groups` and seeds a dedicated **Uploader** group (level 50) carrying it. Staff "promote" a trusted member by assigning them this class via the existing user group dropdown — no new admin UI, and the flag is intentionally *not* a per-group toggle (the group `Create`/`Update` SQL leaves it untouched, so it can't be flipped or cleared by accident from the group editor).
- `model.Group` / `model.Permissions` / `PermissionsFromGroup` carry `CanSelfApprove`; the group reads (`List`/`GetByID`) select it, so it flows into the session and `/auth/me` automatically.
- `TorrentService.canApprove` widens: staff always, plus a `CanSelfApprove` member on their **own** pending upload — recorded with their own name as approver, delivering the "human self-review that's still logged" the class is for. The approve endpoint added in 8.22a needed no route change.
- Frontend: `UserPermissions` gains `can_self_approve`; the torrent detail Approve button shows for a self-approving uploader on their own pending torrent.
- Tests: model (PermissionsFromGroup carries the flag; plain groups never do), repo (the seeded Uploader group reads back with the flag; no other group has it), service (uploader self-approves own → recorded as self; can't approve others'; plain member can't self-approve). Coverage ≥ floor.

##### BE-8.22d: Notify the uploader on approve/reject [DONE]
- Approve/reject were silent — the uploader had no signal their submission went live or got bounced. Both now publish a `TorrentModeratedEvent`; a listener creates a `moderation_decision` notification for the uploader ("Your torrent X was approved/rejected", links to the torrent). The actor is skipped by `NotificationService.Create`, so an Uploader self-approving their own upload isn't pinged. New `model.NotifModerationDecision` added to `AllNotificationTypes`; frontend `notificationDisplay` + preferences label wired. Tests: service (approve/reject publish), listener (uploader notified; self-approver skipped), frontend (message + link, approved/rejected).

#### BE-10: External Notification Connectors [L] [IN PROGRESS — design in `docs/NOTIFICATION_CONNECTORS.md`, plan in `docs/plans/BE-10.md`]
**As a** tracker operator
**I want** to announce released torrents to external/internal channels through one pluggable interface
**So that** adding "announce to chat / IRC / Discord / Telegram / webhook / a live feed" is a small self-contained module rather than a new special case

**Design:** see `docs/NOTIFICATION_CONNECTORS.md` (formalizes the reaction-side plugin idea from `docs/EXTENSIBILITY.md` and the "IRC/Discord announce bot" from `docs/TRACKER_MODS.md`). A single canonical `TorrentPublished` event (emitted on approve + auto-approve upload — never leaks pending/anonymous) → a dispatcher builds an `Announcement` → fans out to enabled, filtered **connector** instances via an **async, isolated, retrying** delivery pipeline (worker). Connectors register at compile time (no hot-loading).

**Phased (each phase is a BE+FE story; let the pattern earn each step):**
- **BE-10.1 — seam + first two connectors:** `TorrentPublished` event, `Announcement`, `Connector` interface + registry, `notification_connectors` table + admin CRUD, delivery pipeline (isolation/retry/timeout/log). **Multiple instances per kind** (except chat, a singleton), each independently enable/disable + optional global kill-switch. Built-ins: **Chat** (reuses `ChatService`) and **generic Webhook** (HMAC + SSRF guard). FE: admin Connectors page + test-send. **[DONE]**
- **BE-10.2 — IRC:** `PersistentConnector` + `ConnectorManager` lifecycle (reconnect, channels, category routing, rate-limit) **+ single-owner leader election** across nodes (Postgres advisory lock / Redis lease; single-process = no election, the zero-config default). FE: IRC config + connection status. **[DONE]**
- **BE-10.3 — SSE live feed:** authenticated `GET /api/v1/announce-stream`, hub fan-out. FE: opt-in "live releases" view. **[DONE]**
- **BE-10.4 — Discord + Telegram:** thin specializations of the webhook/bot-POST path.
- **BE-10.5 (future) — per-user relay:** per-user subscriptions/filters over the same `Announcement`/SSE machinery.

**Security:** announce only published (non-anonymous-safe) torrents; connector secrets **plaintext in DB for now** (operator's single trust boundary) but *marked* so encryption/references can be added later, and **never logged or returned by the API**; webhook SSRF guard; delivery isolated/retried/timed-out/rate-limited/deduped + observable.

##### BE-10.1: Connector seam + Chat + Webhook + admin CRUD [DONE]
- **The seam.** `internal/connector` is the whole contract: `Connector` (Kind/Singleton/SecretFields/ValidateConfig/Deliver), a compile-time `Registry`, the `Announcement` payload, `Filters`, secret redaction/merge helpers, and an SSRF-safe HTTP client factory. `PersistentConnector` is declared now (IRC needs it in BE-10.2) so the interface does not churn later. Adding a destination is one package plus one `Register` call — everything else is inherited.
- **One canonical publish signal.** New `TorrentPublishedEvent`, emitted at exactly two points in `TorrentService`: after a successful `ApproveTorrent`, and after an upload that auto-approves because moderation is off. Connectors never hook moderation internals. `ErrNotPending` already blocks re-approval, so there is no double-emit path; a rejected→approved transition is a legitimate first publish. The event is deliberately **not** subscribed by the activity log — `torrent_uploaded`/`torrent_moderated` already cover the same transitions, and staying out of the log means the payload never reaches `activity_logs.metadata`.
- **Anonymity at the source.** `TorrentPublishedEvent.UploaderName` is empty whenever `Anonymous` is set — the service drops it rather than each renderer redacting it — so no listener, stored payload or connector output can leak it. Renderers print "Anonymous". Pinned by tests at the service, dispatcher and webhook layers.
- **Delivery pipeline.** The dispatcher listener (5s context, inside the approve request) only writes `connector_deliveries` rows and enqueues an asynq drain — it never calls `Deliver`, so a wedged endpoint cannot slow down or fail an approval. Dedupe is the DB's job: `UNIQUE (instance_id, event_key)` with `ON CONFLICT DO NOTHING`, and work is only queued when a row was actually inserted. Retry is app-managed (`attempts`/`next_attempt_at`, 30s doubling to a 10m cap, dead-lettered as `failed` after 5) because coalescing and dedupe need the whole pending set in view, which asynq's per-task retry cannot see. Per-instance `rate_per_min` (default 20) caps sends per minute; when the budget is short the last message is spent on one "+N more" summary and the rows it covered close as `coalesced`, so nothing is ever silently dropped. `recover()` turns a panicking third-party client into a failed attempt instead of a dead worker.
- **Delivery is at-least-once, not exactly-once.** The unique index guarantees one delivery *row* per (instance, event); the wire is a separate question. A drain claims each row with a short lease (`ClaimForDelivery` pushes `next_attempt_at` forward) before delivering, which is what stops two overlapping drains announcing the same torrent twice — asynq's `Unique` window only stops a *duplicate enqueue*, not concurrent execution of two tasks enqueued further apart. A lease rather than a status flag means a worker that dies mid-delivery just becomes due again when it expires, with no attempt burned and no rows stuck in-flight. What remains is the classic gap: if `Deliver` succeeds and the `MarkSent` write then fails, the row retries and the message goes out twice. Receivers that care can deduplicate on `X-Announce-Delivery`.
- **Three findings from the pre-push review shaped the pipeline** and are worth recording, because each was invisible in tests: (1) asynq holds a task's uniqueness lock for the duration of that task, so a drain scheduling its own follow-up under `Unique` was always rejected as a duplicate — the documented 30s/1m/2m/4m backoff never ran and everything silently degraded to the 5-minute maintenance sweep; the follow-up path now enqueues without collapsing, and a miniredis-backed test asserts what actually reaches the queue. (2) Coalescing is now a per-kind decision (`Connector.Coalescable`): folding N announcements into "+N more" is the point for a shoutbox but destroys data for a webhook, whose receiver gets one torrent and a count and can never learn the rest — machine-read kinds spend the whole budget individually and defer the remainder instead. (3) `ValidateTemplate` now renders as well as parses, because `{{.Titel}}` is syntactically valid and only fails at execution, so a single typo used to save cleanly and then dead-letter every announcement for that instance.
- **Secrets are write-only.** GET responses strip secret keys entirely and report only `secrets_set`; on update an omitted or blank secret keeps the stored value, a non-empty one replaces it, and an explicit JSON `null` clears it. Every delivery error passes through `RedactError` before it reaches a log line or `last_error` — the backstop for cases like a Telegram bot token living inside the request URL that `net/http` echoes back.
- **SSRF guard** (`connector/httpguard`): the check runs in the dialer's `Control` callback on the IP actually being connected to, which is what defeats both DNS rebinding and redirect chains in one place. Rejects loopback, RFC1918, link-local (incl. `169.254.169.254`), CGNAT, multicast, broadcast, unspecified and the IPv6 equivalents; proxies are disabled (a proxy would make the guard inspect only the proxy). Site setting `connectors_allow_private_networks` is the homelab escape hatch.
- **Chat connector** posts announcements to the shoutbox as authorless system messages. Migration 072 makes `chat_messages.user_id` nullable and adds `system` — a fake "system user" row was rejected because it would pollute user lists, member counts and admin search. `ChatService.SendSystemMessage` skips the mute/privilege checks (there is no user to police) and the message is broadcast live to WS clients.
- **Webhook connector** POSTs the canonical announcement JSON with `X-Announce-Event`/`-Delivery`/`-Timestamp` and an optional `X-Announce-Signature` (`sha256=hmac(secret, timestamp + "." + body)`); errors carry the status only, never the body or URL.
- **Admin surface.** Seven admin-only routes (list incl. registered kinds, create, get, update, delete, synchronous test-send, paginated delivery log); test-send bypasses filters, the kill-switch and the queue but still goes through the SSRF guard and still records a row. Site settings `connectors_enabled` (global kill-switch), `connector_delivery_retention_days` and `connectors_allow_private_networks` seeded by migration 073. Maintenance prunes the log and re-enqueues instances with due rows — the crash-recovery net.
- **FE:** `AdminConnectorsPage` (table with enable toggle, last-delivery status chip, test/log/edit/delete; per-kind config sub-forms; write-only secret fields with a "clear" checkbox; category/min-size/freeleech/anonymous filters), plus system-message styling in both `Chat` and `Shoutbox` (no profile link, no mod actions — there is no user behind it).
- Migrations 071–073. Tests: connector core + httpguard + both connectors, service CRUD/secret-merge/test-send, dispatcher (kill-switch, filters, dedupe, anonymity), worker drain (backoff, dead-letter, coalescing vs deferral, claim races, panic recovery, redaction), enqueuer collapse semantics against miniredis, handler authz + write-only responses, Postgres integration (DB-enforced dedupe, exclusive claim, singleton index, cascade, retention sparing pending rows), and frontend page + Chat + Shoutbox tests.

**Follow-ups filed from the BE-10.1 review (none blocking):**
- **BE-10.1a — connector rate window should use the database clock.** `CountSentSince` and `ClaimForDelivery` take a Go-clock timestamp and compare it against `updated_at`/`next_attempt_at` written by `NOW()`. A database clock more than a minute behind the app would disable rate limiting entirely. Pass an interval and let Postgres compute the boundary.
- **BE-10.1b — coalesced summaries under-report across batch boundaries.** The "+N more" count comes from the ≤100-row `ListDue` page, so a 300-row backlog produces several summaries whose numbers do not add up to the real total. Needs a count query rather than a slice length.
- **BE-10.1c — redaction only matches exact substrings.** `RedactString` uses `strings.ReplaceAll`, so a percent-encoded or otherwise transformed occurrence of a secret survives. No live leak today, but BE-10.4's Telegram token lives inside the request URL, so add an encoded-form pass before that phase.
- **BE-10.1d — map the singleton unique-violation to 409.** The service checks `CountByKind` first, but two admins creating the second chat instance at the same moment hit the database index instead and get a 500 where the migration comment promises a 409. Unwrap `*pgconn.PgError` and map `23505`.
- **BE-10.1e — webhook `headers` are stored and returned in plain text.** A deliberate v1 tradeoff (the UI now says so and steers credentials to the HMAC secret), but an `Authorization` value typed there is echoed in every `GET /admin/connectors`. Either add per-kind "sensitive value" declarations beyond top-level `SecretFields`, or drop the free-form headers editor.


##### BE-10.2: IRC connector [DONE]
- **The first connector with a lifecycle.** Everything before it was a stateless call; IRC holds a TCP/TLS session, authenticates, joins channels and stays there. `PersistentConnector.Start` (declared in BE-10.1 precisely so this phase needed no interface churn) blocks for the connection's life, and a `ConnectorManager` decides which instances should be running, restarts them when an admin edits one, and stops them on shutdown so a QUIT is actually sent. Reconnection is `ergochat/irc-go`'s job — chosen over `girc` because it ships a reconnect loop and SASL rather than leaving both to us — and channels are joined from the connect callback so a reconnect re-joins.
- **Exactly one node may hold the connection.** Two would join the channel twice and announce everything twice. A Postgres advisory lock keyed `(0x636F6E6E, instance_id)` decides it, held by a dedicated `*sql.Conn` rather than the pool — a session-scoped lock belongs to whichever connection took it, and from the pool it could be released by an unrelated later query. The lock is never explicitly unlocked: ending the session releases it, so the crash path and the graceful path free it by exactly the same mechanism. A single-process deployment acquires instantly and never notices there was an election.
- **The ownership watchdog is the load-bearing part.** Postgres cannot tell us when a session dies, so the owner re-proves the lock every 10s by pinging its own connection; any failure cancels the client's context immediately rather than waiting for anything else. Without that, a network partition leaves two nodes both convinced they are the owner. The test for it was mutation-verified: disabling the check makes it fail.
- **A reconnect is not a delivery failure.** A new `connector.ErrNotReady` sentinel — in the shared package, not the IRC one, so the drain handler stays generic — reschedules a delivery in 15s *without* consuming one of its five attempts. Otherwise a routine reconnect would dead-letter a whole queue of perfectly good announcements. It is bounded: past 15 minutes the delivery fails like anything else, so "not ready" cannot mean "never".
- **CR/LF injection is closed at the inputs, not the output.** A torrent name is chosen by whoever uploaded it and IRC is newline-delimited, so a name carrying CR/LF would end the PRIVMSG and let the rest be read as a fresh command from our authenticated bot. Every template-interpolated field is stripped of control bytes *before* rendering, so a custom template cannot reintroduce the hole; lines are also truncated to ~400 bytes on a rune boundary.
- **Per-channel category routing** lets one connection feed `#movies` and `#tv` from the same instance, with a 1s pause between channels on top of the pipeline's `rate_per_min`.
- **`ConnectorConfigChanged`** is published by every service mutation so the manager reconciles immediately instead of polling; an unchanged instance is left alone rather than being disconnected and reconnected on every save.
- **FE:** IRC sub-form (server/port/TLS/nick, write-only SASL and NickServ passwords, a channel editor with per-channel category filters, template) and a connection badge polled from `GET /admin/connectors/status` every 10s — only when an IRC instance exists. The status is explicitly node-local: a standby honestly reports "another node" rather than guessing about the cluster.
- No migration. Tests: IRC routing/injection/truncation/not-ready/validation against a fake client, manager lifecycle (ownership loss, restart-on-edit, disable, panic recovery, status) against fake leases, the lease's exclusivity and session semantics against real Postgres plus its guard paths with sqlmock, drain not-ready handling, config-change publication, and the status route. `go test -race` clean — it caught a mutable package var the watchdog test was mutating.
- **Two of the plan's decisions were wrong and are superseded.** Decision 14 said the lock "is never explicitly unlocked — session end releases", and that `Confirm` could be a `SELECT 1`. Both were disproven against a real database during review: (a) `(*sql.Conn).Close()` returns the connection to the *pool*, it does not end the Postgres session, so the advisory lock survived `Release()` — wedging every later acquire until `ConnMaxLifetime` recycled it, and meanwhile handing ordinary web traffic a connection still holding a connector lock. `Release` now runs `pg_advisory_unlock` and poisons the connection if that cannot be proven. (b) `SELECT 1` proves the socket is alive, not that the lock is held, so a lock dropped without the session dying left the owner permanently convinced it was the owner. `Confirm` now queries `pg_locks` for this backend's own grant. The tests that covered the old behaviour passed for the wrong reason — `database/sql` reuses free connections LIFO, so on an idle pool the "second node" acquire landed on the very same session and succeeded re-entrantly. The replacements assert against `pg_locks` directly.
- **Operational caveats worth knowing:** status is per-node by design; and a node whose database connection stalls without dropping will keep the lock until `tcp_keepalives_*` (or `statement_timeout`) notices, so those settings bound the split-brain window. Each running persistent instance permanently occupies one pooled connection, so keep the instance count well under `DB_MAX_OPEN_CONNS`.


##### BE-10.3: SSE live feed [DONE]
- **The feed is a connector, which is the point.** `/live` is fed by the same pipeline as everything else, so it inherits filters, the kill-switch, the delivery log and test-send for free — an admin can narrow it to one category or switch it off from the same panel as the webhooks, and nothing about the page needed special-casing in the dispatcher or the worker.
- **`AnnounceHub`** mirrors `ChatHub`: register/unregister/broadcast channels, a small per-client buffer, and drop-the-slow-client. Broadcast is a non-blocking send — it runs on a worker goroutine holding one of ten asynq slots, so a stalled hub must not be able to park it (the same lesson BE-10.1's review taught about `ChatHub.Broadcast`).
- **Auth is a query-param token** because `EventSource` cannot set headers — the same trade `/ws/chat` makes — validated against live sessions on every connect, so logging out stops the feed at the next reconnect. The route is registered outside the auth middleware group for that reason, which makes it exactly the route worth pinning in `route_authz_test.go`.
- **A pre-existing bug had to be fixed first:** `middleware.statusRecorder`'s doc comment claimed it delegated `Flush`, but there was no such method. Embedding `http.ResponseWriter` promotes only that interface's methods, so `w.(http.Flusher)` failed for every handler behind the request logger and no streaming response could ever have flushed. Fixed with the method the comment already promised, plus tests for both `Flush` and the `Unwrap`/`Hijack` path the WebSocket upgrade silently depends on.
- **Bounded by design:** at most five streams per user (one pinned tab in twenty windows would otherwise multiply every announcement twenty times), a 16-frame client buffer, a 25s heartbeat comment so proxies do not reap an idle stream on a quiet tracker, and `X-Accel-Buffering: no` so nginx does not hold frames until its buffer fills.
- **FE:** `/live` (member route, "Live" in the Torrents nav) with a `useAnnounceStream` hook. `EventSource` reconnects on its own, so the hook reports connecting/live/reconnecting rather than reimplementing retry. The list is newest-first, capped at 100, deduped by torrent id (a reconnect replays), and renders a coalesced event as a single "+N more" summary row instead of dropping it.
- **Multi-node caveat:** fan-out reaches only clients connected to the node that ran the delivery. Correct for the single-process default; Redis pub/sub between hubs is future work and deliberately not built.
- **Found in self-review** (the review agents were unavailable): the hook relied on `EventSource`'s built-in retry, which always reconnects to the URL it was constructed with — and the token is in that URL. After a token refresh the browser would have retried a request the server must reject, forever, silently killing the feed until a page reload. The stream is now torn down and rebuilt with a freshly read token. Also fixed: the dedupe set grew for the life of the page even though the visible list is capped, index-based React keys remounted every row on each new event, and registration counted-then-inserted so two simultaneous connects could both pass the per-user cap. One of my own tests was racy — it drained the "healthy" client from a goroutine the scheduler need not run between broadcasts, so that client was sometimes dropped too; it now models keeping up with a buffer larger than the burst.
- No migration — 071's singleton index already covered `sse`. Tests: hub auth/framing/heartbeat/unregister/per-user cap/slow-client drop over a real socket, connector delivery + anonymity + config rejection, the middleware Flusher fix, the unauthenticated route, and the page's states, dedupe, cap, coalesced row and anonymity.

#### BE-9.24: Restrict Sensitive Activity Log Entries to Staff [S] [BUG] [DONE]
**As a** tracker operator
**I want** operationally sensitive activity log entries (backups, cheat flags, ban patterns, moderation actions) hidden from regular members
**So that** the public activity log doesn't hand cheaters detection tells, ban evaders the banned patterns, or expose infrastructure details and private moderation

**Context:** operator-reported — `GET /api/v1/activity-logs` is member-visible by design, but every event type was published to it, including `operator created database backup: <dump name>` and `cheat flag (upload_no_downloaders) raised for <user> on <torrent>`. The endpoint also returned each entry's raw `metadata` (the full marshaled event JSON) to every member — `InviteRedeemedEvent` metadata even carries the invite token.

**Delivered:**
- `event.IsStaffOnly` / `event.StaffOnlyTypeStrings` (`internal/event/staff_only.go`): single source of truth classifying 23 event types as staff-only — backups (opsec), cheat flags (detection tells), email/IP bans + quick bans (evasion aid; quick-ban messages embed reason and +IP/+email scope), password/passkey resets (social-engineering aid), torrent reports + resolutions (reporter anonymity), warnings/restrictions incl. manual warn/unwarn (private moderation), and all invite events (social-engineering surface; redeemed metadata carries the token). Bans, promotions, uploads, forum/chat moderation, registrations stay public. Classification is by event type at read time — no schema change, no backfill, and historical rows are covered retroactively; flipping a type later is a one-line change.
- `ActivityLogService.List` appends the staff-only set to a new `ExcludeEventTypes` repo filter unless `opts.IncludeStaffOnly` is set (repo renders it as `event_type NOT IN (...)`, composing with the existing `event_type`/`actor_id` filters, so a member explicitly querying an excluded type gets nothing). `HandleList` sets `IncludeStaffOnly` from `PermissionsFromContext(...).IsStaff()`, and now only includes `metadata` in the response for staff viewers — members get id/type/actor/message/timestamp only. The admin dashboard's recent-activity feed passes `IncludeStaffOnly: true` (route is behind `RequireAdmin`).
- Frontend: no change needed — `ActivityLogPage` renders whatever the API returns and never used `metadata`; staff continue to see everything through the same page.
- Tests: event classification round-trip, service exclusion on/off, handler member-vs-staff (exclusion opts + metadata gating, incl. that an invite token in metadata is not serialized for members), postgres repo `ExcludeEventTypes` filtering (rows, count, and composition with `event_type`), and the invite-distribution integration test updated to list as staff since `invite_auto_granted` is now staff-only.

#### FE-5.14: Bonus Store Page + Points Display [M] [DONE]
**As a** member
**I want** to see my bonus points and spend them in a store
**So that** seeding visibly pays off

**Delivered (pairs with BE-8.14):**
- `/store` page (`BonusStorePage`): balance card, item cards with price + Buy (shared `Button`; disabled when unaffordable, loading state), disabled-store and empty states, success toast + `refreshUser()` on purchase, server error messages surfaced. Nav entry in the Community dropdown.
- Balance exposed end-to-end: `OwnerProfile.bonus_points` → `/auth/me` → AuthContext; "Bonus Points" stat tile on the own-profile page (owner-only field).
- Admin: "Bonus Points" input on the user detail form (mirrors Invites; recorded to the ledger as admin_adjust); `bonus_enabled` + `bonus_points_per_seeding_torrent` labelled in Site Settings.
- Tests: store page (render, disabled state, purchase + refresh, error message, unaffordable-disabled), user-detail field populated.

---

### Epic FE-6: Static/Info Pages [S]

#### FE-6.1: FAQ Page [S] [DONE]
**As a** user
**I want** to read frequently asked questions
**So that** I can find answers without asking staff

**Acceptance Criteria:**
- Static content page with collapsible Q&A sections
- Content stored as a React component or Markdown file (not in DB)

#### FE-6.2: Rules Page [S] [DONE]
**As a** user
**I want** to read the site rules
**So that** I know what's expected

**Acceptance Criteria:**
- Static content page
- Numbered rules with sections
- Content stored as a React component or Markdown file (not in DB)

#### FE-6.3: Markdown Formatting Reference [S] [DONE]
**As a** user
**I want** a formatting reference
**So that** I can format my posts and descriptions correctly

**Acceptance Criteria:**
- Side-by-side: Markdown syntax example and rendered output (using `MarkdownRenderer` from FE-0.7)
- Covers: headings, bold, italic, strikethrough, links, images, code (inline + block), blockquotes, tables, lists (ordered + unordered), horizontal rules, spoilers (`!!text!!`)
- Linkable from editor toolbars
- No BBCode — this project standardizes on Markdown

> **Depends on:** FE-0.7 (Markdown Rendering System) for live rendered previews

---

### Epic FE-7: Theme Management [M]

#### FE-7.1: Theme Switching UI [S] [DEFERRED — moved to docs/FUTURE_WORK.md]
#### FE-7.2: Admin Theme Configuration [S] [DEFERRED — moved to docs/FUTURE_WORK.md]
#### FE-7.3: Additional Theme (Retro/Classic Tracker) [M] [DEFERRED — moved to docs/FUTURE_WORK.md]

#### FE-7.4: Migrate Remaining `window.confirm` Dialogs to `ConfirmModal` + Escape-to-Close Opt-Out for Edit Modals [S] [BUG] [DONE]
**As a** staff user
**I want** every destructive-action confirmation to use the same styled modal, and edit forms to stop closing (and discarding my input) on a stray Escape press
**So that** the admin UI is visually consistent and I don't silently lose in-progress edits

**Context:** Not part of Theme Management — numbered here simply as the next free `FE-x.y` slot at the time this was filed. BE-9.12 previously migrated forum category/forum delete off `window.confirm` onto `ConfirmModal`, but two more admin pages were missed: `AdminGroupsPage` (group delete) and `AdminCategoriesPage` (**torrent** category delete — a separate page from the forum-category admin UI BE-9.12 covered, easy to conflate by name). Separately, `Modal`'s Escape-to-close handler was unconditional and applied to every modal built on it, including edit/create forms with free-text inputs — a stray Escape press silently closed the form and discarded whatever the user had typed, with no warning.

**Delivered:**
- `AdminGroupsPage.handleDelete` and `AdminCategoriesPage.handleDelete` migrated from `window.confirm` to `ConfirmModal`, following the open-modal-state + async `onConfirm`/`onCancel` pattern `AdminTorrentsPage` already used for torrent delete.
- Repo-wide grep (`window.confirm`, `window.alert`, `window.prompt`, bare `confirm(`/`alert(`/`prompt(`) confirmed no other native-dialog call sites remain in `frontend/src` outside test files, where the only matches are `alert(...)` inside XSS test fixture strings (not real dialogs).
- `Modal` gained a `closeOnEscape?: boolean` prop, default `true` — every existing call site (including `ConfirmModal`, which always passes through to `Modal`) keeps today's behavior with zero changes required.
- Audited every direct `<Modal>` usage and set `closeOnEscape={false}` on the ones hosting free-text/multi-field input the user is actively composing: torrent delete-with-reason (`TorrentDetailPage`), forum topic lock/unlock/pin/unpin/rename/move/delete-with-reason (`ForumTopicViewPage`), report-torrent (`ReportModal`), ban-user (`BanUserModal`), reset-password (`AdminUserDetailPage`), and the group/torrent-category/forum-category/forum edit forms (`AdminGroupsPage`, `AdminCategoriesPage`, `AdminForumsPage`). Left at the default (still closes on Escape) for pure confirmation dialogs and static-content views: every `ConfirmModal` usage, the passkey-regeneration confirm (`UserSettingsPage`), the report-resolution modal (`AdminReportsPage`), and the read-only edit-history viewer (`ForumTopicViewPage`).
- Fixed a pre-existing bug surfaced while auditing `ForumTopicViewPage`'s five moderation modals: they share one `modReason` textarea's state, and only the *open* handlers reset it — the five Cancel buttons didn't, so a reason typed and then cancelled could resurface pre-filled the next time a different one of those modals was opened by the same route. All five now go through a dedicated close handler (used for `onClose` and Cancel alike) that always clears `modReason`.
- Fixed `AdminCategoriesPage`: the leftover-error banner from a previous failed delete is now cleared the moment a *new* delete confirmation is opened, not only on the next successful/failed confirm — previously a stale "category has torrents" banner (now describing the wrong category) could sit on screen through a Cancel.
- For consistency, gave `AdminTorrentsPage`'s delete flow (the pattern the two migrations mirror) the same `deleting`/`loading` double-submit guard the migrated pages now have — it previously had none.
- Tests: `Modal.test.tsx` covers the `closeOnEscape={false}` opt-out; new `ConfirmModal.test.tsx` covers Escape still closing a confirmation dialog and the `loading` state disabling both buttons; `AdminGroupsPage.test.tsx`/`AdminCategoriesPage.test.tsx` updated to drive the `ConfirmModal` UI (click Cancel, click Delete, press Escape) instead of stubbing `window.confirm`, plus a new real-page test per page proving Escape does *not* close the edit/create form and does not discard typed input; `BanUserModal.test.tsx`/`ReportModal.test.tsx` gained the same real-page Escape-preserves-input coverage for their respective forms.

**Deferred / explicitly out of scope:**
- Dirty-tracking / "discard changes?" confirmation-on-close for edit modals — the alternative to the flat Escape opt-out chosen here. Meaningfully more invasive (per-form dirty-state plumbing across every edit modal); out of scope for this pass. Flag as a follow-up if a specific modal is later found to need it.
- Overlay-click and the modal's "×" button still close edit modals unconditionally (only the Escape vector was disabled). This was a deliberate scope boundary, not an oversight: the requester's brief specifically named Escape as the problem to fix and specifically ruled out building the dirty-tracking machinery (above) that would be needed to guard all three close vectors consistently without also making the "×" button/overlay click silently inert in a way that would surprise users used to those affordances always working. A future pass implementing option (b) would naturally cover this too.
- `ChatModMenu.tsx` (the chat moderation dropdown) has its own unconditional `keydown` → close-on-Escape handler guarding a mute-duration/mute-reason mini-form, with the same failure mode as the `Modal`-based dialogs — but it isn't built on `Modal` at all, so it fell outside this story's literal scope ("every place `<Modal>` is used directly"). Worth a follow-up story if this pattern needs the same treatment.

> **Origin:** Follow-up to BE-9.12's `window.confirm` → `ConfirmModal` migration, which missed `AdminGroupsPage`/`AdminCategoriesPage`; combined with a UX request to stop discarding in-progress edits on a stray Escape press. Devil's-advocate + code-reviewer pass (per this doc's Post-Implementation Review process) surfaced the `modReason` carry-over bug, the stale-error-banner bug, the `AdminTorrentsPage` loading-guard inconsistency, and the `ChatModMenu` gap.

#### FE-BUG-1: Invites Page Doesn't Reflect Updated Invite Count After Admin Edit [S] [BUG] [DONE]
**As a** member whose invite count was changed by staff
**I want** the Invites page to show my current invite balance
**So that** I don't have to log out and back in to see (or use) invites an admin just granted me

**Context:** tracked informally in `tasks/todo.md`'s Known Bugs section before this story. `InvitesPage` reads `user.invites` from `AuthContext`, which fetches `/api/v1/auth/me` once on app bootstrap/login and caches the result in React state; `refreshUser()` re-fetches it but nothing calls it automatically. When staff edit a user's `invites` field via `AdminUserDetailPage` (`PUT /api/v1/admin/users/:id`, backed by the race-safe `UserRepo.SetInvites` from BE-8.17/BE-8.18), the edited member's own already-open session never sees the new value — `InvitesPage` only called `refreshUser()` after its own self-service "Generate Invite" action, never on mount. This isn't a one-off: `UserSettingsPage` hit the identical staleness class for `avatar`/`title`/`info`/`passkey` and already fixes it with a dedicated `useEffect(() => { refreshUser(); }, [refreshUser])` on mount — `InvitesPage` had simply never adopted that pattern. Confirmed by reading, not reproduced live: the two effects (`fetchInvites` and `refreshUser`) are independent, fire once per mount, and `refreshUser` is a stable `useCallback` off `AuthContext`, so no race/loop risk.

**Delivered:**
- `InvitesPage.tsx`: added the same mount-time `useEffect(() => { refreshUser(); }, [refreshUser])` `UserSettingsPage` already uses, so the "Remaining invites" count and the Generate-Invite gate reflect the live server value every time the page is opened, not just after the user's own actions.
- `InvitesPage.test.tsx`: `useAuth` mock now exposes `refreshUser`, plus a new test asserting it's called on mount.
- Numbered `FE-BUG-1` rather than the next free `FE-x.y` slot (deliberately, not an oversight): this story shipped from a worktree running in parallel with several sibling stories against the same `main`, all editing this same backlog file. `FE-BUG-1` matches the identifier already used informally in `tasks/todo.md` and avoids a numbering collision with siblings that independently pick "next free slot" from the same starting point.

**Deferred / explicitly out of scope:**
- `BonusStorePage` (`bonus_points`) and `UserProfilePage`/nav chrome (`can_upload`/`can_chat`/`donor`/`warned`/`enabled`) read the same cached `AuthContext.user` and would show the same staleness for admin-edited fields until their own next `refreshUser()` trigger. Not fixed here — out of this bug's scope (Invites page only) — but the fix here is the established, reusable pattern (`UserSettingsPage` already proved it out) for whichever of those is reported next, rather than a one-off endpoint or a broader refetch-on-focus mechanism for the whole app.

#### FE-7.5: Close Overlay-Click/"×" Opt-Out for Edit Modals + Unify ChatModMenu's Escape Handling [S] [BUG] [DONE]
**As a** staff user
**I want** an accidental backdrop click or "×" click on an edit/report/ban form to be as harmless as a stray Escape press, and the chat moderation menu's mute form to survive the same kind of accidental dismissal
**So that** I can't silently lose in-progress input through any of the modal's close affordances, on any modal built this way

**Context:** Follow-up to FE-7.4, which added `closeOnEscape` to `Modal` but explicitly deferred two things: (1) overlay-click and the "×" button still closed those same edit/form modals unconditionally, and (2) `ChatModMenu`'s own bespoke Escape handler was never unified onto `Modal`'s opt-out mechanism because it isn't built on `Modal` at all.

**Delivered:**
- Added a second, independent `closeOnDismissClick?: boolean` prop to `Modal` (default `true`), gating both the overlay-backdrop `onClick` and the `.modal-close` "×" button's `onClick`. Kept as a separate flag from `closeOnEscape` rather than folded into one combined "accidental close" flag: pointer-driven dismissal (backdrop/×) and the Escape key are different enough interaction classes that a future modal could plausibly want to allow one but not the other, and mature modal implementations elsewhere (Radix `onEscapeKeyDown`/`onPointerDownOutside`, MUI's `disableEscapeKeyDown` vs `backdropClick` reason) draw the same line. Overlay-click and "×" are bundled under the *same* new flag because they're the same category of affordance in this codebase already (`Modal.tsx` calls the same `onClose` from both, independent of any per-vector reasoning) and no call site needs them to diverge.
- Set `closeOnDismissClick={false}` on every one of the ~13 call sites FE-7.4 set `closeOnEscape={false}` on (found via `grep -rn "closeOnEscape={false}"`): `ReportModal`, `BanUserModal`, `AdminCategoriesPage`, `AdminForumsPage` (category + forum edit modals), `AdminGroupsPage`, `AdminUserDetailPage` (reset password), `TorrentDetailPage` (delete-with-reason), and `ForumTopicViewPage`'s five moderation modals (lock/unlock, pin/unpin, rename, move, delete-with-reason). Same reasoning as FE-7.4: these all host free-text/multi-field input the user may be actively composing, so a stray backdrop click or reflexive "×" click is now as harmless as a stray Escape press.
- Still explicitly **not** building dirty-tracking/discard-confirmation (option (b) from FE-7.4's brief) — same simpler flat-opt-out judgment call carries forward here. A modal opted out of both flags can still be closed via its own explicit Cancel button.
- `ChatModMenu.tsx` fix: kept it on its own bespoke dropdown/popover implementation rather than migrating it onto the shared `Modal` component. `Modal` always renders a full-screen dimmed overlay via a `document.body` portal; `ChatModMenu` is a small `position: fixed` dropdown anchored to a trigger button inline in the chat message list, with no backdrop and no portal. Forcing it onto `Modal` would change its visual presentation (adding a full-page dim behind a 180px-wide dropdown next to a username) — the wrong shape for what is a contextual menu, not a dialog. Instead, unified its *behavior* with the same "don't silently discard in-progress input" judgment: the existing `mousedown`/`keydown` document listeners that close the whole dropdown now check `showMuteForm` first and no-op while the mute duration/reason mini-form is visible, so neither a stray Escape nor an outside click collapses it out from under the user. Explicit dismissal (toggling "Mute user" again, submitting via "Confirm mute", or navigating away via "View profile") still works exactly as before.
- Tests: extended `Modal.test.tsx` with cases for `closeOnDismissClick={false}` disabling both the backdrop click and the "×" button (plus the pre-existing content-click-never-closes case, re-verified with the flag off too), `aria-disabled` reflecting the flag in both directions, and two cases proving the two flags are independent (Escape-only opt-out still allows overlay-click/× to close, and vice versa). Added `ChatModMenu.test.tsx` (new — none existed before), covering: trigger/open/close basics, Escape and outside-click still closing the menu when the mute form isn't open, Escape and outside-click *not* closing the menu (and not discarding the typed duration/reason) while the mute form is open, explicit re-toggle closing the form, the trigger button as the guaranteed escape valve even mid-mute-form, and the existing mute-submit/delete-all-confirm flows.
- Review (devil's-advocate + code-reviewer pass, per this doc's Post-Implementation Review process) surfaced two findings, both fixed here: the prop was originally named `closeOnOverlayClick`, which was misleading once it also started gating the "×" button (a reader could reasonably assume "×" still worked) — renamed to `closeOnDismissClick` to describe both pointer-dismiss affordances it actually controls. Separately, the "×" button rendered fully active-looking while silently no-op'ing when the flag was off, with no visual or a11y signal — it's now `aria-disabled` and dimmed via CSS (`.modal-close[aria-disabled="true"]`) when inert, so it reads as unavailable rather than broken.

**Deferred / explicitly out of scope:**
- Dirty-tracking/discard-confirmation for any of the three close vectors (Escape, overlay-click, "×") — unchanged from FE-7.4's scope boundary.
- Whether `closeOnEscape` and `closeOnDismissClick` should eventually collapse into one flag: today every call site sets both together, so the two-flag split is currently unexercised flexibility, justified only by precedent in other modal libraries (Radix, MUI) that draw the same line. Revisit if it stays unused.

> **Origin:** Follow-up to FE-7.4, which named both of these as explicit deferrals in its own "Deferred / explicitly out of scope" section.

---

## Migration Tool Epics (MT-)

### Epic MT-0: Foundation [M]

#### MT-0.1: CLI Scaffolding [S] [DONE]
**As a** site operator
**I want** a well-structured CLI tool
**So that** I can run migration commands easily

**Acceptance Criteria:**
- `migration-tool/cmd/migrate/main.go` entry point
- CLI framework (Cobra or similar) with subcommands: `discover`, `validate`, `run`, `verify`, `rollback`
- Config loading: source DB, target DB, file paths, options
- Structured logging with configurable verbosity
- Progress display (percentage, rows/sec, ETA)

#### MT-0.2: Source DB Connector (MySQL/TorrentTrader) [M]
**As a** site operator
**I want** the tool to connect to my TorrentTrader MySQL database
**So that** it can read the old data

**Acceptance Criteria:**
- MySQL connection with configurable DSN
- Schema discovery: `SHOW CREATE TABLE` for all tables
- Compare against known TorrentTrader 3.0 baseline schema
- Report differences: extra columns, missing tables, type mismatches
- Generate YAML mapping file with:
  - Every old column mapped to new column (or `SKIP` or `DERIVE`)
  - Type transformation notes (e.g. `ENUM('yes','no') -> boolean`)
  - Comments explaining each mapping decision
  - `CUSTOM` placeholders for mod-added columns
- Mapping file is human-editable and version-controllable

#### MT-0.3: Target DB Connector (PostgreSQL) [S]
**As a** site operator
**I want** the tool to write to the new PostgreSQL database
**So that** data lands in the correct schema

**Acceptance Criteria:**
- PostgreSQL connection with configurable DSN
- Batch insert support (configurable batch size)
- Transaction management (per-table or per-batch)
- Schema validation: verify target tables exist with expected columns
- Mapping validator: check all source->target mappings are valid before running

---

### Epic MT-1: Data Transformers [L]

#### MT-1.1: User Migration [M]
**As a** site operator
**I want** to migrate all user accounts preserving auth capability
**So that** users can log in to the new system without resetting passwords

**Acceptance Criteria:**
- Migrates `users` table to new split structure:
  - `users` (auth): id, username, email, password_hash, password_scheme, is_enabled, is_confirmed
  - `user_profiles` (display): avatar, bio, signature, title, country, gender, age
  - `user_settings` (prefs): theme, language, timezone, privacy, accept_pms, notifications
  - `user_stats` (tracker): uploaded, downloaded (preserved exactly)
- Password migration:
  - Copy old hash as-is
  - Set `password_scheme = 'legacy_sha1'` (or legacy_md5/legacy_hmac based on config)
  - Optionally wrap: `argon2(old_hash)` with scheme `'wrapped_sha1'`
  - User flag `--wrap-passwords` to choose strategy
- Passkey preservation: copy passkeys exactly (critical for active swarms)
- Role mapping: old class IDs -> new role IDs (configurable in mapping)
- Invited-by relationships: `users.invited_by` -> `user_invites` junction table
- Invitees parsing: split space-separated `users.invitees` into `user_invites` rows
- Mod notes: `users.modcomment` -> `user_mod_notes` table
- Warnings: migrate with status and expiry
- Stats: `users.uploaded` and `users.downloaded` preserved byte-exact

#### MT-1.2: Torrent & File Migration [M]
**As a** site operator
**I want** to migrate all torrents with their metadata and files
**So that** all content is preserved in the new system

**Acceptance Criteria:**
- Migrates `torrents` table with all metadata
- Info hash preserved exactly (byte-for-byte, non-negotiable)
- Migrates `files` table (file list per torrent)
- Migrates `announce` table (tracker URLs per torrent)
- Migrates `categories` and `torrentlang` (with ID remapping if needed)
- Copies physical files via FileStorage interface:
  - `.torrent` files: old `{torrent_dir}/{id}.torrent` -> new storage key
  - NFO files: old `{nfo_dir}/{id}.nfo` -> new storage key
  - Images: old `{torrent_dir}/images/*` -> new storage key
  - `--source-path` flag to specify old file directory
  - `--storage-target` flag: `local` or `s3`
- Boolean conversions: banned, visible, external, freeleech, anon, nfo
- Denormalized counters (seeders, leechers, etc.) recalculated from migrated peers
- Ratings and comments migrated with user FK mapping

#### MT-1.3: Forum Migration [M]
**As a** site operator
**I want** to migrate forum content
**So that** community history is preserved

**Schema Mapping:**
- TorrentTrader `forumcats` → `forum_categories` (id, name, sort_order)
- TorrentTrader `forums` → `forums` (with category_id mapping, min_group_level from group ID mapping)
- TorrentTrader `topics` → `forum_topics` (pinned, locked flags, denormalized counts)
- TorrentTrader `posts` → `forum_posts` (body converted from BBCode→Markdown via MT-1.5)

**Acceptance Criteria:**
- Forum categories: TorrentTrader forumcats → forum_categories
- Forums: TorrentTrader forums → forums, with category_id FK and min_group_level mapped from group IDs
- Topics: TorrentTrader topics → forum_topics, preserving locked/pinned/view_count, recomputing denormalized counts (post_count, last_post_id, last_post_at)
- Posts: TorrentTrader posts → forum_posts, body converted BBCode→Markdown (dependency: MT-1.5)
- Preserve: timestamps, edit history (edited_at, edited_by), topic metadata
- Read tracking: migrated or reset (user choice via flag)
- Shoutbox messages: migrate to new chat history table
- Recompute all denormalized counts (forum.topic_count, forum.post_count, forum.last_post_id) after migration

#### MT-1.4: Social Data Migration [M]
**As a** site operator
**I want** to migrate PMs, comments, ratings, and reports
**So that** all community interactions are preserved

**Acceptance Criteria:**
- Private messages: all messages with sender/receiver/content/dates
- Location mapping: old ENUM('in','out','both','draft','template') -> new model
- Comments: all torrent and news comments with user mapping
- Ratings: all torrent ratings
- Reports: preserve with status
- IP bans: `bans` table with range support
- Email bans: `email_bans` table
- News articles with comments
- Site log: last N entries (configurable, default last 1000)

#### MT-1.5: BBCode to Markdown Converter [M]
**As a** site operator
**I want** all BBCode content converted to Markdown
**So that** the new system uses a single content format

**Acceptance Criteria:**
- Converts: `[b]`, `[i]`, `[u]`, `[url]`, `[img]`, `[quote]`, `[code]`, `[*]` (lists)
- Handles nested tags correctly
- Preserves content that's already plain text
- `--convert-bbcode` flag (default true)
- `--preserve-bbcode` flag to keep raw BBCode (if building a BBCode renderer)
- Handles malformed BBCode gracefully (pass through rather than corrupt)

#### MT-1.6: Tracker Data Migration (Peers & Completed) [S]
**As a** site operator
**I want** to migrate active peers and completion history
**So that** swarms stay alive during the transition

**Acceptance Criteria:**
- **Peers migration** (critical for swarm continuity):
  - Migrate all current peer records from `peers` table
  - Map: torrent ID, user ID, peer_id, ip, port, uploaded, downloaded, to_go, seeder, last_action, connectable, client
  - Set `last_action` to migration time (so cleanup doesn't immediately purge them)
  - Recalculate seeder/leecher counts from migrated peer data
  - Passkey preserved per peer (must match user's passkey)
- **Completed table**: migrate all completion records (userid, torrentid, date)
- Post-migration: run peer cleanup job to remove any stale entries
- This is time-critical: should be one of the last steps before cutover

---

### Epic MT-2: Verification & Cutover [M]

#### MT-2.1: Verification Suite [M]
**As a** site operator
**I want** to verify the migration was successful
**So that** I'm confident no data was lost or corrupted

**Acceptance Criteria:**
- `tt-migrate verify` post-migration checks:
  - Row counts match (old vs new, accounting for expected skips)
  - User count: old confirmed+enabled vs new
  - Torrent count: exact match
  - Info hash spot-check: random sample of 100 torrents, verify hashes match
  - File existence check: verify .torrent and NFO files exist in new storage
  - Passkey spot-check: random sample of 50 users, verify passkeys match
  - Peer count: should roughly match (minus stale peers)
  - Forum post count: exact match
  - PM count: exact match
- Referential integrity check: all FKs valid
- Content spot-check: random sample of posts, verify BBCode->Markdown conversion

#### MT-2.2: Resumable Migration [M]
**As a** site operator
**I want** the migration to resume from where it left off on failure
**So that** I don't have to start over if something goes wrong

**Acceptance Criteria:**
- Checkpoint after each entity type (users, torrents, forums, etc.)
- On resume: skip completed entity types, continue from last checkpoint
- Idempotent operations: re-running on same data doesn't create duplicates
- `--force` flag to restart from scratch
- Progress saved to checkpoint file (JSON)

#### MT-2.3: Cutover Playbook & Dry-Run Mode [S]
**As a** site operator
**I want** a dry-run mode and documented cutover procedure
**So that** I can practice and execute the migration safely

**Acceptance Criteria:**
- `tt-migrate run --dry-run` mode:
  - Reads all old data, transforms, validates
  - Reports per-table: total rows, valid, skipped (with reasons), warnings
  - Does NOT write to new database
  - Shows sample transformations (first 5 rows per table)
- `tt-migrate rollback` - drop all data in new DB (for re-running)
- Cutover playbook documentation covering:
  1. Pre-migration: run `discover`, edit mapping, run `validate`
  2. Test migration: `run --dry-run` on production DB copy
  3. Full test: `run` against staging, verify, test manually
  4. Cutover window plan (announce, read-only, migrate, verify, DNS switch, monitor)
  5. Rollback plan: revert DNS, old site still has all data
- Passkey continuity: old announce URLs keep working
- Existing .torrent files in clients: new tracker must accept old announce path

---

## Dependency Graph

```
INFRA-1 ──┬── BE-0 ──┬── BE-1 ──┬── BE-4
           │          │          ├── BE-7
           │          │          └── FE-2 (needs auth API)
           │          ├── BE-2 (independent of BE-1)
           │          ├── BE-3 ──── FE-1.3, FE-1.4
           │          ├── BE-5 ──┬── BE-6
           │          │          └── FE-3 (needs forum API)
           │          ├── BE-8 ──── FE-5 (needs admin API)
           │          └── BE-10 (independent, needed by BE-2)
           │
           ├── FE-0 ──┬── FE-1 (needs FE-0 components)
           │          ├── FE-4 (needs FE-0 + WebSocket)
           │          ├── FE-6 (independent static pages)
           │          └── FE-7 (extends FE-0.2 theme system)
           │
           └── MT-0 ──── MT-1 ──── MT-2

BE-5.6 (notification infra) ── BE-5.7, BE-5.8, BE-5.9, BE-6.1, BE-7.3
BE-9 runs independently after BE-0
```

---

## Suggested Implementation Order

```
Phase 1 — Foundation
  INFRA-1, INFRA-2, INFRA-3       Monorepo, Docker Compose, Dockerfiles
  BE-0.1 through BE-0.7           Backend foundation
  BE-10.1                         BEncode library
  FE-0.1 through FE-0.4           Frontend foundation (setup, themes, routing, API client)
  MT-0.1                          Migration CLI scaffold

  Milestone: All three projects buildable and testable.
  Backend serves /healthz. Frontend renders hello world with theme toggle.

Phase 2 — Core Features
  INFRA-4, INFRA-5                CI + dev workflow
  BE-1.1, BE-1.2, BE-1.5         Register, login, roles
  BE-2.1, BE-2.4, BE-2.6         HTTP announce, scrape, peer cleanup
  BE-3.1, BE-3.2, BE-3.3         Upload, download, browse
  BE-9.1                          Cleanup job
  FE-0.5, FE-0.6                 Auth state, shared components
  FE-1.1, FE-1.2, FE-1.3        Home, login/signup, browse
  MT-0.2, MT-0.3                 Source + target DB connectors

  Milestone: Functional private tracker. Users register, upload,
  browse, and download. Clients announce/scrape. Frontend shows
  torrents and handles auth.

Phase 3 — Feature Parity + Community
  BE-1.3, BE-1.4                 Password recovery, profile
  BE-2.2, BE-2.3                 Connection limits, wait times
  BE-3.4 through BE-3.8          Detail, search, edit, comments, reports
  BE-3.12                        @mention search endpoint
  BE-4.1, BE-4.2                 Invitations
  BE-5.1 through BE-5.9          Forum + notification system
  BE-6.1, BE-6.2                 WebSocket chat
  BE-7.1, BE-7.3                 PMs + notifications
  FE-1.4, FE-1.5, FE-1.6        Torrent detail, filtered views, RSS builder
  FE-2.1 through FE-2.9          All user pages
  FE-3.1 through FE-3.5          Forum frontend
  FE-4.1, FE-4.2, FE-4.3        Real-time features
  MT-1.1 through MT-1.6          All data transformers

  Milestone: Full community platform. Forum, chat, PMs, invites,
  notifications. Frontend covers all user flows.

Phase 4 — Admin & Migration
  BE-1.6, BE-1.7, BE-1.8         IP bans, warnings, staff page
  BE-2.5, BE-2.7                 UDP tracker, cheating detection
  BE-6.3                         Chat history
  BE-7.2                         PM drafts/templates
  BE-8.1 through BE-8.7          Full admin panel
  BE-9.2                         Ratio warning automation
  FE-5.1 through FE-5.9          Admin frontend
  MT-2.1, MT-2.2, MT-2.3         Verification + cutover

  Milestone: Production-ready. Admin panel complete. Migration tool
  tested and verified. Ready for cutover from TorrentTrader.

Phase 5 — Polish
  BE-3.9, BE-3.10, BE-3.11       Reseed, RSS, categories management
  FE-6.1, FE-6.2, FE-6.3        Static pages (FAQ, rules, reference)
  FE-7.1, FE-7.2, FE-7.3        Theme management + additional theme

  Milestone: Fully polished. All features complete.
```
