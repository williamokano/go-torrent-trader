# Session Resume

Context for picking work back up. **`docs/IMPLEMENTATION_TASKS.md` is the source of
truth for story status** — this file never contradicts it, and anything here that
ends up disagreeing with it is a bug in this file.

Keep it short. A session's detail belongs in its PR description and in the story's
"Delivered" bullets; what belongs here is only what a future session cannot
reconstruct from the repository.

Last verified against the code: **2026-07-25**, at `3412060`.

---

## Where things stand

- `main` is clean, no open PRs, no unmerged branches.
- Backend coverage **82.3%**, CI floor **80.0** (`COVERAGE_FLOOR` in
  `.github/workflows/backend.yml`). `cmd/server` and `internal/testutil` are excluded
  from the denominator.
- Frontend: 862 tests, lint clean (3 long-standing `exhaustive-deps` warnings that
  predate this work), Prettier clean.
- Latest release: **v0.24.0**. Note `release.yml` applies the `latest` Docker tag to
  whatever tag it builds, so re-running it against an older tag moves `latest`
  backwards.

## What is actually left

**The migration tool is the only real block of unbuilt work.** MT-0.2 through MT-2.3
(11 stories) are a ~320-line Cobra skeleton: `run`, `discover`, `verify`, `rollback`
and `validate` are all `// TODO`. Nothing depends on it, so it is schedulable
whenever.

Everything else in the backlog is DONE, DEFERRED or REMOVED. Deferred work lives in
`docs/FUTURE_WORK.md` — currently BE-2.5 (UDP tracker), BE-9.4 (real-time stats push)
and FE-7.1–7.3 (theme switching).

Open by decision rather than by neglect: **live feeds are not access-scoped per
feed.** `can_feed` is one privilege across every feed (BE-10.8). Per-feed gating was
considered and deliberately not built.

## Known bugs / tech debt

Nothing outstanding. Every bug this file used to track — FE-BUG-1, BE-8.19,
BE-STATS-1, BE-STATS-3 — is closed or superseded; see the backlog.

Two traps worth knowing before touching adjacent code, both recorded in
[`lessons.md`](lessons.md):

- `UserRepo.Create` writes every column from the struct, so a new
  `NOT NULL DEFAULT` column silently reads `false` for anything created through Go
  unless the constructor sets it explicitly.
- The `latest` Docker tag, above.

---

## Recent work

**BE-10 — External Notification Connectors** (2026-07-25). Announce a new torrent
anywhere: shoutbox, webhook, IRC, live feed, Discord, Telegram. Design in
`docs/NOTIFICATION_CONNECTORS.md`, plan in `docs/plans/BE-10.md`.

| Story | PR | What landed |
|---|---|---|
| BE-10.1 | #141 | The connector seam: event → dispatcher → `connector_deliveries` → asynq drain. Registry, admin CRUD, Chat + generic Webhook, migrations 071–073 |
| BE-10.2 | #142 | IRC, persistent-connector lifecycle, Postgres advisory-lock leader election |
| BE-10.3 | #143 | SSE live feed, hub fan-out, the "live releases" page |
| BE-10.4 | #144 | Discord embeds + Telegram Bot API, `connector.ErrPermanent` |
| BE-10.6 | #145 | Nested category pickers site-wide, include/exclude filters, several shoutbox instances, template help |
| BE-10.7 | #146 | Several live feeds, each with its own slug and URL |
| BE-10.8 | #147 | `can_feed` — live feed access as a class privilege, revocable per member |

Released as **v0.24.0**.

**Dropped: BE-10.5, the per-user relay.** The multi-feed live stream covers it, and
it would have meant per-user filters, rate limits and delivery log — a second copy of
the pipeline for a want nobody had expressed. Nothing was built for it; the one
concession the design had made, `Announcement.Event` staying a plain string, is worth
keeping anyway so later event kinds can widen it.

**Earlier arcs**, all shipped and documented in the backlog, so no session notes are
kept here: BE-8.22 torrent submission moderation (#136–#139), BE-9.24 staff-only
activity log entries, BE-8.20/FE-5.17 username profile routes and @mention linking,
the admin user detail revamp, and the invite follow-ups.

---

## Working notes

- **Worktrees:** `git worktree list` should show only `main`. Agent worktrees under
  `.claude/worktrees/` are session leftovers — five survived from 2026-07-19 until
  they were cleaned up on 2026-07-25, long after their branches had merged, which
  made `git worktree list` read as though work were in flight when none was.
- **Edit this file in the main working directory.** It has been committed from
  feature worktrees, which is how it came to disagree with itself: a section written
  in one working copy is invisible to another, so an anchored insert silently does
  nothing. Check the anchor exists first — see `lessons.md`.
