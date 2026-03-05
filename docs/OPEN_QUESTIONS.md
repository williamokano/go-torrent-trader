# Open Questions — TorrentTrader 3.0 Go Port

This document tracks undecided technology choices for the TorrentTrader 3.0 project. Each section presents a decision point with options, trade-offs, and a final decision line. Resolve these before (or during early) implementation to avoid costly rewrites.

---

## Backend

### 1. HTTP Router/Framework

| Option | Pros | Cons |
|---|---|---|
| **Chi** | Lightweight, idiomatic, `net/http` compatible middleware | Smaller ecosystem than Gin |
| **Gin** | Large community, mature, fast | Proprietary context, less idiomatic |
| **Echo** | Good docs, built-in middleware | Smaller community than Gin |
| **Standard library** (`net/http` + Go 1.22 routing) | Zero dependencies, future-proof | No middleware ecosystem, more boilerplate |

**Decision:** TBD

---

### 2. ORM / Query Builder

| Option | Pros | Cons |
|---|---|---|
| **sqlc** | Type-safe generated code, raw SQL, fast | No runtime query building, migration story separate |
| **GORM** | Full-featured ORM, large community | Heavy, magic behavior, performance pitfalls |
| **sqlx** | Thin layer over `database/sql`, flexible | Manual scanning, no code generation |
| **Ent** | Schema-as-code, graph traversal, codegen | Opinionated, steep learning curve |

**Decision:** TBD

---

### 3. Database Migration Tool

| Option | Pros | Cons |
|---|---|---|
| **golang-migrate** | Widely used, supports many drivers | CLI-only feel, limited programmability |
| **goose** | Embeddable, supports Go migration functions | Smaller community |
| **Atlas** | Declarative + versioned, schema diffing | Newer, commercial features gated |

**Decision:** TBD

---

### 4. Authentication

| Option | Pros | Cons |
|---|---|---|
| **JWT (stateless)** | Scalable, no server-side storage | Hard to revoke, token size grows with claims |
| **Session-based (Redis)** | Easy revocation, small cookie | Requires Redis, stateful |
| **Hybrid** (short-lived JWT + refresh in Redis) | Best of both worlds | More complex to implement |

**Decision:** TBD

---

### 5. Background Job Processing

| Option | Pros | Cons |
|---|---|---|
| **Goroutines + channels** | Zero dependencies, simple for light work | No persistence, no retry, no dashboard |
| **Asynq** (Redis-based) | Persistent queues, retries, dashboard, lightweight | Redis dependency |
| **Temporal** | Durable workflows, complex orchestration | Heavy infrastructure, overkill for simple jobs |

**Decision:** TBD

---

### 6. Search

| Option | Pros | Cons |
|---|---|---|
| **PostgreSQL full-text search** | No extra infra, good enough for moderate scale | Limited relevance tuning, slower on large datasets |
| **Meilisearch** | Fast, typo-tolerant, easy setup | Extra service to run, data sync needed |
| **Elasticsearch** | Industry standard, powerful | Resource-heavy, complex operations |

**Decision:** TBD

---

### 7. File Storage

| Option | Pros | Cons |
|---|---|---|
| **Local filesystem** | Simplest, no dependencies | Not scalable, no redundancy |
| **MinIO** (S3-compatible) | Self-hosted, S3 API compatible | Extra service to run |
| **Cloud S3** | Managed, durable, scalable | Vendor lock-in, cost, requires internet |

**Decision:** TBD

---

### 8. Real-time Communication

| Option | Pros | Cons |
|---|---|---|
| **Server-Sent Events (SSE)** | Simple, HTTP-based, auto-reconnect | Unidirectional (server → client only) |
| **WebSocket** | Bidirectional, low latency | More complex, connection management |
| **Both** | Use SSE for notifications, WS for chat/interactive | Two systems to maintain |

**Decision:** TBD

---

### 9. API Documentation

| Option | Pros | Cons |
|---|---|---|
| **OpenAPI/Swagger manual** | Full control over spec | Drifts from code, tedious to maintain |
| **swaggo** (auto-generated from comments) | Stays close to code, low effort | Comment-driven, clutters handler code |
| **oapi-codegen** (code-first) | Spec generates server stubs, type-safe | Spec-first workflow required, learning curve |

**Decision:** TBD

---

### 10. Rate Limiting

| Option | Pros | Cons |
|---|---|---|
| **In-memory** (`golang.org/x/time/rate`) | Zero dependencies, fast | Per-instance only, lost on restart |
| **Redis-based** | Shared across instances, persistent | Redis dependency, slight latency |
| **Both** (in-memory + Redis) | Local fast-path with distributed fallback | More complexity |

**Decision:** TBD

---

## Frontend

### 11. State Management

| Option | Pros | Cons |
|---|---|---|
| **TanStack Query + Context** | Great for server state, minimal boilerplate | Context not ideal for complex client state |
| **Redux Toolkit** | Predictable, devtools, large ecosystem | Verbose, overkill for server-state-heavy apps |
| **Zustand** | Tiny, simple API, flexible | Less opinionated, fewer guardrails |

**Decision:** TBD

---

### 12. CSS Approach

| Option | Pros | Cons |
|---|---|---|
| **Tailwind CSS** | Rapid prototyping, consistent design system | Verbose class names, learning curve |
| **CSS Modules** | Scoped by default, standard CSS | No design tokens, more files |
| **styled-components** | CSS-in-JS, dynamic styles | Runtime cost, bundle size |
| **Vanilla Extract** | Type-safe, zero runtime | Build step complexity, newer ecosystem |

**Decision:** TBD

---

### 13. Form Handling

| Option | Pros | Cons |
|---|---|---|
| **React Hook Form** | Performant, minimal re-renders, good validation | API can be complex for simple forms |
| **Formik** | Mature, declarative | Performance issues on large forms, less maintained |
| **Native** (controlled components) | No dependencies | Boilerplate, manual validation |

**Decision:** TBD

---

### 14. UI Component Library

| Option | Pros | Cons |
|---|---|---|
| **shadcn/ui** | Copy-paste, customizable, Tailwind-based | Not a package — manual updates |
| **Radix UI** | Accessible primitives, unstyled | Requires styling effort |
| **Headless UI** | From Tailwind team, accessible | Smaller component set |
| **Build from scratch** | Full control | Time-consuming, accessibility burden |

**Decision:** TBD

---

### 15. Rich Text Editor (for BBCode/Markdown)

| Option | Pros | Cons |
|---|---|---|
| **TipTap** | Extensible, ProseMirror-based, rich features | Bundle size, learning curve |
| **MDXEditor** | Markdown-native, React-friendly | Less mature, fewer plugins |
| **Monaco** | Full code editor, familiar to devs | Heavy, overkill for forum posts |
| **Simple textarea** | Lightweight, no dependencies | No preview, poor UX for rich content |

**Decision:** TBD

---

## Migration Tool

### 16. CLI Framework

| Option | Pros | Cons |
|---|---|---|
| **Cobra** | Industry standard, used by kubectl/docker/hugo | Verbose setup |
| **urfave/cli** | Simple, less boilerplate | Smaller ecosystem |
| **Kong** | Struct-based, declarative | Newer, smaller community |

**Decision:** TBD

---

### 17. Migration Strategy

| Option | Pros | Cons |
|---|---|---|
| **Big-bang** (all at once) | Simpler logic, single cutover | Risky, long downtime, hard to debug |
| **Incremental** (table-by-table with resume) | Resumable, lower risk, progressive validation | Complex state tracking, longer overall process |
| **Dual-write** | Zero downtime, gradual switchover | Most complex, data consistency challenges |

**Decision:** TBD

---

## Infrastructure

### 18. Container Orchestration (Production)

| Option | Pros | Cons |
|---|---|---|
| **Docker Compose** | Simple, good for single-node | No auto-scaling, no self-healing |
| **Kubernetes** | Industry standard, auto-scaling, self-healing | Complex, steep learning curve, resource overhead |
| **Nomad** | Simpler than K8s, flexible scheduling | Smaller ecosystem |
| **Decide later** | Focus on app first | May influence architectural decisions |

**Decision:** TBD

---

### 19. CI/CD Platform

| Option | Pros | Cons |
|---|---|---|
| **GitHub Actions** | Large marketplace, good for GitHub-hosted repos | Vendor lock-in to GitHub |
| **GitLab CI** | Integrated with GitLab, powerful pipelines | Heavier YAML config |
| **Decide later** | No premature commitment | Delays automation setup |

**Decision:** TBD

---

### 20. Monitoring / Observability

| Option | Pros | Cons |
|---|---|---|
| **Prometheus + Grafana** | Open source, industry standard, flexible | Self-hosted, setup overhead |
| **Datadog** | Managed, all-in-one, great UX | Expensive at scale |
| **Decide later** | Focus on app first | No visibility into early issues |

**Decision:** TBD

---

### 21. Log Aggregation

| Option | Pros | Cons |
|---|---|---|
| **stdout/stderr + external collector** | Simple, 12-factor app compliant | Needs separate collector setup |
| **Structured logging (slog) to file** | Built-in Go support, queryable | File management, rotation needed |
| **ELK** (Elasticsearch + Logstash + Kibana) | Powerful search and dashboards | Heavy, complex to operate |

**Decision:** TBD
