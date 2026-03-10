# Session Resume Document

The source of truth for task status is `docs/IMPLEMENTATION_TASKS.md`. This file is for session context only.

## Current State (2026-03-10)

### No Pending PRs

All feature branches merged to main. Clean working tree.

### Recently Completed
- BE-5.1 + FE-3.1/3.2/3.3 — Forum structure, browsing, topic list, topic view
- BE-5.2 — Create topics & post replies
- BE-2.2 — Tracker connection limits
- BE-8.10 — Tiered warning escalation
- BE-8.9 — Privilege restrictions
- BE-8.11 — Quick ban

### What's Next (Forum Track)
- BE-5.3: Edit & Delete Posts
- BE-5.4: Moderation Tools (lock, pin, move, rename)
- BE-5.5: Forum Search
- BE-8.5: Admin Forum CRUD

### What's Next (Other)
- BE-5.6–5.9: Notification infrastructure + delivery
- BE-2.3/2.5/2.7: Tracker hardening
- FE-7: Theme management
- MT-1: Migration tool data transformers

### Known Bugs / Tech Debt
- FE-BUG-1: Invites page doesn't reflect updated count after admin edit (auth context caches)
- BE-STATS-1: Footer stats polling from every client — needs Redis cache or WebSocket push
- BE-STATS-3: Backfill torrent file lists for pre-migration-023 torrents
