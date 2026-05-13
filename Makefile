.PHONY: run build dev test test-coverage lint fmt vet migrate migrate-down migrate-force migrate-status migrate-docker seed docker-up docker-down docker-logs css css-watch clean install-migrate

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

install-migrate:
	go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest

migrate:
	docker compose run --rm migrate

migrate-down:
	migrate -path migrations -database "$$DATABASE_URL" down 1

migrate-force:
	migrate -path migrations -database "$$DATABASE_URL" force $(VERSION)

migrate-status:
	migrate -path migrations -database "$$DATABASE_URL" version

migrate-docker:
	docker compose run --rm migrate

seed:
	go run ./db/seed.go

docker-up:
	docker compose up -d --build

docker-down:
	docker compose down

docker-logs:
	docker compose logs -f app

tunnel-up:
	docker compose -f docker-compose.yml -f docker-compose.cloudflared.yml up -d

tunnel-down:
	docker compose -f docker-compose.yml -f docker-compose.cloudflared.yml down

tunnel-logs:
	docker compose -f docker-compose.yml -f docker-compose.cloudflared.yml logs -f cloudflared

css:
	npx tailwindcss -i ./web/static/css/input.css -o ./web/static/css/tailwind.css --minify

css-watch:
	npx tailwindcss -i ./web/static/css/input.css -o ./web/static/css/tailwind.css --watch

clean:
	rm -rf bin/ coverage.out coverage.html web/static/css/tailwind.css
