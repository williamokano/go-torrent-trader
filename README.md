# TorrentTrader 3.0

A modern rewrite of the classic TorrentTrader private tracker. Go backend, React frontend, standalone migration tool.

## Prerequisites

- **Go** 1.23+
- **Node.js** 22+
- **Taskfile** ([install](https://taskfile.dev/installation/))

Install Taskfile:

```bash
# macOS
brew install go-task

# Other platforms: https://taskfile.dev/installation/
```

## Quickstart

```bash
# Clone the repo
git clone <repo-url> && cd go-torrent-trader

# Copy environment config
cp .env.example .env

# Install frontend dependencies
task frontend:install

# Build all projects
task build

# Run tests
task test
```

## Project Structure

```
go-torrent-trader/
├── backend/          # Go API server (Chi router, sqlc, goose migrations)
├── frontend/         # React SPA (Vite, TypeScript, Tailwind CSS)
├── migration-tool/   # Standalone Go CLI for legacy DB migration (Cobra)
├── docs/             # Architecture docs, implementation tasks
└── Taskfile.yml      # Build orchestration
```

## Available Tasks

| Command | Description |
|---|---|
| `task build` | Build all projects |
| `task test` | Run all tests |
| `task lint` | Lint all projects |
| `task dev` | Start development environment |
| `task backend:build` | Build backend server |
| `task frontend:build` | Build frontend bundle |
| `task migration-tool:build` | Build migration CLI |

Run `task --list` for the full list.

## Configuration

All configuration is via environment variables. See `.env.example` for available options.
