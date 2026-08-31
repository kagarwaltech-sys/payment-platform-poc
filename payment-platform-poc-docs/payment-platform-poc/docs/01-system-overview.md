# 01 - System Overview

## Goal

Build the smallest useful vertical slice of a payment platform that demonstrates architecture-level payment concepts rather than just SDK integration.

## Current Flow

```mermaid
flowchart LR
    C[Client / Future React UI] -->|REST + Idempotency-Key| API[Payment API]
    API --> IDEM[Idempotency Store]
    API --> PS[Payment Service]
    PS --> ROUTER[Processor Router]
    ROUTER --> MOCK[Mock Processor]
    ROUTER --> STRIPE[Stripe Adapter]
    ROUTER --> ADYEN[Adyen Adapter]
    PS --> LEDGER[Ledger Service]
    PS --> DB[(PostgreSQL)]
    LEDGER --> DB
    IDEM --> DB
```

## POS Payment Flow

`POST /api/v1/payments/pay` is the server-controlled path for a POS-style integration. It creates the internal payment, selects a processor by amount, authorizes, captures the full amount, records the ledger journal, and returns the final `CAPTURED` state.

The selected processor name and provider reference are persisted before capture. Capture resolves that persisted processor, so a payment does not switch providers mid-flow.

## Authorization Flow

```mermaid
sequenceDiagram
    participant Client
    participant API as Payment API
    participant Service as Payment Service
    participant PSP as Selected PSP
    participant DB as PostgreSQL

    Client->>API: POST /payments/{id}/authorize
    API->>Service: authorize(paymentId)
    Service->>DB: Load payment
    Service->>PSP: Authorize(amount, currency)
    PSP-->>Service: Approved + processor reference
    Service->>DB: Persist AUTHORIZED state
    Service-->>API: Payment response
    API-->>Client: 200 AUTHORIZED
```

## Capture Flow

```mermaid
sequenceDiagram
    participant Client
    participant API as Payment API
    participant Service as Payment Service
    participant PSP as Selected PSP
    participant Ledger as Ledger Service
    participant DB as PostgreSQL

    Client->>API: POST /payments/{id}/capture
    API->>Service: capture(paymentId, amount)
    Service->>DB: Load authorized payment
    Service->>PSP: Capture(processorRef, amount)
    PSP-->>Service: Capture successful
    Service->>DB: Begin transaction
    Service->>DB: Update captured amount/state
    Service->>Ledger: Create balanced journal
    Ledger->>DB: Insert journal + entries
    Service->>DB: Commit transaction
    Service-->>Client: CAPTURED
```

## Key Architectural Boundaries

- **Payment API** owns HTTP concerns and validation.
- **Payment Service** owns payment business rules and state transitions.
- **PSP Adapter** hides processor-specific APIs.
- **Processor Router** maps an amount-based policy to named processor adapters.
- **Ledger Service** owns accounting journal creation and balancing rules.
- **Idempotency Store** guarantees safe retry behavior for write requests.
- **PostgreSQL** is the initial source of truth for internal payment state and ledger data.

## Implemented Processor Adapters

- `mock`: deterministic local adapter used by tests and low-value routing.
- `stripe`: Stripe test-mode PaymentIntents with manual capture.
- `adyen`: Adyen test Checkout API with manual capture.

Stripe and Adyen credentials are supplied through environment variables and are not stored in the repository.

## First-Slice Invariants

1. Never use floating-point values for money.
2. Currency is explicit on every payment.
3. Capture amount cannot exceed remaining authorized amount.
4. Invalid state transitions are rejected.
5. A successful capture and its ledger journal must commit atomically.
6. Ledger journals are immutable after creation.
7. Every journal must balance: total debits = total credits.
8. Retrying the same operation with the same idempotency key returns the original logical result.
