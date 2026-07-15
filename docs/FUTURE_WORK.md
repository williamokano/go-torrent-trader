# Future Work

Items that are out of scope for the initial release but worth pursuing later. These are not tracked in `IMPLEMENTATION_TASKS.md` and should not be mentioned during regular task planning unless explicitly requested.

---

## UDP Tracker Protocol (BEP 15)

Implement the UDP announce/scrape protocol for lower latency and higher connection throughput. The HTTP tracker covers all functionality; UDP is a performance optimization for high-traffic deployments.

**Key requirements:**
- Connection handshake with protocol_id verification and connection_id with 2-minute TTL
- Announce and scrape sharing the same service layer as the HTTP tracker
- Compact response only (UDP has no room for non-compact)
- IPv4 and IPv6 support

**Why deferred:** HTTP tracker is fully functional and sufficient for the current scale. UDP adds operational complexity (separate port, stateless protocol, connection ID cache) with marginal benefit until the tracker handles thousands of concurrent announces per second.

---

## Theme Management

Full theming system with user-selectable themes and admin controls.

**Theme Switching UI:** Theme selector in user settings (dropdown or preview cards) with instant preview, saved to user settings API + localStorage.

**Admin Theme Configuration:** Set default theme for new users, enable/disable specific themes, theme list in admin settings.

**Additional Theme (Retro/Classic Tracker):** Classic private tracker aesthetic — dark background, monospace elements, compact layout. Uses CSS custom properties only (no structural changes).

**Why deferred:** The current light/dark toggle via the header dropdown covers the immediate need. A full theme system with admin controls and custom themes is a nice-to-have for post-launch.

---

## Real-Time Stats Push (SSE)

Push site stats (peer/torrent/user counts in the footer) to clients instead of polling, ideally via Server-Sent Events.

**Key requirements:**
- One unidirectional SSE stream broadcasting stat updates (no WebSocket — a full duplex connection and a second hub aren't warranted for footer numbers).
- Broadcast on the events that move the numbers (peer announce, torrent upload, user registration).
- Graceful fallback to polling if the stream drops.

**Why deferred:** footer stats are low-stakes, eventually-consistent numbers, and they are already served from the Redis `StatsCache`, so the current polling reads a cache rather than the database — it is cheap. A dedicated WebSocket would add a second hub, per-client goroutines, and reconnect logic for data nobody watches second-by-second; that trade isn't worth it. **If fresher stats are ever wanted, the lever is lowering the `StatsCache` TTL, not changing transport** (polling faster than the TTL just returns the same cached value). SSE is the tasteful upgrade only if stats freshness ever becomes a real product concern.

---

## Announce Event Log — Consumers & Retention

The append-only `announce_events` log now captures every announce (BE-9.21), but nothing consumes it yet and nothing prunes it.

**Key requirements:**
- **Retention cleanup job.** Honour the existing `announce_log_retention_days` setting (currently advisory only — no deletion runs). Add a maintenance-worker pass that deletes rows older than the window. For scale, prefer native monthly partitioning (cheap `DROP` of old partitions) and/or a nightly rollup into a `user_period_stats(user_id, year_month, uploaded, downloaded)` table so raw rows can be expired aggressively while monthly aggregates are kept forever.
- **GDPR/LGPD data export.** A per-user export endpoint over `AnnounceEventRepository.ListByUser` (and the other personal-data tables). Note: `announce_events` stores IP + peer_id (personal data), so this store is also an erasure obligation — deletion already cascades on account removal, but export/retention must stay purpose-scoped.
- **Bonus points / monthly campaigns.** The motivating use case: "top N uploaders this month" and ratio/bonus reconciliation are computable from `SUM(uploaded_delta)`/`SUM(counted_downloaded_delta)` over a time window — impossible from the overwriting projections. Build the aggregation queries (and any bonus ledger) on top of the log.

**Why deferred:** capture was the irreversible, near-free half (the deltas were already computed and discarded); the consumers are real features that can wait until wanted. See BE-9.21.

---

## Auto Invite Distribution

The class ladder now exists (BE-8.13), so invites — which are class-gated — can finally be handed out on merit automatically. This is the "capped weekly drip" designed earlier.

**Key requirements:**
- **Cap-based drip, not unbounded accrual.** On a schedule (weekly), grant a small number of invites (e.g. 1) to each eligible user, but **only if they hold fewer than a cap** (e.g. 3 unused). The cap bounds total outstanding invites to ≈ (eligible users × cap) and stops the drip refilling until an invite is actually used — preventing hoarding/trading.
- **Eligibility gates** (all configurable): class/level at or above a threshold (reuse the promotion ladder), ratio ≥ X with a minimum-downloaded floor, account age ≥ N days, active within N days (`last_access`), enabled/not-parked/not-restricted.
- **Mechanics**: one set-based `UPDATE users SET invites = LEAST(invites + k, cap) WHERE invites < cap AND <eligibility>`, run from the maintenance worker; log the aggregate to the activity log. Remember the two gates already in the code: `groups.can_invite` (permission) **and** `users.invites > 0` (balance, decremented per invite) — the drip fills the balance; the permission is the group's.
- **Optional accountability**: record who invited whom (invite tree) so easy invites don't degrade community quality — most trackers make the inviter partly responsible for invitees.

**Why deferred / sequencing:** needed the merit ladder first (done). The richer successor is a **bonus-point economy** where seeding time (from `announce_events`) earns points spent on invites — the most abuse-resistant model, and the eventual consumer of the announce log.

---

## Editable User Privileges + Invite Capability

**Observed:** a freshly created user cannot send invites, and the admin user-detail page shows the privilege flags (download/upload/chat/forum) but offers no way to edit them.

**Root cause:** invite capability is **group-scoped** — `groups.can_invite` (`perms.CanInvite`, enforced at `backend/internal/middleware/auth.go:114`). The user model has per-user `can_download/can_upload/can_chat/can_forum` but **no** per-user `can_invite`. So a new user's ability to invite depends entirely on their group, and there is currently no admin UI to (a) edit the per-user privilege flags that *are* shown, or (b) grant invite rights.

**Key requirements:**
- Make the per-user privilege flags on the admin user-detail page editable (backend PATCH + frontend controls), not just display.
- Decide the invite model and implement it: either surface/manage `can_invite` through **group management** (see the parallel `feat/admin-group-management` work — editing group permissions including `can_invite` is the natural home), or add a per-user `can_invite` override if per-user granularity is wanted. Prefer the group route unless a concrete need for per-user override emerges.
- Ensure the default registration group's `can_invite` is set intentionally (a new user inheriting a group without invite rights is the surfaced symptom).

**Why deferred:** flagged during testing; grouped with the group-management admin work since that is where invite permissions most naturally live.

---

## Classic Tracker Mods

See [`TRACKER_MODS.md`](./TRACKER_MODS.md) for a catalogue of the famous
TorrentTrader/TBDev/NexusPHP-era mods (bonus points, freeleech tokens,
hit-and-run, client whitelist, IMDb metadata, request bounties, class
promotion, IRC bot, achievements, …), each mapped to where it would land in
this architecture.

See [`EXTENSIBILITY.md`](./EXTENSIBILITY.md) for the exploratory design
direction on making mods stop being core-file edits: a single synchronous
stats resolver (recommended first step) and, later, a compile-time plugin
model, with trade-offs and a recommended sequencing.
