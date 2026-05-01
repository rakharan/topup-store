.PHONY: run build dev test test-coverage lint fmt vet migrate migrate-docker seed docker-up docker-down docker-logs css css-watch clean

run:
	go run ./cmd/server

dev:
	air

build: css
	go build -ldflags="-s -w" -o bin/server ./cmd/server

test:
	go test -v -count=1 ./...

test-coverage:
	go test -v -count=1 -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

lint:
	go vet ./...

fmt:
	go fmt ./...

vet:
	go vet ./...

migrate:
	@echo "Running migrations..."
	@psql "$$DATABASE_URL" -c "CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, filename TEXT NOT NULL, applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW());"
	@for f in migrations/*.sql; do \
		filename=$$(basename "$$f"); \
		version=$$(echo "$$filename" | grep -oE "^[0-9]+" | sed "s/^0*//"); \
		[ -z "$$version" ] && continue; \
		exists=$$(psql "$$DATABASE_URL" -tAc "SELECT 1 FROM schema_migrations WHERE version=$$version"); \
		if [ "$$exists" = "1" ]; then \
			echo "Skipping $$filename (version $$version already applied)"; \
		else \
			echo "Applying $$filename (version $$version)..."; \
			psql "$$DATABASE_URL" -v ON_ERROR_STOP=1 -f "$$f"; \
			psql "$$DATABASE_URL" -c "INSERT INTO schema_migrations (version, filename) VALUES ($$version, '$$filename');"; \
			echo "Applied $$filename successfully"; \
		fi; \
	done
	@echo "Migration complete"

migrate-docker:
	docker compose exec postgres sh -c 'psql -U topup -d topup_store -c "CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, filename TEXT NOT NULL, applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW());" && for f in /migrations/*.sql; do filename=$$(basename "$$f"); version=$$(echo "$$filename" | grep -oE "^[0-9]+" | sed "s/^0*//"); [ -z "$$version" ] && continue; exists=$$(psql -U topup -d topup_store -tAc "SELECT 1 FROM schema_migrations WHERE version=$$version"); if [ "$$exists" = "1" ]; then echo "Skipping $$filename (version $$version already applied)"; else echo "Applying $$filename (version $$version)..."; psql -U topup -d topup_store -v ON_ERROR_STOP=1 -f "$$f"; psql -U topup -d topup_store -c "INSERT INTO schema_migrations (version, filename) VALUES ($$version, '\''$$filename'\'');"; echo "Applied $$filename successfully"; fi; done && echo "Migration complete"'

seed:
	go run ./db/seed.go

docker-up:
	docker compose up -d --build

docker-down:
	docker compose down

docker-logs:
	docker compose logs -f app

css:
	npx tailwindcss -i ./web/static/css/input.css -o ./web/static/css/tailwind.css --minify

css-watch:
	npx tailwindcss -i ./web/static/css/input.css -o ./web/static/css/tailwind.css --watch

clean:
	rm -rf bin/ coverage.out coverage.html web/static/css/tailwind.css
