# IGNORE THIS

# PHASE 1
Create a fullstack Go project called "topup-store" with this structure:

/cmd/server/main.go         → entry point
/internal/config/config.go  → loads .env into a Config struct
/internal/db/db.go          → PostgreSQL connection via pgx
/internal/models/           → Order, Product, User structs
/internal/handlers/         → HTTP handlers (orders, products, webhooks)
/internal/services/         → business logic (payment, topup, whatsapp)
/internal/middleware/        → logging, auth
/web/templates/             → Go html/template files
/web/static/                → CSS, JS assets
/migrations/                → SQL migration files
/.env.example               → all env vars with placeholder values

Use:
- chi router for HTTP
- pgx/v5 for Postgres
- godotenv to load .env
- Go 1.22 modules

Generate go.mod, go.sum stubs, and a Makefile with: make run, make migrate, make build.

# PHASE 2
In /migrations/, create 001_init.sql with these tables:

products
  id UUID PK, game ENUM('free_fire','mobile_legends','pubg_mobile'),
  name TEXT, description TEXT, price_idr INT, diamonds INT,
  sku TEXT (Digiflazz product code), is_active BOOL, created_at TIMESTAMPTZ

orders
  id UUID PK, product_id UUID FK, user_phone TEXT, game_uid TEXT,
  game_server TEXT, amount_idr INT, status ENUM('pending','paid','processing','success','failed'),
  midtrans_order_id TEXT, qris_url TEXT, qris_image_base64 TEXT,
  channel TEXT ENUM('whatsapp','web'), created_at TIMESTAMPTZ, updated_at TIMESTAMPTZ

Also write a db/seed.go that inserts sample products for Free Fire (100, 310, 520 diamonds),
Mobile Legends (86, 172, 257 weekly diamonds), and PUBG Mobile (60, 180, 325 UC).
Use placeholder Digiflazz SKUs like "FF-100", "ML-86", "PUBG-60" — I will fill real ones later.

# PHASE 3
Create /internal/services/payment.go

It should:
1. Accept an Order struct and call Midtrans Snap API to create a QRIS transaction
2. Return the QRIS URL and a base64-encoded QR image (use Midtrans's QR code URL + download it)
3. Expose a method: CreateQRIS(order Order) (qrisURL string, qrisBase64 string, err error)
4. Load credentials from config: MIDTRANS_SERVER_KEY, MIDTRANS_IS_PRODUCTION (bool)
5. Use Midtrans Go SDK: github.com/midtrans/midtrans-go
6. Transaction details: order_id = order.ID, gross_amount = order.AmountIDR, payment_type = "qris"
7. Add all env vars to .env.example with placeholder values

Also create /internal/handlers/webhook.go:
- POST /webhook/midtrans — verify Midtrans signature, update order status to 'paid',
  then call TopupService.ProcessOrder(orderID)

# PHASE 4
Create /internal/services/topup.go

Implement Digiflazz H2H API integration:
1. Method: ProcessOrder(orderID string) error
2. Fetch order from DB, get product SKU
3. POST to https://api.digiflazz.com/v1/transaction with:
   - username: DIGIFLAZZ_USERNAME
   - buyer_sku_code: product.SKU
   - customer_no: order.GameUID (+ order.GameServer for ML format: "UID|SERVER")
   - ref_id: order.ID
   - sign: md5(username + apiKey + ref_id)
4. Poll or accept Digiflazz webhook at POST /webhook/digiflazz to confirm success
5. On success: update order status to 'success', send WhatsApp confirmation
6. Add DIGIFLAZZ_USERNAME, DIGIFLAZZ_API_KEY to .env.example

Game UID format notes:
- Free Fire: just the numeric UID
- Mobile Legends: "UID|SERVER" (e.g. "12345|1234")  
- PUBG Mobile: just the numeric UID

# PHASE 5
Create a /whatsapp-bot/ directory with a Node.js service using Baileys (@whiskeysockets/baileys).

The bot should:
1. Connect to WhatsApp Web via QR code scan on first run (save session to ./auth_info_baileys/)
2. Listen for incoming messages and parse this order format:
   "FF 100 UID:12345" or "ML 86 UID:12345|1234" or "PUBG 60 UID:12345"
3. On valid order:
   a. Call our Go backend: POST http://localhost:8080/api/orders
      body: { game, diamonds, game_uid, game_server, phone: sender }
   b. Receive back { order_id, qris_url, qris_base64 }
   c. Send the QRIS as an image message to the user with caption:
      "Halo! Berikut QRIS untuk pembayaran [Game] [Diamonds] diamonds.\nTotal: Rp[amount]\nID Order: [order_id]\nScan QRIS di bawah untuk membayar:"
4. On unrecognized message, reply with a help menu showing available packages
5. When Midtrans webhook fires and order is 'success', the Go backend calls:
   POST http://localhost:3001/notify with { phone, message }
   The bot then sends the success notification to the user

Create package.json, index.js, and a .env.example with:
  GO_BACKEND_URL=http://localhost:8080
  BOT_PORT=3001

# PHASE 6
Create the frontend using Go html/template in /web/templates/:

1. layout.html  — base layout with navbar: logo "TopUp Store", links: Home, Cek Order
2. index.html   — landing page with:
   - Hero section: "Top Up Game Favoritmu, Cepat & Aman"
   - Game selector tabs: Free Fire | Mobile Legends | PUBG Mobile
   - Product cards grid showing name, diamonds, price in Rupiah, "Beli via WA" button
   - Clicking "Beli via WA" opens: https://wa.me/[WHATSAPP_NUMBER]?text=FF%20100%20UID%3A
3. order.html   — manual order form: select game, select package, enter UID, enter phone → submit
4. status.html  — order status page: enter order ID → show status badge + details
5. admin.html   — simple password-protected page to list all orders with status

Use Tailwind CSS via CDN. Colors: primary orange #F97316, dark bg #1C1C1E.
Add WHATSAPP_NUMBER to .env.example.
Create /internal/handlers/pages.go with handlers for all routes.

# PHASE 7
Wire up all routes and services in /cmd/server/main.go:

Routes:
  GET  /                          → pages.Home
  GET  /order                     → pages.OrderForm
  GET  /status                    → pages.Status
  GET  /admin                     → pages.Admin (Basic Auth)
  POST /api/orders                → handlers.CreateOrder
  GET  /api/orders/:id            → handlers.GetOrder
  GET  /api/products              → handlers.ListProducts
  POST /webhook/midtrans          → webhook.Midtrans
  POST /webhook/digiflazz        → webhook.Digiflazz

Services wired:
  PaymentService  (Midtrans)
  TopupService    (Digiflazz)
  NotifyService   (calls WhatsApp bot /notify endpoint)

Add a Docker Compose file:
  - service: postgres (postgres:16-alpine)
  - service: app (Go backend, port 8080)
  - service: whatsapp-bot (Node 20-alpine, port 3001)

Add CORS middleware allowing all origins for /api/* routes.
Generate a complete .env.example with every variable used across all services.

# PHASE 8
Add these finishing touches:

1. Rate limiting middleware on POST /api/orders (max 5 req/min per IP)
2. Input validation on CreateOrder: UID must be numeric (or numeric|numeric for ML)
3. Order expiry: if order stays 'pending' > 30 min, mark as 'expired' (run a goroutine ticker)
4. README.md with:
   - Prerequisites: Go 1.22, Node 20, PostgreSQL, Docker
   - Setup steps: clone → copy .env.example → fill credentials → docker compose up
   - How to scan WhatsApp QR on first run
   - Midtrans dashboard: where to set webhook URL
   - Digiflazz dashboard: where to get API key and product SKUs
   - Test order flow end-to-end
5. A /health endpoint returning {"status":"ok","version":"1.0.0"}