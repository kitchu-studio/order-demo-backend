.PHONY: tidy run test up down logs migrate-new

tidy:
	go mod tidy

# Run API on host (needs a local Postgres; see README) 
run:
	go run ./cmd/api

test:
	go test ./...

up:
	docker compose up -d --build

down:
	docker compose down

logs:
	docker compose logs -f api

# Usage: make migrate-new name=create_orders
migrate-new:
	docker run --rm -v $(PWD)/migrations:/migrations migrate/migrate:v4.18.1 create -ext sql -dir /migrations -seq $(name)
