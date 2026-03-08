# TorrentTrader Migration Tool

CLI tool for migrating data from a legacy TorrentTrader 3.x MySQL database to the new PostgreSQL schema.

## Tech Stack

- **Go 1.23** with [Cobra](https://github.com/spf13/cobra) CLI framework

## Project Structure

```
migration-tool/
├── cmd/migrate/         # CLI entry point and command definitions
│   ├── main.go          # Root command setup
│   ├── discover.go      # Discover source tables
│   ├── validate.go      # Validate source schema
│   ├── run.go           # Execute migration
│   ├── verify.go        # Verify migrated data
│   └── rollback.go      # Rollback migration
├── internal/
│   ├── config/          # Configuration loading (flags + env vars)
│   ├── source/          # Source DB connector (MySQL) — planned
│   ├── target/          # Target DB connector (PostgreSQL) — planned
│   ├── transform/       # Data transformation logic — planned
│   └── verify/          # Verification logic — planned
├── Dockerfile           # Multi-stage build
└── go.mod
```

## Commands

| Command | Description | Status |
|---------|-------------|--------|
| `discover` | List tables and row counts in the source database | Planned |
| `validate` | Check source DB schema matches expected TorrentTrader format | Planned |
| `run` | Execute the migration from source to target | Planned |
| `verify` | Verify migrated data integrity and completeness | Planned |
| `rollback` | Truncate target tables to undo a migration | Planned |

## Configuration

Configuration via CLI flags with environment variable fallbacks.

### Flags

| Flag | Env Var | Required | Description |
|------|---------|----------|-------------|
| `--source` | `MIGRATION_SOURCE_DSN` | Yes | Source MySQL DSN |
| `--target` | `MIGRATION_TARGET_DSN` | Yes | Target PostgreSQL DSN |
| `--log-level` | | No | `debug`, `info`, `warn`, `error` (default: `info`) |
| `--dry-run` | | No | Preview changes without writing (default: `false`) |

### Example

```bash
migration-tool run \
  --source "mysql://root:password@localhost:3306/torrenttrader_legacy" \
  --target "postgres://torrenttrader:password@localhost:5432/torrenttrader?sslmode=disable"
```

Or via environment variables:

```bash
export MIGRATION_SOURCE_DSN="mysql://root:password@localhost:3306/torrenttrader_legacy"
export MIGRATION_TARGET_DSN="postgres://torrenttrader:password@localhost:5432/torrenttrader?sslmode=disable"
migration-tool run
```

## Current State

The CLI skeleton is complete with all commands registered and tested. The actual migration logic (database connectors, schema introspection, data transformation, and verification) is not yet implemented.

### Implemented
- Root CLI with Cobra framework
- 5 subcommand stubs
- Persistent flags (source, target, log-level, dry-run)
- Config loading from flags + env var fallback
- Dockerfile with multi-stage build
- Unit tests for CLI structure and config

### Planned
- MySQL source connector and schema reader
- PostgreSQL target connector and writer
- Data transformers (users, torrents, forums, comments, etc.)
- BBCode to Markdown converter
- Resumable migration with progress tracking
- Verification suite (row counts, data integrity checks)
- Dry-run mode with diff output

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

### Docker
```bash
docker build -t torrenttrader-migration .
docker run --rm \
  -e MIGRATION_SOURCE_DSN="mysql://..." \
  -e MIGRATION_TARGET_DSN="postgres://..." \
  torrenttrader-migration run
```

## Migration Scope

The tool migrates from TorrentTrader 3.x (PHP/MySQL) to the new Go/PostgreSQL platform:

| Data | Source (MySQL) | Target (PostgreSQL) |
|------|---------------|-------------------|
| Users | users table | users + groups |
| Torrents | torrents table | torrents + categories |
| Forums | forums, topics, posts | forums, forum_topics, forum_posts |
| Comments | comments | comments + ratings |
| Messages | messages | messages |
| Peers | peers | peers |
| Invites | invites | invites |

Password hashes are migrated as-is. The backend supports legacy hash verification (SHA1) with transparent re-hashing to Argon2id on next login.
