# Local Deployment with Cloudflare Tunnel

## Overview

Deploy the TopUp Store locally and expose it to the internet via Cloudflare Tunnel (no public IP needed, no port forwarding).

## Prerequisites

1. A domain managed by Cloudflare (e.g., `sagameda.com`)
2. Docker and Docker Compose installed
3. `cloudflared` CLI installed:
   ```bash
   # Windows (PowerShell)
   winget install Cloudflare.cloudflared
   
   # macOS
   brew install cloudflared
   
   # Linux
   wget -q https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-amd64 -O /usr/local/bin/cloudflared
   chmod +x /usr/local/bin/cloudflared
   ```

## Setup Steps

### 1. Authenticate cloudflared

```bash
cloudflared tunnel login
```

This opens a browser to authenticate with your Cloudflare account. Select the domain you want to use.

### 2. Create a tunnel

```bash
cloudflared tunnel create topup-store
```

Note the tunnel ID from the output (looks like `2f4b6c8e...`).

### 3. Configure DNS routes

```bash
# Route your domain to the tunnel
cloudflared tunnel route dns topup-store sagameda.com

# Optionally route bot subdomain
cloudflared tunnel route dns topup-store bot.sagameda.com
```

### 4. Update cloudflared config

Edit `cloudflared.yml` and replace `<YOUR-TUNNEL-ID>` with the actual tunnel ID from step 2.

```yaml
tunnel: 2f4b6c8e-...  # your tunnel ID
credentials-file: C:\Users\<USER>\.cloudflared\2f4b6c8e-....json

ingress:
  - hostname: sagameda.com
    service: http://localhost:8080
  - hostname: bot.sagameda.com
    service: http://localhost:3001
  - service: http_status:404
```

**Note:** On Windows, the credentials file is at `C:\Users\<USER>\.cloudflared\<TUNNEL-ID>.json`

### 5. Update .env

```env
# For Fonnte webhook callbacks, use your public domain
WA_BOT_BASE_URL=https://bot.sagameda.com

# Or if using the same domain with path
# WA_BOT_BASE_URL=https://sagameda.com
```

### 6. Start the tunnel

**Option A: Run cloudflared directly (development)**

```bash
cd C:\GIT\personal\topup-store
cloudflared tunnel --config cloudflared.yml run
```

**Option B: Run via Docker Compose**

Get your tunnel token:
```bash
cloudflared tunnel token <YOUR-TUNNEL-ID>
```

Add to `.env`:
```env
CLOUDFLARE_TUNNEL_TOKEN=eyJhIjoi...
```

Start services:
```bash
docker compose -f docker-compose.yml -f docker-compose.cloudflared.yml up -d
```

### 7. Start the application

If using Option A (cloudflared directly):
```bash
docker compose up -d
```

If using Option B (cloudflared in Docker):
```bash
docker compose -f docker-compose.yml -f docker-compose.cloudflared.yml up -d
```

### 8. Verify deployment

- **App:** https://sagameda.com
- **Health check:** https://sagameda.com/health
- **API:** https://sagameda.com/api/products
- **Bot:** https://bot.sagameda.com/health

### 9. Configure external webhooks

**Midtrans Dashboard:**
- Webhook URL: `https://sagameda.com/webhook/midtrans`

**Digiflazz Dashboard:**
- Webhook URL: `https://sagameda.com/webhook/digiflazz`

**Fonnte Dashboard:**
- Webhook URL: `https://bot.sagameda.com/webhook` (or your configured bot endpoint)

## Stopping

```bash
# Stop cloudflared (if running directly)
Ctrl+C

# Stop Docker services
docker compose down

# Stop with cloudflared override
docker compose -f docker-compose.yml -f docker-compose.cloudflared.yml down
```

## Troubleshooting

### Tunnel won't connect
- Check `cloudflared tunnel list` to verify tunnel exists
- Check `cloudflared tunnel info <ID>` for status
- Verify credentials file path in `cloudflared.yml`

### 502 Bad Gateway
- Verify app is running: `docker compose ps`
- Check app logs: `docker compose logs app`
- Ensure `service: http://localhost:8080` matches your app port

### Webhooks not reaching local
- Verify public URL is accessible from outside
- Check Fonnte/Midtrans/Digiflazz webhook logs
- Ensure `.env` has correct `WA_BOT_BASE_URL`

### Certificate issues
- Cloudflare handles HTTPS automatically
- No local certificates needed
- Ensure SSL/TLS mode in Cloudflare dashboard is set to "Full (strict)"

## Architecture

```
Internet → Cloudflare Edge → Cloudflare Tunnel → Your PC
                                               ├── localhost:8080 (app)
                                               └── localhost:3001 (bot)
```

No public IP or port forwarding required. All traffic is encrypted end-to-end.
