# External Notification Connectors — Design

**Status:** design only, not implemented. Stored for later.
**Relates to:** `docs/EXTENSIBILITY.md` (reaction-side plugins), `docs/TRACKER_MODS.md`
("IRC / Discord announce bot"), `docs/FUTURE_WORK.md` (Real-Time Stats Push / SSE).

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
- **sse**: `{}` — the in-process live feed has no external config (auth is by session).

### 5.2 Filters / routing

`filters` narrows which announcements an instance receives:
`{ event_types:["torrent.published"], category_ids:[...], min_size, freeleech_only, exclude_anonymous }`.
"One or more" targets (e.g. IRC channels per category) live inside the instance
config (`channels[].categories`) so a single connection can route by category.

### 5.3 Secrets

Config JSON holds secrets (webhook tokens, IRC/SASL passwords, bot tokens).
Requirements (per `CLAUDE.md` security rules — no plaintext secrets, no secret
logging):
- **Encrypt secret fields at rest** with a key from env (envelope-encrypt the
  marked fields, or the whole `config` blob), **or** store 1Password/`env:` refs
  resolved at delivery time.
- **Never** return secrets in API responses (write-only fields; show "•••• set").
- **Never** log config; redact on the delivery log.

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
4. **SSE live feed (internal, authenticated).** `GET /api/v1/announce-stream`
   (SSE) — members enroll and receive published torrents in real time. Reuses the
   WS-hub fan-out pattern (like `ChatHub`); SSE is preferred over WebSocket for a
   one-directional broadcast (per `docs/FUTURE_WORK.md`). This is the natural seam
   for the future per-user relay (§9): same `Announcement`, filtered per subscriber.
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

## 9. Future: per-user relay (explicitly deferred)

Once the connector + `Announcement` + SSE feed exist, let **users** subscribe to
their *own* filtered stream of events (categories, uploaders, metadata, "my
torrents") delivered to a channel they own — their own webhook, the SSE feed, or a
DM. It reuses the exact same `Announcement` and dispatcher, scoped per-user with
per-user filters and per-user rate limits. Out of scope for the first pass; IRC and
chat are the priority.

## 10. Security & ops checklist

- Announce **only** on `torrent.published` — never pending/rejected; **respect the
  anonymous flag** (never reveal the uploader when anonymous).
- Secrets encrypted at rest, never logged, never returned by the API.
- Webhook **SSRF** guard (block internal/metadata targets).
- Delivery **isolated** (recover), **retried** (backoff), **timed out**,
  **rate-limited/batched**, **deduped**.
- Delivery **observability** (log table + admin view).
- IRC connection health surfaced to admins (connected / reconnecting / failing).

## 11. Phased backlog

Sequenced so the pattern earns each step (per `docs/EXTENSIBILITY.md`), not a
framework up front:

- **Phase 1 — the seam + first two connectors.**
  `TorrentPublished` event (emitted from approve + auto-approve upload); the
  `Announcement` payload; the `Connector` interface + compile-time registry; the
  `notification_connectors` table + admin CRUD; the async delivery pipeline
  (isolation/retry/timeout/log). Ship **Chat** (reuses existing infra) and
  **Generic Webhook** (HMAC + SSRF) as the first two instances. FE: admin
  Connectors page + test-send.
- **Phase 2 — IRC.** The `PersistentConnector` shape + `ConnectorManager`
  lifecycle; per-instance channels, category routing, reconnect, rate-limit. FE:
  IRC config form + connection status.
- **Phase 3 — SSE live feed.** Authenticated `GET /api/v1/announce-stream`; hub
  fan-out; graceful reconnect/fallback. FE: an opt-in "live releases" view.
- **Phase 4 — Discord + Telegram.** Both are thin specializations of the webhook /
  bot-POST path once Phase 1 exists.
- **Phase 5 (future) — per-user relay.** Per-user subscriptions + filters over the
  same `Announcement`/SSE machinery (§9).

## 12. Open questions

- **Encryption vs references for secrets** — envelope-encrypt config fields with an
  env key, or store `env:`/1Password references? (Leaning: encrypt-at-rest with an
  env key so config is self-contained; revisit if a secrets manager lands.)
- **Batching semantics** — per-instance rate cap with coalescing ("+N more"), or
  hard drop past the cap? (Leaning: coalesce.)
- **Delivery store retention** — how long to keep `connector_deliveries`; prune via
  the maintenance worker (mirror `announce_log_retention_days`).
- **Multi-event scope** — do we widen beyond `torrent.published` in Phase 1
  (forum/news), or keep the payload generic and add event kinds later? (Leaning:
  generic payload now, more event kinds later.)
