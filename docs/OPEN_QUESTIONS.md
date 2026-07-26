# Architecture Decision Log — TorrentTrader 3.0 Go Port

This document records technology decisions made for the project. All decisions are final unless revisited explicitly.

> **Accuracy pass, 2026-07-26.** Decisions 4 (authentication), 9 (API documentation) and
> 10 (rate limiting) described things the codebase never contained — a JWT, a generated
> spec, and a global rate-limit middleware respectively. Each has been corrected against
> the code, with the original wording and what actually shipped both recorded, so the
> correction is auditable rather than silent. A decision log that disagrees with the source
> is worse than no decision log: it gets cited.

---

## Backend

### 1. HTTP Router/Framework

**Decision:** Chi — lightweight, idiomatic, `net/http` compatible middleware.

### 2. ORM / Query Builder

**Decision:** Raw SQL with pgx driver — no ORM. Repositories write parameterized queries directly. Keeps full control over SQL and avoids ORM magic.

### 3. Database Migration Tool

**Decision:** goose — embeddable, supports Go migration functions, used throughout the project.

### 4. Authentication

**Decision:** Short-lived **opaque** access tokens + Redis-backed sessions for revocation
and persistence. `SessionStore` is an interface (memory for tests, Redis for production).

Access tokens are 64-character hex strings with a 1-hour TTL; refresh tokens are the same
shape with a 30-day TTL and rotate on use. The token carries no claims — every request
resolves it against Redis, which is what makes instant revocation possible.

**Corrected 2026-07-26:** this decision previously read "short-lived **JWT** access
tokens". No JWT was ever implemented — there is no JWT library in `go.mod` and no `jwt`
reference anywhere in the backend. The `JWT_SECRET` environment variable that this wording
produced was read by no code and has been removed from `.env.example`, `stack.env.example`,
`docker-compose.stack.yml` and the README's required-variables table. Opaque-plus-Redis is
the better fit for this project anyway: a stateless JWT cannot be revoked before it expires,
which a tracker with bans and quick-ban tooling actually needs.

### 5. Background Job Processing

**Decision:** Asynq (Redis-based) — persistent queues, retries, scheduler with `ENABLE_SCHEDULER` env toggle, unique task dedup.

### 6. Search

**Decision:** PostgreSQL full-text search — tsvector with prefix matching (`:*` operator). No extra infrastructure needed at current scale.

### 7. File Storage

**Decision:** MinIO (S3-compatible) — self-hosted via Docker Compose in dev, S3-compatible API for production flexibility.

### 8. Real-time Communication

**Decision:** WebSocket — used for chat (shoutbox), real-time private messages, and future notifications. gorilla/websocket with write pump pattern (single writer goroutine per client).

### 9. API Documentation

**Decision:** **Two OpenAPI documents, one hand-maintained source.**

- `backend/api/openapi.yaml` — the **full** spec. Every endpoint, including admin and
  staff routes. This is the source of truth and the file humans edit. The frontend
  generates its TypeScript types from it (`openapi-typescript` → `frontend/src/api/schema.d.ts`),
  which is why it must include the admin surface.
- `backend/api/openapi.public.yaml` — **generated** from the full spec by
  `cmd/openapi-public`, containing only what a member or third-party integrator may call.
  This is the document intended for publication.

Every operation in the full spec carries an `x-audience: public | internal` extension.
The generator drops `internal` operations, prunes the component schemas and security
schemes they alone referenced, and strips the extension from its output. A guard test in
`internal/handler` walks the real Chi router and fails the build when a registered route
is neither documented nor listed in an explicit, shrinking debt ledger — so a new endpoint
cannot be added without being classified.

**Why two documents rather than one.** The web UI is a convenience, not the only client:
members are welcome to drive the API directly or build their own tools against it. The
administrative surface is deliberately not advertised as a supported interface. The project
is open source and those routes are discoverable from the source — "findable in the code"
and "published as a contract" are different commitments, and only the second one implies
stability.

**Corrected 2026-07-26:** this decision previously read "None currently — no OpenAPI spec
generation", while `ARCHITECTURE.md` simultaneously claimed the spec was the source of truth
and that API calls were never hand-written. Both were wrong. A hand-written spec existed and
covered roughly 15% of the ~189 registered routes, and most frontend calls are raw `fetch`.
See `IMPLEMENTATION_TASKS.md` for the story that closes the remaining gap.

### 10. Rate Limiting

**Decision:** Applied per-surface rather than as one global middleware.

- **Login attempts** — capped in `service/auth.go` (5 failures per 15 minutes per IP).
- **WebSocket chat** — per-client limiting (10 messages / 10 seconds) in `handler/chat_ws.go`,
  plus the `chat_mutes` table and anti-spam settings for sustained abuse.
- **Connectors** — per-instance `rate_per_min` with burst coalescing.
- **Live feeds (SSE)** — a per-user concurrent stream cap.

**Corrected 2026-07-26:** this decision previously read "In-memory (`golang.org/x/time/rate`)
— per-instance". No such middleware exists: `internal/middleware/` contains only auth,
activity, CORS and logging, and `golang.org/x/time` is an indirect dependency with no
`rate.Limiter` usage. A general per-IP HTTP limiter remains **unbuilt**; see
`docs/FUTURE_WORK.md`. Anything relying on "requests are rate limited" as an existing
guarantee — notably any anti-abuse proposal — must treat it as work to do, not a given.

### 10a. Time and Timezones

**Decision:** The server is **UTC, always**. There is no site timezone setting and
no per-user timezone preference. Every timestamp is stored as `TIMESTAMPTZ` and
served in UTC; the browser formats it into the reader's local time, which is what
the frontend already does throughout.

Original TorrentTrader had both a site timezone and a per-user timezone. That
parity is **deliberately not ported**. For anything scheduled — a freeleech
window, a contest, a double-bonus weekend — the maintainer decides what "the
weekend" means for their community and configures the window in UTC. On a site
with a global membership there is no correct per-user answer, and making the
schedule depend on a user-controlled field opens an obvious abuse door: a member
could shift their own timezone to enter or extend a window.

Display stays local because that is a pure rendering concern with no accounting
consequence. Scheduling stays UTC because it has one.

---

## Frontend

### 11. State Management

**Decision:** React Context + hooks — `AuthProvider`, `ChatProvider`, `ThemeProvider`, `ToastProvider`. No external state management library.

### 12. CSS Approach

**Decision:** Plain CSS with CSS variables — theme system via `ThemeProvider`, CSS custom properties for theming. No Tailwind or CSS-in-JS.

### 13. Form Handling

**Decision:** Native controlled components — no form library. Simple forms don't justify the dependency.

### 14. UI Component Library

**Decision:** Built from scratch — custom components (`MarkdownRenderer`, `UsernameDisplay`, `ConfirmModal`, etc.). No component library dependency.

### 15. Rich Text Editor

**Decision:** Plain textarea with Markdown — consistent with the rest of the app. No BBCode, no WYSIWYG editor.

---

## Migration Tool

### 16. CLI Framework

**Decision:** Cobra — industry standard, used for the `migration-tool/` CLI.

### 17. Migration Strategy

**Decision:** Table-by-table with transformers — `source/` reads legacy MySQL, `transform/` converts data (BBCode→Markdown, schema mapping), `target/` writes to PostgreSQL.

---

## Infrastructure

### 18. Container Orchestration (Production)

**Decision:** Docker Compose + Portainer — `docker-compose.stack.yml` for single-node deployment. Kubernetes deferred until scaling demands it.

### 19. CI/CD Platform

**Decision:** GitHub Actions — CI runs lint, test, build for both backend and frontend. Release workflow triggered on tags.

### 20. Monitoring / Observability

**Decision:** Deferred — not yet implemented. Structured logging via Go's `slog` is in place.

### 21. Log Aggregation

**Decision:** stdout/stderr — 12-factor app compliant. No external log aggregation yet.

---

## Project

### 22. Licence

**Decision:** **MIT**, copyright "William Okano and the TorrentTrader contributors".

The alternative seriously considered was **AGPL-3.0**, which would have closed the
network loophole: anyone running a *modified* copy as a public service would have had
to offer its source to that service's users. For server software — and a tracker is
nothing but a network service — AGPL is the only copyleft that actually binds, since
plain GPL treats hosting as non-distribution and lets a hosted fork stay private.

MIT was chosen anyway, deliberately trading that protection for reach. The project's
stated purpose is that people run their own tracker for their own community; the
fewer obligations attached to doing that, the more likely they are to. A fork that
disappears behind a closed deployment is an accepted cost, not an oversight.

Practical consequences worth knowing before anyone revisits this:

- **Relicensing later is hard.** Every contributor holds copyright on their work, so
  moving away from MIT would need each of them to agree. Going *from* MIT to
  something stricter is possible for future versions but cannot claw back what has
  already been published.
- **No contributor licence agreement.** `CONTRIBUTING.md` states that opening a pull
  request means accepting MIT for that contribution, which is the lightweight
  convention and is adequate here.
- **No patent grant**, unlike Apache-2.0. Not a practical concern for this project,
  but it is the one thing MIT gives up relative to the other permissive option.

### 23. Multiplier stacking and when multipliers apply

**Decision:** **Additive on a base of 1.0**, applied **at accrual time**.

Every active multiplier contributes a bonus; the bonuses are summed and applied once:

```
effective = 1.0 + reputation_tier_bonus + goal_bonus + event_bonus + …
```

So a member with a +50% rank tier, during a +100% event, having met a +25% goal,
earns 2.75x — not 3.75x, which is what multiplying those three would give.

**Why additive.** The ceiling grows linearly with the number of sources, so adding a
fourth source later raises the top end by a known amount instead of doubling it. It
is also explainable in one line on a member's own page ("+175% right now"), which
matters because a bonus economy nobody can predict is one nobody trusts.

Multiplicative was the alternative. It feels more generous and is the games-industry
norm, but it compounds: three modest modifiers reach 3.75x, and each new source
doubles the top end again. That leads to a cap, and a cap makes a member's rank bonus
silently stop mattering during events, which reads as a bug.

**Why accrual time.** This half was already settled by precedent rather than argument:
`announce_events.counted_downloaded_delta` stores the discounted figure per announce,
so a later configuration change cannot rewrite history. Every multiplier works the
same way — resolved when the award is granted, written to the ledger, never
recomputed on read. Read-time multiplication would contradict the rule that changing
a reward does not re-award the past.

**Not covered by this decision:** freeleech and half-credit are a *discount* on what
is counted, not a multiplier on what is earned, and they keep their existing
"most generous wins" rule (free beats silver). Do not merge the two mechanisms; they
answer different questions and are applied at different points.

**No cap for now.** The ceiling is the sum of whatever sources are configured, which
is knowable by inspection. If one is ever needed it is a setting, not a redesign.

### 24. What member activity may be published

**Decision:** **Reuse the existing per-user privacy levels** (`strong` / `normal` /
`low`) rather than adding a new opt-out. Every surface that names a member declares
which levels it respects.

| Level | Leaderboards and rankings | Followable activity feed | Named in public thanks lists |
|---|---|---|---|
| `strong` | excluded | no | no |
| `normal` | included | yes, limited to entries the activity log already classifies public | yes |
| `low` | included | yes | yes |

Site-wide aggregate numbers — total torrents, active members, library size — name
nobody and are therefore unaffected by every level.

**Why reuse.** The concept already exists, is enforced (strong already hides stats
from non-staff), and members already understand it. A second, parallel opt-out would
mean two settings that both sound like privacy and disagree in edge cases.

**The rule for future surfaces:** anything new that publishes a named member's
behaviour must state which levels it respects, and the default is to respect
`strong`. This is the part that makes the decision durable — it is answered once here
rather than re-litigated per feature.

Rank badges are deliberately **not** hidden by privacy level. A badge is an adornment
on content the member chose to post publicly, not a separate publication of their
behaviour; hiding it would make their own posts look inconsistent to them.

### 25. Access to a resource owned by a team rather than a class

**Decision:** an optional **owning-team reference on the resource**, checked
alongside the existing class-level threshold.

```
may_read = level >= min_group_level
        OR member_of(owner_team)
        OR is_staff
```

Posting uses the same shape against `min_post_level`. The same predicate serves team
forums and, when they exist, team chat rooms — one shared resolver, not two.

**Why not a generic ACL table.** A `(resource_type, resource_id, principal_type,
principal_id)` grant table is the more general answer and would serve anything added
later. It was rejected because generality is the wrong thing to optimise here: every
access check becomes a join against an abstraction, the intent stops being readable
at the call site, and a single bug in the resolver opens *every* resource at once
rather than one. This project has already been bitten by an authorisation check that
failed open when a dependency was missing (see `tasks/lessons.md`), and a single
concrete predicate is far easier to hold to a fail-closed test than a general one.

**Staff always pass.** Deliberate: moderation has to reach team spaces, or a team
forum becomes a place the site cannot police.

**Fail closed.** If the team-membership lookup errors, or the resolver is not wired,
access is denied — never granted. There must be a test that constructs the check with
nothing wired and asserts refusal, matching the `can_feed` precedent.
