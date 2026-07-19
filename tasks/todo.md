# Session Resume Document

The source of truth for task status is `docs/IMPLEMENTATION_TASKS.md`. This file is for session context only.

## Session 2026-07-17 — BE-8.20 / FE-5.17: username profile route + resolved @mentions

Branch `feat/username-profile-and-mention-links` (worktree `username-route-and-mentions`).
Follows two smaller same-session fixes: PR #108 (release workflow Go version) and
PR #109 (mention-dropdown caret positioning, `MarkdownEditor`'s typeahead — a
different feature from the @mention *linking* built here).

**Part A** — `/user/{id}` → `/user/{username}`: backend `GetProfile` resolves via
`GetByUsername`; frontend route/fetch/`UsernameDisplay`/5 direct link sites updated;
6 synthesized-fallback call sites (`comment.username ?? "User #123"`) gained `noLink`
guards so a missing username can't build a garbage link. Clean break, no numeric-ID
fallback (usernames can legally be all-numeric — dual resolution would risk collision).

**Part B** — resolved @mention linking: migration 058 (`mentioned_usernames` JSONB on
`torrent_comments`/`forum_posts`), `UserRepository.GetByUsernames`, new
`ResolveMentionedUsernames` service helper (parallel to, not replacing, the existing
`publishMention` notification pipeline), wired into Create/Update/Edit for comments
and forum posts. Frontend `remarkMention` plugin (mirrors `remarkSpoiler`'s pattern)
linkifies only resolved usernames in `MarkdownRenderer`. v1 scope: comments + forum
posts only — PMs and news deferred (PMs argued to be a different problem, not just
unwired, since they're strictly 1:1 and a mention notification would just duplicate
the existing "new message" one).

- [x] Both parts implemented, backend + frontend, with tests at each layer
- [x] Devil's-advocate + code-reviewer agents run in parallel per CLAUDE.md §5.
      Code reviewer: clean, 0 blocking findings. Devil's advocate: 4 confirmed,
      empirically-verified findings, all fixed:
      1. `EditPost` reordered so `ResolveMentionedUsernames` (a new fallible call)
         runs *before* `CreateEdit`, not between it and `Update` — closes a window
         where edit history could record an edit that never landed.
      2. `remarkMention` only honored the backend's `^`-boundary rule for a
         paragraph's *first* text node; later sibling nodes (after `*emphasis*`,
         links, code spans) treated their own start as a fresh `^`, so
         `*cool*@alice` could wrongly linkify. Fixed with a second boundary-less
         regex for non-first children — verified via the agent's own live
         reproduction before and after.
      3. `scanMentionedUsernames` errors used to abort the whole `ListByTorrent`/
         `ListByTopic` page on one malformed row. Now logs and degrades to `[]`
         for that row only — `mentioned_usernames` is a pure rendering aid, must
         never take the page down.
      4. `AdminService.UpdateUser` set `user.Username` with zero format
         validation, unlike registration — harmless before this PR, now
         consequential since the username *is* the routing key. Added the same
         `usernameRe` check registration already uses.
- [x] Final backend/frontend verification re-run after the 4 fixes: go build/vet/test
      (incl. Docker repo tests — migration 058 applies cleanly), golangci-lint 0
      issues, coverage 80.2% ≥ 80.0% floor, vitest 615 passed, frontend build/lint/
      format green
- [x] `docs/IMPLEMENTATION_TASKS.md` updated (BE-8.20 / FE-5.17 added, DONE)
- [ ] Commit, push, open PR

## Session 2026-07-16 (cont'd 2) — admin user detail revamp + edit history

Branch `feat/admin-user-detail-revamp` (worktree). Brief: revamp `/admin/users/{id}`.
Production screenshot is behind local main (Suspend invite, Privileges panel, and
Bonus points already exist locally), so the remaining scope is:

1. Form spacing/layout — sibling inputs touch each other; fix vertical rhythm in the
   shared form primitives + sectioned page layout, same design system site-wide.
2. Unit-aware editing of Uploaded/Downloaded (B/KB/MB/GB/TB, 1024-based to match
   `formatBytes`) instead of raw bytes.
3. Audit trail for admin edits of user fields (old, new, who, when) — especially
   uploaded/downloaded/invites, recorded for all profile fields; "Edit history"
   panel on the page.

### Backend
- [x] Migration `057_create_user_edit_history.sql` (table + `(user_id, created_at DESC)` index)
- [x] `model.UserEditHistory`
- [x] `repository.UserEditHistoryRepository` (batch Record; ListByUser w/ limit+offset+total)
- [x] `postgres.UserEditHistoryRepo` + testcontainers repo test
- [x] `AdminService`: `SetEditHistoryRepo`, diff-and-record in `UpdateUser` (non-fatal,
      logged on failure), `ListUserEditHistory`; unit tests
- [x] `HandleListUserEditHistory` GET `/api/v1/admin/users/{id}/edit-history` + router + tests
- [x] Wire in `cmd/server/main.go`

### Frontend
- [x] `ByteSizeInput` form component (canonical-bytes state, amount+unit select,
      exact-bytes hint) + tests
- [x] Shared form polish (`.form-stack`, focus ring, textarea min-height) — token-based, site-wide
- [x] `AdminUserDetailPage`: form-stack layout, ByteSizeInput for uploaded/downloaded,
      Edit history panel (bytes fields humanized, load-more)
- [x] Page test updates (30 pass)

### Verification
- [x] backend: build, vet, full tests (Docker repo tests → migration 057 applies), golangci-lint 0 issues, coverage 80.1% ≥ 80.0% floor
- [x] frontend: build, vitest 596 passed, lint 0 errors, format:check clean
- [x] `docs/IMPLEMENTATION_TASKS.md` story BE-8.17 added, marked DONE
- [x] Devil's advocate + code reviewer findings triaged: fixed the critical
      (dirty-fields-only saves — untouched uploaded/downloaded no longer revert
      announce accrual or pollute the audit trail), audit retention (no user_id
      FK; changed_by username snapshot; changed_by index), ByteSizeInput
      empty/clamp/a11y, history dedupe + error state + no-op-looking byte
      diffs, UTC timestamps, limit clamp, derefString reuse. Deferred to
      BE-8.18: SetStats path + tx-integrated audit + keyset pagination.
      Filed BE-8.19 [BUG]: cleanup worker hard-deletes banned users.

## Session 2026-07-16 (cont'd) — BE-8.15 + BE-8.16: invite follow-ups

PR #102 (FE-5.15) merged to main. Follow-up branch `feat/invite-outstanding-management`:

- [x] BE-8.15 — admin view/revoke of a user's outstanding invites: `GET /api/v1/admin/users/{id}/invites`, `DELETE /api/v1/admin/invites/{id}` (`InviteService.RevokeInvite`, hard-deletes unredeemed invites, 409 on redeemed), outstanding-invites panel on `AdminUserDetailPage` with status badges + revoke, `InviteRevokedEvent` → activity log
- [x] BE-8.16 — closed the privilege-flag drift race: `UserRepo.Update` no longer writes can_download/upload/chat/invite at all (mirrors `bonus_points`); new targeted `UserRepo.SetPrivilegeFlag` is the only writer, used by `RestrictionService`; regression test `TestUserRepoUpdate_DoesNotClobberPrivilegeFlags` proves a stale full-row `Update` can't clobber a concurrent restriction
- [x] Verified: go build/vet/test (incl. Docker repo tests), golangci-lint 0 issues, coverage 80.1% ≥ 80.0% floor, vitest 569 passed, frontend build/lint/format green
- [x] `docs/IMPLEMENTATION_TASKS.md` updated (BE-8.15, BE-8.16 marked DONE)

## Session 2026-07-16 — FE-5.15: admin surface rollout + invite restriction

Branch `feat/admin-ui-consistency-invite-restriction` (worktree). All done, verified:

- [x] Backend `invite` restriction type: migration 055 (`users.can_invite`), model/repo/service/handler, `CreateInvite` returns 403 `invite_restricted`, admin + profile views expose `can_invite`
- [x] `AdminUserDetailPage` rebuilt on admin-ui primitives; Privileges panel shows download/upload/chat/invite with suspend + restore
- [x] All remaining admin pages (dashboard, torrents, bans, warnings, reports, chat-mutes, cheat-flags, news, forums, categories, backups, settings) adopted `admin-ui.css` conventions; per-page CSS trimmed
- [x] Verified: go build/vet/test (incl. Docker repo tests → migration 055 applies), golangci-lint 0 issues, coverage 80.2% ≥ 80.0% floor, vitest 561 passed, frontend build/lint/format green
- [x] `docs/IMPLEMENTATION_TASKS.md` updated (FE-5.15 added, BE-8.9 note)

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

- FE-BUG-1 [DONE]: Invites page doesn't reflect updated count after admin edit (auth context caches) — see `docs/IMPLEMENTATION_TASKS.md`
- BE-8.19 [BUG]: cleanup worker step 4 hard-deletes *banned* users (bare `enabled = false AND created_at < 7d` filter matches them), cascading their data away — found 2026-07-17 during BE-8.17 audit-retention review
- BE-STATS-1: Footer stats polling — half-addressed by the Redis `StatsCache`; still no WebSocket/SSE push (BE-9.4)
- BE-STATS-3: Removed — no legacy data to backfill (see BE-9.5)
