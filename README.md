# GigJar

Get paid in USDT for any gig, however small. Payments pool in a per-student jar and sweep to the student's Cleva wallet once the jar passes about $11.

Read [docs/architecture.md](docs/architecture.md) for the domain model, ledger, state machines and open questions.

## Status

| Phase | Scope | State |
|---|---|---|
| 1 | Foundation: config, logging, Postgres, migrations, health checks, API skeleton, Docker, web skeleton | done |
| 2 | Financial core: users, auth, payment links, double-entry ledger | done |
| 3 | Watcher: TRON observation, attribution, confirmations | next |
| 4 | Sweeper: settlement state machine, WDK signer | planned |
| 5 | Frontend screens | scaffolded |
| 6 | End-to-end integration | planned |

## Prerequisites

- Go 1.27+
- Docker with Compose v2
- Node 20+ and pnpm 9 (web app only)

## Quick start

```bash
make up                  # postgres, then migrations, then api on http://localhost:8090
curl localhost:8090/healthz
curl localhost:8090/readyz

make web-install
make web-dev             # http://localhost:3000; /api/* is proxied to the Go API
```

Local Go development without containerising the API:

```bash
make db-up               # postgres on localhost:55432
make run-api             # applies migrations, then runs the API with .env.example defaults
```

## Make targets

| Target | What it does |
|---|---|
| `make up` / `make down` | Full stack in Docker |
| `make db-up` | Postgres only (creates `gigjar` and `gigjar_test` databases) |
| `make migrate` / `migrate-down-one` / `migrate-version` | Schema management (`down-one` is refused in production) |
| `make migration name=create_users` | New numbered up/down migration pair |
| `make test` | Unit tests with the race detector |
| `make test-integration` | Tests against the real Postgres in `gigjar_test` |
| `make check` | gofmt check, `go vet` (with and without the integration tag), unit tests |
| `make contracts` | Regenerate web types from `packages/contracts/openapi.yaml` |

## Configuration

All services read `GIGJAR_*` environment variables. See [.env.example](.env.example). Invalid configuration fails startup and lists every problem at once.

| Variable | Default | Notes |
|---|---|---|
| `GIGJAR_ENV` | `development` | `development`, `test`, `production` |
| `GIGJAR_DATABASE_URL` | none (required) | |
| `GIGJAR_HTTP_ADDR` | `:8080` | Inside the container. Compose publishes it on host port 8090. |
| `GIGJAR_LOG_LEVEL` / `GIGJAR_LOG_FORMAT` | `info` / `json` | Keys that look like secrets are redacted |
| `GIGJAR_CORS_ALLOWED_ORIGINS` | `http://localhost:3000` | Wildcard is refused in production |
| `GIGJAR_HTTP_REQUEST_TIMEOUT` | `15s` | |
| `GIGJAR_SHUTDOWN_TIMEOUT` | `20s` | |
| `GIGJAR_CHAIN_NETWORK` | `simulated` | `tron-mainnet`, `tron-nile` or `simulated`. Production refuses `simulated`. |
| `GIGJAR_SETTLEMENT_THRESHOLD` | `11` | USDT. Must be above Cleva's 10 USDT minimum. |
| `GIGJAR_PUBLIC_WEB_URL` | `http://localhost:3000` | Base for `pay_url` on payment links |
| `GIGJAR_SESSION_TTL` | `720h` | |
| `GIGJAR_SECURE_COOKIES` | on in production | Production refuses `false` |

## Health

- `GET /healthz`: liveness. It does not touch dependencies.
- `GET /readyz`: returns 200 only when Postgres answers and the schema version equals the newest migration embedded in the binary. An API deployed ahead of its migrations reports not ready.

## Conventions

- Source code has no comments. Explanations live in `docs/`. The only exception is generated code (`apps/web/src/lib/api/schema.d.ts`).
- Money is integer USDT base units (1 USDT = 1,000,000) in Go and Postgres, and a decimal string over the API. Never use floats.
- Ledger rows are immutable. Mistakes are fixed with compensating entries.
