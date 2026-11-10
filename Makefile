SHELL := /bin/bash
DATABASE_URL ?= postgres://gigjar:gigjar@localhost:55432/gigjar?sslmode=disable
TEST_DATABASE_URL ?= postgres://gigjar:gigjar@localhost:55432/gigjar_test?sslmode=disable
GO_SERVICES := api migrate

.PHONY: help up down logs db-up migrate migrate-down-one migrate-version migration run-api build test test-integration fmt fmt-check vet check web-install web-dev web-build contracts

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

build:
	@mkdir -p bin
	@for service in $(GO_SERVICES); do go build -o bin/$$service ./services/$$service || exit 1; done

test:
	go test -race ./...

test-integration: db-up
	GIGJAR_TEST_DATABASE_URL="$(TEST_DATABASE_URL)" go test -race -count=1 -p 1 -tags integration ./...

fmt:
	gofmt -w internal services migrations

fmt-check:
	@test -z "$$(gofmt -l internal services migrations)" || (gofmt -l internal services migrations && exit 1)

vet:
	go vet ./...
	go vet -tags integration ./...

check: fmt-check vet test

web-install:
	pnpm --dir apps/web install

web-dev:
	pnpm --dir apps/web dev

web-build:
	pnpm --dir apps/web build

contracts:
	pnpm --dir apps/web contracts
