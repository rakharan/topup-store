# TopUp Store

Fullstack game top-up platform with QRIS payments, Digiflazz H2H integration, and WhatsApp ordering.

## Prerequisites

- **Go** 1.22+
- **Node.js** 20+
- **PostgreSQL** 15+
- **Docker** & **Docker Compose** (optional, for containerized setup)

## Quick Start (Docker)

```bash
git clone https://github.com/your-org/topup-store.git
cd topup-store

cp .env.example .env
# Edit .env and fill in your credentials

docker compose up --build
```

Server runs on `http://localhost:8080`, WhatsApp bot on `http://localhost:3001`.

## Manual Setup

### 1. Backend (Go)

```bash
cp .env.example .env
# Edit .env with your credentials

go mod tidy
make migrate   # run database migrations
make run       # starts server on :8080
```

### 2. WhatsApp Bot (Node.js)

```bash
cd whatsapp-bot
cp .env.example .env
npm install
node index.js
```

## WhatsApp Cloud API Setup

This project uses the official **WhatsApp Cloud API** (Meta) for reliable, long-term WhatsApp integration.

1. Go to [Meta for Developers](https://developers.facebook.com/)
2. Create a new app → **Business** → **WhatsApp**
3. In your app dashboard, go to **WhatsApp → API Setup**
4. Copy your **Temporary access token** (valid for 24h) or generate a permanent one in **Settings → Advanced → System user**
5. Copy your **Phone number ID**
6. Set these in `.env`:
   ```
   WHATSAPP_TOKEN=your_access_token
   WHATSAPP_PHONE_NUMBER_ID=your_phone_number_id
   WHATSAPP_VERIFY_TOKEN=topup-store-verify
   ```
7. Set up the webhook URL in your app dashboard:
   - URL: `https://your-domain.com/webhook` (use ngrok for local dev)
   - Verify token: `topup-store-verify`
   - Subscribe to messages: `messages`

For local development with ngrok:
```bash
ngrok http 3001
# Set webhook URL to: https://xxxx.ngrok-free.app/webhook
```

## Midtrans Webhook Configuration

1. Log in to [Midtrans Dashboard](https://dashboard.midtrans.com)
2. Go to **Settings > Configuration > Webhook Notifications**
3. Set the notification URL to:
   ```
   https://your-domain.com/webhook/midtrans
   ```
   For local development, use ngrok:
   ```
   ngrok http 8080
   # Then set: https://xxxx.ngrok-free.app/webhook/midtrans
   ```
4. Ensure **Payment Notification** is enabled

## Digiflazz API Setup

1. Register at [Digiflazz](https://digiflazz.co.id)
2. Go to your profile/dashboard to find your **Username** and **API Key**
3. Set them in `.env`:
   ```
   DIGIFLAZZ_USERNAME=your_username
   DIGIFLAZZ_API_KEY=your_api_key
   ```
4. To find product SKUs, use the Digiflazz price list API or check their dashboard under **Products**
5. Populate your `products` table with matching SKUs for each game package

## Environment Variables

| Variable | Description |
|---|---|
| `PORT` | Server port (default: 8080) |
| `DATABASE_URL` | PostgreSQL connection string |
| `MIDTRANS_SERVER_KEY` | Midtrans server key |
| `MIDTRANS_IS_PRODUCTION` | `true` for production, `false` for sandbox |
| `DIGIFLAZZ_USERNAME` | Digiflazz account username |
| `DIGIFLAZZ_API_KEY` | Digiflazz API key |
| `WHATSAPP_TOKEN` | Meta WhatsApp API access token |
| `WHATSAPP_PHONE_NUMBER_ID` | WhatsApp phone number ID from Meta |
| `WHATSAPP_VERIFY_TOKEN` | Webhook verification token |
| `ADMIN_PASSWORD` | Admin panel password |
| `WA_BOT_BASE_URL` | WhatsApp bot sidecar URL (default: http://localhost:3001) |

## End-to-End Test Flow

### Via Web

1. Open `http://localhost:8080`
2. Select a game tab (Free Fire / Mobile Legends / PUBG)
3. Click **Beli via WA** on any product — opens WhatsApp with pre-filled message
4. Or go to `/order` to fill the manual order form
5. After submitting, you receive a QRIS image for payment
6. Pay via QRIS (use Midtrans sandbox test card/QR)
7. Check order status at `/status` with your order ID

### Via WhatsApp

1. Send a message to your WhatsApp Business number in this format:
   ```
   FF 100 UID:12345678
   ML 86 UID:12345|1234
   PUBG 60 UID:12345678
   ```
2. Bot replies with QRIS image and payment instructions
3. After payment, bot sends a success confirmation

### Via API

```bash
# Create order
curl -X POST http://localhost:8080/api/orders \
  -H "Content-Type: application/json" \
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

## Architecture

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
               │  WhatsApp    │
               │  Cloud API   │
               │  (Meta)      │
               └──────────────┘
```

## License

MIT
