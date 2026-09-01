# 02 - API Contract

## General Conventions

- Base path: `/api/v1`
- Content type: `application/json`
- Money amounts are integer minor units.
  - USD 100.00 = `10000`
- Currency uses ISO 4217 uppercase codes such as `USD`.
- All write endpoints require an `Idempotency-Key` header.
- IDs are UUIDs.

## Error Shape

```json
{
  "error": {
    "code": "INVALID_STATE_TRANSITION",
    "message": "Payment must be AUTHORIZED before capture"
  }
}
```

---

## 1. Create Payment

`POST /api/v1/payments`

Header:

```text
Idempotency-Key: <unique-key>
```

Request:

```json
{
  "amount": 10000,
  "currency": "USD",
  "capture_method": "manual",
  "reference": "order-1001"
}
```

Response `201 Created`:

```json
{
  "id": "8bf6cb57-b0bd-4a62-9e55-18ae0d16f001",
  "amount": 10000,
  "currency": "USD",
  "authorized_amount": 0,
  "captured_amount": 0,
  "status": "CREATED",
  "capture_method": "manual",
  "reference": "order-1001"
}
```

Rules:

- `amount > 0`
- Initial status is `CREATED`.
- Initial authorized and captured amounts are zero.

This endpoint creates only the internal payment record. Use the POS endpoint for a server-controlled payment that completes authorization and capture in one request.

---

## 2. POS Payment

`POST /api/v1/payments/pay`

This endpoint creates the local payment, selects a processor by amount, authorizes, captures the full amount, records the capture and ledger journal, and returns the final state.

Response `200 OK`:

```json
{
  "id": "8bf6cb57-b0bd-4a62-9e55-18ae0d16f001",
  "amount": 10000,
  "currency": "USD",
  "authorized_amount": 10000,
  "captured_amount": 10000,
  "status": "CAPTURED",
  "capture_method": "manual",
  "processor": "stripe",
  "processor_payment_id": "pi_123",
  "processor_capture_id": "pi_123"
}
```

The client does not call authorization or capture separately for this flow.

---

## 3. Get Payment

`GET /api/v1/payments/{payment_id}`

Response `200 OK`:

```json
{
  "id": "8bf6cb57-b0bd-4a62-9e55-18ae0d16f001",
  "amount": 10000,
  "currency": "USD",
  "authorized_amount": 10000,
  "captured_amount": 0,
  "status": "AUTHORIZED",
  "capture_method": "manual",
  "processor": "mock",
  "processor_payment_id": "mock_pay_123"
}
```

---

## 4. Authorize Payment

`POST /api/v1/payments/{payment_id}/authorize`

Header:

```text
Idempotency-Key: <unique-key>
```

Request:

```json
{}
```

Response `200 OK`:

```json
{
  "id": "8bf6cb57-b0bd-4a62-9e55-18ae0d16f001",
  "status": "AUTHORIZED",
  "authorized_amount": 10000,
  "captured_amount": 0,
  "processor": "mock",
  "processor_payment_id": "mock_pay_123"
}
```

Rules:

- Allowed only from `CREATED` in slice 1.
- PSP authorization must succeed before local state becomes `AUTHORIZED`.
- Full payment amount is authorized in slice 1.

---

## 5. Capture Payment

`POST /api/v1/payments/{payment_id}/capture`

Header:

```text
Idempotency-Key: <unique-key>
```

Request:

```json
{
  "amount": 10000
}
```

Response `200 OK`:

```json
{
  "id": "8bf6cb57-b0bd-4a62-9e55-18ae0d16f001",
  "status": "CAPTURED",
  "authorized_amount": 10000,
  "captured_amount": 10000,
  "processor": "mock",
  "processor_payment_id": "mock_pay_123",
  "processor_capture_id": "mock_cap_456"
}
```

Rules:

- Payment must be `AUTHORIZED` or `PARTIALLY_CAPTURED`.
- Capture amount must be greater than zero.
- Total captured amount cannot exceed authorized amount.
- Full capture results in `CAPTURED`.
- Partial capture results in `PARTIALLY_CAPTURED` for processors that support it. The current Stripe and Adyen adapters require the full remaining authorized amount.
- Payment state update and ledger journal creation must commit in the same DB transaction.

## Suggested HTTP Status Codes

| Case | Status |
|---|---:|
| Successful create | 201 |
| Successful read/write | 200 |
| Invalid request | 400 |
| Payment not found | 404 |
| Duplicate key with mismatched request | 409 |
| Invalid state transition | 409 |
| Internal/processor error | 500 or mapped 5xx |

## Processor Routing

With `PAYMENT_PROCESSOR=amount`, the current policy is:

| Amount in minor units | Processor |
|---:|---|
| `<= PROCESSOR_AMOUNT_THRESHOLD` | mock |
| `> PROCESSOR_AMOUNT_THRESHOLD` and `<= PROCESSOR_ADYEN_THRESHOLD` | stripe |
| `> PROCESSOR_ADYEN_THRESHOLD` | adyen |

The router uses a named processor registry. Additional adapters can be registered without changing the payment service or database schema.

## 6. Refund Payment

`POST /api/v1/payments/{payment_id}/refund`

Header:

```text
Idempotency-Key: <unique-key>
```

Request:

```json
{
  "amount": 4000
}
```

Response `200 OK`:

```json
{
  "id": "8bf6cb57-b0bd-4a62-9e55-18ae0d16f001",
  "amount": 10000,
  "currency": "USD",
  "captured_amount": 10000,
  "refunded_amount": 4000,
  "status": "CAPTURED",
  "processor": "stripe",
  "processor_payment_id": "pi_123",
  "processor_refund_id": "re_123"
}
```

Rules:

- A refund amount must be positive.
- A payment must have enough remaining captured amount to refund.
- Multiple partial refunds are allowed until `refunded_amount == captured_amount`.
- Refund state and the balanced refund ledger journal commit in one database transaction.
- Reusing the same idempotency key returns the original refund result without calling the processor again.

## 7. Payment Activity

`GET /api/v1/payments/{payment_id}/activity`

Returns read-only operational activity for the payment console, including capture records, refund records, and the balanced ledger journals associated with those records. This endpoint does not mutate payment or ledger state.
