.PHONY: run build docker up down lint test load migrate-up

run:
	go run ./cmd/app

build:
	go build -o prsrv ./cmd/app

docker:
	docker build -t prsrv:local .

up:
	docker compose up --build

down:
	docker compose down -v

test:
	TEST_DATABASE_URL="postgres://postgres:postgres@localhost:5432/prsrv?sslmode=disable" \
	go test ./tests/e2e -v -count=1

lint:
	golangci-lint run

# ---- Load test (k6) ----
load:
	BASE_URL=http://localhost:8080 k6 run load/k6-pr.js

migrate-up:
	migrate -path ./migrations -database "postgres://postgres:postgres@localhost:5432/prsrv?sslmode=disable" up

migrate-down:
	migrate -path ./migrations -database "postgres://postgres:postgres@localhost:5432/prsrv?sslmode=disable" down
