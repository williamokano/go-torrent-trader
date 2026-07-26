# Session Resume

Context for picking work back up. **`docs/IMPLEMENTATION_TASKS.md` is the source of
truth for story status** — this file never contradicts it, and anything here that
ends up disagreeing with it is a bug in this file.

Keep it short. A session's detail belongs in its PR description and in the story's
"Delivered" bullets; what belongs here is only what a future session cannot
reconstruct from the repository.

Last verified against the code: **2026-07-26**, at `1f4ef57` plus uncommitted work
(see "In flight" below).

---

## Where things stand

- **`main` has substantial uncommitted work in the working directory** — the
  documentation alignment pass and BE-11.1. Nothing is committed or pushed. No
  branches, no worktrees, no open PRs.
- Backend coverage **82.9%**, CI floor **80.0** (`COVERAGE_FLOOR` in
  `.github/workflows/backend.yml`). `cmd/server`, `internal/testutil`,
  `cmd/backfill-mentions/main.go` and `cmd/openapi-public/` are excluded from the
  denominator.
- Frontend: 868 tests, lint clean (the same 3 long-standing `exhaustive-deps`
  warnings), Prettier clean, `tsc --noEmit` clean.
- Latest release: **v0.24.0**, which is `e966c13` (BE-10.4 / PR #144) — PRs #145,
  #146 and #147 are **not** in it. Note `release.yml` applies the `latest` Docker tag
  to whatever tag it builds, so re-running it against an older tag moves `latest`
  backwards.

## In flight (uncommitted, 2026-07-26)

**A documentation alignment pass plus BE-11.1.** Every doc was checked against the
code; the docs had been describing a system that partly did not exist. Highlights:

- Decision 4 claimed **JWT** access tokens. There is no JWT anywhere — tokens are
  opaque hex resolved against Redis. The `JWT_SECRET` this spawned was documented as a
  **required** deployment variable and read by no code; removed from the README, both
  env examples and the Portainer stack.
- Decision 10 claimed a `golang.org/x/time/rate` middleware. It does not exist;
  limiting is per-surface only. Anything treating "requests are rate limited" as an
  existing guarantee (notably PF-20) must build it.
- `ARCHITECTURE.md` and decision 9 flatly contradicted each other on OpenAPI. Resolved
  by BE-11.1 — two specs now, full and public.
- `BE-3.9` was marked DONE with three unshipped criteria, and **its endpoint is
  unguarded** (see below).
- `sqlc`, Mailhog, and a feature-based frontend structure were all documented and none
  exist.

## What is actually left

**Highest priority: `POST /api/v1/torrents/{id}/reseed` has no server-side guard.**
No seeder check, no banned check, no visibility check — the "0 seeders only" rule
lives only in `TorrentDetailPage.tsx`. Any authenticated user can POST it against any
torrent, and each one emails that torrent's uploader. This is a code fix, tracked as a
Known-gap note on BE-3.9 and as finding 1 in `docs/PROPOSED_FEATURES.md`.

**BE-11.2 — OpenAPI coverage.** 152 of 189 routes are in the debt ledger at
`backend/internal/handler/openapi_undocumented.go`. The ledger may only shrink, and a
guard test enforces that. Prerequisite for PF-27 (site CLI).

**The migration tool.** MT-0.2 through MT-2.3 (11 stories) are a ~320-line Cobra
skeleton: `run`, `discover`, `verify`, `rollback` and `validate` are all `// TODO`.
Nothing depends on it.

**Two open `[BUG]` stories:** `BE-10.8` (audit-log chat message deletions — note this
ID is used twice; this is the audit-log one, not `can_feed`) and `FE-4.6` (a spoiler
inside a link splits the row).

Deferred work lives in `docs/FUTURE_WORK.md` — BE-2.5 (UDP tracker), BE-9.4
(real-time stats push), FE-7.1–7.3 (theme switching), announce-log retention, and the
remaining bonus-economy pieces.

Open by decision rather than by neglect: **live feeds are not access-scoped per
feed.** `can_feed` is one privilege across every feed. Per-feed gating was considered
and deliberately not built.

## Scope decision, 2026-07-26

**Teams and reputation are back in scope.** Both were cut from `NOT_PORTING.md` for
the initial release; with the port feature-complete and stable, the operator
reinstated them. §4 and §14 moved to that file's "Reinstated" section, keeping their
numbers so `§N` citations elsewhere still resolve.

This unblocks roughly a third of `PROPOSED_FEATURES.md`: PF-3 (reputation ledger) is
the most depended-upon proposal in the document — PF-4, PF-5, PF-10, PF-12, PF-18 and
PF-1 all get cheap once it exists — and PF-2 (teams) is what PF-6 team rooms and PF-14
team colours attach to.

Three decisions still gate their dependents: **D3** multiplier stacking, **D4** the
privacy opt-out model, **D5** the membership-based access model shared by team forums
and chat rooms. None of the three is large, but each wants deciding once rather than
per-feature. PF-25's *public* health page is deliberately still open and is a
different kind of question — that gating was a standing privacy posture, not an MVP
scope cut.

## Known bugs / tech debt

- **The unguarded reseed endpoint**, above. The one genuine correctness issue known.
- **`task generate` was broken in two ways and is now fixed** — worth knowing because
  nothing in CI runs it, so it can rot again. `frontend/src/api/schema.d.ts` had been
  hand-edited despite its DO-NOT-EDIT header (two fields existed in the backend and in
  the types but never in the spec), and the generator's output failed
  `npm run format:check`. Both fixed; if you add a response field, put it in
  `backend/api/openapi.yaml`, never in the generated file.
- Three traps worth knowing before touching adjacent code, all in
  [`lessons.md`](lessons.md):
  - `UserRepo.Create` writes every column from the struct, so a new
    `NOT NULL DEFAULT` column silently reads `false` for anything created through Go
    unless the constructor sets it explicitly.
  - A generated artifact that is not verified in CI will be hand-edited eventually.
  - The `latest` Docker tag, above.

---

## Recent work

**Docs alignment + BE-11.1 — published API contract** (2026-07-26, uncommitted). Two
OpenAPI documents: `backend/api/openapi.yaml` (full, hand-maintained, every operation
marked `x-audience`) and the generated `backend/api/openapi.public.yaml`, which is the
one meant for publication. Guard tests walk the real router and fail the build on an
unclassified route. See BE-11.1's Delivered bullets — and note the split had to be
per-operation, not per-path-prefix, because six forum moderation endpoints are
staff-only while sitting on member paths.

**BE-10.7 — shoutbox announcements** (`9de90a6`, PR after v0.24.0). Deletable,
readable, named and linked system messages in the shoutbox. Filed the two open `[BUG]`
stories listed above.

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
