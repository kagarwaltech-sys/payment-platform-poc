# payment-platform-poc
A small POC to show case payment platform to integrate with payment service providers.

## Run in GitHub Codespaces

Codespaces uses `.devcontainer/devcontainer.json` to start the API and PostgreSQL services. The API is forwarded on port 8080.

To start the same stack manually from the repository root:

```powershell
docker compose -f payment-platform-poc-docs/payment-platform-poc/docker-compose.yml up --build
```

See [the application README](payment-platform-poc-docs/payment-platform-poc/README.md) for API and test commands.
