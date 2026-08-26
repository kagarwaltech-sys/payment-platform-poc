# 05 - Ledger Design

## Purpose

The ledger models financial obligations independently from the payment status field.

The first slice creates ledger entries when a payment is captured.

## Principles

1. Double-entry accounting.
2. Journals are immutable.
3. Entries are immutable.
4. Corrections are future compensating journals, never updates to historical entries.
5. Every journal balances.
6. Money is stored as integer minor units.
7. Each business event should generate at most one journal.

## Initial Capture Journal

For a $100.00 USD capture (`10000` minor units):

```text
Journal: PAYMENT_CAPTURED

Debit   Processor Receivable     10000
Credit  Merchant Payable         10000
                                 -----
Total Debits                     10000
Total Credits                    10000
```

Interpretation:

- The processor/acquirer owes funds to the platform/merchant side, so processor receivable increases.
- The system now owes the merchant, so merchant payable increases.

This is intentionally simplified for the first implementation slice. Processor fees, platform revenue, settlement cash, reserves, refunds, and chargebacks will be added later.

## Partial Capture Example

Authorization: $100.00

First capture: $40.00

```text
Debit   Processor Receivable      4000
Credit  Merchant Payable          4000
```

Second capture: $60.00

```text
Debit   Processor Receivable      6000
Credit  Merchant Payable          6000
```

Two separate immutable journals should exist, one for each capture event.

## Atomicity Requirement

After the PSP confirms capture, the local database transaction must include:

1. Insert `payment_captures` row.
2. Update payment cumulative `captured_amount` and status.
3. Insert ledger journal.
4. Insert balanced ledger entries.
5. Mark idempotency operation complete.
6. Commit.

If any of these local operations fails, the local transaction rolls back.

Important: a later project phase must handle the distributed failure case where the external PSP capture succeeds but the local DB transaction fails. That will require recovery/reconciliation rather than pretending a local ACID transaction can include the PSP.
