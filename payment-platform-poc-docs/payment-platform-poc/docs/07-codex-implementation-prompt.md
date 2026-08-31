# 07 - Codex Implementation Prompt

Use this prompt from the repository root after these documents are committed.

> Historical note: this prompt describes the original mock-only implementation slice. The current implementation additionally includes Stripe and Adyen adapters, amount-based processor routing, and the POS one-call endpoint documented in the other files.

---

You are implementing the first backend vertical slice of this payment-platform POC.

Before writing code, read in full:

- README.md
- docs/01-system-overview.md
- docs/02-api-contract.md
- docs/03-data-model.md
- docs/04-state-machine.md
- docs/05-ledger-design.md
- docs/06-idempotency.md

Treat those documents as the implementation contract. Do not invent new product behavior without documenting the reason.

## Technology

Implement the backend in Go.

Preferred stack unless there is a strong reason to deviate:

- Go current stable release
- `net/http` or a lightweight HTTP router such as `chi`
- PostgreSQL
- `pgx` for PostgreSQL access
- SQL migrations using a simple migration tool or versioned SQL files
- Structured logging
- Standard Go testing

Keep dependencies minimal. Avoid heavyweight frameworks.

## Implement only this scope

1. Create payment
2. Get payment
3. Authorize payment
4. Capture payment, including partial capture
5. Mock PSP adapter as the initial processor
6. Payment state machine validation
7. Persistent idempotency handling
8. Immutable double-entry journal creation on successful capture
9. PostgreSQL migrations
10. Unit and integration tests for important invariants
11. Local developer instructions

The follow-up implementation adds Stripe and Adyen adapters behind the same processor interface and selects among registered processors by amount.

Do NOT implement yet:

- refunds
- voids
- disputes
- webhooks
- settlement
- reconciliation jobs
- webhook-driven provider reconciliation
- authentication/authorization
- frontend UI

## Non-negotiable invariants

- Never use float32/float64 for money.
- Store money in integer minor units.
- Capture cannot exceed remaining authorized amount.
- Reject invalid payment-state transitions.
- Protect capture against concurrent over-capture.
- Payment capture state changes and ledger writes must be in one local PostgreSQL transaction.
- Ledger journals and entries are append-only/immutable.
- Every journal must balance.
- One business capture event must not generate more than one ledger journal.
- Same idempotency key + same request returns the original logical result.
- Same idempotency key + different request returns conflict.
- Concurrent requests using the same idempotency key must not execute the PSP operation twice.

## Architecture

Use clear domain boundaries. A reasonable package direction is:

```text
backend/
├── cmd/api/
├── internal/payment/
├── internal/ledger/
├── internal/idempotency/
├── internal/processor/
│   └── mock/
├── internal/httpapi/
├── internal/store/
└── migrations/
```

This is guidance, not a requirement. Prefer clarity over unnecessary abstraction.

Define a processor interface around domain concepts rather than exposing provider SDK objects directly.

The mock PSP should generate stable processor payment/capture references and make it possible for tests to observe how many times authorize/capture were called.

## Testing expectations

At minimum test:

1. Create payment.
2. Successful authorization.
3. Authorization from invalid state is rejected.
4. Full capture.
5. Partial capture.
6. Multiple partial captures up to authorization amount.
7. Over-capture rejected.
8. Capture before authorization rejected.
9. Same capture idempotency key replay does not call processor twice.
10. Reusing idempotency key with changed amount returns conflict.
11. Concurrent capture requests cannot over-capture.
12. Capture journal balances.
13. Duplicate capture event cannot create duplicate journal.
14. Failed local ledger write rolls back local payment/capture changes.

## Deliverables

After implementation:

1. Run formatting and tests.
2. Provide a concise summary of files created.
3. Explain key design choices.
4. List known limitations, especially the distributed failure window between a successful PSP call and local database commit.
5. Do not hide failing tests. Fix them or clearly report them.

Implement the smallest clean solution that satisfies the documented contract.
