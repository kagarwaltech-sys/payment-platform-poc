FROM golang:1.26-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/payment-api ./cmd/api

FROM alpine:3.22

RUN adduser -D -H -u 10001 app
COPY --from=build /out/payment-api /usr/local/bin/payment-api
USER app
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/payment-api"]