# 06 - Idempotency Design

## Why

Payment clients retry requests because of timeouts, network failures, gateway retries, or user behavior. Retrying must not create duplicate payments or duplicate captures.

## Contract

Every write endpoint requires:

```text
Idempotency-Key: <client-generated-unique-key>
```

The server stores a canonical hash of the operation and request payload.

## Desired Behavior

### First request

```text
Idempotency-Key: cap-order1001-01
POST /payments/{id}/capture
{ "amount": 10000 }
```

The server:

1. Creates an `IN_PROGRESS` idempotency record.
2. Executes the operation.
3. Stores the resulting resource/response.
4. Marks the record `COMPLETED`.

### Exact retry

Same key + same operation + same request:

```text
Return the original logical response.
Do not call the PSP again.
Do not create another capture.
Do not create another ledger journal.
```

### Key reused with different payload

Same key:

```json
{ "amount": 10000 }
```

then later:

```json
{ "amount": 5000 }
```

Return `409 Conflict` with an error such as:

```json
{
  "error": {
    "code": "IDEMPOTENCY_KEY_REUSED",
    "message": "The idempotency key was already used with a different request"
  }
}
```

## Concurrency

Two requests with the same idempotency key may arrive simultaneously. The persistence strategy must guarantee only one owns execution.

Use a unique database constraint and transaction/locking semantics rather than an in-memory-only mutex.

## POS Composite Operation

`POST /api/v1/payments/pay` uses the client key for the create operation and deterministic derived keys for authorization and capture. A retry with the same client key reuses the completed internal operations and does not intentionally issue duplicate processor calls.

## Important Distributed Failure

A difficult case is:

```text
1. Service calls PSP capture.
2. PSP capture succeeds.
3. Service crashes before recording local success.
```

Slice 1 should document this limitation and keep processor references sufficient for future recovery. A later phase will add PSP lookup + reconciliation to resolve ambiguous outcomes safely.
