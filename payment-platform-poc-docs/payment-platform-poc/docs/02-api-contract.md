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

---

## 2. Get Payment

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

## 3. Authorize Payment

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

## 4. Capture Payment

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
- Partial capture results in `PARTIALLY_CAPTURED`.
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
