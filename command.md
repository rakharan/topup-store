# Command Cheatsheet

## Local build

```powershell
docker build --no-cache -t sagameda/topup-store-app:latest .
docker build --no-cache -t sagameda/topup-store-whatsapp-bot:latest ./whatsapp-bot
```

## Push images

```powershell
docker login
docker push sagameda/topup-store-app:latest
docker push sagameda/topup-store-whatsapp-bot:latest
```

## VPS deploy

```bash
cd /path/to/topup-store
git pull
docker compose pull app whatsapp-bot
docker compose up -d
docker compose ps
docker compose logs -f app
```

## Tunnel VPS database to local

Keep this terminal open:

```bash
ssh -N -L 5433:127.0.0.1:5432 topup-store
```

Then connect your local database client to `127.0.0.1:5433` with the VPS Postgres database, user, and password from `.env`.

## Run migrations only

```bash
docker compose run --rm migrate
```

## Seed products if DB is empty

Use this on the VPS:

```bash
docker compose run --rm seed
```

Alternative from a shell with Go installed:

```bash
DATABASE_URL='postgres://topup:topup@localhost:5432/topup_store?sslmode=disable' go run ./db/seed.go
```

If running from the VPS, make sure the command can reach the exposed Postgres port on `localhost:5432`.

## Quick checks

```bash
curl -I https://sagameda.com/order
curl https://sagameda.com/health
curl https://sagameda.com/ready
docker compose exec postgres psql -U topup -d topup_store -c "select game, count(*) from products group by game order by game;"
```
