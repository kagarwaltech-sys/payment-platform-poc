# 01 - System Overview

## Goal

Build the smallest useful vertical slice of a payment platform that demonstrates architecture-level payment concepts rather than just SDK integration.

## Initial Flow

```mermaid
flowchart LR
    C[Client / Future React UI] -->|REST + Idempotency-Key| API[Payment API]
    API --> IDEM[Idempotency Store]
    API --> PS[Payment Service]
    PS --> PSP[PSP Adapter]
    PSP --> MOCK[Mock PSP]
    PS --> LEDGER[Ledger Service]
    PS --> DB[(PostgreSQL)]
    LEDGER --> DB
    IDEM --> DB
```

## Authorization Flow

```mermaid
sequenceDiagram
    participant Client
    participant API as Payment API
    participant Service as Payment Service
    participant PSP as Mock PSP
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
    participant PSP as Mock PSP
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
- **Ledger Service** owns accounting journal creation and balancing rules.
- **Idempotency Store** guarantees safe retry behavior for write requests.
- **PostgreSQL** is the initial source of truth for internal payment state and ledger data.

## First-Slice Invariants

1. Never use floating-point values for money.
2. Currency is explicit on every payment.
3. Capture amount cannot exceed remaining authorized amount.
4. Invalid state transitions are rejected.
5. A successful capture and its ledger journal must commit atomically.
6. Ledger journals are immutable after creation.
7. Every journal must balance: total debits = total credits.
8. Retrying the same operation with the same idempotency key returns the original logical result.
