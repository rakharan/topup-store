# Project Status — TopUp Store

**Date:** 2026-04-28
**Version:** 1.0.0
**Stack:** Go 1.22 (chi router, pgx/v5) + Node.js 20 (WhatsApp Cloud API) + PostgreSQL 16 + Tailwind CSS (CLI)

---

## 1. Implemented Files & Purpose

### Entry Point & Config
| File | Purpose |
|------|---------|
| `cmd/server/main.go` | HTTP server entry point. Wires all services, middleware, routes, and background tickers (order expiry + Digiflazz polling). Graceful shutdown via SIGINT/SIGTERM. |
| `internal/config/config.go` | Loads `.env` via godotenv into a `Config` struct. Validates required env vars at startup. |
| `internal/db/db.go` | PostgreSQL connection pool via `pgx/v5` with configurable max/min conns, lifetime, idle time. |

### Models
| File | Purpose |
|------|---------|
| `internal/models/models.go` | Go structs: `Product`, `Order`, `OrderQRIS`, `OrderStatusHistory`, `WebhookLog`. All fields mapped to DB columns. |

### Repositories (Data Access Layer)
| File | Purpose |
|------|---------|
| `internal/repositories/interfaces.go` | Interface definitions: `OrderRepository`, `ProductRepository`, `WebhookRepository`. |
| `internal/repositories/order_repo.go` | `PGOrderRepository` — CRUD for orders, status transitions (`UpdateStatusIf`), QRIS upsert, status history, pagination, `ExpireOldPending`, `ListProcessing`. |
| `internal/repositories/product_repo.go` | `PGProductRepository` — CRUD for products, `ExistsBySKU`, `SyncPrice`, `CreateFromDigiflazz`, soft delete, list by game/all. Includes `cost_price_idr` and `product_type`. |
| `internal/repositories/webhook_repo.go` | `PGWebhookRepository` — Logs all webhook payloads to `webhooks_log` table, paginated listing. |

### Services (Business Logic)
| File | Purpose |
|------|---------|
| `internal/services/interfaces.go` | Interface definitions: `PaymentServiceInterface`, `TopupServiceInterface`, `NotifyServiceInterface`. |
| `internal/services/payment.go` | `PaymentService` — Midtrans Snap QRIS creation, order CRUD passthrough to repo, QRIS data storage. |
| `internal/services/topup.go` | `TopupService` — Digiflazz H2H API integration (order submission, status check, price list fetch), `SyncPricesWithAutoCreate` with tiered/fixed/percent margin calculation, `detectProductType()` keyword matching, `calcTieredPrice()` logic. |
| `internal/services/notify.go` | `NotifyService` — Sends WhatsApp messages via Cloud API (primary) or bot sidecar (fallback). Phone number normalization (0→62). Retry with exponential backoff. |

### Handlers (HTTP Layer)
| File | Purpose |
|------|---------|
| `internal/handlers/pages.go` | Page rendering: Home, OrderForm, Status, Admin (password login via HMAC-signed cookie, CSRF-protected). Template parsing via `filepath.WalkDir`. |
| `internal/handlers/orders.go` | API order endpoints: `CreateOrder` (with input validation), `GetOrder`, `ListOrders`, `LookupOrder`. Validates game UID format per game. |
| `internal/handlers/products.go` | API product endpoints: `ListProducts` (by game or all), `GetProduct`. |
| `internal/handlers/admin.go` | Admin API: `ProcessOrder`, `RetryOrder`, product CRUD (`CreateProduct`, `UpdateProduct`, `DeleteProduct`), `SyncPrices` (manual), `SyncPricesFromDigiflazz` (auto-fetch + upsert). Validates `product_type`. |
| `internal/handlers/webhooks.go` | Webhook receivers: `Midtrans` (SHA-512 signature verification, status mapping), `Digiflazz` (HMAC-SHA1 signature verification, ref_id lookup, status update + serial number save). All payloads logged to `webhooks_log`. |

### Middleware
| File | Purpose |
|------|---------|
| `internal/middleware/auth.go` | `BasicAuth` (unused), `AdminAuth` (HMAC-signed cookie validation, 1-hour expiry). |
| `internal/middleware/csrf.go` | `CSRFStore` (in-memory, mutex-protected, 2-hour TTL, auto-cleanup). `CSRFMiddleware` — generates token on GET, validates on POST/PUT/DELETE. Supports header and form field. |
| `internal/middleware/cors.go` | CORS middleware — allows specific origins (not `*` when configured), sets methods/headers, handles preflight. |
| `internal/middleware/ratelimit.go` | In-memory rate limiter per IP with sliding window cleanup. Trusts `X-Forwarded-For` only from localhost. |
| `internal/middleware/security.go` | Security headers: `X-Content-Type-Options`, `X-Frame-Options: SAMEORIGIN`, `Content-Security-Policy`, `HSTS`, `Permissions-Policy`, `Referrer-Policy`. |
| `internal/middleware/logging.go` | Structured JSON logging via `slog` with method, path, status, duration, request ID, remote addr. `ResponseWriter` wrapper for status capture. |
| `internal/middleware/requestid.go` | Generates UUID per request, sets `X-Request-ID` header, stores in context. |
| `internal/middleware/timeout.go` | `http.TimeoutHandler` wrapper with configurable duration. |
| `internal/middleware/maxbody.go` | Rejects requests exceeding configured body size (default 1MB). |
| `internal/middleware/metrics.go` | In-memory Prometheus-style metrics: request counts and duration averages per route. `MetricsMiddleware` wraps all requests. |

### Error Handling & Constants
| File | Purpose |
|------|---------|
| `internal/apperrors/errors.go` | Standardized error types (`ErrNotFound`, `ErrInvalidInput`, etc.) and JSON response format (`APIResponse` with `success`/`error`/`request_id`). |
| `internal/constants/constants.go` | All enums: order statuses, games, channels, API URLs, validation maps (`ValidGames`), display labels. |

### Retry Logic
| File | Purpose |
|------|---------|
| `internal/retry/retry.go` | Exponential backoff with jitter (3 attempts, 1s base, 10s max). Context-aware cancellation. |

### Database Migrations (17 files)
| File | Purpose |
|------|---------|
| `migrations/001_init.sql` | Core schema: `products`, `orders` tables with enums, indexes. |
| `migrations/002_add_expired_status.sql` | Adds `expired` to order status enum. |
| `migrations/003_seed_products.sql` | Seed data for FF, ML, PUBG products. |
| `migrations/004_add_serial_number.sql` | Adds `serial_number` column to orders. |
| `migrations/005_add_indexes.sql` | Performance indexes on orders and products. |
| `migrations/006_add_product_updated_at.sql` | Adds `updated_at` to products. |
| `migrations/007_deduplicate_products.sql` | Deduplication via `ctid` (pre-existing duplicates). |
| `migrations/008_add_constraints.sql` | Adds foreign key and check constraints. |
| `migrations/009_add_performance_indexes.sql` | Additional composite indexes. |
| `migrations/010_add_digiflazz_ref_id.sql` | Adds `digiflazz_ref_id` to orders. |
| `migrations/011_order_status_history.sql` | Creates `order_status_history` table. |
| `migrations/012_schema_migrations.sql` | Creates `schema_migrations` tracking table. |
| `migrations/013_soft_delete_products.sql` | Adds `deleted_at` to products for soft delete. |
| `migrations/014_move_qris_data.sql` | Moves QRIS data from orders to separate `order_qris` table. |
| `migrations/015_add_cost_price.sql` | Adds `cost_price_idr` to products. |
| `migrations/016_webhooks_log.sql` | Creates `webhooks_log` table for audit trail. |
| `migrations/017_add_product_type.sql` | Adds `product_type` column (diamond/subscription/other) with backfill. |

### Frontend Templates
| File | Purpose |
|------|---------|
| `web/templates/index.html` | Landing page with game tabs (FF/ML/PUBG), product cards, "Beli via WA" buttons. |
| `web/templates/order.html` | Manual order form: game selector, package dropdown, UID, server (ML), phone, CSRF token. |
| `web/templates/status.html` | Order status lookup by order ID. Shows status badge, details, QRIS info. |
| `web/templates/admin.html` | Password-protected admin panel: order list, process/retry buttons, product CRUD table, price sync from Digiflazz, cost/selling price auto-calc with tiered/fixed % margin selector, product type dropdown. |
| `web/templates/partials/` | Shared partials: `head.html`, `nav.html`, `toast.html`, `footer.html`. |

### Static Assets
| File | Purpose |
|------|---------|
| `web/static/css/input.css` | Tailwind CSS input with custom theme (primary orange `#F97316`, dark bg `#1C1C1E`). |
| `web/static/css/tailwind.css` | Generated minified Tailwind output (built by Docker/css-builder stage). |
| `web/static/js/` | Client-side JS (order form handling, status polling, admin interactions). |
| `web/static/favicon.ico` | Favicon icon. |

### WhatsApp Bot (Node.js Sidecar)
| File | Purpose |
|------|---------|
| `whatsapp-bot/index.js` | Express server on port 3001. Handles Meta Cloud API webhook verification + message parsing. Parses order format (`FF 100 UID:12345`), calls Go backend `/api/orders`, sends QRIS image back. `/notify` endpoint (token-protected) for success/failure notifications. Graceful shutdown (SIGTERM/SIGINT). |
| `whatsapp-bot/package.json` | Dependencies: express, axios, dotenv. |
| `whatsapp-bot/Dockerfile` | Node 20-alpine, non-root user, healthcheck on `/health`. |

### Infrastructure
| File | Purpose |
|------|---------|
| `Dockerfile` | Multi-stage: Node (CSS build) → Go (binary build) → Alpine (runtime). Non-root user, healthcheck. |
| `docker-compose.yml` | 4 services: postgres (16-alpine, healthcheck), migrate (runs all SQL), app (Go backend :8080), whatsapp-bot (Node :3001). Shared network, volume for pgdata. |
| `Makefile` | Commands: `run`, `dev` (air), `build`, `test`, `migrate`, `docker-up`, `docker-down`, `css`, `css-watch`. |
| `.dockerignore` | Excludes `.git`, `node_modules`, `.env`, binaries from Docker context. |
| `tailwind.config.js` | Tailwind config with custom colors, content paths for Go templates. |
| `package.json` | Root package for Tailwind CLI build. |

---

## 2. Working vs Placeholder

### Fully Working
| Feature | Status |
|---------|--------|
| PostgreSQL connection pool | Working |
| Product CRUD (admin + API) | Working |
| Order creation via API | Working |
| QRIS payment generation (Midtrans) | Working |
| Midtrans webhook handler | Working (SHA-512 sig verify) |
| Digiflazz order submission | Working (with retry, status check, error handling) |
| Digiflazz webhook handler | Working (HMAC-SHA1 sig verify, ref_id lookup) |
| Digiflazz price sync + auto-create | Working (tiered/fixed/percent margin) |
| WhatsApp Cloud API notifications | Working (with bot fallback) |
| WhatsApp bot message parsing | Working (FF/ML/PUBG format) |
| WhatsApp bot `/notify` endpoint | Working (token-protected) |
| Admin panel (login, orders, products) | Working (HMAC cookie auth, CSRF) |
| Order expiry ticker (30 min) | Working (background goroutine) |
| Digiflazz polling (30 sec) | Working (checks processing orders) |
| Rate limiting | Working (per-IP, sliding window) |
| CSRF protection | Working (in-memory store, all forms) |
| Security headers | Working (CSP, HSTS, etc.) |
| Request ID tracing | Working (UUID per request) |
| Structured logging | Working (slog, JSON) |
| Metrics endpoint | Working (Prometheus-style at `/metrics`) |
| Health endpoint | Working (DB ping check) |
| Soft delete for products | Working |
| Order status history | Working |
| Webhook audit log | Working |
| Docker Compose full stack | Working (build, migrate, run) |
| Cost price + auto selling price calc | Working (tiered + fixed % in admin form) |
| Product type detection | Working (keyword-based: weekly, monthly, pass, etc.) |

### Placeholder / Partial
| Feature | Status | Notes |
|---------|--------|-------|
| Midtrans webhook SHA-512 | Needs verification | Midtrans docs may specify SHA-256; currently using SHA-512 |
| CORS origins | Placeholder | Defaults to empty (no origins allowed); must set `ALLOWED_ORIGINS` |
| Digiflazz webhooks | Not triggering in test mode | Digiflazz test mode doesn't send webhooks; polling compensates |
| Product seed data | Partial | Migration 003 has placeholder SKUs; real SKUs need to be filled |
| User-facing order cancellation | Not implemented | No endpoint for users to cancel pending orders |
| `/ready` health endpoint | Not implemented | Only `/health` exists (liveness, not readiness) |
| Prometheus metrics | Incomplete | No `_bucket` histogram entries; only avg duration |
| BasicAuth middleware | Defined but unused | `AdminAuth` cookie-based auth is used instead |
| `ListByStatus` repo method | Defined but unused | Not wired to any handler |
| `Metrics` struct (non-Prometheus) | Defined but unused | `PrometheusMetrics` is used instead |
| Mobile menu JS | Copy-pasted across templates | Not extracted to shared JS file |
| Table accessibility | Partial | Missing `scope="col"` on admin table headers |
| Skip-to-content link | Not implemented | Accessibility gap for keyboard users |
| Root `package.json` test script | Placeholder | `echo "Error: no test specified" && exit 1` |
| `make migrate` on Windows | Not supported | Requires local `psql` binary |

---

## 3. External Integrations

| Integration | Protocol | Status | Notes |
|-------------|----------|--------|-------|
| **Midtrans** (QRIS payments) | HTTPS API (midtrans-go SDK) | Connected | Sandbox mode by default. Creates Snap transactions, receives webhooks at `/webhook/midtrans`. |
| **Digiflazz** (top-up supplier) | HTTPS API (REST) | Connected | Test mode enabled (`DIGIFLAZZ_TESTING=true`). Submits top-up orders, checks status, fetches price list. Webhooks at `/webhook/digiflazz` (not firing in test mode; polling compensates). |
| **WhatsApp Cloud API** (Meta) | HTTPS API (graph.facebook.com) | Connected | Sends text/image messages. Requires valid access token (24h temp or permanent system user token). Phone number ID required. |
| **WhatsApp Bot Sidecar** | HTTP (internal :3001) | Connected | Receives incoming WhatsApp messages via Meta webhook, parses orders, calls Go backend. `/notify` endpoint for outbound notifications (token-protected). |
| **PostgreSQL** | TCP (pgx/v5) | Connected | Connection pool with health check. Runs in Docker as `postgres:16-alpine`. |

---

## 4. Known Issues & TODOs

### From TODO.md (Unresolved)
- [ ] Midtrans webhook signature: verify SHA-512 vs SHA-256 against Midtrans docs (`webhooks.go`)
- [ ] `http.FileServer` serves entire `web/static/` — no allowlist/blocklist (`main.go`)
- [ ] No user-facing endpoint to cancel an order (`orders.go`)
- [ ] No separate `/ready` health endpoint for readiness probes (`main.go`)
- [ ] `BasicAuth` middleware defined but never wired (`auth.go`)
- [ ] `Metrics` struct defined but never instantiated (`metrics.go`)
- [ ] `ListByStatus` repository method defined but never called (`order_repo.go`)
- [ ] `PrometheusMetrics` handler outputs incomplete format — no `_bucket` entries (`metrics.go`)
- [ ] No migration version tracking table — relies on `IF NOT EXISTS` idempotency (`migrations/`)
- [ ] `game_uid` lookup ambiguous for ML — stores `UID|SERVER` but lookup receives separately (`order_repo.go`)
- [ ] `rand.Read` error silently returns empty CSRF token (`csrf.go`)
- [ ] Fire-and-forget goroutines use `context.Background()` — survive server shutdown (`webhooks.go`, `orders.go`)
- [ ] `os.Exit(1)` in server goroutine bypasses deferred cleanup (`main.go`)
- [ ] Request body not drained on early JSON decode failure (`orders.go`, `admin.go`)
- [ ] `json.NewEncoder(w).Encode()` errors never checked (`errors.go`)
- [ ] `strconv.Atoi` errors silently ignored in product handler (`products.go`)
- [ ] Mobile menu toggle code copy-pasted in all 4 templates (`index.html`, `order.html`, `status.html`, `admin.html`)
- [ ] Table headers missing `scope="col"` in admin tables (`admin.html`)
- [ ] No skip-to-content link for keyboard users (`nav.html`)
- [ ] Root `package.json` has placeholder test script
- [ ] `make migrate` requires local `psql` — won't work on Windows
- [ ] `healthHandler` uses duck-typed `interface{}` — unconventional (`main.go`)
- [ ] Test files ignore `json.Decode` errors — assertions could silently pass (`handlers_test.go`)

### Operational Notes
- Digiflazz webhooks don't fire in test mode; the 30-second polling ticker compensates
- Midtrans sandbox QRIS codes can be scanned with any QR app; payment auto-confirms
- WhatsApp Cloud API tokens expire every 24h unless using a permanent system user token
- CSRF tokens are in-memory; container restart clears all tokens (requires browser refresh)
- Admin auth cookie expires after 1 hour; re-login required

---

## 5. Project Structure

```
topup-store/
├── .air.toml                          # Air hot-reload config
├── .dockerignore                      # Docker build context exclusions
├── .env                               # Environment variables (gitignored)
├── .env.example                       # Template for env vars
├── .gitignore
├── AGENTS.md                          # AI assistant instructions
├── Dockerfile                         # Multi-stage Go + Node build
├── Makefile                           # Build/run/test commands
├── PROJECT_STATUS.md                  # This file
├── README.md                          # Setup & usage docs
├── go.mod                             # Go module definition
├── go.sum                             # Go dependency checksums
├── package.json                       # Root Node deps (Tailwind)
├── package-lock.json
├── postman-collection.json            # API test collection
├── tailwind.config.js                 # Tailwind CSS config
├── TODO.md                            # Audit findings tracker
├── PHASE.md                           # Original build phases (reference)
│
├── cmd/
│   └── server/
│       └── main.go                    # Entry point, route wiring, tickers
│
├── internal/
│   ├── apperrors/
│   │   └── errors.go                  # Standardized error types & responses
│   ├── config/
│   │   └── config.go                  # Env var loading & validation
│   ├── constants/
│   │   └── constants.go               # Enums, labels, validation maps
│   ├── db/
│   │   └── db.go                      # PostgreSQL connection pool
│   ├── handlers/
│   │   ├── admin.go                   # Admin API (orders, products, sync)
│   │   ├── handlers_test.go           # Handler unit tests
│   │   ├── mocks_test.go              # Test mocks
│   │   ├── orders.go                  # Order API (create, get, list, lookup)
│   │   ├── pages.go                   # Page rendering (home, order, status, admin)
│   │   ├── products.go                # Product API (list, get)
│   │   └── webhooks.go                # Midtrans + Digiflazz webhook handlers
│   ├── middleware/
│   │   ├── auth.go                    # BasicAuth + AdminAuth (cookie)
│   │   ├── cors.go                    # CORS headers
│   │   ├── csrf.go                    # CSRF token generation/validation
│   │   ├── logging.go                 # Structured request logging
│   │   ├── maxbody.go                 # Request body size limit
│   │   ├── metrics.go                 # Prometheus-style metrics
│   │   ├── middleware_phase4_test.go  # Middleware tests
│   │   ├── middleware_test.go         # Middleware tests
│   │   ├── ratelimit.go               # Per-IP rate limiting
│   │   ├── requestid.go               # Request ID generation
│   │   ├── security.go                # Security headers (CSP, HSTS, etc.)
│   │   └── timeout.go                 # Request timeout handler
│   ├── models/
│   │   └── models.go                  # Product, Order, OrderQRIS, etc.
│   ├── repositories/
│   │   ├── interfaces.go              # Repo interface definitions
│   │   ├── order_repo.go              # Order CRUD, status, QRIS, history
│   │   ├── product_repo.go            # Product CRUD, sync, soft delete
│   │   └── webhook_repo.go            # Webhook audit logging
│   ├── retry/
│   │   └── retry.go                   # Exponential backoff with jitter
│   └── services/
│       ├── interfaces.go              # Service interface definitions
│       ├── notify.go                  # WhatsApp notifications (Cloud API + bot)
│       ├── payment.go                 # Midtrans QRIS creation
│       ├── services_test.go           # Service unit tests
│       └── topup.go                   # Digiflazz integration, price sync
│
├── migrations/
│   ├── 001_init.sql                   # Core schema (products, orders)
│   ├── 002_add_expired_status.sql     # Add 'expired' status
│   ├── 003_seed_products.sql          # Seed product data
│   ├── 004_add_serial_number.sql      # Add serial_number to orders
│   ├── 005_add_indexes.sql            # Performance indexes
│   ├── 006_add_product_updated_at.sql # Add updated_at to products
│   ├── 007_deduplicate_products.sql   # Deduplicate existing products
│   ├── 008_add_constraints.sql        # FK and check constraints
│   ├── 009_add_performance_indexes.sql# Additional indexes
│   ├── 010_add_digiflazz_ref_id.sql   # Add digiflazz_ref_id to orders
│   ├── 011_order_status_history.sql   # Status history table
│   ├── 012_schema_migrations.sql      # Migration tracking table
│   ├── 013_soft_delete_products.sql   # Soft delete for products
│   ├── 014_move_qris_data.sql         # QRIS data to separate table
│   ├── 015_add_cost_price.sql         # Cost price column
│   ├── 016_webhooks_log.sql           # Webhook audit table
│   └── 017_add_product_type.sql       # Product type (diamond/subscription)
│
├── web/
│   ├── static/
│   │   ├── css/
│   │   │   ├── input.css              # Tailwind input with custom theme
│   │   │   └── tailwind.css           # Generated minified output
│   │   ├── favicon.ico
│   │   ├── images/
│   │   └── js/
│   └── templates/
│       ├── admin.html                 # Admin panel (orders + products)
│       ├── index.html                 # Landing page (game tabs, products)
│       ├── order.html                 # Manual order form
│       ├── status.html                # Order status lookup
│       └── partials/
│           ├── footer.html
│           ├── head.html
│           ├── nav.html
│           └── toast.html
│
├── whatsapp-bot/
│   ├── .dockerignore
│   ├── .env
│   ├── .env.example
│   ├── Dockerfile
│   ├── index.js                       # WhatsApp bot (Express + Cloud API)
│   ├── package.json
│   └── package-lock.json
│
└── docker-compose.yml                 # 4-service orchestration
```

---

## 6. Environment Variables

### Go Backend (Required)
| Variable | Description | Default |
|----------|-------------|---------|
| `PORT` | Server port | `8080` |
| `DATABASE_URL` | PostgreSQL connection string | *(required)* |
| `DB_MAX_CONNS` | Max pool connections | `25` |
| `DB_MIN_CONNS` | Min pool connections | `5` |
| `DB_MAX_CONN_LIFETIME` | Max connection lifetime | `1h` |
| `DB_MAX_CONN_IDLE_TIME` | Max idle time before close | `30m` |
| `MIDTRANS_SERVER_KEY` | Midtrans server key | *(required)* |
| `MIDTRANS_IS_PRODUCTION` | `true` for production, `false` for sandbox | `false` |
| `DIGIFLAZZ_USERNAME` | Digiflazz account username | *(required)* |
| `DIGIFLAZZ_API_KEY` | Digiflazz API key | *(required)* |
| `DIGIFLAZZ_WEBHOOK_SECRET` | HMAC secret for Digiflazz webhook verification | `topup store df wh 2024` |
| `DIGIFLAZZ_API_URL` | Digiflazz transaction API URL | `https://api.digiflazz.com/v1/transaction` |
| `DIGIFLAZZ_TESTING` | `true` for test mode | `true` |
| `ADMIN_PASSWORD` | Admin panel password | *(required)* |
| `REQUEST_TIMEOUT` | HTTP request timeout | `30s` |
| `ALLOWED_ORIGINS` | Comma-separated CORS allowed origins | *(empty = none)* |
| `LOG_LEVEL` | Log level (`debug`, `info`, `warn`, `error`) | `info` |

### Go Backend (WhatsApp Notifications)
| Variable | Description | Default |
|----------|-------------|---------|
| `WHATSAPP_NUMBER` | Business WhatsApp number (for wa.me links) | *(optional)* |
| `WHATSAPP_TOKEN` | Meta WhatsApp Cloud API access token | *(optional)* |
| `WHATSAPP_PHONE_NUMBER_ID` | WhatsApp phone number ID from Meta | *(optional)* |
| `WA_BOT_BASE_URL` | WhatsApp bot sidecar URL | `http://localhost:3001` |
| `WA_BOT_TOKEN` | Shared secret for bot `/notify` endpoint | *(optional)* |
| `BOT_NOTIFY_TOKEN` | Shared secret for bot `/notify` endpoint (bot side) | *(optional)* |

### WhatsApp Bot Sidecar (Node.js)
| Variable | Description | Default |
|----------|-------------|---------|
| `GO_BACKEND_URL` | Go backend URL | `http://localhost:8080` |
| `BOT_PORT` | Bot server port | `3001` |
| `WHATSAPP_TOKEN` | Meta WhatsApp Cloud API access token | *(required for bot)* |
| `WHATSAPP_PHONE_NUMBER_ID` | WhatsApp phone number ID | *(required for bot)* |
| `WHATSAPP_VERIFY_TOKEN` | Webhook verification token | `topup-store-verify` |
| `BOT_NOTIFY_TOKEN` | Shared secret for `/notify` auth | *(optional)* |

### Docker Compose
| Variable | Description | Default |
|----------|-------------|---------|
| `POSTGRES_USER` | PostgreSQL username | `topup` |
| `POSTGRES_PASSWORD` | PostgreSQL password | `topup` |

---

## Architecture Diagram

```
┌─────────────┐     ┌──────────────────┐     ┌────────────────┐
│   Client     │────▶│  Go Backend :8080 │────▶│  PostgreSQL    │
│ (Web/WhatsApp)│    │  (chi router)     │     │  (pgx/v5 pool) │
└─────────────┘     │                  │     └────────────────┘
                    │  Middleware:      │
                    │  - CSRF           │     ┌────────────────┐
                    │  - Rate Limit     │────▶│  Midtrans      │
                    │  - Security Hdrs  │     │  (QRIS)        │
                    │  - Logging        │     └────────────────┘
                    │  - Metrics        │
                    │                  │     ┌────────────────┐
                    │  Services:        │────▶│  Digiflazz     │
                    │  - Payment        │     │  (H2H Top-up)  │
                    │  - Topup          │     └────────────────┘
                    │  - Notify         │
                    └────────┬─────────┘     ┌────────────────┐
                             │               │  WhatsApp      │
                             ▼               │  Cloud API     │
                    ┌──────────────────┐     │  (Meta)        │
                    │  WA Bot :3001    │◀───▶│                │
                    │  (Node/Express)  │     └────────────────┘
                    └──────────────────┘
```

## Supported Games
- **Free Fire** (`free_fire`) — UID: numeric
- **Mobile Legends** (`mobile_legends`) — UID: `UID|SERVER` format
- **PUBG Mobile** (`pubg_mobile`) — UID: numeric

## Product Types
- `diamond` — Standard diamond/UC packages
- `subscription` — Weekly, monthly, membership, pass products
- `other` — Miscellaneous products

## Margin Calculation (Admin Form)
- **Tiered (auto)**: <5k → 15% (min +200), 5k-50k → 10%, >50k → 5% (max +5k)
- **Fixed %**: User-defined percentage (default 10%)
- All prices rounded to nearest 100 IDR
