# Payment Platform Operations Console

This directory contains the browser-based demo console for the payment platform. It is a dependency-free HTML, CSS, and JavaScript client served by Nginx.

## What it demonstrates

- POS payment creation or create-only/manual lifecycle flows.
- Authorization, capture, and partial/full refunds.
- Idempotency-key reuse for safe retry demonstrations.
- Server-side processor routing, including mock, Stripe, and Adyen modes.
- AI / MCP tab showing tool discovery, agent planning, approval, and execution traces.
- Payment state, processor references, captures, refunds, and ledger journals.

The console calls the payment API through relative `/api/...` URLs. In Docker Compose, Nginx reverse-proxies those requests to the `api` service, so the browser does not need a separate CORS configuration.

## Files

```text
frontend/
├── index.html   # console layout and controls
├── styles.css   # responsive console styling
├── app.js       # API calls, state rendering, actions, and activity timeline
├── nginx.conf   # static hosting plus /api and /demo reverse proxies
├── Dockerfile   # Nginx image definition
└── README.md
```

## Run

From this application directory:

```powershell
docker compose up --build
```

Open `http://localhost:3000`. The payment API remains available at `http://localhost:8080`.

Select the `AI / MCP Demo` tab to inspect a payment or request a refund. The browser calls the demo gateway at `/demo`; the gateway launches the real MCP server over stdio. Refund requests show a proposal and require the `Approve and execute` button.

Processor routing is configured on the API, not selected by the browser. For example:

```powershell
$env:PAYMENT_PROCESSOR='amount'
$env:PROCESSOR_AMOUNT_THRESHOLD='10000'
$env:PROCESSOR_ADYEN_THRESHOLD='50000'
docker compose up -d --force-recreate api frontend
```

With that configuration, a payment of `15000` minor units routes to Stripe. The resulting processor is shown in the Payment State panel.
