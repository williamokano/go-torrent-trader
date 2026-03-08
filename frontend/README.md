# TorrentTrader Frontend

React single-page application for the TorrentTrader platform.

## Tech Stack

- **React 19** with TypeScript 5.7
- **Vite 6** for build and dev server
- **React Router 7** for client-side routing
- **openapi-fetch** for type-safe API calls
- **Vitest** + Testing Library for tests
- **ESLint** + **Prettier** for code quality

## Project Structure

```
frontend/
├── src/
│   ├── api/               # OpenAPI client and generated types
│   ├── features/auth/     # Auth context, token management
│   ├── pages/             # Page components
│   │   └── admin/         # Admin panel pages
│   ├── components/        # Reusable UI components
│   │   ├── form/          # Input, Select, Textarea, Checkbox
│   │   ├── toast/         # Toast notifications
│   │   ├── modal/         # Modal dialogs
│   │   ├── Chat.tsx       # Floating side chat
│   │   └── Shoutbox.tsx   # Full-size home page chat
│   ├── layouts/           # RootLayout, AdminLayout
│   ├── routes/            # Router config, ProtectedRoute, AdminRoute
│   ├── themes/            # Light/dark/system theme provider
│   ├── lib/               # Chat WebSocket singleton and context
│   ├── utils/             # Formatting helpers (bytes, dates, etc.)
│   ├── types/             # Shared TypeScript types
│   ├── config.ts          # Runtime configuration
│   └── main.tsx           # Entry point with provider tree
├── public/config.js       # Runtime config placeholder (Docker)
├── index.html
├── vite.config.ts
├── vitest.config.ts
├── tsconfig.json
├── eslint.config.js
├── Dockerfile             # Multi-stage nginx build
├── docker-entrypoint.sh   # Injects runtime config
└── nginx.conf
```

## Pages

### Public
- **Login** — Email/username + password
- **Sign Up** — Registration (open or invite-only)
- **Forgot/Reset Password** — Email-based recovery

### Torrents
- **Browse** — Search, filter by category, sort, paginate
- **Upload** — Drag-drop .torrent file, category, description, NFO
- **Detail** — Stats, file list, NFO viewer, peers link, comments, ratings, uploader info, category breadcrumb
- **Edit** — Modify metadata (owner/staff)
- **Peers** — Active seeders/leechers for a torrent
- **Today's Torrents** — Last 24h uploads
- **Need Seed** — Torrents with 0 seeders

### Community
- **Messages** — Private messaging with inbox/outbox/compose, username autocomplete, reply threading
- **Shoutbox** — Real-time chat on home page (WebSocket)
- **Side Chat** — Floating collapsible chat on all other pages
- **Members** — User directory with search
- **Staff** — Staff/moderator listing
- **Invites** — Create and track invitation tokens

### User
- **Profile** — Stats, group badge, seeding/leeching counts, recent uploads, send message
- **Settings** — Avatar, title, bio, password, passkey

### Admin
- **Users** — List, search, edit (group, enabled, warned)
- **Reports** — Content moderation with resolve
- **Categories** — CRUD with hierarchical display
- **Groups** — Permission group management
- **Settings** — Site-wide configuration
- **Bans** — IP (CIDR) and email pattern bans

### Other
- **RSS Builder** — Custom RSS feed with filters
- **Activity Log** — Site-wide action history

## Configuration

The app reads configuration at runtime, allowing a single Docker image for all environments.

### Development
Set `VITE_API_URL` in the root `.env` file:
```
VITE_API_URL=http://localhost:8080
```

### Docker / Production
The `docker-entrypoint.sh` generates `/config.js` from environment variables at container start:
```
API_URL=https://api.example.com
SITE_NAME=MyTracker
```

This injects `window.__CONFIG__` which the app reads via `getConfig()`.

| Variable | Default | Description |
|----------|---------|-------------|
| `VITE_API_URL` / `API_URL` | `http://localhost:8080` | Backend API URL |
| `VITE_SITE_NAME` / `SITE_NAME` | `TorrentTrader` | Site display name |

## Architecture

### Provider Tree
```
StrictMode > ThemeProvider > ToastProvider > AuthProvider > ChatProvider > App
```

### Auth Flow
- Login/register returns access + refresh tokens, stored in `localStorage`
- `AuthProvider` restores session on mount via `/auth/me`
- `ProtectedRoute` redirects to `/login` if not authenticated
- `AdminRoute` redirects if not admin
- Token refresh happens automatically on 401

### Real-Time Chat
- **ChatSocket** — Module-level singleton managing the WebSocket connection (immune to React re-renders)
- **ChatProvider** — React context that subscribes to the singleton, shares messages/state across components
- **Chat** (side) — Floating collapsible panel, hidden when home page Shoutbox is mounted
- **Shoutbox** (home) — Full-width chat box, signals ChatProvider to hide the side chat

### API Client
Uses `openapi-fetch` with types generated from the backend's OpenAPI spec:
```bash
npm run generate:api
```

All API calls use `getConfig().API_URL` — never relative URLs.

### Theme System
- Three modes: `light`, `dark`, `system`
- Persisted in `localStorage`
- Applied via `data-theme` attribute on `<html>`
- CSS custom properties defined in `themes/tokens.css`

## Development

### Prerequisites
- Node.js 22+
- Backend running on `http://localhost:8080`

### Dev Server
```bash
npm run dev   # http://localhost:5173 with HMR
```

### Build
```bash
npm run build   # TypeScript check + Vite production build → dist/
```

### Preview
```bash
npm run preview   # Serve production build locally
```

### Test
```bash
npm test          # Vitest (watch mode)
npm test -- --run # Single run
```

### Lint & Format
```bash
npm run lint           # ESLint
npm run format         # Prettier (write)
npm run format:check   # Prettier (check only)
```

### Generate API Types
```bash
npm run generate:api   # From backend OpenAPI spec
```

## Docker

```bash
docker build -t torrenttrader-frontend .
docker run -p 8080:8080 -e API_URL=https://api.example.com torrenttrader-frontend
```

Multi-stage build:
1. **Builder** — Node 22 Alpine, `npm ci && npm run build`
2. **Runtime** — nginx-unprivileged Alpine, serves static files, injects runtime config
