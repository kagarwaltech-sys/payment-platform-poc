FROM golang:1.26-alpine AS mcp-build
WORKDIR /src/payment-platform-mcp
COPY payment-platform-mcp/go.mod payment-platform-mcp/go.sum ./
RUN go mod download
COPY payment-platform-mcp/ .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/payment-platform-mcp ./cmd/server

FROM golang:1.26-alpine AS gateway-build
WORKDIR /src/demo-gateway
COPY payment-platform-poc/payment-platform-poc-docs/payment-platform-poc/demo-gateway/go.mod payment-platform-poc/payment-platform-poc-docs/payment-platform-poc/demo-gateway/go.sum ./
RUN go mod download
COPY payment-platform-poc/payment-platform-poc-docs/payment-platform-poc/demo-gateway/ .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/demo-gateway .

FROM alpine:3.22
RUN adduser -D -H -u 10001 app
COPY --from=gateway-build /out/demo-gateway /usr/local/bin/demo-gateway
COPY --from=mcp-build /out/payment-platform-mcp /usr/local/bin/payment-platform-mcp
USER app
EXPOSE 8081
ENTRYPOINT ["/usr/local/bin/demo-gateway"]
