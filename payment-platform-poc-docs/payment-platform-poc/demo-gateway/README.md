# Demo Agent Gateway

The demo gateway makes the stdio-based agent and MCP flow visible to the browser console. It launches the actual `payment-platform-mcp` binary, discovers its tools, plans a small set of payment intents, requires approval for mutations, and executes approved calls through MCP.

## Endpoints

- `GET /demo/health` — MCP connection status.
- `GET /demo/tools` — tools discovered from the MCP server.
- `POST /demo/plan` — plans payment creation, inspection, or refund requests.
- `POST /demo/execute` — executes a proposed call; mutations require `confirmed: true`.

This is demonstration infrastructure, not a production LLM runtime. It keeps the browser away from stdio and preserves the agent → MCP → payment API boundary.

## Configuration

```text
PAYMENT_API_URL=http://api:8080
MCP_SERVER_COMMAND=/usr/local/bin/payment-platform-mcp
PAYMENT_API_TOKEN=
```

The Dockerfile builds both the gateway and MCP server from the sibling component source and runs the gateway on port `8081`.
