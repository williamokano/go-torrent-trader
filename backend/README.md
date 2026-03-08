# TorrentTrader Backend

Go-based REST API and BitTorrent tracker for the TorrentTrader platform.

## Tech Stack

- **Go 1.24** with [Chi](https://github.com/go-chi/chi) router
- **PostgreSQL 16** with [Goose](https://github.com/pressly/goose) migrations
- **Redis** for sessions and background jobs
- **Asynq** for task queue and scheduled jobs
- **MinIO/S3** for torrent file storage
- **gorilla/websocket** for real-time chat

## Project Structure

```
backend/
├── cmd/server/          # Entry point (main.go)
├── internal/
│   ├── config/          # Environment variable configuration
│   ├── database/        # DB connection and migration runner
│   ├── event/           # Domain event bus (publish/subscribe)
│   ├── handler/         # HTTP handlers and router
│   ├── listener/        # Event listeners (activity log, email)
│   ├── middleware/       # Auth, CORS, logging, rate limiting
│   ├── model/           # Domain models
│   ├── repository/      # Data access interfaces
│   │   └── postgres/    # PostgreSQL implementations
│   ├── service/         # Business logic layer
│   ├── storage/         # File storage abstraction (local/S3)
│   ├── testutil/        # Shared test helpers
│   └── worker/          # Background job handlers and scheduler
├── migrations/          # 24 SQL migrations (Goose)
├── api/                 # OpenAPI specification
├── Dockerfile           # Multi-stage production build
└── go.mod
```

## Configuration

All configuration is via environment variables. See `.env.example` at the repo root.

| Variable | Default | Description |
|----------|---------|-------------|
| `DATABASE_URL` | *(required)* | PostgreSQL connection string |
| `REDIS_URL` | `redis://localhost:6379/0` | Redis connection string |
| `SERVER_HOST` | `0.0.0.0` | Listen address |
| `SERVER_PORT` | `8080` | Listen port |
| `SESSION_STORE` | `redis` | `redis` or `memory` |
| `ACCESS_TOKEN_TTL` | `1h` | Access token lifetime |
| `REFRESH_TOKEN_TTL` | `720h` | Refresh token lifetime (30 days) |
| `STORAGE_TYPE` | `local` | `local` or `s3` |
| `S3_ENDPOINT` | | S3-compatible endpoint |
| `S3_ACCESS_KEY` | | S3 access key |
| `S3_SECRET_KEY` | | S3 secret key |
| `S3_BUCKET` | | S3 bucket name |
| `SMTP_HOST` | `localhost` | SMTP server |
| `SMTP_PORT` | `1025` | SMTP port |
| `SMTP_FROM` | `noreply@torrenttrader.local` | Sender address |
| `SITE_NAME` | `TorrentTrader` | Site display name |
| `SITE_BASE_URL` | `http://localhost:5173` | Frontend URL (for emails) |
| `API_URL` | `http://localhost:8080` | Backend URL (for announce URLs) |
| `ENABLE_SCHEDULER` | `true` | Enable periodic background tasks |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |

## API Endpoints

### Public
| Method | Path | Description |
|--------|------|-------------|
| GET | `/healthz` | Health check |
| GET | `/announce` | BitTorrent announce (passkey auth) |
| GET | `/scrape` | BitTorrent scrape |
| GET | `/api/v1/stats` | Site statistics |
| GET | `/api/v1/categories` | Category list |
| GET | `/api/v1/rss` | RSS feed (passkey auth) |

### Auth (`/api/v1/auth`)
| Method | Path | Description |
|--------|------|-------------|
| POST | `/register` | Create account |
| POST | `/login` | Login (returns tokens) |
| POST | `/refresh` | Refresh access token |
| POST | `/forgot-password` | Request password reset |
| POST | `/reset-password` | Reset password with token |
| POST | `/logout` | Invalidate session |
| GET | `/me` | Current user profile + permissions |

### Torrents (`/api/v1/torrents`)
| Method | Path | Description |
|--------|------|-------------|
| GET | `/` | List/search (full-text, filters, pagination) |
| POST | `/` | Upload torrent |
| GET | `/{id}` | Detail (files, peers, category breadcrumb) |
| PUT | `/{id}` | Edit metadata |
| DELETE | `/{id}` | Delete (owner/staff) |
| GET | `/{id}/download` | Download .torrent file |
| GET | `/{id}/comments` | List comments |
| POST | `/{id}/comments` | Add comment |
| GET | `/{id}/rating` | Rating stats |
| POST | `/{id}/rating` | Rate (1-5 stars) |
| POST | `/{id}/reseed` | Request reseed |

### Messages (`/api/v1/messages`)
| Method | Path | Description |
|--------|------|-------------|
| POST | `/` | Send message |
| GET | `/inbox` | Inbox (paginated) |
| GET | `/outbox` | Sent messages |
| GET | `/{id}` | View message (auto-marks read) |
| DELETE | `/{id}` | Soft delete for current user |
| GET | `/unread-count` | Unread count |

### Chat
| Method | Path | Description |
|--------|------|-------------|
| WS | `/ws/chat` | WebSocket (token auth via query param) |
| GET | `/api/v1/chat/history` | Message history (paginated) |
| DELETE | `/api/v1/chat/{id}` | Delete message (staff only) |

### Admin (`/api/v1/admin`)
| Method | Path | Description |
|--------|------|-------------|
| GET | `/users` | List users |
| PUT | `/users/{id}` | Edit user |
| GET | `/groups` | List permission groups |
| GET/PUT | `/settings`, `/settings/{key}` | Site settings |
| CRUD | `/bans/ips`, `/bans/emails` | IP and email bans |
| CRUD | `/categories` | Category management |

### Other
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/users/` | Member list |
| GET | `/api/v1/users/staff` | Staff list |
| GET | `/api/v1/users/{id}` | User profile |
| GET | `/api/v1/invites/` | List invites |
| POST | `/api/v1/invites/` | Create invite |
| GET | `/api/v1/activity-logs/` | Activity log |
| POST | `/api/v1/reports/` | Submit report |

## Architecture

### Layers
```
Handler (HTTP) → Service (business logic) → Repository (data access) → PostgreSQL
                     ↓
              Event Bus → Listeners (activity log, email notifications)
```

### Key Patterns
- **Repository interfaces** in `repository/repository.go`, PostgreSQL implementations in `repository/postgres/`
- **Event-driven**: Services publish domain events, listeners handle side effects
- **Dependency injection**: All dependencies passed via constructors, wired in `main.go`
- **Session store interface**: Redis (production) or in-memory (testing)
- **RBAC**: Groups with capabilities (`upload`, `download`, `comment`, `invite`, `forum`)

### WebSocket Chat
- Single writer goroutine per connection (write pump pattern)
- Buffered send channels with slow client eviction
- Session re-validation every 5 messages
- Rate limiting: 10 messages per 10-second window
- Origin validation via configurable allowed origins

## Development

### Prerequisites
- Go 1.24+
- PostgreSQL 16
- Redis 7
- MinIO (or S3-compatible, for file storage)

### Run
```bash
# From repo root
task backend:dev    # Hot reload with air
task backend:run    # Direct run
```

### Build
```bash
task backend:build  # Outputs ./server binary
```

### Test
```bash
task backend:test   # go test ./...
```

### Lint
```bash
task backend:lint   # golangci-lint run (v2.1.6)
```

### Docker
```bash
docker build -t torrenttrader-backend .
docker run -p 8080:8080 --env-file .env torrenttrader-backend
```

## Migrations

Migrations run automatically on startup via Goose. To run manually:

```bash
goose -dir migrations postgres "$DATABASE_URL" up
goose -dir migrations postgres "$DATABASE_URL" status
```

24 migrations covering: users, groups, torrents, peers, categories, comments, ratings, reports, forums, messages, chat, invites, bans, activity logs, site settings, password resets, reseed requests, and torrent file lists.

## Links

- [Source Code](https://github.com/williamokano/go-torrent-trader)
- [Releases & Changelog](https://github.com/williamokano/go-torrent-trader/releases)
- [Issues](https://github.com/williamokano/go-torrent-trader/issues)
