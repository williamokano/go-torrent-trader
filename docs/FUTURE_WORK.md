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
