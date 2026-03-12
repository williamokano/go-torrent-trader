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

#### BE-3.12: @Mention Search Endpoint [S]
**As a** frontend developer
**I want** a user search endpoint for @mention autocomplete
**So that** users can be mentioned in comments and forum posts

**Acceptance Criteria:**
- `GET /api/v1/users/search?q=prefix`
- Returns matching users (id, username, avatar) limited to 10 results
- Prefix match on username
- Requires authentication
- Used by frontend for typeahead in comment/post editors

#### BE-3.13: Rich Torrent Metadata [M] [RESEARCH]
**As an** uploader
**I want** to add detailed metadata to my uploads
**So that** torrents are well-categorized and searchable by specific attributes

**Research + Implementation:**
- Study the original TorrentTrader 3.x upload form for reference fields
- Common metadata fields to support:
  - Year of release
  - Video codec (H.264, H.265/HEVC, AV1, etc.)
  - Audio codec (AAC, FLAC, DTS, AC3, etc.)
  - Resolution/quality (4K/2160p, 1080p, 720p, SD)
  - Source (Blu-ray, WEB-DL, HDTV, DVD, etc.)
  - Language (with multi-language support)
  - Subtitles (embedded, SRT, none)
  - Runtime (for movies/TV)
  - Genre tags
- Decide on storage approach: dedicated columns vs JSONB metadata field vs separate metadata table
- Consider auto-detection: parse torrent name for quality/codec patterns (e.g., "Movie.2024.1080p.BluRay.x265.DTS" → year=2024, resolution=1080p, source=BluRay, codec=x265, audio=DTS)
- Category-specific fields: Movies need different metadata than Music or Software
- Update upload form (FE) with dynamic fields based on selected category
- Update browse/search to filter by metadata fields
- Migration: ensure metadata schema is compatible with MT-1.2 torrent migration

> **Note:** This is a research task. Start by documenting what fields the original TorrentTrader supports, then design the schema and UI. Low priority — current basic upload (name, category, description) works for MVP.

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

#### BE-4.2: Auto-Invite Distribution [S] [TODO — manual admin grant only for now]
**As a** tracker operator
**I want** invites distributed automatically based on user activity
**So that** active users can grow the community

**Acceptance Criteria:**
- Background job: runs on configurable interval
- Criteria: downloaded GB range, ratio threshold
- Max invites per role (configurable)
- Don't exceed per-role cap (check current count before adding)
- Log distributions

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

#### BE-7.2: Drafts & Templates [S]
**As a** user
**I want** to save message drafts and templates
**So that** I can compose messages later or reuse common messages

**Acceptance Criteria:**
- Save draft: stores incomplete message for later editing
- Save template: stores reusable message pattern
- Load template into compose form
- List/delete drafts and templates

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

#### BE-8.7: Database Backup [S]
**As an** admin
**I want** to create and download database backups
**So that** I have disaster recovery capability

**Acceptance Criteria:**
- Trigger backup via admin panel
- Creates pg_dump compressed file
- Download backup file
- List/delete old backups
- Optional: scheduled backups via cron job

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
- New restriction flags on users table: `can_download`, `can_upload`, `can_chat` (all default `true`)
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

#### BE-9.4: Real-Time Stats via SSE or WebSocket [M]
**As a** user
**I want** the site stats in the footer to update in real time
**So that** I see live tracker activity without page refreshes

**Acceptance Criteria:**
- Stats pushed to clients via SSE or WebSocket (piggyback on chat connection when BE-6.1 lands)
- Broadcast triggered on peer announce, torrent upload, user registration
- Eliminates client-side polling entirely
- Graceful fallback: if WebSocket disconnects, resume polling

#### BE-9.5: Backfill Torrent File Lists [S] [REMOVED — no legacy data to backfill; if needed, handle during MT-1.2 torrent migration]

#### BE-9.6: Increase Test Coverage to 80% [M]
**As a** developer
**I want** comprehensive test coverage across all packages
**So that** regressions are caught early and code quality is maintained

**Acceptance Criteria:**
- Minimum 80% test coverage per package (handler, service, repository, worker, middleware)
- CI gates on coverage threshold — build fails if coverage drops below 80%
- Current low-coverage packages to prioritize:
  - `handler` — add tests for dashboard, admin, chat, news, warning, user activity handlers
  - `worker` — add tests for maintenance, ratio warning, cleanup jobs
  - `repository/postgres` — add integration tests or improve mock coverage
  - `config` — test validation and edge cases
  - `database` — test connection and migration error handling
- All new code must ship with tests above the threshold
- Add `go test -coverprofile` to CI with coverage check step

> **Note:** This is a tech-debt task. Should be addressed incrementally — each new PR must not decrease coverage, and dedicated test sprints can bring existing packages up to the threshold.

#### BE-9.7: Forum Post Soft-Delete & Edit History [M]
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

#### BE-9.11: Hide Delete Button on First Post & Deep-Link Search Results [S]
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

#### BE-9.13: Notification Email Digest [S]
**As a** user
**I want** email digests of my unread notifications
**So that** I don't miss important activity when I'm not on the site

**Acceptance Criteria:**
- Asynq periodic task sends daily/weekly digest emails
- User preference for digest frequency (off, daily, weekly)
- Email summarizes unread notifications since last digest

> **Origin:** Deferred from BE-5.9 — in-app + WS push covers the immediate need.

#### BE-9.14: Notification Batching [S]
**As a** user
**I want** grouped notifications like "5 new replies in topic X"
**So that** my notification list isn't flooded by active threads

**Acceptance Criteria:**
- Collapse multiple `topic_reply` notifications from the same topic into a single entry
- Show count and last few actors
- Expand on click to see individual notifications

> **Origin:** Deferred from BE-5.9.

#### BE-9.15: Notification Cleanup in Maintenance Worker [XS]
**As a** developer
**I want** old read notifications to be automatically purged
**So that** the notifications table doesn't grow unboundedly

**Acceptance Criteria:**
- Wire `NotificationRepository.DeleteOld()` into the periodic maintenance worker
- Configurable retention period (default 90 days for read notifications)

> **Origin:** Review finding from BE-5.6 implementation — DeleteOld exists but is not called.

#### BE-9.16: Notification Listener & Handler Test Coverage [S]
**As a** developer
**I want** tests for the notification listener and HTTP handlers
**So that** event-to-notification mapping and API responses are verified

**Acceptance Criteria:**
- Listener tests: event-to-notification mapping, dedup, self-notify skip, @mention parsing
- Handler tests: HTTP status codes, auth checks, pagination, error mapping
- Meet 80% coverage gate

> **Origin:** Review finding from BE-5.6 implementation.

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
- `MarkdownEditor`: toolbar with common formatting, preview toggle — **use a library** (e.g., `@uiw/react-markdown-editor`), do NOT build from scratch
- `Avatar`: user avatar with fallback to initials
- `Badge`: role badges, status indicators
- All components theme-aware (use CSS custom properties)

#### FE-0.7: Markdown Rendering System [M]
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

#### FE-1.5: Today's Torrents, Need Seed, Completed Views [S] [PARTIAL — Today's + Need Seed done; Completed (user download history) deferred to PM system]
**As a** user
**I want** quick-access filtered views
**So that** I can find torrents matching specific criteria

**Acceptance Criteria:**
- Today's torrents: uploaded in last 24h
- Need seed: torrents with 0 seeders
- Completed: user's download history
- Reuses torrent list component with pre-set filters

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

---

### Epic FE-5: Admin Panel [L] [PARTIAL — foundation, layout, routing, users/reports/groups pages done]

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
- [x] Groups list API (`GET /admin/groups`) and read-only groups page with permission matrix

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
