# Backend

For the full API and PostgreSQL stack, use the Compose instructions in the parent [README](../README.md).

Requires Go 1.26+ and PostgreSQL 14+. Set `DATABASE_URL` (for example, `postgres://postgres:postgres@localhost:5432/payments?sslmode=disable`), create the database, and apply migrations in order:

```powershell
Get-Content migrations/001_initial.sql | psql $env:DATABASE_URL
go mod tidy
go test ./...
go run ./cmd/api
```

The API listens on `:8080`. All write requests require `Idempotency-Key`.

```powershell
$h=@{'Idempotency-Key'='create-1';'Content-Type'='application/json'}
Invoke-RestMethod http://localhost:8080/api/v1/payments -Method Post -Headers $h -Body '{"amount":10000,"currency":"USD","capture_method":"manual","reference":"order-1001"}'
```

`go test ./...` has domain unit tests. PostgreSQL integration tests should be run against an isolated database after applying the migration; the application enforces row locking, idempotency persistence, capture/journal atomicity, and database constraints there.
