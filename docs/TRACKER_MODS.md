# Classic Tracker Mods — Catalogue & How We'd Build Them

The private-tracker scene that grew around TorrentTrader (and its cousins TBDev,
TBSource, U-232, NexusPHP) shared features as **mods**: forum posts that said
"open `announce.php`, find this block, replace with this, then run this
`ALTER TABLE`." There was no plugin system, so every mod was simultaneously a
schema change, a source edit in a hot path, and a template edit — all in the
same file.

This document catalogues the famous ones and maps each to **where it lands in
this codebase**, so a "scary source mod" becomes an ordinary
migration + service + handler + page. It is a reference and a backlog feeder,
not a commitment; nothing here is scheduled unless it also appears in
`IMPLEMENTATION_TASKS.md`.

Status legend: ✅ done · ◐ partial · ⬜ not started.

## Where mods land in this architecture

Group the classic mods by the layer the source edit used to cut into. The whole
point of the rewrite is that these are now separated:

- **Announce-path** (bonus award, freeleech accounting, client whitelist, HnR
  completion accounting): `TrackerService.Announce` + a ledger/settings table.
  This was the scariest area to mod in PHP because a bug broke the tracker; here
  it is the best-tested path (`internal/service/tracker.go`, real-Postgres repo
  tests). HnR's `hnr_records` accumulator is fed from here — `handleCompleted`
  and the leecher→seeder transition open the obligation, seeding announces credit
  it.
- **Cron / maintenance** (class promotion, bonus decay, scheduled freeleech):
  a job in the existing maintenance worker (`internal/worker/maintenance.go`),
  which already resolves expired warnings/bans/mutes/restrictions. HnR evaluation
  is the exception — it outgrew a maintenance job and has its own daemon
  (`internal/worker/hnr.go`, registered in `internal/worker/scheduler.go` at
  `45 * * * *`) with a two-stage advisory lock and a run log.
- **Event-reaction** (IRC/Discord announce, achievements, notifications): a
  subscriber on the event bus (`event/` → `listener/`) — the thing TorrentTrader
  never had, which is exactly why those mods were the ugliest to retrofit.
- **New domain** (bonus shop, requests, polls): the standard
  migration → repo → service → handler → React page pattern.

## The catalogue

### Bonus / seedbonus points ("karma", NexusPHP "magic") ✅
**The most iconic mod.** Users accrue points per hour of seeding (weighted by
size / seeder scarcity) and spend them in a shop on upload credit, invites,
custom titles, freeleech tokens. In TorrentTrader: an announce-path award plus a
whole shop UI and a `seedbonus` column.

Here: **shipped as BE-8.14** (migration 054) — a `users.bonus_points` balance
plus an append-only `bonus_transactions` ledger (every award, purchase and admin
adjustment writes a row), a store (`bonus_store_items`) selling invites and
upload credit, an hourly `bonus:award` job behind a `BonusSource` interface (the
first source is a seeding snapshot rather than an announce-path award), admin
balance adjustment, and the settings `bonus_enabled` (off by default) and
`bonus_points_per_seeding_torrent`. **This is the hub** — freeleech tokens,
request bounties, and upload multipliers all hang off the ledger; the
`freeleech_ticket` and `double_upload` store kinds are seeded but disabled until
their effects exist (see `FUTURE_WORK.md` → "Bonus Economy — Remaining Pieces").

### Freeleech / Silver / Double-upload ◐
Per-torrent freeleech (no download counted), silver (50%), global/scheduled
freeleech ("freeleech weekend"), and freeleech tokens (spend to free one torrent
for yourself). Always an announce-path source mod.

Here: **per-torrent Free and Silver are done** (`Torrent.Free`/`Silver`, wired
into `countedDownload` in the announce path — see the freeleech PR). Still open:
a **global/scheduled freeleech** flag in site settings (checked in
`countedDownload`), **freeleech tokens** (pair with the bonus ledger), and the
inverse **upload multiplier / double-upload event** (`countedUpload`, same
shape). The event-driven variant of multipliers is discussed in the design note
at the end.

### Hit-and-Run (HnR) tracking ✅
Track users who grab a torrent and don't seed it to a required time or ratio,
then escalate leniently from a notice through to a ban.

Here: **shipped** (migration `081_create_hnr.sql`, `internal/service/hnr*.go`,
`internal/worker/hnr.go`, `internal/handler/hnr.go`, admin page
`AdminHitAndRunPage.tsx`, member page `HitAndRunPage.tsx`). It turned out not to
be "a maintenance job that reads transfer history": nothing in the schema records
*how long* a member seeded a torrent — peers rows are reaped, `transfer_history`
is written once on the completed event and never refreshed, and the announce log
is pruned on retention — so HnR keeps its **own accumulator**, `hnr_records`, fed
directly by the announce path rather than derived after the fact
(`081_create_hnr.sql` opens with the reasoning). What shipped:

- **`hnr_records`** — one row per (user, torrent) snatch: an accumulator
  (`seeded_seconds`, `uploaded`) and a state machine
  (`active → hnr → satisfied | cleared | waived`). Opened by `handleCompleted`
  and the leecher→seeder transition — but not when the snatch list already dates
  the completion more than an hour back, so a re-announce for a torrent finished
  long ago cannot open a fresh obligation (#268). Credited by every seeding
  announce, capped per gap by `hnr_seed_credit_cap_minutes`. Tracking starts from
  enablement forward — no backfill (#267 declined).
- **`hnr_rules`** — per-class policy (required seed hours, required ratio,
  inactivity grace, hard cap). A class with no row is exempt, mirroring
  `promotion_rules`.
- **`hnr_penalty_stages`** — a site-wide, admin-editable five-stage ladder
  (notify → warn → restrict download/forum/chat → final notice → ban), with
  per-user position in `hnr_user_state` as a compare-and-swap target so
  escalation and de-escalation are idempotent across worker processes.
- **The daemon** — `internal/worker/hnr.go`, scheduled `45 * * * *`, with a
  two-stage `pg_advisory_lock` (not `asynq.Unique`) so concurrent invocations
  from retries, a manual "run now", or another node are safe. Evaluates open
  records, marks breach/satisfy/waive, walks the ladder, purges resolved rows
  past `hnr_retention_days`, and writes an `hnr_runs` log row.
- **Points clearing** — a member can pay off an open obligation with bonus
  points, priced `fixed` or by upload `deficit` (`hnr_clear_*` settings), spent
  through the same `bonus_transactions` ledger.
- **Exemptions** — `torrents.hnr_exempt` (staff-set, same shape as Free/Silver)
  stops a record being created and waives any already open; `hnr_exempt_donors`
  keeps donor classes out of tracking entirely (`shouldTrackHnR`,
  `internal/service/tracker.go`), which is the concrete form of the "immunity
  from HnR" perk noted under Donations / VIP tiers below.
- **UI + settings** — a member page showing obligations and their clear price, an
  admin page for the rules, the ladder, the run log and per-record staff actions,
  and an `hnr_*` block in site settings (off by default).

### IMDb / TMDb metadata + mediainfo / screenshots ◐
Auto-fetch cover art, plot, rating, cast from an external ID; parse mediainfo;
thumbnail screenshots. Huge for movie/TV trackers.

Here: **BE-3.13 (Rich Torrent Metadata) is done** — a category-driven JSONB
metadata schema, with six shipped follow-ups (BE-3.13a–f: browse/search filters
over the JSONB fields, name auto-detection on upload, the category editor page
and tree view, drag-and-drop reorder, and a missing-required-fields report).
Still open is the *external* half: auto-fetch from IMDb/TMDb, mediainfo parsing
and screenshots — a metadata service plus a nightly enrichment worker filling
the fields the schema already defines. The undecided part there is provider
choice, caching, and rate limits.

### Request system with bounties ⬜
Users request content and pledge bonus points; whoever fills it collects the
bounty. We have *reseed* requests but not open requests.

Here: a new mini-domain (requests, pledges, fill/claim) that leans on the bonus
ledger. **The block is cleared** — the ledger shipped with BE-8.14 (migration
054), so this is now buildable. Proposed as `PROPOSED_FEATURES.md` PF-17, which
notes it shares its escrow machinery (held funds, claim, confirmation, timeout,
dispute) with PF-1 and should be built first as the cheaper of the two
consumers.

### Client whitelist / blacklist (anti-cheat) ⬜
Parse the `peer_id` prefix on announce, allow only known-good client versions,
ban RatioMaster-style spoofers. Classic announce source mod.

Here: extends **BE-2.7 cheat detection**. A `client_whitelist` table checked in
`TrackerService.Announce`, feeding the existing cheat-flag pipeline. The announce
already has the peer_id in hand.

### Automatic class promotion / demotion ✅
Move users between groups on ratio + upload + account age (e.g. Power User at
25 GB and ≥ 1.05 ratio). Pure cron + source.

Here: **shipped as BE-8.13** (migration 053) — `promotion_rules` (per-class
thresholds: ratio, uploaded bytes, account age, torrent count, seed hours
estimated from `announce_events`) plus a `promotion_runs` audit table, a daily
`promotion:run` job that moves a user **±1 rung per run**, and staff classes
excluded as rungs *and* as destinations, so auto-promotion can never reach
staff. Admin UI in FE-5.11/5.12.

### IRC / Discord announce bot ✅
Announce new uploads and site events to a channel. In the PHP era this was a
separate Perl/Python bot polling the DB, or fragile source hooks.

Here: a **listener** on the event bus that posts to a webhook. `ForumPostCreated`
and torrent-upload events already flow through the bus, which is why this was
cheap given the architecture.

**Designed out and shipped** as a full pluggable connector framework (chat / IRC /
Discord / Telegram / webhook / SSE feed behind one interface) — backlog epic
**BE-10**: per-instance templates and filters, a delivery outbox with retry,
dead-lettering and rate-limit coalescing, and advisory-lock leader election so
the persistent IRC connection stays a singleton across nodes. Design and
decisions in [`NOTIFICATION_CONNECTORS.md`](./NOTIFICATION_CONNECTORS.md).

### Gamification: achievements / medals / user levels ⬜
Badges for milestones (uploaded X, seeded Y torrents, N years). Big on the
NexusPHP lineage. A table plus insertion points scattered through the source.

Here: a **listener**-driven achievements engine reacting to existing events
(upload, completion, anniversary via the maintenance worker) plus a
`user_achievements` table and a profile section.

### Donations / VIP tiers ⬜
Donor status with perks (extra invites, immunity from HnR, custom title, VIP
torrents). Source edits across many gates.

Here: a `donor`/`vip` flag (the user model already has `Donor`) consulted by the
relevant services — most perks are just conditions checked where the
corresponding rule lives (HnR job, invite service, wait-time check).

### Polls, enhanced shoutbox, thanks button, subtitles ⬜
The long tail of small mods — each a table plus a few insertion points. Straight
migration + repo + service + handler + page work here, with the shoutbox already
present (`internal/handler/chat_ws.go`). Note that **polls were explicitly
dropped** in [`NOT_PORTING.md`](./NOT_PORTING.md) §5 ("low usage feature") and
sit under its "May Be Added Later" list — cheap to build, but not decided.

## Already core (were mods elsewhere)

Several things that shipped as TorrentTrader mods are baseline features here, so
they are noted for completeness rather than as work:

- Passkey-based announce auth (standard now; was a mod on the earliest scripts)
- Forums (BE-5.x), shoutbox/chat (BE-6.x), notifications (BE-5.6–5.9)
- Warnings + auto-ban, ratio-warning automation, tiered escalation
- Cheat detection (BE-2.7), privilege restrictions (BE-8.9)
- Invites (BE-4.x), comments & ratings (BE-3.7), reseed requests, RSS
- Wait-time system (BE-2.3), NFO viewer, categories with images

## Making mods stop being core edits

The recurring cost of these mods is that ratio-affecting ones each want a branch
in the announce hot path, and behaviour changes want to edit core files. Two
directions for avoiding that — a single synchronous stats resolver (recommended
first step) and, later, a compile-time plugin model — are written up with their
trade-offs and a recommended sequencing in [`EXTENSIBILITY.md`](./EXTENSIBILITY.md).
Both are exploratory; neither is committed.
