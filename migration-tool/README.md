# TorrentTrader Migration Tool

CLI tool for migrating data from a legacy TorrentTrader 3.x MySQL database to the new PostgreSQL schema.

## Tech Stack

- **Go 1.23** with [Cobra](https://github.com/spf13/cobra) CLI framework
- [go-sql-driver/mysql](https://github.com/go-sql-driver/mysql) for the source database
- [lib/pq](https://github.com/lib/pq) for the target, and
  [goose](https://github.com/pressly/goose) in tests to build the target schema
  from the backend's own migrations
- [yaml.v3](https://gopkg.in/yaml.v3) for the mapping file, which is generated with
  its comments intact

## Project Structure

```
migration-tool/
├── cmd/migrate/         # CLI entry point and command definitions
│   ├── main.go          # Root command setup
│   ├── source.go        # Shared "open the legacy database" plumbing
│   ├── print.go         # Report writing
│   ├── discover.go      # List tables, or describe one
│   ├── validate.go      # Compare against the TorrentTrader 3.0 baseline
│   ├── mapping.go       # Generate the column mapping file
│   ├── run.go           # Execute migration — not implemented
│   ├── verify.go        # Verify migrated data — not implemented
│   └── rollback.go      # Rollback migration — not implemented
├── internal/
│   ├── config/          # Configuration loading (flags + env vars)
│   ├── schema/          # Schema model shared by the baseline and the reader
│   ├── baseline/        # The TorrentTrader 3.0 schema, as shipped
│   ├── source/          # Source DB connector (MySQL) and schema reader
│   ├── compare/         # Baseline diff
│   ├── mapping/         # Mapping plan and YAML generation
│   ├── bbcode/          # BBCode to Markdown conversion
│   ├── target/          # PostgreSQL: schema declaration, connection, checks
│   │                    # and the batched writer
│   ├── testenv/         # Test-only: MySQL from the corpus, PostgreSQL from
│   │                    # the backend's migrations, and golden-file support
│   ├── transform/       # Data transformation logic — planned
│   └── verify/          # Verification logic — planned
├── Dockerfile           # Multi-stage build
└── go.mod
```

## Commands

| Command | Description | Status |
|---------|-------------|--------|
| `discover` | List tables, engines, row counts and column counts; describe one table with `--table` | Implemented |
| `validate` | Pre-flight: compare the source against the baseline, check the target has the schema this tool writes into, and check a mapping file against both | Implemented |
| `mapping` | Generate the reviewable YAML column mapping | Implemented |
| `run` | Execute the migration from source to target | Not implemented — fails rather than exiting 0 |
| `verify` | Verify migrated data integrity and completeness | Not implemented — fails rather than exiting 0 |
| `rollback` | Truncate target tables to undo a migration | Not implemented — fails rather than exiting 0 |

The three unimplemented commands exit non-zero and say so. A cutover script that
read `migrate run` exiting 0 as "the migration ran" is the worst thing this tool
could do.

### Command flags

| Command | Flag | Description |
|---------|------|-------------|
| `discover` | `--exact` | Count rows with `COUNT(*)` instead of the engine's estimate |
| `discover` | `--table <name>` | Describe one table's columns and print its `SHOW CREATE TABLE` |
| `discover` | `--ddl` | Dump `SHOW CREATE TABLE` for every table |
| `validate` | `--strict` | Fail on any difference from the stock schema, not just blocking ones |
| `validate` | `--target <dsn>` | Also check the target database has the schema this tool writes into |
| `validate` | `--mapping <path>` | Also check a mapping file against the source and, with `--target`, the target |
| `mapping` | `--out <path>` | Where to write the mapping (default `mapping.yaml`; `-` for stdout) |
| `mapping` | `--force` | Overwrite an existing mapping file |

`--dry-run` works on `mapping`: it reports what would be written and writes
nothing.

## Configuration

Configuration via CLI flags with environment variable fallbacks.

### Flags

| Flag | Env Var | Required | Description |
|------|---------|----------|-------------|
| `--source` | `MIGRATION_SOURCE_DSN` | For `discover`, `validate`, `mapping` | Source MySQL DSN |
| `--target` | `MIGRATION_TARGET_DSN` | For `run`, `verify`, `rollback` | Target PostgreSQL DSN |
| `--log-level` | | No | `debug`, `info`, `warn`, `error` (default: `info`) |
| `--dry-run` | | No | Preview changes without writing (default: `false`) |

### Source DSN

Both spellings are accepted:

```
mysql://user:password@host:3306/torrenttrader
user:password@tcp(host:3306)/torrenttrader
user:password@unix(/var/run/mysqld/mysqld.sock)/torrenttrader
```

The URL form percent-decodes the password, so a password containing `@`, `/` or
`?` works if it is escaped (`p%40ss`). Query parameters are passed to the driver,
so `?tls=skip-verify` and friends work. A database name is required either way.

The password is never printed: logs and error messages show `user:***@host/db`.

### Example

```bash
migrate validate \
  --source "mysql://root:password@localhost:3306/torrenttrader_legacy"

migrate mapping \
  --source "mysql://root:password@localhost:3306/torrenttrader_legacy" \
  --out mapping.yaml
```

Or via environment variables:

```bash
export MIGRATION_SOURCE_DSN="mysql://root:password@localhost:3306/torrenttrader_legacy"
migrate discover --exact
```

## Current State

The tool reads a legacy database and tells you what a migration would involve.
It does not move data yet.

### Implemented

- MySQL connector, accepting both `mysql://user:pass@host:port/db` and the
  driver's own `user:pass@tcp(host:port)/db` spelling
- Schema discovery through `information_schema`, plus `SHOW CREATE TABLE`
- The TorrentTrader 3.0 baseline schema, transcribed from
  `docs/FULL_FEATURE_DOCUMENTATION.md` section 1
- Comparison against that baseline: missing tables, mod-added tables and
  columns, and type mismatches. A column the migration does not read is
  reported when it is missing but does not stop a run — 35 baseline columns
  are skipped outright, and an install that dropped one migrates fine
- Character set reporting. A stock 2008 TorrentTrader is `latin1` throughout
  and the target stores UTF-8, so `validate` names the encodings it found and
  the mapping records them per column. Copying those bytes across unconverted
  is what turns accented usernames into mojibake
- Mapping file generation, with the reasoning for each decision kept as comments
- Reading a mapping back, so a hand-edited file is checked rather than trusted
- PostgreSQL target connector, schema check, and a batched writer with
  per-batch or per-table transactions
- BBCode to Markdown conversion, for every piece of member-written text
- Integration tests against a real MySQL 8 server via testcontainers

### Planned

- Data transformers (users, torrents, forums, comments, etc.)
- Resumable migration with progress tracking
- Verification suite (row counts, data integrity checks)
- Dry-run mode with diff output

## The mapping file

`migrate mapping` writes a YAML file proposing where every column in *your*
database should land. It is generated from the database in front of it rather
than from the baseline, so a column some mod added ten years ago appears as an
entry to decide on instead of going missing.

Each column gets an action:

| Action | Meaning |
|--------|---------|
| `map` | Copy to the target column, through `transform` where the types differ |
| `derive` | The value reaches the target some other way — folded into another column, spread across a JSONB document, or turned into rows of another table |
| `skip` | Deliberately not migrated. The comment says why |
| `custom` | A mod added this column and only the operator knows what it is for |
| `review` | A stock column this tool has no rule for yet |

Each entry also records the column's source `type`, and its `charset` when that
is not Unicode, so the file can be reviewed — and consumed — without
reconnecting to the old database.

`custom` and `review` are the entries needing a decision; the command prints how
many there are and where. The file is meant to be edited and kept in version
control.

```yaml
    # Members keep their id and passkey, so existing .torrent files keep
    # announcing and every foreign key in the dump resolves without a
    # translation table.
    users:
        action: migrate
        target: users
        columns:
            # Carried across as-is, and it will not let anyone log in yet: the
            # the backend verifies this scheme and upgrades it on first login.
            password:
                type: varchar(40)
                charset: latin1
                action: map
                target: password_hash
                transform: legacy_hash
            # Reversed sense: forumbanned='yes' becomes can_forum=false.
            forumbanned:
                type: char(3)
                charset: latin1
                action: map
                target: can_forum
                transform: yes_no_to_bool_inverted
            # Who's-online scratch state.
            page:
                type: text
                charset: latin1
                action: skip
            seedbonus:
                type: float
                action: custom
        # Target columns filled from something other than a single legacy column.
        derived:
            password_scheme: the scheme the legacy hash is in. The backend verifies it and re-hashes to argon2id on the next login, so this value decides whether the member can log in at all
            warn_until: NULL — legacy warnings are a flag with no expiry
```

The file records the server it came from as `host:port/database`, with no
username and no password, because it is meant to be committed.

## BBCode

Every piece of member-written content in the legacy site is BBCode — torrent
descriptions, forum posts, private messages, comments — and this project is
Markdown-only. `internal/bbcode` converts it.

It runs once, over text people wrote, with nobody to check the result. Two
decisions follow from that.

**Malformed markup passes through as written.** Fifteen years of hand-typed
posts contain crossed tags, tags nobody closed, and square brackets that were
never markup. A closing tag closes the nearest matching open tag and anything
left open in between comes back as literal text, so `[quote][b]broken[/quote]`
keeps its quote and hands back the `[b]`. A stray `[b]` somebody can see is a
smaller loss than a post rearranged into something nobody typed.

**Text that was never markup is escaped.** BBCode gave no meaning to `*` or `_`
or `#`, so people typed them freely. Markdown gives all of them meaning:
`2*3*4` becomes partly italic, `snake_case_name` loses its underscores, and
`#1 release` becomes a heading. Text is escaped so it renders as it was
written; URLs and the inside of `[code]` are not, because escaping them there
would corrupt them instead.

| BBCode | Markdown |
|---|---|
| `[b]` `[i]` `[s]` | `**` `*` `~~` |
| `[u]` | `<u>` — Markdown has no underline, and dropping the tag loses the intent |
| `[url=x]y[/url]` | `[y](x)`, with `javascript:` and `data:` targets defused |
| `[img]x[/img]` | `![](x)` |
| `[quote=who]` | `> **who wrote:**`, every line marked so blank lines do not end the quote |
| `[code]` | a fence long enough to survive backticks in the content |
| `[list]` `[*]` | `-`, or `1.` for `[list=1]` |
| `[color]` `[size]` `[center]` | dropped, keeping the words |

`Options.Preserve` returns the text untouched, for an operator who intends to
render BBCode in the new site instead. The conversion is lossy in one direction
and nothing can undo it afterwards, so that choice belongs to them.

## What the baseline knows

`internal/baseline` holds the 37 tables section 1.1 of the reference document
lists. Nine of them — `users`, `groups`, `torrents`, `peers`, `completed`,
`messages`, `forum_forums`, `forum_topics`, `forum_posts` — are the ones section
1.2 breaks down, and only those have their columns checked. The rest are known
by name, and the tool says so rather than reporting every column of a table it
cannot describe as mod-added.

Six tables are marked required, meaning the migration cannot run without them:
`users`, `groups`, `torrents`, `peers`, `completed` and `categories`. A missing
optional table is reported and not treated as fatal — an install that dropped
polls years ago still migrates.

## The pre-flight check

`validate` is what an operator runs before a cutover. Given all three
connections it answers, in one pass and in seconds, the questions that
otherwise get answered halfway through a million rows:

```bash
migrate validate \
  --source  "mysql://root:password@old-host:3306/torrenttrader" \
  --target  "postgres://tt:password@new-host:5432/torrenttrader?sslmode=disable" \
  --mapping mapping.yaml
```

- **Is this a TorrentTrader database, and what has been done to it?** Missing
  tables, mod-added tables and columns, changed types, and the character set the
  text is stored in.
- **Does the target have the schema this tool writes into?** The ordinary
  failure is pointing at a database whose migrations were never run, which
  otherwise dies partway with a message about one column, having already
  written half the members.
- **Does the mapping fit both databases?** Every column it reads must exist in
  the source, every column it writes must exist in the target, every transform
  must be one that was written, and every `custom` or `review` entry must have
  been decided. All of them are reported at once: fixing a mapping one error per
  run, each run needing two database connections, is not a reasonable thing to
  ask of somebody mid-cutover.

A mapping that has drifted from its source — generated before a mod was added,
or against a different install — is caught in both directions. A column the
mapping reads that the database has lost is fatal; so is a column the database
has that the mapping has never heard of, because deciding silently that it does
not matter is exactly the failure the mapping exists to prevent.

## The writer

`internal/target` inserts in batches. Sending rows one at a time is the
difference between a migration that takes minutes and one that takes hours: the
cost is round trips, not inserts.

Two transaction modes, because the right answer differs by table:

| Mode | |
|---|---|
| `TxPerBatch` | Commits every batch. A failure leaves earlier batches committed, which is what makes a run resumable and a half-finished run visible |
| `TxPerTable` | Commits once at the end. A failure rolls the whole table back, so the target never holds a partial table, at the cost of a long-lived transaction |

The batch size is clamped rather than obeyed: PostgreSQL refuses more than
65535 bind parameters in one statement, so a 40-column table cannot take 2000
rows at once however large the configured batch. An operator asking for a bigger
batch wants speed, not an error about a limit they had no way to know.

`DeclaredTables` fixes the order tables are written in, so a table always
follows what it refers to. It is written down rather than derived from foreign
keys, because a topological sort silently picks one of many valid orders and the
one thing that must not happen is torrents landing before the members that own
them.

## What the target knows

`internal/target` declares the PostgreSQL tables and columns the mapping writes
into. It is declared here rather than imported because `migration-tool` is its
own module and may not import `backend/internal` — see **Shared Nothing** in
`docs/ARCHITECTURE.md`.

A declaration nothing checks drifts, and this one did: the first version of the
mapping named three target columns that were wrong, including a `forums`
permission column it claimed did not exist. So `internal/target` is checked two
ways:

- `target_live_test.go` applies the backend's own migrations to a real
  PostgreSQL and fails if the declaration disagrees with the result. It skips
  when those files are absent, so the module still builds standalone. Reading
  the migration SQL instead would answer a slightly different question, and did
  so wrongly: the parser it replaced missed three columns added by a single
  multi-column `ALTER TABLE`
- `internal/mapping/target_test.go` fails if a rule names a target column that
  does not exist, or if any target column has neither a rule nor a `derived`
  note — which is what stops a column being dropped by accident

`internal/target` also records which target tables ship with rows already in
them. `groups`, `categories`, `countries`, `languages` and `forum_categories`
are seeded, several with unique constraints, so the "keep the legacy id"
strategy that works everywhere else collides there. The mapping says so on each
of those tables.

## Development

### Build
```bash
# From repo root
task migration-tool:build
```

### Test
```bash
task migration-tool:test
```

Use `go test -short ./...` to skip the containers when Docker is unavailable.

## The verification harness

`internal/testenv` builds the two databases the tests run against:

- **MySQL**, loaded with the legacy corpus at
  `internal/testenv/testdata/legacy.sql`
- **PostgreSQL**, built by running the backend's own goose migrations, so the
  target schema is the real one rather than a transcription that drifts

Both are real servers rather than fakes, because what is most likely to be
wrong here is not the logic but the assumptions about what a server does: how
MySQL 8 reports a type it was given in 2008, whether a legacy zero date
survives the trip, what PostgreSQL does with a foreign key MyISAM never
enforced. A fake agrees with whatever its author believed, which is the thing
under test.

### The corpus is adversarial, not large

A million clean rows prove less than fifty awkward ones — migrations do not
fail on ordinary data. The corpus carries, each on a commented row:

| | |
|---|---|
| `'0000-00-00 00:00:00'` | MyISAM accepts it; PostgreSQL rejects it outright |
| `invited_by = 0` | the legacy "nobody" sentinel meeting a real foreign key |
| a 27-character username | against the target's `varchar(20)` |
| latin1 high bytes | `Café`, `Größe`, `Amélie` — the only proof the text is converted rather than copied |
| a malformed `info_hash` | not 40 hex characters, so it cannot become 20 bytes |
| orphans | a peer whose member was deleted, a completion whose torrent was |
| a duplicate passkey | UNIQUE in the target, unconstrained in MyISAM |
| a dangling `class` | pointing at a group that no longer exists |
| unclosed and nested BBCode | what fifteen years of hand-typed markup looks like |
| a torrent with no file rows | and another with several |

Strict mode is off for the load, which is how such rows came to exist in the
first place — MySQL 8 would otherwise refuse half of them.

### Golden files

`cmd/migrate/testdata/*.golden.*` hold what an operator actually reads: the
generated mapping, and the `validate` report.

They exist because counts cannot see a wrong value. A mapping that quietly
starts sending passkeys somewhere else still produces the right number of
entries. The golden files put the output itself in the pull request, so
changing what an operator is told has to be read and agreed with by somebody.

```bash
go test ./cmd/migrate -update   # regenerate, then read the diff
```

### Checking the target declaration

`internal/target/target_live_test.go` applies the backend's migrations to a real
PostgreSQL and reads back `information_schema`, rather than parsing the
migration SQL. That distinction earned its keep immediately: the parser it
replaced missed three columns added by a single multi-column `ALTER TABLE`.

`.github/workflows/migration-tool.yml` therefore triggers on
`backend/migrations/**` as well as `migration-tool/**`. Without that path a
backend schema change could not fail the check that exists to catch it.

### Docker
```bash
docker build -t torrenttrader-migration .

# Write a mapping into the current directory.
docker run --rm -v "$PWD:/out" \
  -e MIGRATION_SOURCE_DSN="mysql://user:password@host:3306/torrenttrader" \
  torrenttrader-migration mapping --out /out/mapping.yaml
```

The container runs as a non-root user, so the directory bound to `/out` has to
be writable by it.

## Migration Scope

The tool migrates from TorrentTrader 3.x (PHP/MySQL) to the new Go/PostgreSQL
platform. Target tables are as named in `backend/migrations`; the generated
mapping file is the authority on individual columns.

| Data | Source (MySQL) | Target (PostgreSQL) |
|------|---------------|-------------------|
| Members | `users`, `groups` | `users`, `groups` |
| Torrents | `torrents`, `files` | `torrents` (file lists as JSONB) |
| Categories | `categories` | `categories` |
| Swarm | `peers` | `peers` |
| Completions | `completed` | `transfer_history` |
| Forums | `forumcats`, `forum_forums`, `forum_topics`, `forum_posts` | `forum_categories`, `forums`, `forum_topics`, `forum_posts` |
| Comments and ratings | `comments`, `ratings` | `torrent_comments`, `torrent_ratings` |
| Messages | `messages` | `messages` |
| Shoutbox | `shoutbox` | `chat_messages` |
| Bans | `bans`, `email_bans` | `banned_ips`, `banned_emails` |

Two things must survive the cutover, and the mapping treats both as such:

- **Passkeys.** Every `.torrent` file a member has already downloaded announces
  with their passkey, so `users.passkey` is carried across unchanged.
- **Info hashes.** The legacy schema stores 40 hex characters; the target stores
  the 20 raw bytes the tracker compares against. A torrent whose hash does not
  survive stops announcing.

### Passwords survive the cutover

Password hashes are migrated as-is, and members log in with the password they
already had. The backend verifies the legacy schemes TorrentTrader's `passhash()`
produced — SHA1, MD5 and the HMAC-SHA1 variant — and **re-hashes to Argon2id on
that first successful login**, so each member walks the legacy path exactly once
and every login afterwards uses the modern hash.

`password_scheme` is what tells the backend which digest to compute, so it is the
column that decides whether a member can log in at all rather than a piece of
metadata. Getting it wrong locks them out.

If the old install used the HMAC variant, set `LEGACY_PASSWORD_SECRET` on the
backend to the site secret it was configured with. The SHA1 and MD5 variants are
unsalted and need nothing. Without it, affected members cannot be verified — the
backend logs that specifically rather than reporting a bad password, because
sending them to the reset form with no explanation is the worse failure.

**Optionally, wrap them instead.** With `--wrap-passwords` the migration stores
`argon2id(legacy_digest(password))` under a `wrapped_*` scheme, so no bare SHA1 or
MD5 is ever at rest in the new database. Members still log in with their original
password, at the cost of one extra digest per login until they upgrade. This is
the stronger position and worth taking if the cutover window allows it.

Passkeys are unaffected either way: those are copied exactly and keep working, so
existing `.torrent` files keep announcing.

Features the port deliberately dropped — polls, the widget system, server-side
themes, the word censor and others — have their tables marked `skip` with the
reason. See `docs/NOT_PORTING.md`.

## Links

- [Source Code](https://github.com/williamokano/go-torrent-trader)
- [Releases & Changelog](https://github.com/williamokano/go-torrent-trader/releases)
- [Issues](https://github.com/williamokano/go-torrent-trader/issues)
