# TorrentTrader 3.0

A modern rewrite of the classic TorrentTrader private tracker. Go backend, React frontend, standalone migration tool.

## Prerequisites

- **Go** 1.24+
- **Node.js** 22+
- **Docker** (for infrastructure services)
- **Taskfile** ([install](https://taskfile.dev/installation/))

Install Taskfile:

```bash
# macOS
brew install go-task

# Other platforms: https://taskfile.dev/installation/
```

## Quickstart (Development)

```bash
# Clone the repo
git clone <repo-url> && cd go-torrent-trader

# Copy environment config
cp .env.example .env

# Install dev tools (golangci-lint, air)
task tools

# Install frontend dependencies
task frontend:install

# Start infrastructure (Postgres, Redis, MinIO, Mailpit)
task dev:up

# Start backend + frontend with hot reload
task dev

# Backend: http://localhost:8080
# Frontend: http://localhost:5173
# Mailpit UI: http://localhost:8025
# MinIO Console: http://localhost:9001
```

## Project Structure

```
go-torrent-trader/
├── backend/          # Go API server (Chi router, goose migrations, pgx)
├── frontend/         # React SPA (Vite, TypeScript, React Router)
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
| `task dev` | Start full dev environment (infra + backend + frontend) |
| `task dev:up` / `task dev:down` | Start/stop infrastructure services |
| `task tools` | Install dev tools (golangci-lint, air) |
| `task docker:build` | Build all Docker images |
| `task generate` | Run code generation (API client types) |

Run `task --list` for the full list.

## Configuration

All configuration is via environment variables. See `.env.example` for available options.

## Frontend Configuration

The frontend uses **runtime configuration** so the same Docker image works across environments (dev, staging, production) without rebuilding.

### How it works

The frontend loads `/config.js` before the app starts. This file provides configuration like the API URL and site name.

### Docker deployment

The Docker entrypoint generates `config.js` from environment variables on container startup:

```bash
docker run -e API_URL=https://api.mytracker.com -e SITE_NAME="My Tracker" \
  williamokano/torrenttrader-frontend:latest
```

Available env vars for the frontend container:

| Variable | Default | Description |
|---|---|---|
| `API_URL` | `http://localhost:8080` | Backend API base URL |
| `SITE_NAME` | `TorrentTrader` | Site display name |

### Local development (without Docker)

When running `task frontend:dev`, Vite serves `public/config.js` which contains development defaults. Edit `frontend/public/config.js` to change them.

### Static hosting (without Docker)

After `npm run build`, the `dist/` folder contains everything needed. Edit `dist/config.js` before deploying to set your production values:

```js
window.__CONFIG__ = {
  API_URL: "https://api.mytracker.com",
  SITE_NAME: "My Tracker",
};
```

## Docker Deployment

### Portainer Stack

Use `docker-compose.stack.yml` for a complete production deployment via Portainer:

```bash
docker compose -f docker-compose.stack.yml up -d
```

Required environment variables (no defaults):

| Variable | Description |
|---|---|
| `POSTGRES_PASSWORD` | Database password |
| `JWT_SECRET` | Secret for token signing |
| `MINIO_ROOT_PASSWORD` | MinIO admin password |
| `S3_SECRET_KEY` | S3 storage secret (same as MinIO password) |

See `docker-compose.stack.yml` for all available configuration options.

### Build Docker images locally

```bash
task docker:build
```
