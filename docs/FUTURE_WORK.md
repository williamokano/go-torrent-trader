# Future Work

Items that are out of scope for the initial release but worth pursuing later. These are not tracked in `IMPLEMENTATION_TASKS.md` and should not be mentioned during regular task planning unless explicitly requested.

Sections headed **— shipped** are kept as short pointers rather than deleted, so a reader who remembers an item from here is not left thinking it is still unbuilt; the work itself is recorded in `IMPLEMENTATION_TASKS.md`.

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

**Update:** the transport is no longer hypothetical — BE-10.3 shipped an authenticated SSE hub for the live release feeds (`docs/NOTIFICATION_CONNECTORS.md`), so this would reuse an existing fan-out pattern rather than introduce one. The reasoning below is unchanged: the cost was never the plumbing.

**Why deferred:** footer stats are low-stakes, eventually-consistent numbers, and they are already served from the Redis `StatsCache`, so the current polling reads a cache rather than the database — it is cheap. A dedicated WebSocket would add a second hub, per-client goroutines, and reconnect logic for data nobody watches second-by-second; that trade isn't worth it. **If fresher stats are ever wanted, the lever is lowering the `StatsCache` TTL, not changing transport** (polling faster than the TTL just returns the same cached value). SSE is the tasteful upgrade only if stats freshness ever becomes a real product concern.

---

## Announce Event Log — Retention & Maintenance

The append-only `announce_events` log captures every announce (BE-9.21). **It is never pruned, so it grows without bound.** `announce_log_retention_days` (seeded at 90 by migration 052) is **advisory only** — the setting exists and is validated, but no job reads it and nothing deletes a row. The table also carries two indexes (`(user_id, announced_at DESC)` and `(announced_at)`), so the write amplification compounds. This is the maintenance half; the consumer-side proposal is `PROPOSED_FEATURES.md` PF-33, which depends on this work landing before anything queries the table over date ranges at page speed.

**Key requirements:**
- **Retention cleanup job.** Honour `announce_log_retention_days` — a maintenance-worker pass that deletes rows older than the window, of the same shape as the connector delivery-log prune that already runs (`ConnectorDeliveryRetention` in `internal/worker/maintenance.go`). For scale, prefer native monthly partitioning (cheap `DROP` of old partitions) and/or a nightly rollup into a `user_period_stats(user_id, year_month, uploaded, downloaded)` table so raw rows can be expired aggressively while monthly aggregates are kept forever.
- **GDPR/LGPD data export.** A per-user export endpoint over `AnnounceEventRepository.ListByUser` (and the other personal-data tables). Note: `announce_events` stores IP + peer_id (personal data), so this store is also an erasure obligation — deletion already cascades on account removal, but export/retention must stay purpose-scoped.
- **Announce-log bonus source / monthly campaigns.** The bonus economy now exists (BE-8.14) with an hourly *snapshot* source; the announce-log successor — actual seed-time from `SUM` over `announce_events` deltas, harder to game at cycle boundaries — plugs in as another `BonusSource`. Same for "top N uploaders this month" campaigns. (The promotion engine already computes seed hours this way, so the SQL exists.)

**Why deferred:** capture was the irreversible, near-free half (the deltas were already computed and discarded); the consumers are real features that can wait until wanted. The retention job is the part that is not optional forever — it is maintenance the table needs regardless of whether any consumer is ever built. See BE-9.21.

---

## Auto Invite Distribution — shipped

**Shipped as BE-4.2** (migration 056, admin page FE-5.16): per-group `invite_distribution_rules` (ratio floor, downloaded-bytes range, and a per-user `max_invites` ceiling), `invite_distribution_runs` bookkeeping, the `invite_distribution_enabled` / `invite_distribution_interval_days` settings, a daily `30 5 * * *` job that grants +1 invite per cycle only while the balance is under the ceiling, and admin CRUD plus run-now. The invite tree the section wanted for accountability already existed (`users.invited_by`, migration 019).

Residue, if it is ever wanted: the eligibility gates that did **not** ship are account age ≥ N days and "active within N days" (`last_access`) — the shipped rules gate on group, ratio and downloaded range only.

---

## Bonus Economy — Remaining Pieces

The foundation shipped in BE-8.14/FE-5.14: balance + append-only ledger, hourly seeding-snapshot award behind the `BonusSource` interface, store with invites + upload credit, admin balance adjustment, master switch. Still future:

- **Freeleech ticket effects.** The `freeleech_ticket` kind is seeded (disabled). Needs: a per-user ticket balance or per-torrent activation, tracker announce integration (count download as free while a ticket is active), and UI. Then flip the item to enabled.
- **Double upload effects.** The `double_upload` kind is seeded (disabled). Needs: a per-user expiry timestamp, announce-time upload-delta multiplication while active, and UI showing time remaining.
- **Announce-log award source.** Replace/augment the snapshot source with actual seed-time computed from `announce_events` (same gap-sum SQL as the promotion engine) — resistant to clients that only announce at cycle boundaries. Plugs in as a second `BonusSource`.
- **Ledger UI.** A "points history" page over `bonus_transactions` (the data is already recorded for every movement).
- **Promotion synergy.** Class-promotion rules could gate on lifetime points earned; store prices could vary by class.

---

## Editable User Privileges + Invite Capability — shipped

**Observed at the time:** a freshly created user could not send invites, and the admin user-detail page showed the privilege flags but offered no way to edit them.

**Shipped across BE-8.12/8.15/8.16 and FE-5.10/5.15.** Both halves are closed:
- **Group side** — `AdminGroupsPage` is full CRUD over groups including the capability checkboxes, so `groups.can_invite` (the permission gate, built from the group in `model.PermissionsFromGroup` and enforced by `middleware.RequireCapability` in `backend/internal/middleware/auth.go`) is editable per class. The seeded ladder grants it from Power User upward and withholds it from the default registration group — the "new user can't invite" symptom, now a deliberate, editable setting rather than an accident.
- **User side** — a per-user `can_invite` column exists (migration 055) alongside `can_download/can_upload/can_chat`, and the user-detail Privileges panel suspends and restores each of them with a reason, an optional expiry and a restriction history table. BE-8.16 made those four columns writable only through a targeted `SetPrivilegeFlag`, so a full-row `Update` can no longer clobber a restriction.

The model that settled: the **group** carries the capability, the **per-user flag** is a punitive override, and the two never drift.

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
