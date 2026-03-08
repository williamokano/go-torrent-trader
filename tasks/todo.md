# Session Resume Document

## Current State (2026-03-07)

### Pending PR
- `feat/wire-worker` — worker wiring + multi-instance fixes (password resets in DB, scheduler toggle, unique tasks, security fixes)

### What's Done (merged to main)
**Infrastructure:** INFRA-1 through INFRA-5
**Backend Foundation:** BE-0.1 through BE-0.7, BE-10.1
**Frontend Foundation:** FE-0.1 through FE-0.6 (scoped)
**Auth:** BE-1.1, BE-1.2, BE-1.2.2 (Redis sessions), BE-1.3, BE-1.4, BE-1.5
**Tracker:** BE-2.1, BE-2.4, BE-2.6, BE-9.1
**Torrents:** BE-3.1-3.3, BE-3.5-3.8 + FE-1.1, FE-1.3, FE-1.4, FE-2.4
**User:** FE-2.1
**Migration Tool:** MT-0.1

### Architecture Notes
- SessionStore: INTERFACE (memory for tests, Redis for production)
- PasswordResetStore: INTERFACE (memory for tests, Postgres for production)
- EmailSender: INTERFACE (NoopSender for tests, SMTP for production)
- Password reset uses atomic ClaimByTokenHash (UPDATE...RETURNING) — no TOCTOU race
- Worker: asynq server + scheduler, ENABLE_SCHEDULER env var, Unique task dedup
- SITE_BASE_URL = frontend URL, API_URL = backend API URL
- Search: PostgreSQL tsvector with prefix matching, 250ms debounce on FE
- All SQL audited — parameterized queries, no injection

### What's Next
- Merge feat/admin-panel PR
- Build first Docker image for POC
- Phase 3 remaining: forums, chat, PMs, invites, notifications
- BE-1.2.3: Move test doubles out of domain code (testutil package)
- BE-3.13: Rich torrent metadata (research task)

### Future: Reseed Notification via PM
- [ ] BE-3.9.1: Reseed PM notification listener — when `ReseedRequested` event fires, send a PM to the torrent uploader notifying them of the reseed request
  - **Depends on:** BE-7.1 (Private messaging: send & receive)
  - Listener in `listener/` package, wires into event bus, calls PM service to create a system message

### Future: UX Bugs
- [ ] FE-BUG-1: Invites page doesn't reflect updated invite count after admin edit — the auth context caches user data (including `invites` count) and only refreshes on login or manual page reload. When admin grants invites via the admin panel, the inviter's session still has the old count. Need to either poll `/auth/me` periodically, invalidate on navigation, or use a WebSocket push to refresh user data when it changes server-side.

### Future: Admin Panel Enhancements (BE-8.x)
- [ ] BE-8.1: Invalidate sessions on user disable — when admin toggles `enabled=false`, also call `sessions.DeleteByUserID()` so the ban is immediate
- [ ] BE-8.2: Admin torrent moderation — add torrent delete action from admin panel, ideally linked from reports page ("resolve & delete torrent" flow)
- [ ] BE-8.3: Report action flow — resolve with action (warn uploader, delete torrent, ban user) instead of just toggling resolved status
- [ ] BE-8.4: Admin user detail view — click username to see their torrents, reports against them, session history
- [ ] BE-8.5: Admin audit log — record who changed what and when (user group changes, bans, torrent deletions, report resolutions)
- [ ] BE-8.6: Admin dashboard — site stats landing page (registrations, uploads, active peers, pending reports count)
