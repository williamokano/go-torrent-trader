# Session Resume Document

The source of truth for task status is `docs/IMPLEMENTATION_TASKS.md`. This file is for session context only.

## Current State (2026-07-14)

Main is clean, no open PRs, no stray worktrees.

Recently merged: BE-9.11 (#69), BE-9.15 (#71), CI quality gates (#72, #73), repository testcontainers harness + migration 039 fix (#74), BE-9.16 (#75), postgres coverage to 80% (#76).

## Backlog audit (2026-07-14)

All 145 stories were checked against the code, not against their labels.

- **118 marked DONE — all verified genuinely implemented.** Every one has a real artifact behind it, and the multi-part stories (forum moderation lock/pin/move/rename, multi-device sessions, admin forum CRUD, admin password + passkey reset, wait-time system, quick ban) were checked at the behaviour level rather than by filename.
- **2 corrections still standing:** FE-0.7 is `[PARTIAL]`, not open — the renderer shipped, but the `!!spoiler!!` plugin and `MarkdownEditor` do not exist and it reaches only 4 of its target surfaces. BE-3.12 was rescoped — the endpoint it specs is already served by `GET /api/v1/users?search=`.
- **2 stories added** for work that was implemented but tracked nowhere: BE-9.17 (testcontainers harness + the migration 039 fix) and BE-9.18 (CI quality gates).
- **No open story turned out to be secretly finished.** BE-4.2, BE-7.2, BE-8.7, BE-9.13, BE-9.14 and BE-3.13 have zero implementation.

## Coverage

Overall backend **61.2%** (was 42.1%). `repository/postgres` **80.7%**. The floor in `.github/workflows/backend.yml` gates at 59 and ratchets upward.

The remaining gap is almost entirely `internal/handler` (~1,600 uncovered statements) — see BE-9.6.

## What's Next

**Biggest block of unbuilt work: the migration tool.** MT-0.2 through MT-2.3 (11 stories) are a 320-line Cobra skeleton — `run`, `discover`, `verify`, `rollback` and `validate` are all `// TODO` stubs. Nothing depends on it, so it is schedulable whenever.

**Other open work:**
- BE-9.6 — coverage to 80% (the `handler` package is the gap)
- BE-9.4 — real-time stats (the footer still polls `/api/v1/stats`; the Redis `StatsCache` cut DB load but there is no push)
- BE-9.13 / BE-9.14 — notification email digest and batching (an `email.go` service already exists to build on)
- FE-0.7 — spoiler plugin, `MarkdownEditor`, and the unreached surfaces
- BE-3.12 — frontend @mention typeahead for the forum/comment editors
- BE-8.7 — database backup (nothing exists at all)
- BE-4.2 — auto-invite distribution; BE-7.2 — PM drafts; BE-3.13 — rich metadata (research)
- FE-1.5 — the "Completed" view (deferred to the PM system)

## Known Bugs / Tech Debt

- FE-BUG-1: Invites page doesn't reflect updated count after admin edit (auth context caches)
- BE-STATS-1: Footer stats polling — half-addressed by the Redis `StatsCache`; still no WebSocket/SSE push (BE-9.4)
- BE-STATS-3: Removed — no legacy data to backfill (see BE-9.5)
