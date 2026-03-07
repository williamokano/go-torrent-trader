# Session Resume Document

## Current State (2026-03-07)

### Completed: Multi-instance/K8s readiness fixes (feat/wire-worker)
- [x] Issue 1: PasswordResetStore extracted to INTERFACE, in-memory renamed to MemoryPasswordResetStore
- [x] Issue 1: Postgres implementation created at repository/postgres/password_reset.go
- [x] Issue 1: AuthService updated to use interface; all methods return errors
- [x] Issue 1: main.go wired to postgres.NewPasswordResetRepo(db)
- [x] Issue 1: All tests updated to use MemoryPasswordResetStore
- [x] Issue 2: ENABLE_SCHEDULER config added (default true), scheduler conditionally started
- [x] Issue 3: asynq.Unique added to cleanup peers (14min) and recalc stats (59min) tasks
- [x] All tests pass, go vet clean, golangci-lint 0 issues

### Pending PRs (merge in this order)
1. `feat/redis-sessions` — BE-1.2.2: SessionStore interface + Redis impl + configurable TTLs
2. `feat/search` — BE-3.5: Full-text search with tsvector/GIN index
3. `feat/comments-ratings` — BE-3.7: Comments CRUD + ratings + FE widgets
4. `feat/reports` — BE-3.8: Report system + FE modal

**Merge conflicts expected** on router.go, main.go, repository.go — resolve after each merge.

### What's Done (merged to main)
**Infrastructure:** INFRA-1 through INFRA-5
**Backend Foundation:** BE-0.1 through BE-0.7, BE-10.1
**Frontend Foundation:** FE-0.1 through FE-0.6 (scoped)
**Auth:** BE-1.1, BE-1.2, BE-1.3, BE-1.5, FE-1.2
**Tracker:** BE-2.1, BE-2.4, BE-2.6, BE-9.1
**Torrents:** BE-3.1, BE-3.2, BE-3.3, BE-3.6 + FE-1.1, FE-1.3, FE-1.4, FE-2.4
**User:** BE-1.4 + FE-2.1
**Migration Tool:** MT-0.1

### What's Next (after merging pending PRs)
Phase 3 remaining:
- BE-1.6: IP & email bans
- BE-1.7 + BE-9.2: User warnings & ratio automation
- BE-1.8 + FE-2.7: Staff page & member list
- BE-2.2, BE-2.3: Connection limits, wait times
- BE-2.5: UDP tracker
- BE-2.7: Cheating detection
- BE-3.9-3.12: Reseed, RSS, categories admin, @mention search
- BE-4.1-4.2 + FE-2.6: Invite system
- BE-5.1-5.9 + FE-3.x: Forum system + notifications
- BE-6.1-6.3 + FE-4.1: WebSocket chat
- BE-7.1-7.3 + FE-2.5: Private messages
- BE-8.x + FE-5.x: Admin panel
- FE misc: FE-1.5, FE-1.6, FE-2.2, FE-2.9, FE-6.x, FE-7.x

### Key Architecture Notes
- SessionStore is an INTERFACE (memory + Redis implementations)
- PasswordResetStore is an INTERFACE (memory + Postgres implementations)
- EmailSender is an INTERFACE (SMTP implementation)
- SITE_BASE_URL = frontend URL, API_URL = backend URL
- Sessions persist in Redis (survives restarts)
- Password resets persist in PostgreSQL (survives restarts, multi-instance safe)
- Full-text search uses PostgreSQL tsvector with GIN index
- All SQL queries audited — no injection vulnerabilities
- ENABLE_SCHEDULER=false disables periodic task scheduling (for worker-only pods)
