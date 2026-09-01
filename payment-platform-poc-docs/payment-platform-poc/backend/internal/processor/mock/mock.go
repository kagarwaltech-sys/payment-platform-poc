package mock

import (
	"context"
	"fmt"
	"sync"
)

// Processor is deterministic and exposes call counts for tests.
type Processor struct {
	mu                                  sync.Mutex
	authorizations, captures, refunds   int
	AuthorizeErr, CaptureErr, RefundErr error
}

func (p *Processor) Name() string { return "mock" }

func (p *Processor) Authorize(_ context.Context, paymentID string, _ int64, _ string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.authorizations++
	if p.AuthorizeErr != nil {
		return "", p.AuthorizeErr
	}
	return "mock_pay_" + paymentID, nil
}
func (p *Processor) Capture(_ context.Context, paymentRef string, amount int64, key string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.captures++
	if p.CaptureErr != nil {
		return "", p.CaptureErr
	}
	return fmt.Sprintf("mock_cap_%s_%d_%s", paymentRef, amount, key), nil
}
func (p *Processor) Refund(_ context.Context, paymentRef string, amount int64, key, _ string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.refunds++
	if p.RefundErr != nil {
		return "", p.RefundErr
	}
	return fmt.Sprintf("mock_ref_%s_%d_%s", paymentRef, amount, key), nil
}
func (p *Processor) Counts() (int, int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.authorizations, p.captures
}
