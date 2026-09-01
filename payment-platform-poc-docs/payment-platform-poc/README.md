# Payment Platform POC

## Run with Docker Compose

From this directory, start the API and PostgreSQL services:

```powershell
docker compose up --build
```

By default the API uses Stripe test mode. Set `STRIPE_SECRET_KEY` to a Stripe test secret or restricted key before starting Compose:

```powershell
$env:STRIPE_SECRET_KEY='rk_test_...'
docker compose up --build
```

The adapter uses Stripe's `pm_card_visa` test PaymentMethod and manual-capture PaymentIntents. Set `PAYMENT_PROCESSOR=mock` to run locally without Stripe.

To route by amount, set `PAYMENT_PROCESSOR=amount`. Payments at or below `PROCESSOR_AMOUNT_THRESHOLD` use mock, payments up to `PROCESSOR_ADYEN_THRESHOLD` use Stripe, and larger payments use Adyen:

```powershell
$env:PAYMENT_PROCESSOR='amount'
$env:PROCESSOR_AMOUNT_THRESHOLD='10000'
$env:PROCESSOR_ADYEN_THRESHOLD='50000'
docker compose up --build
```

For Adyen routing, also set `ADYEN_API_KEY` and `ADYEN_MERCHANT_ACCOUNT`. Without Adyen credentials, mock and Stripe routes remain available, while amounts above the Adyen threshold return a clear unavailable-processor error. The default Adyen test payment method is a test Visa card; override it with `ADYEN_PAYMENT_METHOD_JSON` when needed.

The API is available at `http://localhost:8080`. The operations console is available at `http://localhost:3000`; it exercises the real API and displays payment state, processor references, captures, refunds, and ledger journals. The first database start applies `backend/migrations/001_initial.sql` automatically. Postgres data is stored in the `postgres-data` volume.

For a POS-style server-controlled payment, use the single-call endpoint:

```powershell
$headers=@{'Idempotency-Key'='pos-payment-1';'Content-Type'='application/json'}
$body='{"amount":10000,"currency":"USD","capture_method":"manual","reference":"pos-order-1"}'
Invoke-RestMethod http://localhost:8080/api/v1/payments/pay -Method Post -Headers $headers -Body $body
```

This creates, authorizes, captures, and records the ledger entry before returning `CAPTURED`. The existing endpoints remain available for flows that need separate authorization and capture.

Refund a captured payment with `POST /api/v1/payments/{id}/refund` and a positive amount. Full and partial refunds are supported up to the total captured amount; Stripe and Adyen use their provider refund APIs.

Existing databases must apply `backend/migrations/002_add_refunds.sql`; fresh databases apply all migrations from the Docker initialization directory.

Run the Go tests against the Compose database from `backend`:

```powershell
$env:TEST_DATABASE_URL='postgres://payments:payments@localhost:5432/payments?sslmode=disable'
go test ./...
```

To reset the database and rerun migrations:

```powershell
docker compose down -v
```

GitHub Codespaces uses `.devcontainer/devcontainer.json` and starts the same Compose services automatically.

A portfolio-grade proof of concept for a modern card-payment backend implemented in Go.

## What this demonstrates

- Payment lifecycle modeling: create, authorize, capture
- Explicit payment state machine
- Idempotent write APIs
- PSP abstraction with mock, Stripe sandbox, and Adyen test adapters
- Registry-based processor routing by amount
- Money represented in minor units; no floating-point arithmetic
- Immutable double-entry ledger entries
- Transactional consistency between payment state and ledger writes
- Clear architecture documentation suitable for design review and portfolio discussion
- Operations console for end-to-end lifecycle demonstrations

## Current Scope

The first implementation slice is backend-only and intentionally narrow:

1. Create payment
2. Authorize payment
3. Capture payment
4. Persist payment state
5. Record ledger entries on capture
6. Support idempotent retries
7. Use mock, Stripe, or Adyen through the processor registry
8. Complete a POS-style payment with one API call
9. Full and partial refunds with idempotent provider calls

Not included yet: disputes, webhooks, settlement, reconciliation, or automatic recovery after ambiguous PSP results.

## Repository Structure

```text
payment-platform-poc/
├── README.md
├── docs/
│   ├── 01-system-overview.md
│   ├── 02-api-contract.md
│   ├── 03-data-model.md
│   ├── 04-state-machine.md
│   ├── 05-ledger-design.md
│   ├── 06-idempotency.md
│   └── 07-codex-implementation-prompt.md
├── backend/                 # Go API implementation
└── frontend/                # static operations console served by Nginx
```

## Architecture Principle

A payment is not represented by one mutable status alone. Authorization, capture, settlement, and payout are distinct concepts and will be modeled explicitly as the project grows.

See the `docs/` directory for the implementation contract.

The AI and MCP extension design is documented in `docs/08-ai-mcp-architecture.md`. The MCP server and future agent are separate repositories that call this API over authenticated HTTP.
