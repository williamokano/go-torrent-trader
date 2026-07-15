# Extensibility Direction — Events & Plugins

**Status: exploratory. Nothing here is committed or scheduled.** This captures a
maintainer discussion about how to add mod-style features (ratio multipliers,
bonus-shop boosts, and beyond) *without* editing core files every time — so the
reasoning is durable when the decision is actually faced. See
[`TRACKER_MODS.md`](./TRACKER_MODS.md) for the catalogue of features this is
about.

## The problem it solves

In TorrentTrader there was no extension mechanism, so every mod edited the hot
paths (`announce.php`, `takeupload.php`) directly. This rewrite already splits
schema / logic / presentation, but one drift risk remains: **every
ratio-affecting feature wants a branch in the announce path.** Per-torrent
freeleech and silver are there now (`countedDownload` in
`internal/service/tracker.go`). Next come global/scheduled freeleech, freeleech
tokens, a bought 24h double-upload, per-group multipliers, seedbox multipliers —
and naively each is another `if` in the announce. The announce is the most
critical, most-tested path in the system; growing a branch per policy there is
exactly the drift we want to avoid.

Two directions were discussed, from lightest to heaviest. They are not
alternatives to each other — the first is a stepping stone to the second.

## Direction 1 — a single stats resolver (recommended first step)

The instinct is "emit a raw stats event from the announce, let listeners apply
multipliers." That instinct is right, but the naive form — *many listeners, each
mutating the user's stats* — is the wrong shape. It scatters ratio logic across
handlers, makes composition depend on listener registration order, and destroys
the property that today's `countedDownload` is a **pure, trivially-tested
function**.

The right shape is **one resolver, many inputs**:

```
effectiveDelta(user, torrent, rawUp, rawDown) -> (countedUp, countedDown)
```

A single function that consults everything relevant — the torrent's Free/Silver
flags, a global-freeleech site setting or window, a double-upload the user
bought from the bonus shop, a group perk — and returns the adjusted deltas. New
multipliers become **new inputs to the resolver**, not new branches in the
announce. The announce keeps calling one function; the policy all lives in one
place with a defined composition (e.g. freeleech beats a bought 2×; multipliers
on the same side multiply).

Crucially this can be done **synchronously, with no event bus at all**. It is a
refactor of one function, not an architectural change:

- Extract `countedDownload` (and a symmetric `countedUpload`) into
  `effectiveDelta`.
- Feed it the site-settings service and, later, the bonus ledger.
- Keep it pure and table-test it the way `countedDownload` is tested now.

This removes the "branch per mod" problem and takes on **zero async-accounting
risk**. It is the recommended first move, and the freeleech PR's
`countedDownload` is literally its seed.

### If/when it goes async

If multiplier resolution later needs to react to things the announce doesn't
know (an external freeleech schedule, a real-time shop purchase), the announce
can emit `PeerStatsRecorded{userID, torrentID, rawUp, rawDown}` — it already has
to write the raw peer baseline for the next delta anyway — and the resolver runs
in a listener. Before doing that, weigh:

- **Consistency window.** `IncrementStats` is synchronous inside the announce
  today. Behind an async listener, the user's ratio lags their announce, and a
  dropped event silently loses accounting. Mitigation: keep *this* event
  synchronous, or make the listener idempotent against the raw peer row so it
  can be replayed.
- **Ordering / composition.** Still needs the single resolver — do not let
  multiple listeners each write stats. The event only moves *where* the resolver
  runs, not *how many* there are.

## Direction 2 — a plugin model (heavier; only if drift persists)

The deeper goal is that mods stop editing `tracker.go` / `router.go` / services
at all — behaviour is added through plugins, and "the core" becomes a thin
orchestrator. The honest split:

### Reaction-side is already pluggable

The event bus (`event/` → `listener/`) is a reaction-side extension point
**today**. A plugin that only *reacts* — an IRC/Discord announce bot, an
achievements engine, an external notifier — is just a registered listener. It
needs nothing new and cannot break a core flow, because it runs after the fact
and its failure is isolated. Any mod that is purely "when X happens, also do Y"
belongs here and is low-risk.

### Decision-side is the hard part

A plugin that must *change* a flow — veto an announce, alter a stats delta, add a
route, add a shop item — is where it gets tricky, exactly as noted in the
discussion: **a decision-side plugin can break a flow it never textually
touched.** It can return a wrong veto, resolve a nonsensical multiplier, or panic
mid-announce. Making that safe needs:

- **Narrow, well-typed interfaces**, not "here is the whole request, do
  anything." Examples: `StatsResolver` (Direction 1's function as an interface),
  `AnnounceInterceptor` (may only veto with a reason), `RouteProvider`,
  `ShopItemProvider`.
- **A resolver / chain with defined composition** rather than free-for-all
  mutation — so the result of several plugins is predictable.
- **Recover-and-isolate** around every plugin call: one bad plugin degrades to a
  logged skip, never a 500 on announce.
- **A capability boundary**: a stats plugin cannot mount routes; a route plugin
  cannot touch accounting.

### Compile-time, not hot-loaded

Full dynamic plugins (separately compiled, loaded at runtime) are heavy in Go
and rarely worth the operational cost and the safety surface. The pragmatic form
is **compile-time plugins**: internal packages that register against these
interfaces at startup. The core is a thin orchestrator; features are
self-contained modules that never edit the orchestrator. That captures most of
the anti-drift benefit without runtime-loading risk.

## Recommended sequencing

Do **not** build a framework first. Let the pattern earn each step:

1. **Extract the stats resolver** (Direction 1) — synchronous, pure, one
   function. This alone solves the immediate freeleech/multiplier drift and is a
   small, safe refactor.
2. **Prove the reaction side is already pluggable** by building IRC/Discord
   announce and/or achievements as pure event listeners. If that feels clean, the
   reaction-side plugin story needs no new machinery.
3. **Introduce exactly one decision-side interface** where the pain is real —
   promote the stats resolver from step 1 to a `StatsResolver` interface with a
   registry — and live with it for a while.
4. **Generalise to a plugin model only if** steps 1–3 show the interface pattern
   repeatedly earning its keep across more join points.

The whole point is that steps 1 and 2 deliver most of the value with almost none
of the risk, and each later step is optional.
