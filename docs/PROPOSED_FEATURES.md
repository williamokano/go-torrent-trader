# Proposed Features

> ## ⚠️ Frozen — new proposals go to GitHub Issues
>
> **As of 2026-07-26, GitHub Issues is the source of truth for work**, including
> proposals. Do not add a PF item here. Open an issue, and use the analysis below as
> the model for what a good one contains: what it is in plain language, what already
> exists that it can reuse, what it depends on, and the questions still open.
>
> **The drain is finished** — every PF item below now exists as a GitHub issue, and
> the issues are the live version. This document stays only as **the reasoning behind
> the ideas**: the verification pass against the codebase, the dependency graph, and
> the shared machinery several items assume are worth keeping in one place rather
> than scattered across issue comments. See `CLAUDE.md` for the full rule.

A staging area for features that have been **proposed but not specified**. Nothing
here is committed work.

`IMPLEMENTATION_TASKS.md` is the living backlog: everything in it is either done
or specified well enough to build. This file is upstream of that. An item moves
out of here and into the backlog as a real story once its open questions are
answered — at which point delete it here rather than leaving two copies.

Open questions are recorded **as questions**, not resolved into an assumption.
Where an item overlaps something already built, that is called out — and as of
this revision, every such claim has been **verified against the code** (schema
through migration 078, release v0.24.0, 2026-07-26). Claims that turned out to
be wrong have been corrected in place; the significant ones are listed in
[What verification found](#what-verification-found) so the corrections are
visible rather than silent.

Numbering (`PF-n`) is stable so items can be referenced from discussion. PF-1 to
PF-15 and PF-26 to PF-28 came from the operator; PF-16 to PF-25 were raised in
review; PF-29 to PF-33 are shared machinery surfaced by the verification pass —
pieces two or more proposals were each about to build independently. The later
operator items sit in Part B because they arrived later, not because they matter
less — PF-26 is flagged as likely next.

---

## Reading order

Three sections matter more than the individual items:

1. **[Decisions this document reopens](#decisions-this-document-reopens)** —
   several proposals reverse recorded decisions. **Reputation (PF-3) and teams
   (PF-2) were approved on 2026-07-26**, which unblocks about a third of this
   document; three smaller reversals are still open and need sign-off before
   anything downstream of them is specified.
2. **[Part C — shared machinery](#part-c--shared-machinery)** — five pieces
   (escrow, torrent edit history, polymorphic reports, file uploads, announce-log
   lifecycle) that multiple items silently assume. Naming them prevents building
   them twice.
3. **[Dependencies and build order](#dependencies-and-build-order)** — six items
   depend on the reputation ledger, two on escrow, and the DAG has real edges the
   first draft missed (the CLI wants API tokens; the designer system wants file
   uploads; adopt-a-torrent wants an edit history that does not exist).

---

## What verification found

The first draft of this document was checked against the codebase and the rest
of `docs/`. Corrections are folded into each item below. The findings were
**actionable independently of any proposal**, and a documentation alignment pass
on 2026-07-26 closed most of them:

1. **The reseed endpoint is unguarded — STILL OPEN, and it is a code fix, not a
   doc fix.** `POST /api/v1/torrents/{id}/reseed` performs no seeder check, no
   banned check and no visibility check server-side; the "0 seeders only" rule
   lives entirely in the frontend (`TorrentDetailPage.tsx`). A direct POST
   succeeds against any torrent, and each one emails that torrent's uploader.
   This is the highest-priority item on this page and it does not depend on any
   proposal being accepted.
2. **`BE-3.9: Reseed Request` was marked `[DONE]` with unshipped acceptance
   criteria** — FIXED in the backlog. The 24-hour rate limit (actual behaviour:
   one request per user per torrent *forever*, via a UNIQUE constraint), the PM
   fan-out to completers, and the background job were never implemented; only
   the uploader email listener exists. The story now says so and carries the
   gap from item 1 as a visible note.
3. **`BE-8.3` was `[DONE]` with unchecked boxes** — FIXED. Six of its eight
   boxes were unchecked, not two: torrent search by name/info_hash, ban/unban,
   toggle freeleech, resolve-with-action, bulk actions, and the freeleech/banned
   listing. PF-26 absorbs the freeleech-related ones.
4. **`ARCHITECTURE.md` contradicted decision 9 on OpenAPI** — RESOLVED, and the
   resolution went further than a wording fix: the project now maintains **two**
   specs (full and public), which is what unblocks PF-27. See decision 9.
5. **Three shipped stories promised a per-user timezone** (`BE-1.4`, `FE-2.1`
   ACs; `MT-1.1` migration mapping), contradicting decision 10a — FIXED, and
   `NOT_PORTING.md` gained the missing entry recording that this parity was
   dropped deliberately.
6. **Documentation described three things that do not exist** — all FIXED, and
   worth knowing because proposals kept leaning on them:
   - **No JWT.** Decision 4 said "short-lived JWT access tokens"; there is no JWT
     library and no `jwt` reference in the backend. Tokens are opaque 64-char hex
     resolved against Redis. The `JWT_SECRET` variable that this wording spawned
     was documented as **required for deployment** and read by nothing — it has
     been removed from the README, both env examples and the Portainer stack.
   - **No rate-limiting middleware.** Decision 10 claimed `golang.org/x/time/rate`
     per-instance limiting. It does not exist. Limiting is per-surface only
     (login attempts, WebSocket messages, connector sends, SSE stream count).
     **PF-20 must treat a general limiter as work to build, not a foundation to
     build on.**
   - **No `sqlc`.** Referenced by two docs, used nowhere, and contradicting
     decision 2 (raw SQL + pgx).
7. **Small factual drift**, corrected throughout: the event bus has **64** types
   (plus 2 activity-log-only strings — the "66" counted both); `can_chat` is a
   **per-user restriction column, not a group capability** (the group set is
   `can_upload, can_download, can_invite, can_comment, can_forum, can_feed,
   can_self_approve` plus `is_admin/is_moderator/is_immune`); and the
   `bonus_transactions` ledger's actual columns are
   `delta / reason / ref_id / created_at`, which is the shape PF-3 should copy.

---

## Decisions this document reopens

Several proposals contradict recorded decisions. That is allowed — decisions are
revisable — but it must be **explicit**: each needs an operator yes/no before its
dependents are specified, and a matching edit to the source document so the two
never disagree silently.

**Two are now decided.**

| Proposal | Contradicted | Outcome |
|---|---|---|
| **PF-3 Reputation** | `NOT_PORTING.md` §14 | ✅ **Approved, 2026-07-26.** Karma was cut to reach a working tracker; the port is now stable, so scope is open again. §14 moved to "Reinstated". Six proposals unblocked. |
| **PF-2 Teams** | `NOT_PORTING.md` §4 | ✅ **Approved, 2026-07-26.** Same reasoning. §4 moved to "Reinstated". Unblocks PF-6 team rooms and PF-14 team colours. |
| **PF-22 Collections** | `NOT_PORTING.md` §15 | Open, but soft — bookmarks were dropped for MVP and already sit under "May Be Added Later". Given §4 and §14 went the way they did, this one is a formality. |
| **PF-25 Public health page** | `BE-9.22 / FE-4.4` (shipped) | **Still open, and the one to be careful with.** Site stats were deliberately moved *behind* auth: membership and activity figures are not put in front of anonymous visitors. Unlike Teams and Karma this was not an MVP scope cut — it is a standing privacy posture. |
| **PF-16 Subscriptions** | `BE-10.5` (dropped 2026-07-25) | Open. Per-user relay was dropped as "a want nobody had expressed." PF-16 *is* that want, now expressed — new evidence, but the reversal should be conscious. |

Recommendation on the two that remain: PF-25 has a **members-only** variant that
contradicts nothing and captures most of the community value — ship that and
treat "public" as a separate, later decision, because it is a different kind of
question from the two just approved. PF-16 differs from BE-10.5 in a way worth
stating in its spec: it routes matches into the **notification system** (which
exists, with batching and digests) rather than a second delivery pipeline, which
is precisely the part of BE-10.5 that made it not worth building.

---

# Part A — Operator proposals

## PF-1: Designer system (commissioned artwork)

Users place "orders" for artwork — typically an avatar and/or a banner/signature,
together called a **kit**. A designer claims an order and delivers it. Payment is
held in escrow until the requester marks the order fulfilled.

**Mechanics as described:**
- Designers are identified somehow — a tag, a role, a group capability. **TBD.**
- Orders may be free or priced in bonus points, or something else. **TBD.**
- A designer *claims* an order; claiming is exclusive.
- Funds move into a **trust account** at order time, so the requester cannot spend
  the balance before the artist is paid.
- The requester may **cancel at any time**.
- The requester clicks **Fulfilled** to release payment.
- If the designer delivered and the requester never clicks Fulfilled, payment
  **auto-releases after a configurable number of days**.
- The designer gets a **Report** button raising a staff-visible report, so staff
  can act against the requester and make the artist whole.
- The whole feature is admin-disableable.

**Open questions:**
- Is "designer" a group capability or an independent flag any class can carry?
  Precedent cuts both ways: `can_feed` (migration 076) is a group capability
  *with* a per-user override column, and `can_self_approve` (070) is a
  capability deliberately **hidden from the group-editor UI** and granted by
  moving a user into a seeded class. A "designer" per-user flag following the
  `can_self_approve` pattern is the smallest change.
- What currency? Bonus points is the only balance that exists today.
- Can an order be cancelled *after* a designer has claimed and started work?
  "Cancel at any time" and "protect the artist" are in tension — this is the
  main fairness decision in the feature.
- Does cancellation after delivery refund, or does it force the report path?
- Are delivered files hosted by the site or linked externally? **This is a bigger
  fork than it looks** — see the correction below.

**Reuses (verified):** `bonus_transactions` ledger + `users.bonus_points`
(migration 054) for the currency; the report-queue *shape* from torrent reports.

**Corrections from verification:**
- **"MinIO is already wired" overstated the reuse.** The storage abstraction
  exists (`internal/storage/`, S3 + local backends), but its only consumer is
  server-side `.torrent` bytes. There is **no user-facing upload endpoint
  anywhere** — no multipart handler, no size/type validation, no serving route,
  no quota. Avatars today are *external URLs* (`users.avatar`, validated as
  http/https). Hosting delivered kits means building PF-32 first — or shipping
  v1 with external links only, which sidesteps it entirely.
- **Reports are torrent-only.** `reports` has a `torrent_id` column, not a
  polymorphic target. The designer Report button needs PF-31 (or its own queue).

**Escrow:** the held-funds/claim/fulfil/timeout/dispute machinery is PF-29,
shared with PF-17 (request bounties). Build it once — and note PF-17 is the
better first consumer, because it needs no file handling at all.

## PF-2: Team system (release groups)

A team is a group of people who release together. Teams get a public page so users
can find releases by the group they trust.

**Approved 2026-07-26.** This reversed `NOT_PORTING.md` §4, which has moved to
that document's "Reinstated" section. Teams were an MVP scope cut, not a
permanent judgement, and the port has reached a stable tracker.

**Mechanics as described:**
- Team page shows members, release count, and a team leader.
- The leader can invite and remove members.
- Invitations must be **accepted or denied** by the invitee.
- Teams get their **own private forum**, visible only to team members and staff,
  **independent of class level**.
- Team admins/moderators get scoped moderation powers over that forum only:
  create topics, lock, pin — the same verbs staff have site-wide.
- Public team page is **Markdown**, editable by the team, organised as **tabs**:
  `SITE_URL/team/{team_slug}/{page_slug}`.
- Tabs load together and switch without a reload, but each must be **deep
  linkable**.
- Page count per team is capped by an admin setting.
- A team defines a **default page**, served at `/team/{team_slug}`.
- Admin-disableable.

**What original TorrentTrader did (§12.1), for calibration:** teams were
**admin-created** (name, owner, description, logo URL), a user joined via
profile settings, membership was a **single FK** (`users.team`), and — verbatim
from the spec — teams were *"social/organizational, no permission
implications."* This proposal inverts that last property deliberately (private
forum, scoped moderation). The spec should state that inversion rather than
inherit it silently, because it is where all the cost lives.

**Open questions:**
- Can a user belong to several teams? Original TT says no (single FK), and
  everything downstream (colour, badge, chat) is simpler if that parity holds.
  If multi-team is wanted, say so explicitly — it changes every dependent's shape.
- Is the leader transferable? What happens to a team whose leader is banned or
  goes inactive?
- Are releases attributed to a team by the uploader choosing one, or inferred?
- Does removing a member retroactively affect release attribution?
- Are teams admin-created (parity) or user-created with staff approval? The
  badge-approval flow in PF-15 suggests the latter is wanted; it needs saying.

**Reuses (verified):** slug-addressed resources (live feed slugs, migration 075,
with the `COALESCE(NULLIF(...))` unique-index pattern); `MarkdownRenderer` +
`MarkdownEditor`; forum tables.

**Watch out — confirmed against the schema:** forum access is *purely numeric*:
`forums.min_group_level` (read) and `forums.min_post_level` (post), compared
against `perms.Level` at every check site in `internal/service/forum.go`. A
forum visible to "members of team X and all staff, regardless of class" is a
**new authorisation axis** — a membership predicate where only a threshold
exists today. PF-6's per-room chat authorisation is the *same* new axis. Decide
the membership-based-access model **once**, for both, before either is built —
and per the fail-closed lesson in `tasks/lessons.md`, it must fail closed at
every layer that decides, with a nothing-wired test.

## PF-3: Reputation system (gamification)

Distinct from bonus points: reputation is earned by participating, not by seeding
economics, and is displayed as a rank badge.

**Approved 2026-07-26.** This reversed `NOT_PORTING.md` §14, which has moved to
that document's "Reinstated" section. Six items hang off this one, so it is now
the natural first build after PF-26 — and the note there is worth carrying: the
original "ratio is already the reputation metric" rationale does not actually
conflict with this proposal, because ratio measures seeding economics while this
measures participation.

**Mechanics as described:**
- Staff define the tiers in admin: names, point thresholds, optionally rules.
- Many actions grant reputation. Named so far, explicitly "not limited to":
  being online (x points/hour), commenting, uploading torrents, **moderating
  torrents (staff earn reputation too)**, forum posts, forum replies, artwork
  via PF-1, seeding, shoutbox activity, thanking a torrent (PF-4).
- Ranks work like an **Elo** — all tiers and their thresholds are admin-configured.
- **TBD:** an Apex Legends / GunBound style top tier, where only the top *N* users
  hold the rank even if others have the points.
- Every award is written to a **ledger row with a date**, *and* materialised on the
  user row. Both, deliberately: the ledger keeps seasons and breakdowns possible
  later; the materialised column keeps queries fast.
- Admin-disableable.
- When implementing, leave a comment planning further point sources to investigate.

**Templates to copy (verified — this is better news than the first draft knew):**
- **Ledger:** `bonus_transactions` is exactly the shape, and its actual columns
  are `delta` (signed), `reason`, `ref_id`, `created_at` with an
  `(user_id, created_at DESC)` index. Mirror that — signed delta covers PF-5
  downvotes for free.
- **Sources:** `BonusService.AddSource` is an explicit plug-in point for award
  sources, run by an hourly asynq task with `asynq.Unique` dedup. A
  `ReputationSource` interface of the same shape gets scheduling, dedup and the
  master-switch pattern for free.
- **Tiers-with-thresholds:** `promotion_rules` + `promotion_runs` (migrations
  053, BE-8.13) is *already* an admin-configured ladder with per-tier
  thresholds, an audit trail of every engine run, and a service-level refusal
  to attach rules to staff classes. **PF-3's tier admin duplicates its shape** —
  copy it, don't invent a second rules idiom.

**A relation the first draft missed entirely:** the site already *has* an
automatic ladder — BE-8.13 promotes and demotes **class** on upload/ratio
thresholds. Reputation tiers would be a **second, parallel ladder** rendered
next to the same username. The spec must define how they relate (answer is
probably "completely independent: class = privileges, reputation = cosmetics"),
because members will immediately ask why they rank high on one and low on the
other, and because PF-12 wants reputation to feed the *economy*, which stops it
being purely cosmetic.

**Open questions:**
- Does reputation ever decay, or only accumulate? This decides whether "seasons"
  are cosmetic or structural.
- Can reputation go negative (PF-5 downvotes suggest it can)? Floor at zero?
- For a capped top tier, what happens when someone is displaced — silent demotion,
  or a notification?
- How do high-frequency sources award? The event bus is **synchronous and
  in-memory** (`internal/event/bus.go`) — a listener that writes a ledger row
  per shout/announce adds latency to the publishing path. Batch through the
  hourly source job (like seeding bonus) rather than per-event listeners for
  anything high-volume.

**Design risk worth stating plainly:** *"being online, x points per hour"*
rewards leaving a tab open rather than participating, and is the easiest signal
on the list to automate. Worse, there is no presence infrastructure to hang it
on — rate limiting is per-instance in-memory (decision 10), and "online" would
have to be defined from announces or WS connections, both trivially farmable.
If it ships at all, it wants the anti-abuse spine from PF-20, a daily cap, and
the lowest rate of any source. Every other source on the list is an action
someone actually took.

## PF-4: Torrent thanks

A Thank button on a torrent, a list of who thanked, and the button disabled — or
switched to Unthank/Remove — once used. Admin-disableable.

**Open questions:**
- Is the thanks list public to everyone, or only to the uploader? (Interacts
  with the site's existing privacy levels — see the privacy note in
  cross-cutting concerns.)
- Does un-thanking revoke the reputation it granted? (Consistency with PF-5's
  vote-removal answer matters — decide once for both.)

**Confirmed greenfield:** nothing thanks-shaped exists in any migration or Go
file. The nearest template is the torrent **rating** vote (BE-3.7): one row per
`(torrent, user)` with a UNIQUE constraint — thanks is the same uniqueness shape
minus the score. Small, and the right first consumer of the reputation ledger.

## PF-5: Forum voting

Upvote and downvote on both topics and posts. Points only, for now. Votes feed
reputation the way Reddit karma does, and a downvote may remove reputation.

**Anti-abuse is part of the ask:** users who serially downvote to drain someone's
reputation should be flagged, and be warnable/bannable/markable as cheaters.

Admin-disableable.

**Open questions:**
- Is the vote count public per post, and is *who voted* ever visible?
- Does a downvote cost the voter anything? (The cheapest brake on brigading.)
- Should downvoting be gated behind a class or reputation floor?

**Reuses (verified):** the flagging genuinely has somewhere to go —
`cheat_flags` (migration 042) has a free-form `flag_type`, a `details` JSONB, a
nullable torrent reference, a dismissal workflow, a cooldown index and a staff
dashboard. A `vote_abuse` flag type slots in without schema change, which is
exactly the PF-20 pattern. Warnings and restrictions are the enforcement outlets.
Note: forum posts have no voting/karma columns anywhere — fully greenfield —
and per-post vote *counts* will want materialised columns beside the vote table,
same ledger+column discipline as PF-3.

## PF-6: Chat rooms, and closing the shoutbox

Two parts:

1. **A button to hide/close the global shoutbox.** Sometimes you just don't want
   to see it.
2. **Multiple chat rooms:** team chat (PF-2), staff chat (admins + moderators),
   admin chat, and a VIP chat.

Noted explicitly: some communities will prefer Discord, so this should not be
assumed to be the primary channel.

**Part 1 is ~80% built already.** `Chat.tsx` ships collapsed by default with a
header toggle; a `mainChatVisible` mechanism already suppresses the floating
widget when a page-embedded shoutbox is visible. The only missing piece is
*persistence* of the choice — and original TT answers the open question: it
stored `hideshoutbox` **on the account** (§1.2, editable in settings §4.7).
Parity says account setting, not per-device. This slice is nearly free and does
not need to wait for rooms.

**Part 2 remains the largest item on the list.** Verified: `chat_messages` has
no room/channel column (only migrations 009 and 072 touch it — 072 added
`system` messages and nullable author); the `ChatHub` keeps one client set and
one broadcast channel that loops over every client. Rooms mean a schema change,
per-room authorisation on both history and the socket, and a hub that routes
instead of broadcasts. Note the *announce* SSE hub already keys clients per feed
slug — that is the in-repo precedent for a routing hub.

**It also touches the shoutbox connector.** Verified: the chat connector is
`Singleton() false` since migration 074 — several instances already exist, each
with its own template and filters, posting into the one stream. Once rooms
exist, each instance's config needs a target room. The connector also posts via
`SendSystemMessage`, which **deliberately bypasses mute and `can_chat`** — rooms
must preserve that property for system posts.

**Open questions:**
- ~~Is the hide-shoutbox preference per-device or stored on the account?~~
  **Answered by parity: account** (see above). Confirm or override.
- Is room membership derived (staff = staff class, team = team members) or
  explicitly managed? Derived is far less to maintain — and note the **VIP class
  already exists** (seeded group, level 60), so "VIP chat" derives cleanly.
- Is history per room retained on the same schedule?
- Do rooms need their own mute state, or is a chat mute global? **Today mute is
  global** (`chat_mutes`, migration 029) with two callers — staff `MuteUser` and
  anti-spam `SystemMuteUser`. Keeping it global preserves both; per-room mute
  means teaching the anti-spam path about rooms.
- Team-room authorisation is the same membership axis as PF-2's team forum —
  specify that model once for both (see PF-2).

## PF-7: Real-time page updates

Pages should update live rather than only notifying. The torrent detail page is
the worked example: seeder/leecher counts, an arriving moderation message,
description or metadata edits, new comments, new thanks — all should appear while
the user is still on the page.

Explicitly noted: subscribing a page to *all* events and filtering client-side is
the wrong shape. The page should subscribe to the events it cares about.

**The transport question already has a recorded answer.** `BE-9.4` (real-time
stats) was deferred to `FUTURE_WORK.md`, which states: *one unidirectional SSE
stream; no WebSocket — a full duplex connection and a second hub aren't
warranted.* PF-7 should either adopt that (SSE, alongside the existing
`AnnounceHub` which already does per-topic client sets) or explicitly argue
against it — not rediscover the question.

**Open questions:**
- Do live updates require the same authorisation as the page itself? (Yes, and
  that is the main correctness risk: a live feed must not leak a field the page
  would have hidden. Fail-closed, per the `can_feed` lesson.)
- What is the update granularity — a nudge to refetch, or the changed payload?
  A nudge is far easier to keep consistent with page-level authorisation, and it
  sidesteps the `tasks/lessons.md` trap that a frontend fixture is not evidence
  the backend sends a field.
- The internal bus is **synchronous** — fan-out to page subscribers must not add
  latency to the publishing service. `CheatDetectionService.CheckAnnounce`
  already demonstrates the escape hatch (goroutine on `context.Background()`)
  and its trade-offs (no cancellation, no backpressure). A real answer probably
  looks like a buffered dispatch layer, decided once for PF-7 and PF-20.

**Reuses:** 64 event types are already published on the internal bus; the SSE
announce hub and the chat hub are both live transports.

## PF-8: Reseed requests — *less built than previously claimed*

The first draft said this was "mostly already built." Verification says
otherwise — see [What verification found](#what-verification-found). What
actually exists: the table (migration 016, `UNIQUE(torrent_id, requester_id)`),
the endpoint, the button, and **one email to the uploader** via a synchronous
listener. What the DONE story promised but never shipped: the 24-hour rate limit
(reality: one request per user per torrent, forever), the PM fan-out to everyone
who completed the torrent, and the background job. And the 0-seeder eligibility
check is **frontend-only** — the endpoint accepts a POST for any torrent.

**So PF-8 is now three layers, in order:**
1. **Fix the live gap** (do this regardless of the rest): enforce eligibility
   server-side, and reconcile `BE-3.9`'s status with reality.
2. **Finish BE-3.9 as specified:** time-based rate limit (needs a `created_at`
   check or replacing the permanent UNIQUE), PM fan-out via a queued job — PMs
   already generate notifications, so the "message them, don't notify about the
   request" requirement holds once the PMs actually exist.
3. **The genuinely new enhancements from the proposal:**
   - An **admin kill switch** for the feature.
   - **Configurable eligibility rules** — minimum transfer speed, minimum seeder
     count, where `0` means "always allowed". (Original TT allowed requests at
     `seeders <= 1`, not 0 — worth considering as the default.)
   - **Staff bypass:** staff can request a reseed from everyone who downloaded
     and is not currently seeding, regardless of those rules.

Eligibility rules should read PF-9's dead/health definition rather than invent
their own thresholds.

## PF-9: Dead torrent detection

Surface torrents with no seeders whose last seed was more than X ago, ideally with
leecher counts, so staff can see the health of the library and act.

**More exists than the first draft noted:** browse and search already take an
`?alive=` filter, and the "Need seed" view (torrents with 0 seeders) already
ships (FE-1.5). What is missing is the *time* dimension — "last seed was more
than X ago" — and a definition stable enough to hang policy on. Candidate
sources for "last seeded at": `transfer_history.last_announce` (one row per
user×torrent, overwritten in place), the `peers` table (current state only), or
`announce_events` (true history, but see PF-33 for its unbounded growth).

**Open questions:**
- Is "dead" a stored, recomputed status or a query? A stored status can be indexed
  and reported on; a query can never be stale. (A nightly asynq job writing a
  `last_seeded_at` / `dead_since` onto `torrents` is the middle path: cheap to
  query, at most a day stale, and honest about being derived.)
- Does a dead torrent become visually marked to members, or is this staff-only?

**Feeds:** PF-8's eligibility rules, PF-10's abandonment definition, and PF-25's
dead-ratio stat all need this. Build it once, as one definition.

## PF-10: Adopt a torrent

Dead torrents lose their owners — the uploader stops logging in, or leaves. Staff
must be able to reassign ownership. Members may be able to claim abandoned
torrents to gain edit rights over them.

**Explicit requirements:**
- **Staff reassignment is mandatory** — it is the safe path and always available.
- Member self-adoption is **admin-disableable**, because it is dangerous.
- History is preserved: the original uploader is **honoured, never dismissed** —
  "uploaded by X, now owned by Y".
- Adoption may grant reputation.

**Correction from verification — the provenance trail has nowhere to live.**
The first draft claimed "torrent edit history already exists." **It does not.**
Editing a torrent emits a transient event that becomes one prose line in the
activity log; there are no before/after values and no per-field rows. What does
exist, as shape templates, is `user_edit_history` (migration 057 — including the
`changed_by_username` snapshot pattern for when the actor is later deleted) and
`forum_post_edits` (048/062). Torrent edit history is now PF-30, and PF-10
depends on it — "uploaded by X, now owned by Y" is exactly an edit-history row
over the owner field.

**Open questions:**
- What exactly defines "abandoned"? Owner inactivity, torrent deadness, or both?
  (See PF-9.)
- Does adoption transfer the upload credit and any bonus tied to it, or only the
  edit rights?
- Can an adopted torrent be reclaimed if the original uploader returns?

## PF-11: Reputation rewards are documented and configurable

Every reputation source is documented and admin-configurable. **Changing a reward
does not apply retroactively**, even though every award is retained in the ledger.

**The precedent is verified and strong:** the connector admin page derives its
template-field help from the backend by reflection
(`internal/connector/templatefields.go`), with guard tests in *both* directions —
one failing the build when a field lacks a description, one failing when a
description outlives its field, plus one asserting examples come from the real
renderer. That is the pattern: a hand-maintained list of reputation sources in
the frontend would drift silently, and the failure mode is an admin configuring
a source that no longer fires.

**Verification found the same drift risk already live in settings, which PF-11
should absorb:** the `site_settings` validation switch covers only **8 of 38**
keys — `bonus_enabled`, `cheat_detection_enabled`, `promotion_enabled` and every
other boolean accept arbitrary strings and are coerced at read time — and the
frontend `SETTING_DEFINITIONS` list is hand-maintained with a bare-text-input
fallback for unknown keys. A features/settings registry with a reflection-style
guard (every setting constant has a definition, every definition has a constant)
fixes the existing gap and gives PF-3/PF-19/PF-26 their config surface. That is
the natural scope of PF-11: not just reputation rewards, but *the* documented
registry all these toggles land in.

## PF-12: Bonus × reputation multipliers

Reputation tiers carry a configurable multiplier applied when seeding bonus is
awarded.

**What exists (verified):** the bonus award path is a snapshot job
(`SeedingBonusSource`, hourly), so a multiplier applies naturally at award time.
The store already seeds a `double_upload` item, disabled, and `FUTURE_WORK.md`
names its missing pieces (per-user expiry, announce-time delta multiplication) —
a second multiplier consumer of the same pipeline.

**Open questions:**
- Do multipliers stack with PF-13's goal multipliers and PF-19's event modifiers?
  Additive or multiplicative? This needs a single answer covering every
  multiplier source, decided once, or the economy becomes impossible to reason
  about. (See cross-cutting concerns — this is *the* economy decision.)
- Is the multiplier applied at award time, or at read time? Award time is honest
  with the ledger; read time is retroactive and contradicts PF-11. (PF-26's
  `counted_downloaded_delta` precedent says award time — the discount is fixed
  when granted and history is never rewritten.)

## PF-13: Site and user goals

Configurable goals — torrents uploaded, data **currently** seeding, user maximum
and average speed, and so on — shown on a page where a user can see where they
stand. Meeting goals grants extra seeding-bonus multipliers.

**The averaging problem, as raised:** a user seeding with no leechers online has
no upload through no fault of their own. The proposal is to count only intervals
where the user had a non-zero transfer delta in either direction. That is the
right instinct; it needs a precise definition before it can be built.

**Verified data reality:**
- **No speed metric is stored anywhere.** The cheat detector computes speed
  transiently (delta ÷ time since last announce, discarded unless a flag fires).
  Max/average speed must be *derived* from `announce_events` deltas — possible,
  the right table, but a new aggregation, not a read.
- The non-zero-delta interval idea maps cleanly onto `announce_events` rows, and
  **BE-8.13 already solved the adjacent problem**: seed-hours estimated by
  gap-summing announce events *with a per-gap cap* so offline stretches aren't
  credited. Copy that definition style.
- `announce_events` grows unbounded (retention setting is advisory — no pruning
  job exists) and carries two indexes. Goal queries over date ranges need PF-33
  first, or they will be the query that discovers the problem.
- Goal *rules* are another admin-configured rule table — same `promotion_rules`
  shape as PF-3's tiers and PF-26's freeleech rules.

Also noted: these metrics help identify cheaters.

**Open questions:**
- Are goals per-user, site-wide, or both? The title says both.
- Are goals seasonal or lifetime?
- Does losing a goal (dropping below the seeding threshold) remove the multiplier
  immediately?

## PF-14: Team colours

Each team gets a staff-assigned colour, and the frontend renders the member's name
in that colour wherever it is drawn — the way admin, VIP and moderator names are
already coloured. Designers too.

## PF-15: Badges

Badges render next to a username, like a verified tick: admin, staff, per-group,
per-team. Staff-configurable, and a team's badge must be **staff-approved**.
Approval authority is itself configurable (which class may approve).

**PF-14 and PF-15 are one system.** Both answer "what is drawn next to a username,
and who decides". They share the same render path, the same approval flow, and
the same caching concern. Specifying them separately will produce two overlapping
implementations.

**The render path already exists (verified):** groups carry a `color` column
with a picker in group management; `BE-8.20 / FE-5.17` routed every username on
the site through one resolved display path (`UsernameDisplay`); the shared
component library already has a `Badge` component; chat already renders role
badges. The adornment system is therefore mostly a *data* question — what does
the username-resolution payload carry (group colour, team colour, badge set),
computed where, cached how — plus the approval flow. Per `tasks/lessons.md`:
assert the new fields on the **backend serializer**, not just frontend fixtures.

**Open questions:**
- How many badges can one user display at once, and in what precedence?
- Are badges purely decorative, or can one imply a capability? (Decorative is much
  safer — otherwise badges become a second, informal permission system.)
- Colour precedence when several apply: group colour vs team colour vs designer —
  one ordering, decided here, rendered everywhere.

---

# Part B — Additional proposals

Raised in review, in response to "what else would help community and retention?".
Ordered by expected value relative to cost, given what already exists.

## PF-16: Subscriptions and a wanted list

Let a user subscribe to a saved search, a category subtree, an uploader, or a team
(PF-2), and be notified when something matching is uploaded.

**This partially reverses the BE-10.5 drop (2026-07-25)** — see
[Decisions this document reopens](#decisions-this-document-reopens). The honest
framing: BE-10.5 was dropped because per-user delivery over the announcement
pipeline was speculative. PF-16 expresses the want, but routes it differently —
**matches land in the notification system, not a second delivery pipeline.**
Notifications already have preferences, batching, and email digests; that is the
half of BE-10.5 that no longer needs building.

**Why this one first (verified):** the matching engine exists and is tested —
`Filters.Matches` resolves an announcement's full category ancestor chain and
matches include/exclude filters, size floors and freeleech flags, per instance.
A per-user subscription is the same call with a different destination. Two
cautions from verification:
- The only subscribe primitive today is `topic_subscriptions` — a subscriptions
  table for searches/categories/uploaders/teams is new schema.
- `tasks/lessons.md` records a bug in exactly this code path: a test double for
  `CategoryAncestorIDs` erring differently from production hid a truncated
  ancestor chain that would have leaked an excluded category. Reuse the code
  *and* the lesson: doubles must fail the way production fails.

Retention-wise this is the strongest item in Part B: it is the mechanism that
brings a lapsed user back for a specific thing they asked for.

## PF-17: Torrent requests with bounties

A user requests content that is not on the site and optionally attaches a bonus
point bounty; whoever fills it collects. Others can contribute to the bounty.

This is a classic private-tracker retention loop, already anticipated in
`TRACKER_MODS.md` ("a new mini-domain — requests, pledges, fill/claim — that
leans on the bonus ledger; blocked on the bonus system existing first"). **That
block is cleared** — the bonus ledger shipped in migration 054.

It is **the same escrow machinery as PF-1** (PF-29): held funds, a claim, a
fulfilment confirmation, a timeout, a dispute path. Build the escrow once with
two consumers — and build **this one first**: unlike PF-1 it needs no file
hosting, no designer role, and no delivered-artwork storage, so it exercises the
whole state machine with the least surrounding machinery.

Fills content gaps the operator would otherwise have to fill personally.

## PF-18: Leaderboards and seasons

Top uploaders, seeders, thanked, reputation. Nearly free **if** PF-3's ledger is
dated from day one, which the proposal already requires — seasons are then a date
range over the ledger rather than a schema migration.

Verified: nothing leaderboard-shaped exists; `FUTURE_WORK.md` already names
"top N uploaders this month" as an intended `announce_events` consumer. Range
queries over that table need PF-33 (retention/rollups) to stay cheap, and the
site-stats cache (BE-9.3) is the precedent for not recomputing on every view.

Pairs naturally with the capped top tier floated in PF-3. Publishing per-user
rankings is opt-out-relevant — see the privacy note in cross-cutting concerns.

## PF-19: Site-wide events and modifiers

Scheduled, admin-defined periods that change the economy: freeleech weekends,
double bonus, double reputation, upload contests.

**See PF-26**, which specifies the freeleech half of this. If both are built,
freeleech should be PF-19's first modifier type rather than a parallel mechanism —
the scheduler (asynq, decision 5), the UTC answer (decision 10a) and the
computed-not-written discipline are the same for all of them. `TRACKER_MODS.md`
already lists the inverse case — double-upload as `countedUpload`, "same shape"
as `countedDownload` — so the modifier pipeline has a named second consumer from
day one.

Cheap given PF-12 and PF-13 (a multiplier pipeline already has to exist) and it is
the highest-leverage retention tool on this whole list: a recurring, announced
reason to come back on a specific day. It also has an announcement channel
already — the connectors post to Discord, Telegram, IRC and the shoutbox.

## PF-20: A shared anti-abuse spine

PF-5 asks for abuse detection on downvotes. The same need appears in PF-3
(reputation farming, especially the per-hour online award), PF-4 (thanks rings),
PF-1 and PF-17 (escrow disputes and collusion), and PF-18 (leaderboard gaming).

Proposed as one subsystem rather than five: a rate/pattern evaluator that emits
into the **cheat flag** machinery that already exists, so every signal lands in
one staff queue with one triage workflow.

**Verified: the landing zone is genuinely ready.** `cheat_flags` takes a
free-form `flag_type` and `details` JSONB — new signal types need no schema
change — with a dismissal workflow, a cooldown mechanism
(`cheat_flag_cooldown_hours`), staff-only activity-log classification already in
place, and a dashboard. The existing detector also demonstrates the required
execution pattern: checks run in a goroutine off the hot path, never adding
latency to the action being evaluated.

**Two constraints to design around, one of them newly discovered:**
- The event bus is synchronous and in-memory — batch or spawn, never inline.
- **There is no general rate-limiting middleware.** Decision 10 claimed one
  existed (`golang.org/x/time/rate`, per-instance); the 2026-07-26 audit found
  no such code. What exists is per-surface: login attempts, WebSocket messages,
  connector sends, SSE stream count. So this proposal cannot assume "requests are
  already rate limited" as a foundation — either it builds the general limiter as
  part of the spine, or every pattern detector queries the ledger it protects
  (votes, thanks, reputation awards) rather than holding counters. Ledger-querying
  is the better fit anyway: it survives restarts and multiple instances, and the
  ledgers are being built regardless.

Building this once, early, is what makes the gamification safe to turn on.

## PF-21: Follow users, and a personal activity feed

Follow an uploader or a friend; see their public activity in one feed.

**Correction from verification — "largely a filter" was too optimistic.** The
activity log's staff-only-vs-public classification does exist (a curated
allowlist in `internal/event/staff_only.go`, 26 types), so the *privacy* half is
answered. But entries are **pre-rendered prose strings** with loose JSONB
metadata — there is no structured subject/object, so "everything user X did" is
not queryable structurally. A follow feed needs either a metadata contract on
activity entries (actor_id is indexed, so filter-by-actor works; anything richer
does not) or its own projection fed from the event bus. Still modest, but it is
a filter *plus a data contract*, not a filter.

The follow table itself is greenfield (verified: nothing follow-shaped exists).
Turns a library into a social graph, which is what actually retains people.
Interacts with the existing per-user privacy levels — see cross-cutting.

## PF-22: Collections

Curated lists of torrents — staff-made ("Essential 90s Anime") or user-made.
Pure discovery, no economy implications, no new permission axis. Good candidate
for a low-risk community feature that gives high-reputation users something to
make.

`NOT_PORTING.md` §15 dropped torrent *bookmarks* but filed them under "May Be
Added Later" — cite and close that entry when this ships; a private bookmark
list is arguably just a personal collection.

## PF-23: Warning and restriction appeals

A member who is warned or restricted can appeal once, and staff can accept or
reject with a reason. Warnings, restrictions, escalation and the moderation queue
all exist; this closes the loop.

Verified genuinely greenfield — no appeal flow existed in original TT either
(the nearest thing today is the free-text `lifted_reason` on warnings). All the
enforcement machinery it hangs off is DONE. One design note from
`tasks/lessons.md`: hang appeal *outcomes* (lift the warning, restore the
privilege) off the service/event layer, not the HTTP handler — the `can_feed`
revocation bug came from exactly that shortcut.

Retention framing: silent, unappealable moderation is one of the most common
reasons people leave a tracker for good. It is also the item most likely to
reduce staff workload, by moving appeals out of PMs and into a queue.

## PF-24: Torrent version grouping

Group releases of the same underlying title so one page shows every encode,
edition and resolution, instead of ten near-identical search results.

**Flagged as expensive.** It touches browse, search, upload, RSS and the
announcement payload, and it needs an identity model for "same title". The
metadata substrate it would build on is the category-driven JSONB schema from
the BE-3.13 family — which gives *fields*, not identity. Genuinely
transformative for browsing on trackers that have it, but this is a
project-sized item, not a feature — worth scoping properly before it is even
estimated.

## PF-25: Site health page

A dashboard: total torrents, dead ratio (PF-9), active users, library growth.
Cheap once PF-9 and PF-13 compute the numbers, and it makes the community feel
like a shared project rather than a service.

**"Public" reverses a shipped decision** — `BE-9.22 / FE-4.4` deliberately moved
site stats behind authentication so membership/activity numbers are not exposed
on a private tracker. Two-step framing: a **members-only** health page
contradicts nothing and captures most of the community value; a **public** one
is a decision to reopen, recorded above. Recommend shipping members-only and
deferring the public question until there is a reason.

## PF-26: Scheduled and rule-based freeleech

*Operator proposal, flagged as likely next.* Freeleech should apply itself from
rules and a schedule instead of being toggled by hand.

**Mechanics as described:**
- **Rules** grant freeleech automatically — e.g. anything larger than X, to push
  downloads onto content that needs seeders. Other conditions **TBD**.
- **Global rules** — "everything downloaded between now and next week is free".
- **Deadlines**, so a window ends on its own. A recurring weekend freeleech should
  be configured once and never touched again.

**What already exists — verified, with corrections:**
- `torrents.free` and `torrents.silver` (migration 004) — silver is half credit,
  free is none.
- `countedDownload()` (`internal/service/tracker.go`) is the **single** place
  policy is applied: free → 0, silver → half (integer division), free wins when
  both are set; upload is never touched. Every route into ratio accounting goes
  through it, and it has a dedicated test file.
- `announce_events.counted_downloaded_delta` (migration 052) already persists the
  discounted figure per announce, alongside the raw delta.
- `freeleech_ticket` is seeded in the bonus store as a catalogue-only kind,
  disabled and purchase-rejected at the service level "until effects exist" — a
  per-user freeleech grant is already anticipated and would resolve through the
  same place.
- The **connector** filter matches on freeleech (`FreeleechOnly`).
- **Corrections:** the first draft assumed browse and RSS also surface
  freeleech. **Neither does** — `ListTorrentsOptions` has no freeleech field, and
  the RSS handler contains no freeleech reference at all. Both become *new*
  surfaces this feature adds, not existing ones it must keep consistent — and a
  computed effective state cannot be a simple `WHERE free = true` predicate,
  which is a real cost to price in. Also: the connector announcement carries
  `torrent.Free` **only** — `silver` never reaches connectors or the live feed —
  so the resolver must decide what a silver torrent announces as.

So the work is a **policy resolver above `countedDownload`**, not a change to how
accounting works. Effective freeleech becomes the union of: the torrent's own
flags, any active global window, any matching rule, and eventually a user's
freeleech ticket. The *rules* half (admin-defined windows/conditions with an
audit of what fired) should copy the `promotion_rules` + `promotion_runs` shape
rather than invent a new idiom.

**The trap: do not let a scheduled job flip `torrents.free`.**

It is the obvious implementation and it destroys information. Once a job writes
the column, a manual staff grant and an automatic one are indistinguishable — so
when the window closes and the job clears the flag, it also clears every
permanent grant staff made by hand, and nothing records that it happened. The
same collision appears the other way: a rule matching a torrent staff had
deliberately set to *not* free will silently override that decision.

Effective state should be **computed at announce time** from the manual flags plus
the active windows and rules, leaving `torrents.free` meaning exactly what it
means today: a deliberate, permanent, human decision. This also makes the feature
reversible — turning the whole thing off restores prior behaviour with no data to
repair.

**Retroactivity is already correct, for free.** Because the discounted figure is
computed and stored per announce, a window closing cannot claw back credit already
granted, and a window opening does not retroactively refund. That falls out of the
existing design; it just needs to not be broken.

**Timezone: decided — UTC.** See decision 10a in `OPEN_QUESTIONS.md`. Windows are
configured and evaluated in UTC, and the maintainer decides what "the weekend"
means for their community. Original TorrentTrader's site and per-user timezone
settings are deliberately not ported: on a global membership there is no correct
per-user answer, and a user-controlled timezone would let a member shift their own
clock to enter or extend a freeleech window. Display remains browser-local, which
is a rendering concern with no accounting consequence. **Housekeeping for the
same PR:** three stories (`BE-1.4`, `FE-2.1`, `MT-1.1`) still carry per-user
timezone acceptance criteria that predate 10a — fix them so the backlog and the
decision log agree.

**Also absorb here:** `BE-8.3`'s two unchecked boxes ("Toggle freeleech per
torrent" already exists via `BE-3.6`'s staff edit; "View all freeleech torrents"
does not) — an effective-freeleech admin view answers the second properly,
including *why* each torrent is currently free.

**Open questions:**
- Do rules and windows compose with `silver`, or does any freeleech grant simply
  win? (Today free already beats silver — the most generous reading. Extending
  that rule is probably right and is the simplest to explain to members.)
- Should a torrent display *why* it is currently free — rule, window, or manual?
  Members will ask, and "it was free yesterday" support requests are much cheaper
  to answer when the page says which window applied.
- Does a rule evaluate once at upload, or continuously? A size rule is static, but
  "fewer than N seeders" would have to be re-evaluated, and that is a different
  cost profile.
- Do announcements, browse and (new) RSS surfaces carry the *effective* state?
  They should, or filters will disagree with what the tracker charges — but note
  browse/RSS freeleech surfaces are new work (above), not existing behaviour to
  preserve.

**Relationship to PF-19:** PF-19 proposed site-wide scheduled modifiers and named
freeleech weekends as an example. This is that feature, specified. If PF-19 is
built, freeleech should be its first modifier type rather than a parallel
mechanism — double-upload and double-bonus windows want the same scheduler, the
same timezone answer and the same "computed, not written" discipline.

## PF-27: Site CLI

*Operator proposal.* A fully-fledged CLI for administering the site from a
terminal — with completion, help, and enough structure that it is pleasant rather
than a `curl` wrapper.

**Considered and set aside: an MCP server.** The original thought was an MCP
server so agents could manage the site. The conclusion was that a CLI reaches the
same goal and is useful for automation an MCP server is not — cron jobs, shell
pipelines, CI, an operator on a box with no browser. Worth recording that an MCP
server can still be layered on later and would be thin, because it would front the
same commands.

Everything here is already reachable over REST — the API is complete and is what
the frontend uses. The argument for a CLI is ergonomics and automation, not
capability.

**Reuses:** `migration-tool/` is already a Cobra CLI in this monorepo (cobra
v1.10.2), so the framework choice is made and the conventions exist — decision 16
in `OPEN_QUESTIONS.md`. A site CLI is a sibling binary, not a new ecosystem.

**Constraints verification added:**
- **A new dependency edge: PF-28 first.** A CLI authenticates as *something*.
  Without API tokens it stores a password or a refresh token in shell config —
  the exact thing PF-28 exists to prevent. Scoped tokens should exist before the
  CLI teaches everyone to script against the API.
- **Module placement is constrained by the architecture:** `ARCHITECTURE.md`'s
  shared-nothing rule means a third module cannot import `backend/internal/`
  types. Either the CLI is a pure REST client with its own types (consistent
  with the boundary), or it lives as a second `cmd/` inside `backend/` (sharing
  types, blurring the boundary). The migration tool precedent is the former.
- **The generated-vs-hand-written question is now answerable.** Decision 9 was
  rewritten on 2026-07-26: there are two specs, `openapi.yaml` (full) and the
  generated `openapi.public.yaml`. A CLI has a real artifact to generate from —
  and a useful split falls out of it: a *member* CLI generated from the public
  spec is a genuine third-party-grade client, while admin commands generated from
  the full spec stay an operator tool. The remaining blocker is not architectural,
  it is coverage: the full spec documents 37 of 189 routes, so generating today
  would produce a CLI with holes. **Closing spec coverage is therefore a PF-27
  prerequisite** — it is `BE-11.2` in the backlog, not an open question here.
- **Sequencing:** the migration tool is 11 open stories and the only real block
  of unbuilt work in the backlog. A second Cobra surface should be scheduled
  against it, not beside it — two half-finished CLIs is a worse place than one.

**Open questions:**
- Is the CLI a thin REST client, or does it get direct database access for
  break-glass operations? Thin-client is far safer and keeps one authorisation
  path; direct access is what you want at 3am when the API is down. They are
  different tools and probably should not be one binary.
- One binary with an admin subtree, or two binaries (member vs operator) matching
  the two specs? The spec split makes the second option cheap and makes the
  security story much easier to explain.
- Shipped how — in the release workflow, as a container, or `go install`?

**Note on skills:** the operator raised the idea of shipping skills that teach an
agent how to use it. That is downstream of the CLI existing and having stable
command names, but it is a reason to treat the command surface as an interface
worth designing rather than accreting.

**Pairing:** once this exists, the "features ship in BE+FE pairs" rule in
`CLAUDE.md` should be reconsidered as **BE+FE+CLI**, so the CLI does not
immediately fall behind the web UI. Not adopted yet — there is no CLI to pair
with, and a rule that cannot be followed is worse than no rule.

## PF-28: API keys / personal access tokens

*Operator proposal, explicitly a separate feature from PF-27.* Long-lived,
non-expiring keys so automation does not have to hold a password or refresh a
session.

Recognised in the proposal as **extremely dangerous** on its own, hence
fine-grained per-token permissions: a token that can only do the one thing the
script needs.

**Verification found this is already specified — and half-promised.**
`BE-1.2` is marked `[DONE — core auth, sessions/API keys deferred]`, and its
deferred acceptance criteria describe this feature almost exactly: named keys
with no expiry, manual revocation, **scoped permissions chosen at creation
(read-only / upload / full)**, a list endpoint showing name/created/last-used
and never the secret, and a delete endpoint. `FE-2.1`'s AC already lists the
management UI, and `BE-6.1`'s AC already assumes the WS handshake accepts "access
token **or API key**". PF-28 should be written as *un-deferring BE-1.2's scope*
— refining its three-level scoping toward the fine-grained model — rather than
as a new feature.

**What exists today (verified).** `middleware.RequireAuth` extracts a Bearer
token and hands it to a `SessionValidator`, which resolves a Redis-backed session
(decision 4). Authorisation downstream is `perms.Level` comparisons plus the
boolean capability fields on `model.Permissions`; `RequireCapability` hardcodes
a five-string switch (`upload|download|invite|comment|forum`).

**How much has to change depends entirely on the scope answer:**
- **Coarse (token acts as the user).** Small: a second validator that resolves a
  key to a user and produces the same context. Everything downstream is unchanged.
  But a leaked key is then equivalent to a stolen session with no expiry — the
  thing the proposal is worried about.
- **Fine-grained (token carries its own permission set).** Larger: the context
  needs to carry an *effective* permission set that is the intersection of the
  user's capabilities and the token's grants, and **every** authorisation check
  has to read that rather than the user's class directly. Miss one and the token
  quietly has full rights there — which is exactly the failure mode the feature
  exists to prevent, and it fails silently.

The second is the one worth having, and it is the one that must fail closed at
every check, not just at the middleware — that is lesson #1 in
`tasks/lessons.md`, recorded from the `can_feed` work. **One subtlety
verification surfaced:** `can_feed` is deliberately *not* on `model.Permissions`
— it is re-read from the database per request, bypassing the session entirely.
The token-intersection design must account for authorisation checks that never
consult the session context, or feed access becomes the check a scoped token
silently passes.

**Open questions:**
- Truly non-expiring, or expiring-with-renewal? Non-expiring is the ask; the
  last-used timestamp BE-1.2 already specifies plus an inactivity sweep is a
  middle ground that keeps the ergonomics.
- What is the permission vocabulary? BE-1.2's read-only/upload/full is a
  starting point; anything finer is the RBAC question below.
- Are token actions distinguishable in the activity log? They should be — "who
  did this" is different from "which key did this".

## Open architectural question: RBAC vs class/level parity

Raised alongside PF-28 and recorded here because it does not belong to any single
feature.

The site currently authorises on **class/level plus group capability columns**
(`can_upload`, `can_download`, `can_invite`, `can_comment`, `can_forum`,
`can_feed`, `can_self_approve`), which is what original TorrentTrader did and
what this port deliberately mirrors. The observation is that **role-based
assignment would be the better model** — and that PF-28's fine-grained tokens
need a permission vocabulary that it would supply naturally.

**A terminology note verification forced:** `BE-1.5` is already recorded as
"[DONE — RBAC with group permissions…]" — the backlog uses "RBAC" for the
*shipped* capability-column model, while this discussion uses it for the
*unshipped* roles-and-permissions model. Any future story must define its terms,
or "not now" will read as "already done".

The operator's position, recorded as stated: **not now.** The project is a port,
and switching authorisation models would disrupt rules that are currently built on
level. Worth revisiting only if parity can be preserved through configuration —
i.e. classes become preset role bundles rather than a parallel mechanism.

Flagged because several proposals lean on it: PF-1 (designer), PF-2 (team
moderator), PF-6 (VIP), PF-15 (badges) and PF-28 (token scopes) each introduce
something role-shaped. Each one added independently makes the eventual migration
more expensive. No action proposed — just a note that the cost of deferring is not
zero, and that it compounds.

---

# Part C — shared machinery

Pieces surfaced by the verification pass that two or more items were each about
to build. Numbered so the DAG can reference them; each is deliberately small and
should be specified with its first consumer, not speculatively.

## PF-29: Escrow / trust account service

The held-funds → claim → fulfil/timeout → dispute state machine, over the bonus
ledger. Consumers: **PF-17** (request bounties — build with this one first; no
file handling, exercises every state) and **PF-1** (designer orders). Funds held
in escrow must leave the spendable balance at hold time — which means either a
second materialised column (`bonus_held`) or ledger-derived available balance;
decide once. Every transition writes a ledger row (`reason` values for
hold/release/refund/forfeit) so disputes are auditable from data that already
exists.

## PF-30: Torrent edit history

Per-field before/after rows for torrent edits — **does not exist today** (the
current edit path emits one prose activity-log line). Templates:
`user_edit_history` (migration 057, including the username-snapshot pattern) and
`forum_post_edits` (048/062). First consumer: **PF-10** (ownership provenance —
"uploaded by X, now owned by Y" is an edit-history row over the owner field).
Independently valuable for moderation even if PF-10 never ships.

## PF-31: Polymorphic report targets

`reports` is torrent-only (`torrent_id` column), despite the backlog's claim of
reportable users/comments/posts. Consumers who need a non-torrent target:
**PF-1** (report a requester), **PF-5** (report a post/voter — though vote
*pattern* abuse goes to cheat flags via PF-20 instead), **PF-23** (appeals want
the same queue ergonomics). Either generalise `reports` (type + id columns, one
staff queue) or accept parallel queues deliberately. Generalising is a
migration on a live table — cheaper before the new consumers than after.

## PF-32: User file uploads

A user-facing upload path over the existing `FileStorage` abstraction: multipart
handler, size/type validation, a serving route, per-user quota. **Nothing like
it exists** — MinIO's only consumer is server-side `.torrent` bytes, and avatars
are external URLs. First consumer: **PF-1** (delivered kits). Obvious second:
locally-hosted avatars, which would close a long-standing external-dependency
wart. Not needed if PF-1 v1 ships with external links.

## PF-33: Announce-log lifecycle

`announce_events` is the substrate for PF-9 (deadness), PF-13 (goals/speeds),
PF-18 (leaderboards) and PF-25 (health) — and it currently **grows without
bound**: `announce_log_retention_days` is advisory, no pruning job exists, and
the table has two indexes. Before anything queries it over date ranges at page
speed: implement the retention sweep the setting already promises, and decide
the rollup story (periodic aggregates per user/day, in the spirit of the
site-stats cache) so leaderboards and goal pages read summaries, not raw
announces. This is maintenance the table needs *anyway*; the proposals just make
it urgent.

**Split of ownership, so this does not live in two documents:** the *maintenance*
half — the unbuilt retention sweep — belongs to `docs/FUTURE_WORK.md`, which
already tracks it and is where deferred work lives. PF-33 covers only the
*consumer* half: the rollup shape these proposals need. If the retention sweep
ships first, delete this item's maintenance framing and keep the rollup question.

---

# Dependencies and build order

Most of Part A converges on shared machinery. Building it in the right order
turns several later items into small features. Edges the first draft missed are
marked ●new.

```
 DECISIONS (cheap, block everything under them)
 ├─ D1: reopen NOT_PORTING §14 (karma)      ✅ APPROVED 2026-07-26 — PF-3 unblocked
 ├─ D2: reopen NOT_PORTING §4 (teams)       ✅ APPROVED 2026-07-26 — PF-2 unblocked
 ├─ D3: multiplier stacking + accrual-time  → gates PF-12, PF-13, PF-19, PF-26 interplay
 ├─ D4: privacy opt-out model               → gates PF-4 lists, PF-18, PF-21
 └─ D5: membership-based access model       → gates PF-2 forum + PF-6 rooms (one design)

 PF-3 Reputation ledger (+ PF-11 registry)
 ├── PF-4  thanks            ├── PF-12 multipliers
 ├── PF-5  voting            ├── PF-18 leaderboards
 ├── PF-10 adopt (award)     └── PF-1  designer pay
 └── PF-20 anti-abuse spine ←— also protects PF-4/PF-5/PF-17/PF-18

 PF-29 Escrow ──→ PF-17 bounties (first consumer) ──→ PF-1 designer
 PF-32 uploads ─────────────────────────────────────→ PF-1 designer   ●new
 PF-31 polymorphic reports ─────────────────────────→ PF-1, PF-5      ●new

 PF-30 torrent edit history ──→ PF-10 adopt                           ●new
 PF-9 dead definition ──→ PF-8 reseed rules, PF-10 abandonment, PF-25 ●new(PF-25)
 PF-33 announce lifecycle ──→ PF-13 goals, PF-18 boards, PF-25 health ●new

 PF-2 Teams ──→ PF-6 team rooms, PF-14 colours, PF-15 badges
 PF-14 + PF-15 = one adornment system (render path already exists)

 PF-28 API tokens ──→ PF-27 site CLI                                  ●new
 PF-26 freeleech ──→ PF-19 modifiers (first modifier type) ──→ PF-12/PF-13 economy
```

**Suggested order:**

0. **Fix the live reseed gap** (server-side eligibility + honest BE-3.9 status).
   Not a feature — a correctness fix that should not wait for PF-8.
1. **The remaining decision batch (D3, D4, D5).** One sitting. D1 and D2 are
   **done** — reputation and teams are both in, so the third of this document
   that was downstream of "is reputation in or out" is now buildable. What is
   left still wants deciding before its dependents are specified: how multipliers
   stack (D3), what members can opt out of publishing (D4), and the one
   membership-based access model shared by team forums and chat rooms (D5).
2. **PF-26 — freeleech.** Independent of everything above, flagged as likely
   next, and it forces the resolver/scheduler/computed-not-written patterns the
   whole economy layer will reuse. Include the timezone-AC cleanup and the
   BE-8.3 leftovers.
3. **PF-3 + PF-11 — reputation ledger and the config registry.** Mirror
   `bonus_transactions` (`delta/reason/ref_id/created_at`), copy the
   `promotion_rules`/`promotion_runs` shape for tiers, reuse the
   source-interface + hourly-job pattern from bonus.
4. **PF-20 — anti-abuse spine.** Before the gamification is switched on, not
   after. It emits into the existing cheat-flag queue.
5. **PF-4 — thanks.** Small, greenfield, first real consumer of the ledger. A
   good shakedown for PF-3.
6. **PF-14 + PF-15 together — one adornment system.** Mostly a data-payload and
   approval-flow question; the render path exists. Needed by PF-2 anyway.
7. **PF-9 + PF-33 — dead-torrent definition and announce-log lifecycle.** One
   definition; the retention job is overdue regardless.
8. **PF-8 — finish and enhance reseed.** Layers 2 and 3 of the PF-8 plan.
9. **PF-30 → PF-10 — edit history, then adopt.** Staff reassignment first;
   member self-adoption behind the flag, later.
10. **PF-29 → PF-17 → PF-1.** Escrow with bounties as the shakedown consumer;
    the designer system follows once PF-32 (uploads) exists or v1 goes
    links-only.
11. **PF-5 — voting.** Needs PF-3 and PF-20 in place first.
12. **PF-28 — API tokens.** Un-defer BE-1.2's scope. Independent of the
    community track, prerequisite for the CLI.
13. **PF-16 — subscriptions.** Independent; high value for the cost; needs the
    BE-10.5 framing recorded above.
14. **PF-13, PF-12, PF-19 — the economy layer**, specified together under D3 so
    multiplier stacking is decided once. PF-26 has already forced the hard
    patterns by this point.
15. **PF-2 — teams → PF-6 — chat rooms.** Approved, and the two big
    authorisation lifts. They share the D5 membership model, so settle that once
    across both rather than twice. PF-6's hide-shoutbox slice ships early and
    independently of all of it (it is nearly done already).
16. **PF-21, PF-22, PF-23, PF-25 — community slot-ins.** Each independent;
    PF-23 (appeals) is the highest-empathy item and touches nothing else.
17. **PF-27 — site CLI.** After PF-28, sequenced against the migration tool's
    11 open stories.
18. **PF-7 — real-time.** Independent, but cheaper after there are more things
    worth updating live; adopt the FUTURE_WORK SSE stance or argue it down.
19. **PF-24 — version grouping.** Scope before estimating.

---

# Cross-cutting concerns

**One multiplier decision, made once (D3).** PF-12 (reputation tiers), PF-13
(goals), PF-19 (site events) and PF-26 (freeleech rules) all modify the same
accounting. Whether they stack additively or multiplicatively, and whether they
apply at accrual or read time, must be decided once and written down — otherwise
the economy becomes impossible to reason about or to audit. PF-26 already has
the right answer available to copy: `counted_downloaded_delta` is computed per
announce and stored, so the discount is fixed at the moment it is granted and no
later change can rewrite history. Every other modifier should work the same way.

**Scheduled state must be computed, never written back.** Any feature that turns
something on for a window — freeleech, double bonus, a contest — is tempting to
implement as a cron job that flips a column and another that flips it back. That
destroys the distinction between a temporary automatic grant and a permanent
manual one, so the "off" job silently clears deliberate staff decisions, and
nothing records that it happened. Compute effective state at read/accrual time and
leave the manual columns meaning exactly what they mean today.

**Feature flags: one registry, not eleven toggles.** Eleven of these items ask
for their own admin on/off switch. `site_settings` is the right home and the
pattern exists (seed migration → `Setting*` constant → validation case →
frontend definition) — but verification found the pattern is honoured more in
shape than substance: only 8 of 38 keys are validated, and the frontend
definition list is hand-maintained with a silent fallback. PF-11's registry-with-
guard-test is the fix; adopt it before adding eleven more keys, and give the
settings page a shared "Features" section.

**Rule tables: one idiom.** PF-3 (tiers), PF-13 (goals), PF-19 (modifiers) and
PF-26 (freeleech rules) each need admin-defined rules evaluated by a job.
`promotion_rules` + `promotion_runs` (rules, plus an audit row per engine run,
plus service-level guards on what a rule may target) is the shipped idiom — copy
it four times rather than inventing four.

**Capability vs badge vs role.** PF-1 (designer), PF-2 (team membership, team
moderator), PF-6 (VIP), PF-15 (badges) and PF-28 (token scopes) each introduce
something role-shaped. The shipped model is group capability columns plus
per-user override/restriction columns (and note `can_self_approve`: a capability
deliberately hidden from the group UI, granted via a seeded class — useful
precedent for "designer"). Decide deliberately whether new concepts extend that
model or sit beside it — and keep badges decorative, so they never become an
informal second permission system. See the RBAC note: deferring the model change
is a reasonable call, but every role-shaped addition raises the eventual cost.

**Privacy (D4).** Reputation, leaderboards, follow feeds and public thanks lists
all publish behaviour that is currently private. The site already has the
mechanism: per-user privacy levels (strong/normal/low) are ported and enforced
(strong hides stats from non-staff). The decision is not *whether* an opt-out
exists but **which of these surfaces respect which level** — one mapping, taken
before the first surface ships rather than retrofitted after.

**Retroactivity.** PF-11 fixes the rule for reputation: changing a reward does
not re-award history. The same question arises for goals, multipliers and tier
thresholds. Same answer, stated once.

**Engineering lessons that already bind these designs** (from
`tasks/lessons.md`, all learned the hard way in this repo):
- *Fail closed at every layer that decides* — PF-2 team forums, PF-6 rooms, PF-7
  live updates, PF-28 token scopes. Write the nothing-wired test.
- *Struct-writing inserts defeat column defaults* — every proposal adding a user
  column (reputation, held balance, team, hide-shoutbox) must test the default
  with a raw INSERT.
- *A frontend fixture is not evidence the backend sends that field* — PF-14/15
  adornment payloads, PF-7 live payloads: assert on the backend serializer.
- *Optional booleans in write requests revoke things* — every admin config
  object here (tiers, rules, windows, connector rooms) wants pointer fields.
- *Test doubles must fail the way production fails* — PF-16 reuses the exact
  code path where this bug lived.
- *Side effects hung on HTTP handlers miss every other caller* — PF-23 appeal
  outcomes, PF-19/PF-26 window transitions, PF-5 vote-driven reputation belong
  on the service/event layer.
- The event bus is synchronous and in-memory — high-volume awards and fan-outs
  (PF-3, PF-7, PF-20) must batch or spawn, never inline.

**Writing the stories.** When an item graduates to `IMPLEMENTATION_TASKS.md`:
take a **fresh epic prefix** (`BE-11.x` onward — the `BE-10` prefix is already
doubled up and `BE-10.7`/`BE-10.8` each name two stories); use the
`[DONE — scope qualifier]` convention honestly (the reseed story shows the cost
of not doing so); decide staff-only activity-log classification for every new
event type (the open `BE-10.8` bug is that question unanswered for chat
deletions); ship BE+FE in one PR (and revisit BE+FE+CLI once PF-27 lands); and
delete the PF entry here in the same PR. Read migration 039's header comment
before writing any schema migration.
