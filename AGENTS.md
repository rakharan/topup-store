# AGENTS

## Repo
- topup-store — fullstack game top-up website (Go backend + Node.js WhatsApp bot)

## Stack
- Backend: Go 1.22, chi router, pgx/v5, PostgreSQL
- Frontend: Go html/template + Tailwind CSS (CDN)
- Payments: Midtrans QRIS (midtrans-go SDK)
- Top-up supplier: Digiflazz H2H API
- WhatsApp bot: Baileys (@whiskeysockets/baileys), Node.js 20, runs as sidecar on port 3001

## Commands
- First setup: `go mod tidy` (generates go.sum)
- Build Go: `make build`  or  `go build ./...`
- Run Go: `make run`  or  `go run ./cmd/server`
- Migrate DB: `make migrate`
- Run WA bot: `cd whatsapp-bot && node index.js`
- Docker: `docker compose up`

## Project structure
/cmd/server/main.go          → entry point
/internal/config/            → env config struct
/internal/db/                → PostgreSQL connection (pgx/v5)
/internal/models/            → Order, Product, User structs
/internal/handlers/          → HTTP handlers + webhook handlers
/internal/services/          → payment, topup, notify services
/internal/middleware/         → logging, auth, rate limiting
/web/templates/              → Go html/template files
/web/static/                 → CSS, JS
/migrations/                 → SQL migration files
/whatsapp-bot/               → Node.js Baileys sidecar

## Conventions
- All env vars loaded via godotenv from .env (never hardcode secrets)
- UUIDs for all primary keys
- All prices in Indonesian Rupiah (IDR) as INT
- Game UIDs: FF = numeric, ML = "UID|SERVER", PUBG = numeric
- Verify commands against Makefile or package.json before running
- Prefer existing patterns over assumptions when adding new code

## Supported games
- Free Fire (free_fire)
- Mobile Legends (mobile_legends)
- PUBG Mobile (pubg_mobile)

## Key env vars (see .env.example for full list)
- MIDTRANS_SERVER_KEY, MIDTRANS_IS_PRODUCTION
- DIGIFLAZZ_USERNAME, DIGIFLAZZ_API_KEY
- DATABASE_URL
- WHATSAPP_NUMBER
- ADMIN_PASSWORD