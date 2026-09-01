package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"strconv"

	"payment-platform/backend/internal/httpapi"
	"payment-platform/backend/internal/payment"
	adyenprocessor "payment-platform/backend/internal/processor/adyen"
	"payment-platform/backend/internal/processor/mock"
	processorrouter "payment-platform/backend/internal/processor/router"
	stripeprocessor "payment-platform/backend/internal/processor/stripe"
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
	var processor payment.Processor
	processorMode := os.Getenv("PAYMENT_PROCESSOR")
	if processorMode == "mock" {
		processor = &mock.Processor{}
	} else {
		stripeKey := os.Getenv("STRIPE_SECRET_KEY")
		if stripeKey == "" {
			log.Error("STRIPE_SECRET_KEY is required when PAYMENT_PROCESSOR is not mock")
			os.Exit(1)
		}
		stripe := stripeprocessor.New(stripeKey, os.Getenv("STRIPE_PAYMENT_METHOD"))
		if processorMode == "amount" {
			threshold, err := strconv.ParseInt(os.Getenv("PROCESSOR_AMOUNT_THRESHOLD"), 10, 64)
			if err != nil {
				threshold = 10000
			}
			adyenThreshold, err := strconv.ParseInt(os.Getenv("PROCESSOR_ADYEN_THRESHOLD"), 10, 64)
			if err != nil {
				adyenThreshold = 50000
			}
			processors := map[string]payment.Processor{
				"mock":   &mock.Processor{},
				"stripe": stripe,
			}
			adyenKey := os.Getenv("ADYEN_API_KEY")
			adyenMerchant := os.Getenv("ADYEN_MERCHANT_ACCOUNT")
			if adyenKey != "" && adyenMerchant != "" {
				adyen, err := adyenprocessor.New(adyenKey, adyenMerchant, os.Getenv("ADYEN_BASE_URL"), os.Getenv("ADYEN_PAYMENT_METHOD_JSON"))
				if err != nil {
					log.Error("invalid Adyen configuration", "error", err)
					os.Exit(1)
				}
				processors["adyen"] = adyen
			} else {
				log.Warn("Adyen is not configured; amounts above the Adyen threshold will be unavailable")
			}
			processor = processorrouter.New(processors, func(amount int64) string {
				if amount <= threshold {
					return "mock"
				}
				if amount <= adyenThreshold {
					return "stripe"
				}
				return "adyen"
			})
		} else {
			processor = stripe
		}
	}
	api := &httpapi.API{Service: &store.Service{Store: s, Processor: processor}, Logger: log}
	log.Info("listening", "addr", ":8080")
	if err := http.ListenAndServe(":8080", api.Router()); err != nil {
		log.Error("server stopped", "error", err)
	}
}
