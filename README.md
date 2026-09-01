# Payment Platform POC

A Go-based payment platform proof of concept with a REST API, PostgreSQL persistence, processor adapters, idempotent mutations, refunds, and separate MCP and agent components.

## Components

| Component | Responsibility | Default interface |
| --- | --- | --- |
| `payment-platform-poc` | Payment API, state machine, persistence, processors, and ledger | HTTP on `:8080` |
| `payment-platform-mcp` | MCP tools backed by the payment API | MCP stdio |
| `payment-platform-agent` | Interactive, approval-gated client of the MCP server | CLI |

The API is authoritative for payment state. MCP never accesses PostgreSQL or payment providers directly, and the agent never calls the API or providers directly.

## Features

- Create, authorize, capture, retrieve, and refund payments.
- Complete a POS-style payment with one request.
- Support full and partial refunds up to the captured amount.
- Require idempotency keys for write operations and protect retries from duplicate processor calls.
- Route payments to mock, Stripe, or Adyen based on a configured amount policy.
- Store money as integer minor units and record balanced ledger journals.
- Expose `get_payment`, `pay_payment`, and approval-gated `refund_payment` through MCP.

Disputes, webhooks, settlement, reconciliation, and recovery after ambiguous provider results are outside the current scope.

## Repository layout

```text
payment-platform-poc/
├── payment-platform-poc-docs/payment-platform-poc/
│   ├── backend/                 # Go HTTP API and PostgreSQL integration
│   │   ├── cmd/api/              # API entrypoint and configuration
│   │   ├── internal/httpapi/     # HTTP routes and error mapping
│   │   ├── internal/payment/     # Domain types and state rules
│   │   ├── internal/store/       # Transactions, idempotency, and ledger writes
│   │   ├── internal/processor/   # Mock, Stripe, Adyen, and routing adapters
│   │   └── migrations/            # PostgreSQL schema migrations
│   ├── docs/                     # API, data model, state, ledger, and MCP design
│   └── docker-compose.yml        # API plus PostgreSQL development stack
├── payment-platform-mcp/        # MCP server component
└── payment-platform-agent/      # Interactive agent component
```

See the [application README](payment-platform-poc-docs/payment-platform-poc/README.md) for backend configuration and the [API contract](payment-platform-poc-docs/payment-platform-poc/docs/02-api-contract.md) for endpoint details.

## Quick start

GitHub Codespaces uses `.devcontainer/devcontainer.json` to start the API and PostgreSQL services. To start the same stack manually from the repository root:

```powershell
docker compose -f payment-platform-poc-docs/payment-platform-poc/docker-compose.yml up --build
```

The API is available at `http://localhost:8080`. For a provider-free local run:

```powershell
$env:PAYMENT_PROCESSOR='mock'
docker compose -f payment-platform-poc-docs/payment-platform-poc/docker-compose.yml up --build
```

The default mode is Stripe test mode and requires `STRIPE_SECRET_KEY`. Amount routing requires `PAYMENT_PROCESSOR=amount` plus Stripe and Adyen configuration.

## Example API call

```powershell
$headers=@{'Idempotency-Key'='pos-payment-1';'Content-Type'='application/json'}
$body='{"amount":10000,"currency":"USD","capture_method":"manual","reference":"order-1001"}'
Invoke-RestMethod http://localhost:8080/api/v1/payments/pay -Method Post -Headers $headers -Body $body
```

All write requests use integer minor units and require `Idempotency-Key`.

## Testing

```powershell
Push-Location payment-platform-poc-docs/payment-platform-poc/backend
go test ./...
go vet ./...
Pop-Location

Push-Location payment-platform-mcp
go test ./...; go vet ./...; go build ./...
Pop-Location

Push-Location payment-platform-agent
go test ./...; go vet ./...; go build ./...
Pop-Location
```

Backend integration tests use `TEST_DATABASE_URL` and an isolated PostgreSQL database. To reset Compose data and rerun migrations:

```powershell
docker compose -f payment-platform-poc-docs/payment-platform-poc/docker-compose.yml down -v
```
