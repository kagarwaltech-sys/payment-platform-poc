# 03 - Data Model

PostgreSQL is the initial persistence layer.

## payments

| Column | Type | Notes |
|---|---|---|
| id | UUID PK | Internal payment ID |
| amount | BIGINT | Requested amount in minor units |
| currency | CHAR(3) | ISO 4217 |
| authorized_amount | BIGINT | Cumulative authorized amount |
| captured_amount | BIGINT | Cumulative captured amount |
| status | VARCHAR | Payment state |
| capture_method | VARCHAR | `manual` initially |
| reference | VARCHAR NULL | Merchant/order reference |
| processor | VARCHAR NULL | `mock` initially |
| processor_payment_id | VARCHAR NULL | PSP reference |
| created_at | TIMESTAMPTZ | |
| updated_at | TIMESTAMPTZ | |

Constraints:

- `amount > 0`
- `authorized_amount >= 0`
- `captured_amount >= 0`
- `captured_amount <= authorized_amount`
- `authorized_amount <= amount` for first slice

## payment_captures

| Column | Type | Notes |
|---|---|---|
| id | UUID PK | Internal capture ID |
| payment_id | UUID FK | Parent payment |
| amount | BIGINT | Captured amount |
| processor_capture_id | VARCHAR | PSP capture reference |
| created_at | TIMESTAMPTZ | |

This table is append-only.

## idempotency_records

| Column | Type | Notes |
|---|---|---|
| idempotency_key | VARCHAR | Key supplied by client |
| operation | VARCHAR | e.g. `CREATE_PAYMENT`, `CAPTURE_PAYMENT` |
| request_hash | VARCHAR | Hash of canonical request data |
| resource_id | UUID NULL | Related payment ID |
| response_code | INT NULL | Original HTTP result |
| response_body | JSONB NULL | Original logical response |
| status | VARCHAR | `IN_PROGRESS`, `COMPLETED`, `FAILED` |
| created_at | TIMESTAMPTZ | |
| updated_at | TIMESTAMPTZ | |

Recommended uniqueness:

```text
UNIQUE(idempotency_key, operation)
```

## ledger_accounts

| Column | Type | Notes |
|---|---|---|
| id | UUID PK | |
| code | VARCHAR UNIQUE | Stable account code |
| name | VARCHAR | Human-readable name |
| account_type | VARCHAR | ASSET / LIABILITY / REVENUE / EXPENSE |
| currency | CHAR(3) | Initial implementation may use one currency per account |
| created_at | TIMESTAMPTZ | |

Initial accounts for a simple merchant payment:

- `PROCESSOR_RECEIVABLE_USD` - asset
- `MERCHANT_PAYABLE_USD` - liability

## ledger_journals

| Column | Type | Notes |
|---|---|---|
| id | UUID PK | |
| event_type | VARCHAR | e.g. `PAYMENT_CAPTURED` |
| reference_type | VARCHAR | e.g. `payment_capture` |
| reference_id | UUID | Capture ID |
| currency | CHAR(3) | |
| created_at | TIMESTAMPTZ | Immutable |

Recommended uniqueness:

```text
UNIQUE(event_type, reference_id)
```

This prevents accidental duplicate journals for the same capture.

## ledger_entries

| Column | Type | Notes |
|---|---|---|
| id | UUID PK | |
| journal_id | UUID FK | Parent journal |
| account_id | UUID FK | Ledger account |
| direction | VARCHAR | `DEBIT` or `CREDIT` |
| amount | BIGINT | Positive minor units |
| created_at | TIMESTAMPTZ | Immutable |

Invariant per journal:

```text
SUM(DEBIT amounts) = SUM(CREDIT amounts)
```

## Concurrency

Capture must protect against concurrent requests. Implementation should use either row locking (`SELECT ... FOR UPDATE`) or an equivalent optimistic-concurrency strategy so cumulative capture cannot exceed authorization.
