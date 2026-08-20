.PHONY: db db-down run test lint build

db:
	docker compose -f deploy/docker-compose.yml up -d

db-down:
	docker compose -f deploy/docker-compose.yml down

build:
	go build -o bin/nba-digest ./cmd/nba-digest

run:
	go run ./cmd/nba-digest

test:
	go test ./... -race

lint:
	test -z "$$(gofmt -l .)" || (gofmt -l . && exit 1)
	go vet ./...
