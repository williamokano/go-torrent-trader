# Proposed Features

A staging area for features that have been **proposed but not specified**. Nothing
here is committed work.

`IMPLEMENTATION_TASKS.md` is the living backlog: everything in it is either done
or specified well enough to build. This file is upstream of that. An item moves
out of here and into the backlog as a real story once its open questions are
answered — at which point delete it here rather than leaving two copies.

Open questions are recorded **as questions**, not resolved into an assumption.
Where an item overlaps something already built, that is called out: several of
these are smaller than they look, and at least one is mostly built already.

Numbering (`PF-n`) is stable so items can be referenced from discussion. PF-1 to
PF-15 and PF-26 to PF-28 came from the operator; PF-16 to PF-25 were raised in
review. The later operator items sit in Part B because they arrived later, not
because they matter less — PF-26 is flagged as likely next.

---

## Reading order

The single most important thing in this document is **[Dependencies and build
order](#dependencies-and-build-order)** at the bottom. Six of the fifteen items
depend on the reputation ledger, two independently need an escrow/trust account,
and two describe halves of the same "things drawn next to a username" system.
Built in the order proposed, several of them get much cheaper.

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
- Is "designer" a group capability (like `can_upload`, `can_feed`) or an
  independent flag that any class can carry? A capability column matches the
  existing `groups` shape; a flag matches "some users are tagged as designer".
- What currency? Bonus points is the only balance that exists today.
- Can an order be cancelled *after* a designer has claimed and started work? "Cancel
  at any time" and "protect the artist" are in tension — this is the main
  fairness decision in the feature.
- Does cancellation after delivery refund, or does it force the report path?
- Are delivered files hosted by the site (MinIO is already wired) or linked
  externally?

**Reuses:** `bonus_transactions` ledger + `users.bonus_points` (migration 054);
the report/staff-queue shape from torrent reports; MinIO for uploads.

**Note:** the escrow described here is the same machinery as PF-17 (request
bounties). Build it once — see build order.

## PF-2: Team system (release groups)

A team is a group of people who release together. Teams get a public page so users
can find releases by the group they trust.

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

**Open questions:**
- Can a user belong to several teams? Everything else (colour, badge, chat)
  changes shape depending on the answer.
- Is the leader transferable? What happens to a team whose leader is banned or
  goes inactive?
- Are releases attributed to a team by the uploader choosing one, or inferred?
- Does removing a member retroactively affect release attribution?

**Reuses:** slug-addressed resources already exist (live feed slugs, migration
075); `MarkdownRenderer` + `MarkdownEditor` are built and sanitised; forum
tables exist.

**Watch out:** forum access is currently class-based. A forum visible to "members
of team X and all staff, regardless of class" is a new authorisation axis, not a
lower `min_class`.

## PF-3: Reputation system (gamification)

Distinct from bonus points: reputation is earned by participating, not by seeding
economics, and is displayed as a rank badge.

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

**Open questions:**
- Does reputation ever decay, or only accumulate? This decides whether "seasons"
  are cosmetic or structural.
- Can reputation go negative (PF-5 downvotes suggest it can)? Floor at zero?
- For a capped top tier, what happens when someone is displaced — silent demotion,
  or a notification?

**Reuses:** `bonus_transactions` + `users.bonus_points` is exactly this shape
(ledger + materialised column) and should be the template.

**Design risk worth stating plainly:** *"being online, x points per hour"* rewards
leaving a tab open rather than participating, and is the easiest signal on the
list to automate. If it ships, it wants the anti-abuse spine from PF-20 and
probably a daily cap. Every other source on the list is an action someone
actually took.

## PF-4: Torrent thanks

A Thank button on a torrent, a list of who thanked, and the button disabled — or
switched to Unthank/Remove — once used. Admin-disableable.

**Open questions:**
- Is the thanks list public to everyone, or only to the uploader?
- Does un-thanking revoke the reputation it granted? (Consistency with PF-5's
  vote-removal answer matters.)

**Note:** PF-3 lists thanking as a reputation source and notes it does not exist
yet — confirmed, there is nothing matching it in the schema. This is greenfield
and small, which makes it a good first consumer of the reputation ledger.

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

**Reuses:** warnings, restrictions and cheat flags already exist as enforcement
outlets — the flagging has somewhere to go.

## PF-6: Chat rooms, and closing the shoutbox

Two parts:

1. **A button to hide/close the global shoutbox.** Sometimes you just don't want
   to see it.
2. **Multiple chat rooms:** team chat (PF-2), staff chat (admins + moderators),
   admin chat, and a VIP chat.

Noted explicitly: some communities will prefer Discord, so this should not be
assumed to be the primary channel.

**This is the largest item on the list.** Today `chat_messages` has no room or
channel column — it is one global stream — and the WebSocket hub broadcasts every
message to every connected client. Rooms mean a schema change, per-room
authorisation on both history and the socket, and a hub that routes instead of
broadcasts.

**It also touches the shoutbox connector.** The chat connector posts announcements
into "the one shoutbox"; several instances already exist, each with its own
template and filters. Once rooms exist, each instance needs to name its target
room, and the singleton reasoning in that connector needs revisiting.

**Open questions:**
- Is the hide-shoutbox preference per-device or stored on the account?
- Is room membership derived (staff = staff class, team = team members) or
  explicitly managed? Derived is far less to maintain.
- Is history per room retained on the same schedule?
- Do rooms need their own mute state, or is a chat mute global?

## PF-7: Real-time page updates

Pages should update live rather than only notifying. The torrent detail page is
the worked example: seeder/leecher counts, an arriving moderation message,
description or metadata edits, new comments, new thanks — all should appear while
the user is still on the page.

Explicitly noted: subscribing a page to *all* events and filtering client-side is
the wrong shape. The page should subscribe to the events it cares about.

**Open questions:**
- Which transport? An SSE connector and a chat WebSocket hub both exist — a third
  mechanism would be one too many.
- Do live updates require the same authorisation as the page itself? (Yes, and
  that is the main correctness risk: a live feed must not leak a field the page
  would have hidden.)
- What is the update granularity — a nudge to refetch, or the changed payload?
  A nudge is far easier to keep consistent with page-level authorisation.

**Reuses:** 66 event types are already published on the internal bus; the SSE
connector and the chat hub are both live transports.

## PF-8: Reseed requests — *mostly already built*

**This exists.** `BE-3.9: Reseed Request` is marked DONE: `reseed_requests`
(migration 016), a button on the torrent detail page, a 24-hour per-user
per-torrent rate limit, a background job that **PMs everyone who completed the
torrent plus the owner**, and an email to the uploader. PMs already generate a
notification about the message, which matches the "message them, don't notify
about the request" requirement.

**What is genuinely new in the proposal:**
- An **admin kill switch** for the feature.
- **Configurable eligibility rules** — minimum transfer speed, minimum seeder
  count, where `0` means "always allowed". Today eligibility is hardcoded to
  *0 seeders*.
- **Staff bypass:** staff can request a reseed from everyone who downloaded and
  is not currently seeding, regardless of those rules.

Scoped that way this is a small enhancement, not a feature.

## PF-9: Dead torrent detection

Surface torrents with no seeders whose last seed was more than X ago, ideally with
leecher counts, so staff can see the health of the library and act.

**Open questions:**
- Is "dead" a stored, recomputed status or a query? A stored status can be indexed
  and reported on; a query can never be stale.
- Does a dead torrent become visually marked to members, or is this staff-only?

**Feeds:** PF-8's eligibility rules and PF-10's abandonment definition both need
this. Build it once, as one definition.

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

**Open questions:**
- What exactly defines "abandoned"? Owner inactivity, torrent deadness, or both?
  (See PF-9.)
- Does adoption transfer the upload credit and any bonus tied to it, or only the
  edit rights?
- Can an adopted torrent be reclaimed if the original uploader returns?

**Reuses:** torrent edit history already exists, which is where the provenance
trail belongs.

## PF-11: Reputation rewards are documented and configurable

Every reputation source is documented and admin-configurable. **Changing a reward
does not apply retroactively**, even though every award is retained in the ledger.

**Note:** the connectors admin page already derives its template-field help from
the backend by reflection, with a guard test that fails the build when a field is
added without a description. That is the right precedent — a hand-maintained list
of reputation sources in the frontend would drift silently, and the failure mode
is an admin configuring a source that no longer fires.

## PF-12: Bonus × reputation multipliers

Reputation tiers carry a configurable multiplier applied when seeding bonus is
awarded.

**Open questions:**
- Do multipliers stack with PF-13's goal multipliers? Additive or multiplicative?
  This needs a single answer covering every multiplier source, decided once, or
  the economy becomes impossible to reason about.
- Is the multiplier applied at award time, or at read time? Award time is honest
  with the ledger; read time is retroactive and contradicts PF-11.

## PF-13: Site and user goals

Configurable goals — torrents uploaded, data **currently** seeding, user maximum
and average speed, and so on — shown on a page where a user can see where they
stand. Meeting goals grants extra seeding-bonus multipliers.

**The averaging problem, as raised:** a user seeding with no leechers online has
no upload through no fault of their own. The proposal is to count only intervals
where the user had a non-zero transfer delta in either direction. That is the
right instinct; it needs a precise definition before it can be built.

Also noted: these metrics help identify cheaters.

**Open questions:**
- Are goals per-user, site-wide, or both? The title says both.
- Are goals seasonal or lifetime?
- Does losing a goal (dropping below the seeding threshold) remove the multiplier
  immediately?

**Reuses:** announce event logging and cheat detection already compute
per-announce deltas.

## PF-14: Team colours

Each team gets a staff-assigned colour, and the frontend renders the member's name
in that colour wherever it is drawn — the way admin, VIP and moderator names are
already coloured. Designers too.

## PF-15: Badges

Badges render next to a username, like a verified tick: admin, staff, per-group,
per-team. Staff-configurable, and a team's badge must be **staff-approved**.
Approval authority is itself configurable (which class may approve).

**PF-14 and PF-15 are one system.** Both answer "what is drawn next to a username,
and who decides". They share the same render path (every username on the site),
the same approval flow, and the same caching concern. Specifying them separately
will produce two overlapping implementations.

**Open questions:**
- How many badges can one user display at once, and in what precedence?
- Are badges purely decorative, or can one imply a capability? (Decorative is much
  safer — otherwise badges become a second, informal permission system.)

---

# Part B — Additional proposals

Raised in review, in response to "what else would help community and retention?".
Ordered by expected value relative to cost, given what already exists.

## PF-16: Subscriptions and a wanted list

Let a user subscribe to a saved search, a category subtree, an uploader, or a team
(PF-2), and be notified when something matching is uploaded.

**Why this one first:** the matching engine already exists and is tested. The
connector system resolves an announcement's full category ancestor chain and
matches it against include/exclude filters, size floors and freeleech flags, per
instance. A per-user subscription is the same `Filters.Matches` call with a
different destination — the notification system instead of a webhook.

Retention-wise this is the strongest item in Part B: it is the mechanism that
brings a lapsed user back for a specific thing they asked for.

## PF-17: Torrent requests with bounties

A user requests content that is not on the site and optionally attaches a bonus
point bounty; whoever fills it collects. Others can contribute to the bounty.

This is a classic private-tracker retention loop, and it is **the same escrow
machinery as PF-1** — held funds, a claim, a fulfilment confirmation, a timeout,
a dispute path. Specifying the two together and building one escrow service is
substantially less work than building it twice.

Fills content gaps the operator would otherwise have to fill personally.

## PF-18: Leaderboards and seasons

Top uploaders, seeders, thanked, reputation. Nearly free **if** PF-3's ledger is
dated from day one, which the proposal already requires — seasons are then a date
range over the ledger rather than a schema migration.

Pairs naturally with the capped top tier floated in PF-3.

## PF-19: Site-wide events and modifiers

Scheduled, admin-defined periods that change the economy: freeleech weekends,
double bonus, double reputation, upload contests.

**See PF-26**, which specifies the freeleech half of this. If both are built,
freeleech should be PF-19's first modifier type rather than a parallel mechanism —
the scheduler, the timezone answer and the computed-not-written discipline are the
same for all of them.

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

Building this once, early, is what makes the gamification safe to turn on.

## PF-21: Follow users, and a personal activity feed

Follow an uploader or a friend; see their public activity in one feed. The
activity log already exists and already classifies entries as staff-only versus
public, so the hard privacy question is answered — this is largely a filter and a
follow table.

Turns a library into a social graph, which is what actually retains people.

## PF-22: Collections

Curated lists of torrents — staff-made ("Essential 90s Anime") or user-made.
Pure discovery, no economy implications, no new permission axis. Good candidate
for a low-risk community feature that gives high-reputation users something to
make.

## PF-23: Warning and restriction appeals

A member who is warned or restricted can appeal once, and staff can accept or
reject with a reason. Warnings, restrictions, escalation and the moderation queue
all exist; this closes the loop.

Retention framing: silent, unappealable moderation is one of the most common
reasons people leave a tracker for good. It is also the item most likely to
reduce staff workload, by moving appeals out of PMs and into a queue.

## PF-24: Torrent version grouping

Group releases of the same underlying title so one page shows every encode,
edition and resolution, instead of ten near-identical search results.

**Flagged as expensive.** It touches browse, search, upload, RSS and the
announcement payload, and it needs an identity model for "same title" that the
metadata schema may not currently support. Genuinely transformative for browsing
on trackers that have it, but this is a project-sized item, not a feature — worth
scoping properly before it is even estimated.

## PF-25: Public site health page

A public dashboard: total torrents, dead ratio (PF-9), active users, library
growth. Cheap once PF-9 and PF-13 compute the numbers, and it makes the community
feel like a shared project rather than a service.

## PF-26: Scheduled and rule-based freeleech

*Operator proposal, flagged as likely next.* Freeleech should apply itself from
rules and a schedule instead of being toggled by hand.

**Mechanics as described:**
- **Rules** grant freeleech automatically — e.g. anything larger than X, to push
  downloads onto content that needs seeders. Other conditions **TBD**.
- **Global rules** — "everything downloaded between now and next week is free".
- **Deadlines**, so a window ends on its own. A recurring weekend freeleech should
  be configured once and never touched again.

**What already exists.** More than it looks:
- `torrents.free` and `torrents.silver` (migration 004) — silver is half credit,
  free is none.
- `countedDownload()` (`internal/service/tracker.go`) is the **single** place
  policy is applied: free → 0, silver → half, free wins when both are set. Every
  route into ratio accounting goes through it.
- `announce_events.counted_downloaded_delta` (migration 052) already persists the
  discounted figure per announce, alongside the raw delta.
- `freeleech_ticket` is already seeded in the bonus store as a catalogue-only kind,
  deliberately disabled "until effects exist" — a per-user freeleech grant is
  already anticipated and would resolve through the same place.

So the work is a **policy resolver above `countedDownload`**, not a change to how
accounting works. Effective freeleech becomes the union of: the torrent's own
flags, any active global window, any matching rule, and eventually a user's
freeleech ticket.

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
is a rendering concern with no accounting consequence.

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
- Do announcements and RSS carry the *effective* freeleech state or the stored
  flag? Connector filters already match on freeleech, and browse already has a
  freeleech-only filter — both would otherwise disagree with what the tracker
  actually charges.

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

**Open questions:**
- Does it live in the monorepo as a third Go module, or inside `backend/` as a
  second `cmd/`? The migration tool is its own module, which is a precedent either
  way depending on whether the CLI should share `internal/` types.
- Is the CLI a thin REST client, or does it get direct database access for
  break-glass operations? Thin-client is far safer and keeps one authorisation
  path; direct access is what you want at 3am when the API is down. They are
  different tools and probably should not be one binary.
- Generated from the OpenAPI spec, or hand-written? Generated stays in sync for
  free; hand-written gives the ergonomics that are the whole point.
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

**What exists today.** `middleware.RequireAuth` extracts a Bearer token and hands
it to a `SessionValidator`, which resolves a Redis-backed session (decision 4:
short-lived JWT + Redis sessions for revocation). Authorisation downstream is the
user's class and their group capability columns.

**How much the middleware has to change depends entirely on the scope answer:**
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
every check, not just at the middleware. There is precedent in this codebase: the
`can_feed` work found handlers waving everyone through when the check was nil, and
the lesson recorded from it was that a fail-closed guarantee belongs at every layer
that can decide.

**Open questions:**
- Truly non-expiring, or expiring-with-renewal? Non-expiring is the ask; a
  last-used timestamp plus an inactivity sweep is a middle ground that keeps the
  ergonomics.
- Are tokens revocable individually, and visible to the user in a management page?
  (Both, surely — but that is UI work that comes with the feature.)
- What is the permission vocabulary? This is the hard part, and it is the same
  question as the RBAC discussion below.
- Are token actions distinguishable in the activity log? They should be — "who did
  this" is different from "which key did this".

## Open architectural question: RBAC vs class/level parity

Raised alongside PF-28 and recorded here because it does not belong to any single
feature.

The site currently authorises on **class/level plus group capability columns**
(`can_upload`, `can_chat`, `can_feed`, …), which is what original TorrentTrader
did and what this port deliberately mirrors. The observation is that **RBAC would
be the better model** — and that PF-28's fine-grained tokens need a permission
vocabulary that RBAC would supply naturally.

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

# Dependencies and build order

Most of Part A converges on three pieces of shared machinery. Building those
first turns several later items into small features.

```
                    ┌──────────────────────┐
                    │ PF-3  Reputation     │  ledger + materialised column
                    │       (+ PF-11 docs) │
                    └──────────┬───────────┘
        ┌──────────┬───────────┼───────────┬──────────┬──────────┐
     PF-4       PF-5        PF-10       PF-12      PF-18      PF-1
    thanks     voting      adopt      multipliers  boards   designer pay

                    ┌──────────────────────┐
                    │ Escrow / trust acct  │
                    └──────────┬───────────┘
                     ┌─────────┴─────────┐
                  PF-1                 PF-17
              designer orders      request bounties

                    ┌──────────────────────┐
                    │ PF-2  Teams          │
                    └──────────┬───────────┘
              ┌───────────┬────┴──────┐
           PF-6         PF-14      PF-15
         team chat    colours      badges
```

**Suggested order:**

1. **PF-3 + PF-11 — reputation ledger.** Six items depend on it, and every one of
   them is cheap once it exists. Mirror `bonus_transactions`.
2. **PF-20 — anti-abuse spine.** Before the gamification is switched on, not
   after. It emits into the existing cheat-flag queue.
3. **PF-4 — thanks.** Small, greenfield, and the first real consumer of the
   ledger. A good shakedown for PF-3.
4. **PF-14 + PF-15 together — one adornment system.** Specify as a single feature.
   Needed by PF-2 anyway.
5. **PF-9 — dead-torrent definition.** One definition, then PF-8 and PF-10 both
   land quickly.
6. **PF-8 — reseed rules.** Small enhancement to a done feature.
7. **PF-10 — adopt.** Staff reassignment first; member self-adoption behind the
   flag, later.
8. **Escrow service → PF-1 and PF-17.** Build the held-funds/claim/fulfil/timeout/
   dispute state machine once, with two consumers.
9. **PF-5 — voting.** Needs PF-3 and PF-20 in place first.
10. **PF-2 — teams.** Large, and it gates PF-6.
11. **PF-16 — subscriptions.** Independent; can slot in anywhere. High value for
    the cost.
12. **PF-13, PF-12, PF-19 — the economy layer**, specified together so multiplier
    stacking is decided once. **PF-26 (freeleech) is the natural first slice** and
    can be built ahead of the rest: it depends on nothing else here, and it forces
    the two decisions the rest of the layer needs anyway — a site timezone, and
    the rule that effective state is computed at accrual time rather than written
    back onto the row.
13. **PF-7 — real-time.** Independent, but cheaper after there are more things
    worth updating live.
14. **PF-6 — chat rooms.** Largest single lift; wants teams first.
15. **PF-24 — version grouping.** Scope before estimating.

---

# Cross-cutting concerns

**One multiplier decision, made once.** PF-12 (reputation tiers), PF-13 (goals),
PF-19 (site events) and PF-26 (freeleech rules) all modify the same accounting.
Whether they stack additively or multiplicatively, and whether they apply at
accrual or read time, must be decided once and written down — otherwise the
economy becomes impossible to reason about or to audit.

PF-26 already has the right answer available to copy: `counted_downloaded_delta`
is computed per announce and stored, so the discount is fixed at the moment it is
granted and no later change can rewrite history. Every other modifier should work
the same way.

**Scheduled state must be computed, never written back.** Any feature that turns
something on for a window — freeleech, double bonus, a contest — is tempting to
implement as a cron job that flips a column and another that flips it back. That
destroys the distinction between a temporary automatic grant and a permanent
manual one, so the "off" job silently clears deliberate staff decisions, and
nothing records that it happened. Compute effective state at read/accrual time and
leave the manual columns meaning exactly what they mean today.

**Feature-flag sprawl.** Eleven of these items ask for their own admin on/off
switch. `site_settings` is the right home, and there is an established pattern
(seed migration → `Setting*` constant → validation case → frontend definition).
Worth a shared "features" section on the settings page rather than eleven toggles
scattered through the list.

**Capability vs badge vs role.** PF-1 (designer), PF-2 (team membership, team
moderator), PF-6 (VIP), PF-15 (badges) and PF-28 (token scopes) each introduce
something that looks like a role. The site currently has exactly one model: group
capability columns (`can_upload`, `can_chat`, `can_feed`, …). Decide deliberately
whether these new concepts extend that model or sit beside it — and keep badges
decorative, so they never become an informal second permission system. See the
RBAC note above: deferring the model change is a reasonable call, but every
role-shaped addition raises the eventual cost.

**Privacy.** Reputation, leaderboards, follow feeds and public thanks lists all
publish behaviour that is currently private. Some members will not want a public
profile of their activity. Worth one opt-out decision covering all of them, taken
before the first one ships rather than retrofitted after.

**Retroactivity.** PF-11 fixes the rule for reputation: changing a reward does not
re-award history. The same question arises for goals, multipliers and tier
thresholds. Same answer, stated once.
