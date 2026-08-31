package router

import (
	"context"
	"fmt"

	"payment-platform/backend/internal/payment"
)

type Router struct {
	processors map[string]payment.Processor
	selectName func(int64) string
}

func New(processors map[string]payment.Processor, selectName func(int64) string) *Router {
	return &Router{processors: processors, selectName: selectName}
}

func (r *Router) ProcessorForAmount(amount int64) payment.Processor {
	if r.selectName == nil {
		return nil
	}
	return r.processors[r.selectName(amount)]
}

func (r *Router) ProcessorByName(name string) (payment.Processor, bool) {
	processor, ok := r.processors[name]
	return processor, ok
}

func (r *Router) Authorize(ctx context.Context, paymentID string, amount int64, currency string) (string, error) {
	processor := r.ProcessorForAmount(amount)
	if processor == nil {
		return "", fmt.Errorf("no processor configured for amount %d", amount)
	}
	return processor.Authorize(ctx, paymentID, amount, currency)
}

func (r *Router) Capture(context.Context, string, int64, string) (string, error) {
	return "", fmt.Errorf("processor must be resolved before capture")
}

var _ payment.Processor = (*Router)(nil)
