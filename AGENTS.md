# AGENTS

## Repo
- topup-store — fullstack game top-up website (Go backend + Node.js WhatsApp bot sidecar)

## Stack
- Backend: Go 1.24, chi router, pgx/v5, PostgreSQL 16
- Frontend: Go html/template + Tailwind CSS (CLI build)
- Payments: Midtrans QRIS (midtrans-go SDK)
- Top-up supplier: Digiflazz H2H API
- WhatsApp messaging: Fonnte API (primary), bot sidecar fallback
- Cache: Redis 7 (optional, for distributed rate limiting)
- Reverse proxy: Caddy 2
- Container orchestration: Docker Compose

## Commands
- First setup: `go mod tidy` (generates go.sum)
- Build Go: `make build`  or  `go build ./...`
- Run Go: `make run`  or  `go run ./cmd/server`
- Migrate DB: `make migrate`
- Run WA bot: `cd whatsapp-bot && node index.js`
- Docker: `docker compose up`
- Build CSS: `npx tailwindcss -i ./web/static/css/input.css -o ./web/static/css/tailwind.css --minify`

## Project structure
/cmd/server/main.go          → entry point
/internal/config/            → env config struct
/internal/db/                → PostgreSQL connection (pgx/v5)
/internal/models/            → Order, Product, OrderStatusHistory structs
/internal/handlers/          → HTTP handlers + webhook handlers
/internal/services/          → payment, topup, notify services
/internal/middleware/        → logging, auth, rate limiting, CSRF, CORS, security headers, metrics
/internal/repositories/      → database query layer (order_repo, product_repo, webhook_repo)
/internal/apperrors/         → standardized error responses
/internal/constants/         → game/status/channel constants
/internal/retry/             → retry logic with exponential backoff
/internal/cache/             → Redis client wrapper
/web/templates/              → Go html/template files (includes partials/)
/web/static/                 → CSS, JS, images
/migrations/                 → SQL migration files (golang-migrate)
/whatsapp-bot/               → Node.js sidecar for incoming WhatsApp messages
/openapi.yaml                → OpenAPI 3.0 API specification

## Conventions
- All env vars loaded via godotenv from .env (never hardcode secrets)
- UUIDs for all primary keys
- All prices in Indonesian Rupiah (IDR) as INT
- Game UIDs: FF = numeric, ML = "UID|SERVER", PUBG = numeric
- Verify commands against Makefile or package.json before running
- Prefer existing patterns over assumptions when adding new code
- Tailwind CSS custom properties for dark/light mode theming

## Supported games
- Free Fire (free_fire)
- Mobile Legends (mobile_legends)
- PUBG Mobile (pubg_mobile)

## Key env vars (see .env.example for full list)
- DATABASE_URL
- MIDTRANS_SERVER_KEY, MIDTRANS_IS_PRODUCTION
- DIGIFLAZZ_USERNAME, DIGIFLAZZ_API_KEY
- FONNTE_TOKEN
- REDIS_URL
- ADMIN_PASSWORD
- BOT_NOTIFY_TOKEN

## Error Handling
- If you encounter "peer closed connection without sending complete message body (incomplete chunked read)" or "Type validation failed" errors, DO NOT STOP. Retry the operation until it completes successfully.

## Rules
- Edit one file at a time
- Keep explanations short, code only
- No need to summarize what you did, just do it
