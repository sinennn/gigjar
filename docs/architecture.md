# GigJar Architecture

GigJar collects small USDT (TRC20) payments for student freelancers into a per-student jar and sweeps the jar to the student's Cleva deposit address once it passes the settlement threshold (about $11, above Cleva's $10 minimum credit).

## 1. Repository layout

```
gigjar/
├── apps/web/                 Next.js student dashboard and public payment page (calls /api/*, rewritten to the Go API)
├── services/
│   ├── api/                  HTTP API process (main package only)
│   ├── watcher/              chain observation process
│   ├── sweeper/              settlement process
│   ├── migrate/              schema migration runner
│   └── signer/               Node.js process wrapping Tether WDK (holds the seed)
├── internal/                 shared Go packages (one Go module at the repo root)
│   ├── config/  logging/  database/  httpapi/
│   ├── identity/  jars/  ledger/  paylinks/  usdt/
│   ├── chain/ (+ trongrid/, simulated/)  tron/  transfers/  watcher/
│   └── settlement/  wallet/ (+ simwallet/, tronwallet/)  energy/ (+ tronsave/)
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
| `incoming_transfers` | unique `(network, tx_hash, to_address)`. All USDT logs to one jar in one transaction form one row, and `log_indexes` records which. Also `from_address`, `amount`, `block_number`, status, `payment_link_id` (nullable). Frozen once resolved. |
| `jar_watch_state` | per provisioned jar: the address, the poll cursor (`min_block_timestamp_ms`), the next poll time, failure count and last error. |
| `settlements` | state machine below, `amount`, `destination_address`, `tx_id`, attempt metadata. |
| `ledger_accounts`, `journal_entries`, `postings` | double-entry ledger below. |

### Payment attribution

USDT TRC20 transfers carry no memo, so the receiving address is the only reliable link between a transfer and a student. Every student has one jar address (as in the pitch deck), and every link that student creates pays into it.

- **Money is attributed by address, so attribution is always correct.** A confirmed transfer to a jar address credits that jar's owner, even when no link matches.
- **The link is attributed by amount.** A transfer matches the oldest open link of that student with exactly the same expected amount. A transfer that matches no link is still credited, recorded with `payment_link_id = NULL` and shown as "unlinked". Attaching it to a link by hand is not built yet. A wrong link match changes only a label, never a balance.

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

A repeat observation of the same `(network, tx_hash, to_address)` is a no-op through `INSERT … ON CONFLICT DO NOTHING`. TRON has no reorgs past solidification. A transaction that stays unsolidified for 24h is marked `failed` with `not_solidified`.

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
- **Jar addresses** are allocated a `derivation_index` at sign-up. On real networks the address stays null until the wallet service provisions it (Phase 4). Until then the public pay page answers `503 jar_not_ready` rather than showing an address that can't be swept. (Since Phase 3, simulated jars get a simulated address at sign-up.)
- **Cleva destinations** are append-only, and the newest row is the current one. A settlement will copy the destination it uses, so changing the address never redirects a sweep that is already in flight.

## 10. Phase 3 decisions

- **Two-step observation.** Discovery and confirmation use different TronGrid sources:
  - *Discovery* polls `GET /v1/accounts/{jar}/transactions/trc20?only_to=true&contract_address=<USDT>` per jar address. It includes unconfirmed transactions, so payments show as `pending` within seconds.
  - *Confirmation* calls `POST /walletsolidity/gettransactioninfobyid`. That answers only once the block is solidified (irreversible, about a minute on TRON). GigJar then credits only if the receipt is `SUCCESS` and the logs contain a `Transfer` event from the real USDT contract to the jar.
  - The credited amount comes from those logs, never from the indexer listing.
  - Zero-value transfers (address-poisoning spam) and other tokens are ignored.
- **Crediting is atomic.** In one DB transaction, `transfers.Confirm`:
  1. locks the transfer row;
  2. marks it confirmed;
  3. attributes it to a link: the oldest open link with exactly that amount, created no later than the block time;
  4. marks that link paid;
  5. posts the ledger deposit.
- **Credits are idempotent, three ways.** The row lock, the unique `(network, tx_hash, to_address)` and the ledger idempotency key `deposit:{transfer_id}` each prevent a double credit. `journal_entries.incoming_transfer_id` is now a foreign key, so a deposit without a real transfer row is impossible.
- **Polling is per jar, scheduled in the database.** `jar_watch_state.next_poll_at` is claimed with `FOR UPDATE SKIP LOCKED` and leased forward, so several watcher processes split the work safely.
  - Jars with open links or pending transfers are polled every 15s; all others every 5 minutes.
  - Failures back off exponentially, up to 10 minutes.
  - The cursor is the newest block timestamp seen, and each poll looks back 10 minutes past it so it catches indexer lag.
- **Pending confirmations back off** (5s, 10s, 20s … up to 2 minutes) and use the same claim-and-lease pattern.
- **Reconciliation** runs every 10 minutes, or once with `watcher reconcile`. It checks three things:
  - cached balances against postings;
  - every confirmed transfer against exactly one deposit of the same amount;
  - for a rotating batch of jars, the on-chain USDT balance against the ledger's available + reserved.
  - A chain balance below the ledger is an error. Extra funds on chain beyond pending are a warning, for example a transfer the indexer missed.
- **Simulated chain.** `simulated_chain_transfers` stands in for the chain. `POST /v1/simulator/transfers` writes to it, and the simulated provider reads it with the same interface and semantics as TronGrid, including a configurable delay before a transfer is final. The route is mounted only on the simulated network, and production refuses that network.
- **Public pending signal.** The public pay page reports `pending_payment_observed` when a pending transfer of exactly the link's amount has reached the jar since the link was created. It reveals nothing about other payments.
- **Verified against Nile.** `make test-trongrid` runs the adapter and the full watcher against the live Nile testnet. Both tests are read-only. The watcher test points a jar at a public Nile address and credits a real solidified 1000 USDT transfer (block 72540166).

## 11. Phase 4 decisions

- **When a jar is swept.** A sweep is queued automatically once these three hold:
  - `jar_available` ≥ `GIGJAR_SETTLEMENT_THRESHOLD` ($12 by default);
  - the student has a Cleva address;
  - the jar has an address.

  It takes the whole available balance. The settlement row is created and the reserve entry posted in one transaction, under a lock on the available account. A partial unique index allows one in-flight sweep per student. There is no "sweep now" button, because every sweep costs the same fixed amount of energy.
- **The fee stays in the jar.** Each sweep sends `amount − fee` to Cleva; the fee is $1 by default. The completion entry posts three lines: reserved −amount, settled +net, `platform_fees` +fee. The fee USDT physically stays at the jar address, because moving it would cost a second transfer. Reconciliation expects this: chain balance = available + reserved + confirmed fees. Collecting fees from jars is a later ops task.
- **The Cleva address is locked when a sweep starts.** Each settlement copies the destination at queue time, so later changes never redirect a sweep in flight.
- **Signing.** `services/signer` (Node, `@tetherto/wdk-wallet-tron` 1.0.0-beta.17) holds the seed:
  - Jars are WDK accounts `m/44'/195'/0'/0/{derivation_index}`. The treasury is `m/44'/195'/1'/0/0`.
  - Derivation matches the standard BIP-44 test vector.
  - The signer only builds and signs transactions from narrow intents: a USDT transfer from jar N, or a capped TRX transfer from the treasury.
  - It refuses token contracts and the treasury as recipients, requires a bearer token, and never broadcasts.
  - It extends each transaction's expiry to about 10 minutes.
  - **WDK supports signing without broadcasting** (`signTransaction`), which resolves the open question in §7.
- **Settlement states.**
  - **Queued:** resources are being prepared (below).
  - **Signing:** the signed transaction and its tx id are saved **before** broadcast.
  - **Broadcast:** it is rebroadcast while unconfirmed and not yet expired.
  - **Confirmed:** only when the solidified transaction contains the exact USDT transfer jar → destination for the net amount.
  - **Failed:** funds are released back to available. This happens only when the receipt failed, or the signed transaction expired and the node has never seen it. A final transaction without the expected transfer is never released; it is flagged for manual review.
  - Failed jars cool down for 10 minutes before a retry, with at most 5 failures a day.
- **Energy and gas, per sweep.**
  1. If the jar account doesn't exist, the treasury sends 0.1 TRX, which activates it. TRON also charges a fixed 1.1 TRX account-creation fee. If an existing jar lacks bandwidth, it gets 0.4 TRX.
  2. `triggerconstantcontract` estimates the exact energy for this transfer. That matched measured mainnet costs exactly: 64,285 energy to a holder, 130,285 to an empty address.
  3. If the jar lacks that energy, it rents the estimate plus 5% from TronSave for 15 minutes. TronSave's minimum order is 65,000. Payment is a treasury-signed TRX transfer passed to TronSave's API, so no account is needed. The renter is refused above `GIGJAR_MAX_ENERGY_PRICE_SUN`.
  4. If renting fails for any reason, or the energy doesn't arrive within 3 minutes, the treasury tops up the jar with enough TRX to burn instead. A rejected rental order costs nothing, because TronSave does not broadcast the payment.

  Every TRX cost is recorded in `settlement_expenses`.
- **Measured economics** (mainnet, 10 Nov 2026, TRX = $0.319):
  - Burning costs 6.43 TRX ($2.05) to a USDT holder, or 13.03 TRX ($4.16) to an empty address.
  - TronSave's floor price is 65 sun, which works out to about $1.36 or $2.72 for the same two cases.
  - Cheaper markets list about 28–45 sun but need an account. `energy.Renter` is an interface, so a second provider can be added.
  - A $1 fee does not yet cover a sweep to an empty address.
- **Provisioning.** On real networks the sweeper derives addresses for new jars from the signer and starts watching them, usually within seconds of sign-up.
- **Verified live on Nile, 10 Nov 2026** (`make test-nile-sweep`):
  - A client paid 12 USDT into a fresh jar. The watcher credited it after about 1 minute, once the block was final.
  - The sweeper activated the jar, rented 65,000 energy from TronSave, which was delegated within about 10 seconds, then signed, broadcast and confirmed the sweep about 1.5 minutes after queueing.
  - The destination received exactly 11 USDT. The sweep receipt shows all of its energy came from the rental, bandwidth came from the jar's free allowance, and 0 TRX was burned.
  - The treasury spent 5.425 TRX on the first sweep from that jar: 1.2 TRX activation and 4.225 TRX rental.
- **Bugs the live runs found and fixed:**
  - TronSave confirms orders with HTTP 201.
  - TronSave's minimum order is 65,000.
  - TronGrid's account endpoint omits unactivated addresses, so `TokenBalance` now calls `balanceOf` on solidified state.
  - Activation needs only 0.1 TRX.
- **Known gaps:**
  - The treasury's own transactions use its 600 free daily bandwidth. After that each costs about 0.27 TRX, which is not yet recorded in `settlement_expenses`. Staking TRX for bandwidth removes it.
  - Rolling back migrations 6 or 7 is refused once ledger deposits or settlements exist.

## 12. Google sign-in and AI features

- **Google sign-in** uses OpenID Connect with an authorization code, PKCE (S256), a `state` value and a `nonce` (`internal/googleauth`).
  - The flow values live in a 10-minute, HMAC-signed, HttpOnly, SameSite=Lax cookie.
  - The ID token's signature, issuer, audience, expiry and nonce are verified with `coreos/go-oidc`.
  - Google's discovery document is fetched on first use, so an outage at Google never stops the API from starting.
  - `/v1/auth/google/start` is a full-page navigation through the site's `/api` proxy, so the session cookie lands on the site's own origin.
- **Accounts.** `user_identities` stores Google's stable subject.
  - A verified email that matches an existing account links to it.
  - Otherwise a new account, with its jar and ledger accounts, is created with no password; password login for it always fails.
  - Unverified Google emails are refused.
  - Migration 000008 refuses to roll back while password-less accounts exist.
- **AI** runs on Groq (`openai/gpt-oss-20b`, strict JSON-schema output, low reasoning effort, reasoning not returned) in `internal/ai`. It never changes state:
  - **Link writer** returns a draft the student submits themselves. Prices are parsed and capped like any link.
  - **Insights and the assistant** receive a JSON summary computed by our code from that student's own rows only: balances, 30-day earnings, links, recent transfers and sweeps. The model only phrases it.
  - Prompts state the rules: no actions, never ask for secrets, use only the given data, and treat user text as content rather than instructions.
  - Outputs are length-capped.
  - Insights are cached for 10 minutes per student while the data is unchanged.
- **Limits:**
  - 30 AI requests per student per hour, and 25 per minute across the API, to stay under Groq's developer tier of 30 requests a minute and 1,000 a day.
  - Groq 429s and 5xx responses surface as `503 ai_busy` with `Retry-After`.
  - With no key configured the endpoints answer `503 ai_unavailable`, and `/v1/auth/providers` tells the UI which features are on.

