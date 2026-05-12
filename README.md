# Faza TopUP Store

Fullstack game top-up platform with QRIS payments, Digiflazz H2H integration, and WhatsApp notifications via Fonnte.

## Features

- **QRIS Payments** — Instant QRIS generation via Midtrans
- **Game Top-Up** — Free Fire, Mobile Legends, PUBG Mobile
- **WhatsApp Notifications** — Order confirmations and status updates via Fonnte
- **Admin Dashboard** — Order management, product CRUD, analytics, webhook logs
- **Dark/Light Mode** — Toggle between themes, persists in localStorage
- **Auto-Retry** — Failed top-ups automatically retried with exponential backoff
- **Webhook Idempotency** — Duplicate webhook protection
- **Audit Logging** — Order status history, admin actions, webhook payloads
- **Rate Limiting** — Per-IP rate limits with Redis fallback
- **CSRF Protection** — PostgreSQL-backed token store
- **OpenAPI Documentation** — Full API spec at `openapi.yaml`

## Prerequisites

- **Go** 1.24+
- **Node.js** 20+ (for WhatsApp bot sidecar)
- **PostgreSQL** 16+
- **Redis** 7+ (optional, for distributed rate limiting)
- **Docker** & **Docker Compose** (recommended)

## Quick Start (Docker)

```bash
git clone https://github.com/your-org/topup-store.git
cd topup-store

cp .env.example .env
# Edit .env and fill in your credentials

docker compose up --build
```

Services:
- App: `http://localhost:8080`
- WhatsApp Bot: `http://localhost:3001`
- Caddy reverse proxy: `http://localhost` (production)

## Manual Setup

### 1. Backend (Go)

```bash
cp .env.example .env
# Edit .env with your credentials

go mod download
make migrate   # run database migrations
make run       # starts server on :8080
```

### 2. WhatsApp Bot (Node.js)

The bot handles incoming WhatsApp messages via Fonnte webhooks and parses orders.

```bash
cd whatsapp-bot
cp .env.example .env
npm install
node index.js
```

## External Integrations

### Fonnte (WhatsApp Messaging)

This project uses [Fonnte](https://fonnte.com) for WhatsApp messaging.

1. Register at [Fonnte](https://fonnte.com)
2. Get your API token from the dashboard
3. Set in `.env`:
   ```
   FONNTE_TOKEN=your_fonnte_token
   ```
4. Configure your Fonnte webhook URL to point to your bot:
   ```
   https://your-domain.com/webhook
   ```

### Midtrans (QRIS Payments)

1. Log in to [Midtrans Dashboard](https://dashboard.midtrans.com)
2. Go to **Settings > Configuration > Webhook Notifications**
3. Set the notification URL to:
   ```
   https://your-domain.com/webhook/midtrans
   ```
4. Ensure **Payment Notification** is enabled

### Digiflazz (Top-Up Supplier)

1. Register at [Digiflazz](https://digiflazz.co.id)
2. Find your **Username** and **API Key** in your profile
3. Set them in `.env`:
   ```
   DIGIFLAZZ_USERNAME=your_username
   DIGIFLAZZ_API_KEY=your_api_key
   ```
4. Product SKUs can be fetched via the Digiflazz price list API or found in their dashboard

## Environment Variables

### Required

| Variable | Description | Default |
|----------|-------------|---------|
| `DATABASE_URL` | PostgreSQL connection string | *(required)* |
| `MIDTRANS_SERVER_KEY` | Midtrans server key | *(required)* |
| `DIGIFLAZZ_USERNAME` | Digiflazz account username | *(required)* |
| `DIGIFLAZZ_API_KEY` | Digiflazz API key | *(required)* |
| `ADMIN_PASSWORD` | Admin panel password | *(required)* |
| `FONNTE_TOKEN` | Fonnte API token for WhatsApp | *(optional)* |

### Optional

| Variable | Description | Default |
|----------|-------------|---------|
| `PORT` | Server port | `8080` |
| `REDIS_URL` | Redis connection string | *(empty = in-memory)* |
| `DB_MAX_CONNS` | Max DB connections | `25` |
| `DB_MIN_CONNS` | Min DB connections | `5` |
| `REQUEST_TIMEOUT` | HTTP request timeout | `30s` |
| `ALLOWED_ORIGINS` | CORS origins (comma-separated) | *(empty = none)* |
| `LOG_LEVEL` | Log level (debug/info/warn/error) | `info` |
| `LOG_FORMAT` | Log format (json/text) | `text` |
| `AUTO_MIGRATE` | Auto-run migrations on startup | `false` |
| `ADMIN_PATH` | Admin panel path | `/admin` |
| `WA_BOT_BASE_URL` | WhatsApp bot fallback URL | `http://localhost:3001` |
| `WA_BOT_TOKEN` | Shared secret for bot fallback | *(optional)* |
| `BOT_NOTIFY_TOKEN` | Bot /notify endpoint token | *(optional)* |
| `DIGIFLAZZ_TESTING` | Digiflazz test mode | `true` |
| `DIGIFLAZZ_WEBHOOK_SECRET` | Digiflazz webhook HMAC secret | *(default)* |

## API Documentation

OpenAPI 3.0 spec is available at [`openapi.yaml`](./openapi.yaml).

### Key Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Liveness probe |
| `/ready` | GET | Readiness probe (DB + Redis) |
| `/metrics` | GET | Prometheus metrics |
| `/api/orders` | POST | Create order |
| `/api/orders/{id}` | GET | Get order |
| `/api/orders/{id}/cancel` | POST | Cancel pending order |
| `/api/products` | GET | List products |
| `/webhook/midtrans` | POST | Midtrans payment webhook |
| `/webhook/digiflazz` | POST | Digiflazz status webhook |

### Example API Calls

```bash
# Create order
curl -X POST http://localhost:8080/api/orders \
  -H "Content-Type: application/json" \
  -H "X-CSRF-Token: <token-from-homepage>" \
  -d '{
    "game": "free_fire",
    "game_uid": "12345678",
    "product_id": "<product-uuid>",
    "phone": "6281234567890"
  }'

# Check order status
curl http://localhost:8080/api/orders/<order-id>

# List products
curl "http://localhost:8080/api/products?game=free_fire"
```

## Order Flow

```
┌──────────┐     ┌──────────┐     ┌──────────────┐
│  Client   │────▶│ Go API   │────▶│  PostgreSQL  │
│ (Web/WA)  │     │  :8080   │     └──────────────┘
└──────────┘     │          │
                 │          │────▶ Midtrans (QRIS)
                 │          │────▶ Digiflazz (H2H)
                 └──────────┘
                      │
                      ▼
               ┌──────────────┐
               │   Fonnte     │
               │  (WhatsApp)  │
               └──────────────┘
```

### Status Flow

```
pending → paid → processing → success
                ↓
              failed
                ↓
              expired
                ↓
            cancelled
```

## Admin Panel

Access at `/admin` (password-protected).

**Features:**
- Order management (list, search, filter, paginate)
- Process/retry orders
- Product CRUD with auto price calculation
- Digiflazz price sync
- Analytics (revenue, conversion rate, orders by game)
- Webhook logs with payload viewer
- Retry queue monitoring
- Order export to CSV
- Status override with audit trail

## Testing

```bash
# Run all tests
make test

# Run with coverage
make test-coverage

# Run specific package
go test -v ./internal/handlers
```

## CI/CD

GitHub Actions workflows:
- **CI** — `go vet`, `gofmt`, unit tests, integration tests (PostgreSQL + Redis), Docker build
- **Docker Build & Push** — Multi-platform images pushed to Docker Hub

## Docker Compose Services

| Service | Image | Port | Description |
|---------|-------|------|-------------|
| postgres | postgres:16-alpine | 5432 | Database |
| redis | redis:7-alpine | 6379 | Cache / rate limiting |
| migrate | migrate/migrate | — | Database migrations |
| app | topup-store-app | 8080 | Go backend |
| whatsapp-bot | topup-store-whatsapp-bot | 3001 | Node.js sidecar |
| caddy | caddy:2-alpine | 80/443 | Reverse proxy |

## Project Structure

```
topup-store/
├── cmd/server/main.go          # Entry point
├── internal/
│   ├── config/                 # Env config
│   ├── handlers/               # HTTP handlers
│   ├── middleware/             # CSRF, rate limit, auth, logging
│   ├── models/                 # Data models
│   ├── repositories/           # Database access
│   ├── services/               # Business logic
│   └── retry/                  # Exponential backoff
├── web/templates/              # Go html/template
├── web/static/                 # CSS, JS, images
├── migrations/                 # SQL migrations
├── whatsapp-bot/               # Node.js sidecar
├── docker-compose.yml          # Full stack orchestration
├── Dockerfile                  # Multi-stage build
├── openapi.yaml                # API documentation
└── Makefile                    # Build commands
```

## License

MIT
