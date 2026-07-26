# TorrentTrader

**A private BitTorrent tracker you run yourself.** Go backend, React frontend, one
`docker compose up`.

TorrentTrader is a ground-up rewrite of the classic PHP tracker of the same name —
the software a lot of small communities were built on in the 2000s, and which has
not aged well. This is that idea rebuilt: the same shape of community, without the
PHP, the Smarty templates, or the decade of unmaintained mods.

It is **open source and free of charge.** Fork it, change it, run it for your own
community — that is the point of the project, not a concession. Contributions back
are genuinely appreciated, but nobody owes them.

---

## What you get

A complete private tracker, not a starting point you have to finish.

**The tracker itself** — HTTP announce and scrape, passkey-authenticated, with
per-user ratio accounting, freeleech and half-credit torrents, configurable peer
limits and wait times, and an append-only announce log. Cheat detection runs off the
announce path and files flags into a staff queue: impossible upload speeds, upload
with no downloaders, and `left` values that disagree with what was transferred.

**Content** — uploads with per-category metadata schemas, full-text search,
comments and ratings, an RSS builder, a live release stream, and staff moderation
with a claim/approve/reject queue. Members can request a reseed on a dead torrent.

**A community, not just a file list** — forums with categories, moderation, edit
history and search; a WebSocket shoutbox; private messages with threads, drafts and
templates; `@mentions` that notify; news posts; and notifications delivered in-app,
pushed over WebSocket, and batched into email digests.

**Membership that manages itself** — invite trees with optional automatic
distribution, classes with per-capability permissions, automatic promotion and
demotion on ratio and seeding thresholds, warnings that escalate to restrictions and
bans on their own, per-user privilege revocation, and IP and email ban lists.

**An economy** — bonus points earned by seeding, an append-only ledger of every
movement, and a store that spends them.

**Operations** — an admin panel over every one of those systems, an activity log
that separates public entries from staff-only ones, database backups, and outbound
connectors that announce new uploads to Discord, Telegram, IRC, a webhook, the
shoutbox, or a live SSE feed — each with its own filters and message template.

**Nearly all of it can be switched off.** Roughly forty settings decide what your
site actually is. Do not want an economy? Turn bonus points off. Do not want
automatic promotions, or an anti-cheat system, or outbound announcements? Off, off,
off — from the admin panel, without a deploy.

## Run it

```bash
git clone https://github.com/williamokano/go-torrent-trader.git
cd go-torrent-trader
cp .env.example .env      # then edit it
docker compose -f docker-compose.stack.yml up -d
```

Full instructions, including reverse proxy and first-admin setup, are on the
**[project site](https://williamokano.github.io/go-torrent-trader/)**.

## Use the API directly

The web UI is one client, not a privileged one. Everything it does goes through a
documented REST API, and members are welcome to build their own tools against it —
`backend/api/openapi.public.yaml` is the published contract. Administrative
endpoints are deliberately kept out of that document; they exist, they are readable
in the source, but they are not advertised as a stable interface.

The tracker also speaks the parts of the BitTorrent protocol you would expect, and
exposes a passkey-authenticated RSS feed for clients that want to watch for new
releases.

## Migrating from the old TorrentTrader

`migration-tool/` is a standalone CLI that reads a legacy TorrentTrader 3.x MySQL
database and writes it into this one — users with their passwords intact, torrents
with their info hashes byte-for-byte, forums with BBCode converted to Markdown, and
live peers so your swarms survive the cutover. It is **still being built**; see the
[migration issues](https://github.com/williamokano/go-torrent-trader/issues?q=is%3Aissue+label%3Aarea%3Amigration-tool).

## Contributing

Work is tracked in [GitHub Issues](https://github.com/williamokano/go-torrent-trader/issues).
Setup, architecture, testing and the conventions this project holds itself to are in
**[DEVELOPMENT.md](DEVELOPMENT.md)**.

Forking and running your own modified version is explicitly encouraged — you do not
need permission and you do not need to contribute anything back. If you do want to
send something upstream, an issue first is usually the fastest route.

## Status

The tracker is feature-complete and stable. The migration tool is not yet finished.
Current work is visible in the issue tracker; nothing is hidden in a spreadsheet.
