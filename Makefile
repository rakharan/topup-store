.PHONY: run build dev test test-coverage lint fmt vet migrate migrate-docker seed docker-up docker-down docker-logs clean

run:
	go run ./cmd/server

dev:
	air

build:
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
	@for f in migrations/*.sql; do \
		echo "Applying $$f..."; \
		psql "$$DATABASE_URL" -f "$$f"; \
	done

migrate-docker:
	docker compose exec postgres sh -c 'for f in /migrations/*.sql; do echo "Applying $$f..."; psql -U topup -d topup_store -f "$$f"; done'

seed:
	go run ./db/seed.go

docker-up:
	docker compose up -d --build

docker-down:
	docker compose down

docker-logs:
	docker compose logs -f app

clean:
	rm -rf bin/ coverage.out coverage.html
