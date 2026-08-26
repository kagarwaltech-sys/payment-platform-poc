package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"payment-platform/backend/internal/httpapi"
	"payment-platform/backend/internal/processor/mock"
	"payment-platform/backend/internal/store"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Error("DATABASE_URL is required")
		os.Exit(1)
	}
	s, err := store.New(context.Background(), dsn)
	if err != nil {
		log.Error("database unavailable", "error", err)
		os.Exit(1)
	}
	defer s.Pool.Close()
	api := &httpapi.API{Service: &store.Service{Store: s, Processor: &mock.Processor{}}, Logger: log}
	log.Info("listening", "addr", ":8080")
	if err := http.ListenAndServe(":8080", api.Router()); err != nil {
		log.Error("server stopped", "error", err)
	}
}
