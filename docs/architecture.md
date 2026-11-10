# GigJar Architecture

GigJar collects small USDT (TRC20) payments for student freelancers into a per-student jar and sweeps the jar to the student's Cleva deposit address once it passes the settlement threshold (about $11, above Cleva's $10 minimum credit).

## 1. Repository layout

```
gigjar/
├── apps/web/                 Next.js student dashboard and public payment page (calls /api/*, rewritten to the Go API)
├── services/
│   ├── api/                  HTTP API process (main package only)
│   ├── watcher/              chain observation process            (Phase 3)
│   ├── sweeper/              settlement process                   (Phase 4)
│   ├── migrate/              schema migration runner
│   └── signer/               Node.js process wrapping Tether WDK   (Phase 4)
├── internal/                 shared Go packages (one Go module at the repo root)
│   ├── config/  logging/  database/  httpapi/
│   ├── ledger/  users/  paylinks/  usdt/        (Phase 2)
│   ├── chain/  tron/  watch/                    (Phase 3)
│   └── settlement/  wallet/                     (Phase 4)
├── migrations/               SQL migrations, embedded into the Go binaries
├── packages/contracts/       OpenAPI contract shared by API and web
├── infra/docker/             Dockerfiles
├── docs/
├── docker-compose.yml
└── Makefile
```

Deviations from the brief, and why:

- **`internal/` at the root.** The brief asks for one Go module. Go's `internal/` rule makes shared packages private to this module, and each `services/*` directory holds only a thin `main.go`. This keeps HTTP, chain and settlement logic in separate packages instead of inside one service.
- **`services/migrate`.** Migrations run as an explicit step, not as a side effect of API startup. Several API replicas then can't race on the schema.
- **Web talks to the API through a same-origin `/api` rewrite.** Session cookies stay first-party and the browser needs no CORS. The API keeps a strict CORS allow-list for anything else.
- **`services/signer` (Node.js).** Tether WDK (`@tetherto/wdk`, `@tetherto/wdk-wallet-tron`) is a JavaScript SDK, and there is no Go port. Key material and signing live in this small internal process. The Go side reaches it through the `wallet.Provider` interface. The signer is never exposed publicly, and it is the only process that holds the seed.

## 2. Domain model

```
User 1───1 Jar (WDK account index → TRON jar address)
User 1───* PaymentLink (gig title, expected amount, reference)
User 1───1 ClevaDestination (TRC20 deposit address, versioned)
Jar  1───* IncomingTransfer (observed on chain: pending → confirmed | failed)
IncomingTransfer *──0..1 PaymentLink (attribution)
User 1───* Settlement (sweep attempt with state machine)
Ledger: Account (per user × kind, plus system accounts)
        JournalEntry 1───* Posting (sum of postings = 0)
```

| Entity | Key points |
|---|---|
| `users` | email (citext, unique), argon2id password hash, display name. |
| `jars` | one per user. `derivation_index` (unique, sequential) is the WDK account index. `address` is the derived TRON address (unique). |
| `cleva_destinations` | TRC20 address validated by base58check with the `0x41` prefix. New rows supersede old ones, and each settlement records the destination it used. |
| `payment_links` | public `reference` (short, unguessable), title, description, `expected_amount` (base units), status `open → paid → closed` (also `expired`). |
| `incoming_transfers` | unique `(network, tx_hash, log_index)`, `to_address`, `from_address`, `amount`, `block_number`, status, `payment_link_id` (nullable). |
| `settlements` | state machine below, `amount`, `destination_address`, `tx_id`, attempt metadata. |
| `ledger_accounts`, `journal_entries`, `postings` | double-entry ledger below. |

### Payment attribution

USDT TRC20 transfers carry no memo, so the receiving address is the only reliable link between a transfer and a student. Every student has one jar address (as in the pitch deck), and every link that student creates pays into it.

- **Money is attributed by address, so attribution is always correct.** A confirmed transfer to a jar address credits that jar's owner, even when no link matches.
- **The link is attributed by amount.** A transfer matches the oldest open link of that student with exactly the same expected amount. A transfer that matches no link is still credited, recorded with `payment_link_id = NULL` and shown as "unlinked". The student can attach it to a link later. A wrong link match changes only a label, never a balance.

## 3. Money representation

- USDT TRC20 has 6 decimals. All amounts are `BIGINT` base units (1 USDT = 1,000,000) in Postgres and the `usdt.Amount` (int64) type in Go. Parsing and formatting work on decimal strings only, never floats.
- The API sends amounts as decimal strings (`"4.000000"`), never JSON numbers.

## 4. Ledger

Double-entry. A journal entry holds two or more postings whose signed amounts sum to zero, which a deferred constraint trigger enforces. `journal_entries` and `postings` reject `UPDATE` and `DELETE` through triggers. Corrections are compensating entries.

Accounts:

| Account | Owner | Meaning | Constraint |
|---|---|---|---|
| `jar_available` | user | confirmed funds in the jar, free to sweep | balance ≥ 0 |
| `jar_reserved` | user | funds locked by an in-flight settlement | balance ≥ 0 |
| `settled` | user | lifetime total delivered to Cleva | balance ≥ 0 |
| `chain_inflow` | system | counter-account for observed deposits | may be negative |

`ledger_accounts.balance` is a cached sum, updated in the same database transaction as its postings, under `SELECT … FOR UPDATE` row locks taken in a deterministic order. `CHECK (balance >= 0)` on user accounts is the final guard against overspending. A reconciliation query recomputes every balance from the postings and compares.

Each movement has an `idempotency_key` with a unique index:

| Event | Postings | Idempotency key |
|---|---|---|
| deposit confirmed | `chain_inflow −a`, `jar_available +a` | `deposit:{incoming_transfer_id}` |
| settlement queued | `jar_available −a`, `jar_reserved +a` | `settlement-reserve:{settlement_id}` |
| settlement confirmed | `jar_reserved −a`, `settled +a` | `settlement-complete:{settlement_id}` |
| settlement failed | `jar_reserved −a`, `jar_available +a` | `settlement-release:{settlement_id}` |

Balances shown to the student:

- **Pending**: the sum of `incoming_transfers` in `pending` status. These are observed on chain but not yet solidified, and they are not in the ledger.
- **Available**: `jar_available`.
- **Settling**: `jar_reserved`.
- **Settled**: `settled`.
- **Remaining until auto-settlement**: `max(0, threshold − available)`.

## 5. State machines

### Incoming transfer

```
          observed (unsolidified)
                 │
                 ▼
             pending ──── receipt != SUCCESS / reorged out ──▶ failed
                 │
     solidified + receipt SUCCESS + contract/recipient/amount verified
                 ▼
            confirmed  ── ledger credit in the same DB transaction
```

A repeat observation of the same `(network, tx_hash, log_index)` is a no-op through `INSERT … ON CONFLICT DO NOTHING`.

### Settlement

```
 queued ──▶ signing ──▶ broadcast ──▶ confirmed
   │           │            │
   │           │            ├── receipt FAILED ───────────────▶ failed
   │           │            └── not on chain after expiry ────▶ failed
   │           └── signer rejected before any tx exists ─────▶ failed
   └── (created together with the reserve entry, in one DB transaction)

 failed ─▶ release entry returns the funds to jar_available. The next sweeper
           cycle may create a new settlement, and the attempt count is kept.
```

- **Queued.** Created only when `jar_available ≥ threshold`, within the transaction that posts the reserve entry. A partial unique index `(user_id) WHERE state IN ('queued','signing','broadcast')` allows at most one in-flight settlement per user, and the ledger's `CHECK (balance >= 0)` prevents a double spend.
- **Signing.** The sweeper claims a row with `FOR UPDATE SKIP LOCKED`. The signer builds and signs the transfer and returns the signed transaction and its `tx_id`. The tx_id is persisted **before** broadcast.
- **Broadcast.** The signed transaction has been sent. Because the tx_id is known in advance, a crash at any point is recoverable: the sweeper looks up the tx_id, rebroadcasts while the transaction has not expired, and declares it dropped after expiry plus a safety margin. A broadcast is never treated as success.
- **Confirmed.** Only after the transaction is in a solidified block with receipt `SUCCESS`. This posts the completion entry.

Gas: a TRC20 transfer from the jar address needs TRX (energy and bandwidth). Before signing, the sweeper tops up the jar address with TRX from a platform gas wallet. This is a separate on-chain transfer, recorded in `gas_topups`, outside the USDT ledger.

## 6. Simulated vs real

`CHAIN_NETWORK` is one of `tron-mainnet`, `tron-nile` (testnet) or `simulated`. Every `incoming_transfers` and `settlements` row stores its network. The simulated network has its own provider, which records fake transfers only in `simulated_*` tables. The API returns `"network": "simulated"`, and the UI shows a persistent "Simulated" badge. In production, startup refuses a simulated provider.

## 7. External dependencies and open questions

| Dependency | Status | Uncertainty |
|---|---|---|
| Tether WDK (`@tetherto/wdk-wallet-tron` 1.0.0-beta) | JS only, beta | Need to confirm whether it exposes sign-without-broadcast. If not, the signer builds the transaction with `tronweb` (a WDK dependency) and signs with the WDK-derived key. |
| TRON RPC (TronGrid or a full node) | API key needed for mainnet rate limits | Watching uses per-jar-address polling of TRC20 transfers, which is fine at campus scale. Scanning the whole USDT contract is the scale-out path. |
| Cleva | no public API | Today GigJar only sends to the student's Cleva TRC20 deposit address. It cannot confirm that Cleva credited the deposit, only that the chain confirmed the transfer. `cleva.Client` is an interface for future partner access. |
| Gas economics | open | Each sweep costs one USDT transfer from the jar (about 65k energy) plus a TRX top-up. Who bears this (the platform, or a fee withheld from the sweep) is a product decision. The MVP has the platform pay. |
| Custody | custodial in the MVP | The signer holds one seed for all jars. Self-custody is on the roadmap (deck slide 7). |

## 8. Implementation plan

1. **Foundation.** Monorepo, config, logging, Postgres, migrations, health checks, API skeleton, Docker, web skeleton.
2. **Financial core.** `usdt.Amount`, users and auth (argon2id, session tokens), Cleva destination, payment links, ledger schema and operations, transaction history. Unit tests, and integration tests against Postgres.
3. **Payment infrastructure.** `chain.Provider` interface, TronGrid adapter, simulated adapter, watcher loop with persisted cursors, confirmation handling, attribution, reconciliation job.
4. **Settlement.** Sweeper, state machine, locking, retries, the `wallet.Provider` interface, the WDK signer service and gas top-ups.
5. **Frontend.** All ten screens against the contract.
6. **Integration.** End-to-end flow on the simulated network and on Nile, failure scenarios, documentation.

## 9. Phase 2 decisions

- **Balances are maintained by the database.** An `AFTER INSERT` trigger on `postings` applies each posting to `ledger_accounts.balance`, so the cached balance cannot drift from the postings. Go code locks the two affected accounts with `SELECT ... FOR UPDATE ORDER BY id` before posting, so locks are always taken in the same order and posts cannot deadlock. An overdraft trips `ledger_accounts_user_balance_non_negative` and surfaces as `ledger.ErrInsufficientFunds`.
- **Balanced entries are enforced in the database.** Deferred constraint triggers reject, at commit, any journal entry with fewer than two postings or postings that don't sum to zero.
- **Replays are idempotent.** Every movement has a deterministic key (`deposit:{transfer}`, `settlement-reserve:{settlement}` and so on). Replaying the same key returns `AlreadyRecorded`. Replaying it with a different user, kind or amount is rejected as `ErrIdempotencyKeyConflict`.
- **`chain_inflow` is one shared row.** Every deposit locks it. This is fine at campus scale. If deposits ever contend on it, it can be split into a per-user or per-shard inflow account without changing the API.
- **Sessions** are opaque 256-bit tokens in an httpOnly, SameSite=Lax cookie. Only their SHA-256 is stored. Passwords use argon2id (19 MiB, t=2, p=1). Unknown emails still run a decoy hash, so response timing doesn't reveal which emails exist.
- **CSRF** protection is Go's `http.CrossOriginProtection` on `/v1`, with the configured web origins trusted. Browsers reach the API same-origin through the Next `/api` rewrite.
- **Login throttling** allows 10 attempts per email per minute, in memory per API process. Running several API replicas needs a shared limiter.
- **Jar addresses** are allocated a `derivation_index` at sign-up. The address stays null until the wallet service provisions it (Phase 4). Until then the public pay page answers `503 jar_not_ready` rather than showing an address that can't be swept.
- **Cleva destinations** are append-only, and the newest row is the current one. A settlement will copy the destination it uses, so changing the address never redirects a sweep that is already in flight.
