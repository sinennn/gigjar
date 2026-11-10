SHELL := /bin/bash
DATABASE_URL ?= postgres://gigjar:gigjar@localhost:55432/gigjar?sslmode=disable
TEST_DATABASE_URL ?= postgres://gigjar:gigjar@localhost:55432/gigjar_test?sslmode=disable
GO_SERVICES := api migrate watcher sweeper gigjar

.PHONY: help up down logs db-up migrate migrate-down-one migrate-version migration run-api run-watcher run-sweeper reconcile signer-install signer-test build test test-integration test-trongrid test-nile-sweep fmt fmt-check vet check web-install web-dev web-build contracts

help:
	@grep -E '^[a-zA-Z_-]+:' Makefile | cut -d: -f1 | sort

up:
	docker compose up --build -d

down:
	docker compose down

logs:
	docker compose logs -f

db-up:
	docker compose up -d --wait postgres

migrate: db-up
	GIGJAR_DATABASE_URL="$(DATABASE_URL)" GIGJAR_LOG_FORMAT=text go run ./services/migrate up

migrate-down-one: db-up
	GIGJAR_DATABASE_URL="$(DATABASE_URL)" GIGJAR_LOG_FORMAT=text go run ./services/migrate down-one

migrate-version: db-up
	GIGJAR_DATABASE_URL="$(DATABASE_URL)" GIGJAR_LOG_FORMAT=text go run ./services/migrate version

migration:
	@test -n "$(name)" || (echo "usage: make migration name=create_users" && exit 1)
	@next=$$(printf "%06d" $$(( 10#$$(ls migrations/*.up.sql | sed -E 's#migrations/([0-9]+)_.*#\1#' | sort | tail -1) + 1 ))); \
	touch migrations/$${next}_$(name).up.sql migrations/$${next}_$(name).down.sql; \
	echo "created migrations/$${next}_$(name).{up,down}.sql"

run-api: migrate
	GIGJAR_DATABASE_URL="$(DATABASE_URL)" GIGJAR_LOG_FORMAT=text go run ./services/api

run-watcher: migrate
	GIGJAR_DATABASE_URL="$(DATABASE_URL)" GIGJAR_LOG_FORMAT=text go run ./services/watcher run

run-sweeper: migrate
	GIGJAR_DATABASE_URL="$(DATABASE_URL)" GIGJAR_LOG_FORMAT=text go run ./services/sweeper

reconcile: db-up
	GIGJAR_DATABASE_URL="$(DATABASE_URL)" GIGJAR_LOG_FORMAT=text go run ./services/watcher reconcile

build:
	@mkdir -p bin
	@for service in $(GO_SERVICES); do go build -o bin/$$service ./services/$$service || exit 1; done

test:
	go test -race ./...

test-integration: db-up
	GIGJAR_TEST_DATABASE_URL="$(TEST_DATABASE_URL)" go test -race -count=1 -p 1 -tags integration ./...

test-trongrid:
	@key="$${GIGJAR_TRONGRID_API_KEY:-$$(grep -s '^GIGJAR_TRONGRID_API_KEY=' .env | cut -d= -f2-)}"; \
	test -n "$$key" || (echo "set GIGJAR_TRONGRID_API_KEY or add it to .env" && exit 1); \
	GIGJAR_TRONGRID_API_KEY="$$key" go test -count=1 -tags trongrid -run Live -v ./internal/chain/trongrid/ && \
	GIGJAR_TRONGRID_API_KEY="$$key" GIGJAR_TEST_DATABASE_URL="$(TEST_DATABASE_URL)" go test -count=1 -p 1 -tags "integration trongrid" -run Live -v ./internal/watcher/

test-nile-sweep: db-up
	./scripts/test-nile-sweep.sh

fmt:
	gofmt -w internal services migrations

fmt-check:
	@test -z "$$(gofmt -l internal services migrations)" || (gofmt -l internal services migrations && exit 1)

vet:
	go vet ./...
	go vet -tags integration ./...
	go vet -tags trongrid ./internal/chain/trongrid/
	go vet -tags "integration trongrid" ./internal/watcher/ ./internal/settlement/

check: fmt-check vet test signer-test

signer-install:
	pnpm --dir services/signer install

signer-test:
	pnpm --dir services/signer test

web-install:
	pnpm --dir apps/web install

web-dev:
	pnpm --dir apps/web dev

web-build:
	pnpm --dir apps/web build

contracts:
	pnpm --dir apps/web contracts
