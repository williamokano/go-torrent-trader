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
  completion): `TrackerService.Announce` + a ledger/settings table. This was the
  scariest area to mod in PHP because a bug broke the tracker; here it is the
  best-tested path (`internal/service/tracker.go`, real-Postgres repo tests).
- **Cron / maintenance** (class promotion, HnR evaluation, bonus decay,
  scheduled freeleech): a job in the existing maintenance worker
  (`internal/worker/maintenance.go`), which already resolves expired
  warnings/bans/mutes/restrictions.
- **Event-reaction** (IRC/Discord announce, achievements, notifications): a
  subscriber on the event bus (`event/` → `listener/`) — the thing TorrentTrader
  never had, which is exactly why those mods were the ugliest to retrofit.
- **New domain** (bonus shop, requests, polls): the standard
  migration → repo → service → handler → React page pattern.

## The catalogue

### Bonus / seedbonus points ("karma", NexusPHP "magic") ⬜
**The most iconic mod.** Users accrue points per hour of seeding (weighted by
size / seeder scarcity) and spend them in a shop on upload credit, invites,
custom titles, freeleech tokens. In TorrentTrader: an announce-path award plus a
whole shop UI and a `seedbonus` column.

Here: a `user_bonus` ledger table, an award step in `TrackerService.Announce`
(the peer state is already in hand there), a decay/award job in the maintenance
worker, and a shop service + page. **This is the hub** — freeleech tokens,
request bounties, and upload multipliers all hang off the ledger. Highest single
feature for making the tracker read as "modded" rather than stock.

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

### Hit-and-Run (HnR) tracking ⬜
Track users who grab a torrent and don't seed to a required ratio/time, then
warn or restrict them. Needed a snatch-completion record and a cron.

Here: the pieces already exist — `transfer_history`, the maintenance worker, and
the warning/restriction system. HnR is a **maintenance job** that reads transfer
history against a policy and issues a restriction. Arguably the cleanest famous
mod to add because nothing new is invented.

### IMDb / TMDb metadata + mediainfo / screenshots ◐
Auto-fetch cover art, plot, rating, cast from an external ID; parse mediainfo;
thumbnail screenshots. Huge for movie/TV trackers.

Here: this is **BE-3.13 (Rich Torrent Metadata, RESEARCH)** in
`IMPLEMENTATION_TASKS.md`. A metadata service + a nightly enrichment worker +
fields on the torrent. The "research" is provider choice, caching, and rate
limits.

### Request system with bounties ⬜
Users request content and pledge bonus points; whoever fills it collects the
bounty. We have *reseed* requests but not open requests.

Here: a new mini-domain (requests, pledges, fill/claim) that leans on the bonus
ledger. Blocked on the bonus system existing first.

### Client whitelist / blacklist (anti-cheat) ⬜
Parse the `peer_id` prefix on announce, allow only known-good client versions,
ban RatioMaster-style spoofers. Classic announce source mod.

Here: extends **BE-2.7 cheat detection**. A `client_whitelist` table checked in
`TrackerService.Announce`, feeding the existing cheat-flag pipeline. The announce
already has the peer_id in hand.

### Automatic class promotion / demotion ⬜
Move users between groups on ratio + upload + account age (e.g. Power User at
25 GB and ≥ 1.05 ratio). Pure cron + source.

Here: a **maintenance job** that reads user stats and updates `group_id`. The
same worker already does time-based restriction expiry, so it is the same shape.

### IRC / Discord announce bot ⬜
Announce new uploads and site events to a channel. In the PHP era this was a
separate Perl/Python bot polling the DB, or fragile source hooks.

Here: a **listener** on the event bus that posts to a webhook. `ForumPostCreated`
and torrent-upload events already flow through the bus; this is a few hours of
work given the architecture.

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
present (`internal/handler/chat_ws.go`).

## Already core (were mods elsewhere)

Several things that shipped as TorrentTrader mods are baseline features here, so
they are noted for completeness rather than as work:

- Passkey-based announce auth (standard now; was a mod on the earliest scripts)
- Forums (BE-5.x), shoutbox/chat (BE-6.x), notifications (BE-5.6–5.9)
- Warnings + auto-ban, ratio-warning automation, tiered escalation
- Cheat detection (BE-2.7), privilege restrictions (BE-8.9)
- Invites (BE-4.x), comments & ratings (BE-3.7), reseed requests, RSS
- Wait-time system (BE-2.3), NFO viewer, categories with images

## Design note — multipliers via events instead of announce edits

*(Forward-looking; not committed. Captured from a maintainer question.)*

The concern with the pattern above is that every ratio-affecting mod (freeleech,
silver, double-upload, per-user seedbox multipliers, bonus-shop-purchased
boosts) means another branch in the announce hot path. An alternative:

**Emit a raw stats-delta event from the announce, and let listeners apply
multipliers.** The announce publishes something like
`PeerStatsRecorded{userID, torrentID, rawUp, rawDown}` and writes the raw peer
baseline (which it must do anyway for the next delta). A `ratio` listener then
resolves the effective multipliers — per-torrent freeleech, global freeleech
window, a 24h double-upload the user bought, a group perk — and calls
`IncrementStats` with the adjusted values. The announce path stops growing a
branch per policy; new multipliers become new (or extended) listeners.

Trade-offs to weigh before going this way:
- **Consistency window.** Today `IncrementStats` is synchronous inside the
  announce. Moving it behind an async listener means the user's ratio lags their
  announce slightly, and a dropped event loses accounting. Either keep the bus
  synchronous for this event, or make the listener idempotent against the raw
  peer row so it can be replayed.
- **Ordering / composition.** Multiple multipliers must compose to a defined
  result (does a bought 2× stack with a global freeleech? freeleech usually
  wins). That policy has to live somewhere explicit — a resolver — rather than
  emerging from listener registration order.
- **Testability.** The current `countedDownload` is a pure function, trivially
  tested. A resolver keeps that property; scattered listeners each mutating
  stats do not. Prefer "one resolver, many inputs" over "many listeners, each
  writes."

The lighter first step, short of this, is to keep the announce synchronous but
factor multiplier resolution into a single `effectiveDelta(user, torrent, raw)`
resolver that consults torrent flags, site settings, and (later) the bonus
ledger. That removes the "branch per mod" problem without taking on async
accounting risk, and it is a refactor of one function rather than an
architectural change.

## Design note — a plugin model

*(Forward-looking; not committed. Captured from a maintainer question.)*

The deeper goal is to stop core files drifting as mods accumulate — to add/modify
behaviour through plugins rather than editing `tracker.go` / `router.go` /
services. How it could work, and the honest catch:

- **Join points already exist in embryo.** The event bus (`event/` → `listener/`)
  is a reaction-side extension point today. A plugin that only *reacts* to events
  (announce bot, achievements, external notifications) is safe and needs nothing
  new — it is just a registered listener.
- **The hard part is decision-side hooks** — a plugin that must *change* a flow:
  veto an announce, alter a stats delta, add a route, add a shop item. That needs
  defined interfaces (`AnnounceInterceptor`, `StatsResolver`, `RouteProvider`,
  `ShopItemProvider`) with a documented contract about what each may read and
  return, plus a registry the core consults at the join point.
- **Why it is genuinely tricky, as you noted:** a decision-side plugin can break a
  flow it never textually touched — return the wrong veto, resolve a nonsensical
  multiplier, panic mid-announce. Mitigations: narrow, well-typed interfaces (not
  "here's the whole request, do anything"); a resolver/chain with defined
  composition rather than free-for-all mutation; recover-and-isolate around each
  plugin call so one bad plugin degrades to a logged skip instead of a 500; and a
  capability boundary (a stats plugin cannot mount routes).
- **Realistic middle ground.** Full dynamic plugins (separately compiled,
  hot-loaded) are heavy in Go and rarely worth it. The pragmatic version is
  **compile-time plugins**: internal packages that register against these
  interfaces at startup, so "the core" is a thin orchestrator and features are
  self-contained modules that never edit the orchestrator. That captures most of
  the anti-drift benefit without the runtime-loading risk.

Recommended sequencing if this path is ever taken: (1) prove the reaction-side is
already pluggable by moving the IRC/achievements features to pure listeners; (2)
introduce **one** decision-side interface where the pain is real — the
`StatsResolver` from the multiplier note — and see how it feels; (3) generalise
only if the pattern earns its keep. Do not build the framework first.
