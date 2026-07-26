# External Notification Connectors — Design

**Status:** implemented. Phases 1–4: BE-10.1 — the connector seam, Chat and
Webhook connectors, the delivery pipeline and admin CRUD; BE-10.2 — IRC, the
persistent-connector lifecycle and advisory-lock leader election; BE-10.3 — the
authenticated SSE live feed; BE-10.4 — Discord and Telegram. Since then:
**BE-10.6** — connector admin usability (nested category pickers, exclude
filters, several independent chat instances, per-kind template help); **BE-10.7**
— several live feeds, each with its own slug and URL; **BE-10.8** — `can_feed`,
live feed access as a class privilege revocable per member. Phase 5, the per-user
relay, was **dropped** by operator decision (§9) — it is not outstanding work.

Phase 4 is the evidence the seam works: two packages, two registration lines and
one helper promoted into the shared package, with no change to the dispatcher,
the repositories, the handlers or the schema. Both kinds inherited filters, the
kill-switch, dedupe, retry, dead-lettering, rate-limit coalescing, test-send and
the delivery log by existing.

It also showed up one genuine gap, now closed: a connector could say "not ready"
but not "this will never work", so a permanently-broken destination in a fan-out
made every healthy one receive the same announcement on every retry.
`connector.ErrPermanent` dead-letters such a delivery immediately. Implementation plan: `docs/plans/BE-10.md`.
Where this document and the plan differ, the plan's §1 records the decision.

**Relates to:** `docs/EXTENSIBILITY.md` (reaction-side plugins), `docs/TRACKER_MODS.md`
("IRC / Discord announce bot"), `docs/FUTURE_WORK.md` (Real-Time Stats Push / SSE).

Three things the implementation settled that this document left open:

- **Delivery is at-least-once.** The `(instance_id, event_key)` unique index
  gives exactly one delivery *row* per event per instance, and a per-row lease
  stops two overlapping drains sending the same announcement. A success whose
  bookkeeping write then fails still retries, so a receiver that cares should
  deduplicate on the `X-Announce-Delivery` header.
- **Coalescing is per-kind, not global.** Replacing a backlog with "+N more" is
  the desired behaviour for a destination a person reads and data loss for one a
  program reads, so `Connector.Coalescable()` decides. Machine-read kinds spend
  their whole rate budget on individual deliveries and defer the rest to the
  next window instead.
- **"Not ready" is a third delivery outcome**, alongside success and failure.
  A connector that is mid-reconnect, or running on a node that does not own the
  connection, returns `connector.ErrNotReady`; the delivery is rescheduled
  without consuming a retry attempt, bounded to 15 minutes. Leader election is
  the Postgres advisory-lock variant, with a 10s ownership re-check that stops
  the client the moment ownership cannot be proven.

## 1. Motivation

When a torrent is **released** (becomes publicly visible), we want to announce it
to places beyond the in-app notification list: the site's general chat, an IRC
channel, Discord/Telegram, a generic webhook, or a live authenticated feed clients
can subscribe to. Every classic tracker grows a pile of one-off announce bots for
this; we want **one interface, many connectors**, so adding "announce to X" is a
small, self-contained module rather than a new special case.

`docs/EXTENSIBILITY.md` already establishes the shape: a thing that only *reacts*
to an event ("when a torrent is published, also announce it to X") is a **pure
event-bus listener** — it runs after the fact, its failure is isolated, and it
cannot break a core flow. This document formalizes that reaction-side idea into a
configurable **Connector** framework, and — respecting that doc's advice to *"not
build a framework first; let the pattern earn each step"* — sequences the work so
the first connectors ship as plain listeners and the registry/config layer is
introduced only once there are enough connectors to justify it.

## 2. Core concepts

- **`Announcement`** — a canonical, connector-agnostic payload describing *what
  happened* (e.g. a torrent was published), with everything any connector might
  need to render a message. Connectors never see domain models or the request.
- **`TorrentPublished` event** — a single domain event emitted at the moment a
  torrent becomes public. Connectors key off this, never off moderation internals.
- **`Connector`** — a driver that delivers an `Announcement` to one destination
  kind (chat, irc, discord, telegram, webhook, sse). Registered at compile time.
- **Connector instance** — a *configured, enabled* connector stored in the DB
  (which IRC server, which webhook URL, which category filter). One kind can have
  many instances (e.g. two IRC channels, three webhooks).
- **Dispatcher** — the event-bus listener that turns a `TorrentPublished` event
  into an `Announcement` and fans it out to every matching enabled instance.
- **Delivery pipeline** — async, isolated, retrying delivery of each announcement
  to each instance (a slow IRC/HTTP call must never block the request that
  triggered it).

```
domain service ──emit──> event bus ──> ConnectorDispatcher (listener)
                                             │  builds Announcement
                                             │  loads enabled instances, applies filters
                                             ▼
                                     delivery queue (worker)
                                     ├─ chat connector      (in-process → ChatService)
                                     ├─ sse connector       (in-process → SSE hub)
                                     ├─ irc connector       (persistent IRC client)
                                     ├─ discord connector   (HTTP webhook POST)
                                     ├─ telegram connector  (Bot API POST)
                                     └─ webhook connector    (generic HTTP POST)
```

## 3. Canonical event: `torrent.published`

A torrent becomes public in exactly two places today:
- **On approval** (`TorrentService.ApproveTorrent`), when moderation is on.
- **On upload** (`TorrentService.Upload`), when `moderation_enabled=false`.

Introduce a single `event.TorrentPublished` emitted at *both* transition points
(and nowhere else). This is the only place connectors hook, which guarantees:
- **Pending / rejected / anonymous-sensitive** torrents never leak — we only ever
  announce something already approved for public listing.
- Connectors are decoupled from moderation; if the publish rule changes later,
  they don't.

`TorrentPublishedEvent` payload (superset of what any connector needs):

```
TorrentID, Name, InfoHashHex,
Category { ID, Name, Path },       // breadcrumb, for routing/labels
Size, FileCount,
Uploader { ID, Name } | Anonymous, // respect the anonymous flag — never leak
Flags { Freeleech, Silver, ... },
URL,                               // absolute torrent-detail URL
PublishedAt
```

The **`Announcement`** the dispatcher builds is a lightly-rendered view of this:
a `Title`, a `Body` (default template), the structured fields above, and the URL.
Each connector formats it its own way (IRC = one line; Discord = an embed; webhook
= JSON).

It also carries **`category_path`** — the category's ancestor chain, root first,
ending with `category_id`. It is what makes a category filter cover a subtree
(§5.2), it is part of the stored payload and therefore of the webhook body and
the SSE frame, and IRC re-reads it at delivery time so per-channel routing agrees
with the instance filter. The dispatcher resolves it once per event, for every
announcement, so its presence never depends on how some unrelated instance
happens to be filtered.

> Note: the same pattern generalizes to other event kinds later (`forum.post`,
> `news.published`, `user.registered`). Start with `torrent.published`; keep the
> `Announcement.Event` field so connectors and filters can widen without a
> redesign.

## 4. The Connector interface(s)

Two shapes, because they have genuinely different lifecycles:

### 4a. One-shot connectors (chat, discord, telegram, webhook, sse-fanout)

```go
type Connector interface {
    Kind() string                                   // "webhook", "discord", ...
    // ValidateConfig checks an instance's config at save time (fail fast in the UI).
    ValidateConfig(cfg json.RawMessage) error
    // Deliver renders and sends one announcement. Errors are retried/logged by the
    // pipeline; Deliver itself must be side-effect-safe to retry (idempotent-ish).
    Deliver(ctx context.Context, cfg json.RawMessage, a Announcement) error
}
```

### 4b. Persistent connectors (IRC — a long-lived connection)

IRC is not a one-shot POST: it holds a TCP/TLS connection, authenticates, joins
channels, and stays connected. It needs a lifecycle:

```go
type PersistentConnector interface {
    Connector
    // Start opens/maintains the connection (reconnect with backoff) until ctx is done.
    Start(ctx context.Context, cfg json.RawMessage) error
    // Deliver on a PersistentConnector writes to the already-open connection.
}
```

A **ConnectorManager** owns the lifecycle of persistent instances: on startup and
on config change it (re)starts the IRC clients; `Deliver` for an IRC instance
hands the line to the running client (or drops with a logged error if
disconnected). This is the one connector that carries real operational weight;
everything else is a stateless HTTP/in-process call.

### 4d. Single-owner across nodes (leader election) — persistent connectors only

A persistent IRC connection **must be a singleton across the deployment**. If the
app runs as more than one process/replica, two nodes both connected to IRC would
double-post and collide on the nick. So each persistent instance needs
**exactly-one-owner** semantics with automatic failover:

- **Single-process deployment (the common self-hosted case):** nothing to do — the
  one process owns every persistent connector. This must stay the zero-config
  default; leader election is only engaged when more than one node is detected.
- **Multi-node deployment:** elect one owner per persistent instance via a
  **lease** on infrastructure we already run — no new dependency:
  - **Postgres advisory lock** (`pg_try_advisory_lock` keyed by connector id): the
    node holding it runs the client; if that node crashes, its session ends and
    the lock releases, so another node acquires it and reconnects. Simple and
    crash-safe.
  - **or Redis lease** (`SET owner NX PX <ttl>` renewed on a heartbeat; on expiry
    another node takes over). We already use Redis for sessions.
- On **takeover**, the new owner reconnects to IRC and resumes announcing. The
  brief gap during failover is acceptable for an announce bot; combined with the
  pipeline's at-least-once + dedup (§6), the window costs at most a short delay or
  a harmless duplicate, never a lost-forever guarantee we promised.
- **One-shot connectors need none of this** — webhook/Discord/Telegram/chat/SSE
  are stateless, so any node can deliver. The only cross-node concern there is not
  delivering the *same job* twice, which is job-ownership in the delivery queue
  (§6), not connection ownership.

Because the whole HA question only bites persistent connectors, it's contained to
Phase 2 (IRC); Phase 1's connectors are stateless and multi-node-safe as-is.

**Guarantees & limitations — this is single-owner coordination, NOT consensus.**
Don't mistake the advisory lock for strict, always-exactly-one mutual exclusion:
- **How it works:** each node holds a dedicated DB connection and calls
  `pg_try_advisory_lock(connector_id)` (non-blocking; returns whether it won). A
  session-level advisory lock is **auto-released when that DB session ends** — so a
  crashed leader frees the lock and a standby acquires it on its next try. Liveness
  rides the TCP session, so there's no lease TTL to tune.
- **No fencing → a bounded split-brain window.** The lock release (DB side) and the
  old leader noticing and stopping (app side) aren't atomic. A leader partitioned
  from Postgres but still connected to IRC can keep posting briefly *after* a
  standby takes over → a duplicate line or nick collision until it self-detects.
  Advisory locks can't hand IRC a fencing token to reject the stale leader, so this
  window can be shrunk but not eliminated.
- **Detection latency:** a clean crash frees the lock immediately; a hard partition
  only frees it once Postgres reaps the dead session (tune `tcp_keepalives_*` so
  that's seconds, not minutes).
- **Primary-only:** advisory locks aren't replicated and don't survive a Postgres
  restart/failover — fine here, since the app is down then anyway.
- **Why it's acceptable:** announce delivery is low-stakes and deduped, so a rare
  duplicate or a short failover gap costs nothing. The mitigations are: the leader
  **continuously re-checks it still holds the lock and stops the IRC client the
  moment it can't confirm ownership**, plus the pipeline's dedup + at-least-once
  (§6). If leadership ever guarded something dangerous (not announcements), this
  would be insufficient — you'd need fencing tokens or a real consensus store
  (etcd/Consul/Raft). For an announce bot, that's overkill.

The Redis-lease variant is the same class of guarantee (arguably weaker: adds a
TTL/clock dependency and the known Redlock fencing critique) — which is why the
advisory lock is the lean.

### 4c. Registry (compile-time)

```go
type Registry struct { /* map[kind]Connector */ }
func (r *Registry) Register(c Connector)
func (r *Registry) Get(kind string) (Connector, bool)
```

Built-in kinds register at bootstrap — **compile-time modules, not hot-loaded**
(per `docs/EXTENSIBILITY.md`). Adding a connector = one new package that
`Register`s itself; the dispatcher and config layer don't change.

## 5. Configuration & storage

**Multiple instances, independently toggleable.** Every connector kind **except
sse** can have **several independent instances**, each with its own config and
its own `enabled` flag — e.g. two IRC connections to different networks, a Discord
webhook per category, three generic webhooks. Chat is included: several shoutbox
instances post to the one shoutbox, but each carries its own template and filters,
which is how a site words the Anime line differently from the Movies line. Two
chat instances whose filters overlap really do post twice — the admin's call to
make. SSE is no longer a singleton either: feeds are told apart by slug and a
watcher subscribes to exactly one, so a second instance adds a feed rather than a
duplicate.

"Just decide to not notify" is simply `enabled=false` on an instance — the
config is kept, delivery stops; deleting an instance removes it entirely. An
optional **global kill-switch** site setting (`connectors_enabled`) mutes *all*
external delivery at once (maintenance, incident) without touching per-instance
config; the internal chat/SSE feeds can be exempted or included as configured.

Table `notification_connectors`:

| column        | notes |
|---------------|-------|
| `id`          | PK |
| `kind`        | `chat` \| `irc` \| `discord` \| `telegram` \| `webhook` \| `sse` |
| `name`        | admin label ("Main IRC", "Staff Discord") |
| `enabled`     | bool |
| `config`      | JSONB — per-kind connection details (see below) |
| `filters`     | JSONB — routing (see §5.2) |
| `created_at`, `updated_at` | |

### 5.1 Per-kind config (examples)

- **chat**: `{ template }` — the system message to post.
- **irc**: `{ server, port, tls, nick, sasl_user, sasl_pass(secret), channels:[{name, categories?}], template, rate_per_min }`.
- **discord**: `{ webhook_url(secret), username?, avatar_url?, template }`.
- **telegram**: `{ bot_token(secret), chat_ids:[...], template }`.
- **webhook**: `{ url, method, headers:{...}, hmac_secret(secret)?, body_template? }` — the fully generic one.
- **sse**: `{ slug }` — the feed's URL segment. Several feeds can exist, each with
  its own filters: one carrying everything, another everything except a category.

  **A feed's filters are a presentation choice, not an access control.** They
  narrow what a feed carries; they do not decide who may read it. Access is one
  privilege across every feed — `can_feed`, granted by class and revocable per
  member (BE-10.8) — so a member either gets the live feeds or does not. A feed
  filtered to staff-only categories is still visible to anyone holding
  `can_feed`; filters are not the place to express that.

  **`can_feed` is site-wide, not per-feed — deliberately.** Per-feed access
  scoping was asked for in the BE-10.7 review and **considered and not built**
  (operator decision, 2026-07-25; recorded in `tasks/todo.md`): a per-feed
  dimension would be a second permission model to maintain, for a need nobody
  had stated. If a truly restricted stream is ever wanted, that is a new decision
  to take, not a filter to configure.

  Revoking `can_feed` closes the member's open streams, because the gate runs at
  connect and a stream held open would otherwise outlive the privilege. Editing a
  class's `can_feed` disconnects every watcher rather than just that class's, for
  the same reason and because the hub keys clients by user, not by group — they
  reconnect within seconds and are re-checked.

  Renaming or deleting a feed disconnects every watcher, on purpose: the hub keys
  watchers by slug, so a freed slug later taken by another feed would otherwise
  silently rebind them to it. Browsers reconnect within seconds and re-resolve.

### 5.2 Filters / routing

`filters` narrows which announcements an instance receives:
`{ event_types:["torrent.published"], category_ids:[...], category_mode, min_size, freeleech_only, exclude_anonymous }`.
"One or more" targets (e.g. IRC channels per category) live inside the instance
config (`channels[].categories`) so a single connection can route by category.
Those channel categories match the ancestor chain too: the two selects look
identical in the admin UI, so picking a parent has to mean the same thing in
both — otherwise routing `#movies` to "Movies" would post nothing for a torrent
filed under "Movies / Action" while the delivery was still recorded as sent.

`category_mode` is `include` (the default, and what an absent value means) or
`exclude`. Exclude is what makes "everything except 18+" expressible without
listing every other category — and it is the reason a listed category stands for
its **whole subtree**: an announcement carries only the category the torrent sits
in, so a filter excluding "Adult" while a torrent lives in "Adult / 4K" would
otherwise let it straight through. The dispatcher resolves the category's
ancestor chain onto `Announcement.CategoryPath` and the filter tests against
that.

If the chain cannot be resolved, only instances filtering in **exclude** mode are
held back — an include filter falling back to the leaf can under-match but never
over-deliver, so withholding it would lose announcements for no safety gain. A
withheld announcement is still written to the delivery log as a **failed** row
carrying the reason, because an announcement that silently never happened is the
one thing that log must not hide. A leak into the feed configured to exclude it
would not be recoverable at all.

### 5.3 Secrets — what they are and how hard we need to protect them

**What "secret" means here.** Some connector configs contain credentials that are
usable *verbatim* by anyone who has them:
- a **Discord webhook URL** — itself a bearer token; whoever has it can post to that
  channel;
- a **Telegram bot token** — full control of the bot;
- an **IRC NickServ/SASL password**;
- a generic-**webhook `Authorization` header** or **HMAC signing secret**.

Unlike user passwords (hashed one-way), these can't be hashed — the connector must
send them at delivery time, so they must be stored *recoverably*. "Recoverable +
sensitive" is the case where storage protection is worth thinking about.

**Why I raised encryption — and whether you actually need it.** The only thing
encryption-at-rest buys you is protection against someone who can **read the DB but
not the app's environment**: a leaked/stolen backup, a SQL-injection that reads the
table, a shared/hosted database, a support engineer with read access. If your DB
and app share one trust boundary (same box, backups stay local and protected — the
typical self-hosted tracker), then **plaintext-in-DB is a perfectly defensible
choice**, and encryption is defense-in-depth you may not want the key-management
hassle for. It is **not** a hard requirement. (Note: `CLAUDE.md`'s "no hardcoded
secrets" rule is about secrets in *code/committed files*, not admin-entered runtime
config — so it doesn't force this either way.)

**The one non-negotiable baseline** (cheap, do it regardless):
- **Never return secrets in API responses** — write-only fields; show "•••• set".
- **Never log** config; redact secrets in the delivery log and errors.

**Then pick a storage posture for your threat model** (decided below and recorded
in §12):
1. **Plaintext in DB** + the baseline above. Simplest; fine when DB and app share a
   trust boundary and backups are protected.
2. **Encrypt secret fields at rest** with an app key from env (envelope-encrypt the
   marked fields). Protects DB dumps / backups / SQLi reads, at the cost of managing
   one key. Recommended *if backups leave the box or the DB is shared/hosted*.
3. **Store references, not values** (`env:DISCORD_MAIN`, a secrets-manager path)
   resolved at delivery. Nothing sensitive in the DB at all; pushes config to
   ops/env. Cleanest for regulated/shared environments.

**Decision (for now): option 1 — plaintext in the DB**, plus the non-negotiable
baseline (never logged, never returned by the API). The operator runs the DB and
app inside one trust boundary with protected backups, so this is fine for now and
avoids key-management overhead. Still **mark** secret fields in the `config` schema
(a per-kind list of which keys are secret) so a later move to encryption (2) or
references (3) is a localized change rather than a feature-wide migration.

## 6. Delivery pipeline

The in-memory event bus dispatches **synchronously**, so the dispatcher must not
call IRC/Discord/webhook inline — it **enqueues**. Use the existing `worker`
package (it already runs digests, cleanup, bonus, backups).

- **Enqueue** one delivery job per (announcement × matching instance).
- **Isolation** — `recover()` around each `Deliver`; one bad connector degrades
  to a logged skip, never a panic or a 500 on approve (mirrors the "recover-and-
  isolate" rule in `docs/EXTENSIBILITY.md`).
- **Retry** with capped exponential backoff for transient failures (5xx, network,
  IRC not-yet-connected); a small max-attempts, then dead-letter to the log.
- **Timeout** per delivery (e.g. 10s) so a hung endpoint can't wedge a worker.
- **Rate-limit / batch** per instance — bulk approvals (staff clearing a queue)
  must not flood IRC/Discord. Optional coalescing ("+12 new torrents") when the
  rate cap is hit.
- **Dedupe / idempotency** — never announce the same (torrent, instance) twice
  (guard on re-published events, retries).
- **Delivery log** table (`connector_deliveries`: instance_id, event ref, status,
  attempts, last_error, timestamps) for admin observability + test-send results.

## 7. Built-in connectors

1. **Chat (internal, built-in).** Posts a system message to the shoutbox via the
   existing `ChatService`/`ChatHub`. This *is* the "publish to general chat" flag —
   modeled as a `chat` connector instance you enable/disable, implementing the same
   interface. Lowest-risk first connector; reuses existing infra.
2. **Generic Webhook.** POST to a configurable URL with configurable method +
   headers; body is the canonical `Announcement` JSON (or a template). Optional
   **HMAC** signature header so receivers verify authenticity. **SSRF guard**:
   admin-supplied URLs must be blocked from internal ranges / cloud metadata
   endpoints (169.254.169.254, RFC1918, localhost) unless explicitly allowlisted.
3. **IRC.** The persistent connector (see §4b). A managed client per instance,
   joins channels, announces one line per publish, routes by category, reconnects
   with backoff. Use a maintained library (per "prefer libraries" convention),
   e.g. `github.com/lrstanley/girc` or `github.com/ergochat/irc-go`.
4. **SSE live feed (internal, authenticated).** `GET /api/v1/announce-stream/{slug}`
   (SSE; the unslugged path predates multiple feeds and resolves to the default
   one) — members enroll and receive published torrents in real time. Reuses the
   WS-hub fan-out pattern (like `ChatHub`); SSE is preferred over WebSocket for a
   one-directional broadcast (per `docs/FUTURE_WORK.md`). This is the seam
   several named feeds are built on: same `Announcement`, one client set per
   feed.
5. **Discord.** Incoming-webhook POST (an embed). Effectively a specialized
   webhook connector with Discord formatting.
6. **Telegram.** Bot API `sendMessage` to configured chat id(s).

## 8. Admin UI

New admin page **Notifications → Connectors** (admin-only):
- List instances (kind, name, enabled, last delivery status).
- Add/edit with a **kind-specific config form** (secrets are write-only).
- Enable/disable toggle.
- **Test send** button (delivers a sample announcement; shows the result).
- Delivery log / recent failures per instance.

## 9. Per-user relay — dropped

This was to let **users** subscribe to their own filtered stream of events
delivered to a channel they own — their own webhook, the SSE feed, or a DM.

**Dropped by operator decision (2026-07-25).** The live feeds cover the need:
several can exist, each with its own filters and its own URL, and a member picks
one. A per-user relay would have meant per-user filters, per-user rate limits and
a per-user delivery log — a second copy of the whole pipeline, aimed at a want
nobody had expressed.

Nothing was built for it, and nothing is left behind: the only concession the
design ever made was keeping `Announcement.Event` a plain string, which is worth
having anyway so later event kinds (forum posts, news) can widen it.

## 10. Security & ops checklist

- Announce **only** on `torrent.published` — never pending/rejected; **respect the
  anonymous flag** (never reveal the uploader when anonymous).
- Secrets: **baseline non-negotiable** — never logged, never returned by the API
  (write-only). Encryption-at-rest / reference-based storage is a threat-model
  choice (§5.3), not a hard requirement.
- Webhook **SSRF** guard (block internal/metadata targets).
- Delivery **isolated** (recover), **retried** (backoff), **timed out**,
  **rate-limited/batched**, **deduped**.
- Delivery **observability** (log table + admin view).
- Persistent connectors are **single-owner across nodes** (§4d); IRC connection
  health surfaced to admins (connected / reconnecting / failing).

## 11. Phased backlog

Sequenced so the pattern earns each step (per `docs/EXTENSIBILITY.md`), not a
framework up front. **Everything below has shipped or been dropped; nothing here
is outstanding.**

- **Phase 1 — the seam + first two connectors. [DONE — BE-10.1]**
  `TorrentPublished` event (emitted from approve + auto-approve upload); the
  `Announcement` payload; the `Connector` interface + compile-time registry; the
  `notification_connectors` table + admin CRUD; the async delivery pipeline
  (isolation/retry/timeout/log). **Chat** (reuses existing infra) and
  **Generic Webhook** (HMAC + SSRF) shipped as the first two instances. FE: admin
  Connectors page + test-send.
- **Phase 2 — IRC. [DONE — BE-10.2]** The `PersistentConnector` shape +
  `ConnectorManager` lifecycle; per-instance channels, category routing,
  reconnect, rate-limit, and advisory-lock leader election (§4d). FE: IRC config
  form + connection status.
- **Phase 3 — SSE live feed. [DONE — BE-10.3]** Authenticated SSE endpoint; hub
  fan-out; graceful reconnect. FE: an opt-in "live releases" view.
- **Phase 4 — Discord + Telegram. [DONE — BE-10.4]** Both are thin
  specializations of the webhook / bot-POST path, and shipped as two packages and
  two registration lines with no change to the dispatcher, repositories, handlers
  or schema.
- **Phase 5 — per-user relay: DROPPED** (§9). The multi-feed live stream covers it.

Follow-ups since, all **[DONE]**, none of them phases:

- **BE-10.6 — connector admin usability.** Nested category pickers, `exclude`
  filter mode (§5.2), several independent chat instances (§5), and per-kind
  template field help.
- **BE-10.7 — several live feeds.** SSE stopped being a singleton: feeds are told
  apart by slug, each with its own filters and URL, and a watcher subscribes to
  exactly one (§5.1).
- **BE-10.8 — `can_feed`.** Feed access as a class privilege, revocable per
  member, enforced at connect and closing open streams on revocation (§5.1).

## 12. Resolved decisions

Every question this section opened has been decided and, where it needed code,
built. Kept as a record of what was chosen and why.

- **Secret storage — plaintext in the DB**, with the baseline redaction (never
  logged, never returned by the API) and secret fields *marked* so encryption or
  references can be added later without a feature-wide migration (§5.3). The
  operator runs DB and app inside one trust boundary.
- **Deployment topology / leader election — Postgres advisory lock**, as the
  leaning predicted, shipped with BE-10.2. Single-process stays the zero-config
  default; a multi-node deployment elects one owner per persistent instance with
  a 10s ownership re-check that stops the IRC client the moment ownership cannot
  be proven. Crash-safe, no TTL to tune. The guarantees and the bounded
  split-brain window it does *not* close are spelled out in §4d.
- **Batching semantics — coalesce, but per-kind, not globally.** The connector's
  `Coalescable()` decides: destinations a person reads (chat, IRC, Discord,
  Telegram, SSE) replace a rate-limited backlog with "+N more"; destinations a
  program reads (webhook) never do, because a summary names nothing a receiver
  can act on — they spend the whole rate budget on individual deliveries and
  defer the rest to the next window.
- **Delivery store retention — `connector_delivery_retention_days`** (seeded at
  30 by migration 073, non-positive disables pruning), swept by the existing
  maintenance-worker pass. It mirrors `announce_log_retention_days` in shape —
  though not yet in fate: that one is still advisory with no job behind it (see
  `docs/FUTURE_WORK.md`).
- **Multi-event scope — generic payload now, more event kinds later**, as the
  leaning predicted. `Announcement.Event` stays a plain string rather than a
  closed enum, so `forum.post` or `news.published` can widen it without a
  redesign; only `torrent.published` is emitted today. This also happens to be
  the only thing the dropped per-user relay (§9) ever asked the design for, and
  it is worth having on its own merits.
