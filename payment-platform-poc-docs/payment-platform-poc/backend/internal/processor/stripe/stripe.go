package stripe

import (
	"context"
	"fmt"

	"payment-platform/backend/internal/payment"

	stripego "github.com/stripe/stripe-go/v86"
)

type Processor struct {
	client        *stripego.Client
	paymentMethod string
}

func New(secretKey, paymentMethod string) *Processor {
	if paymentMethod == "" {
		paymentMethod = "pm_card_visa"
	}
	return &Processor{client: stripego.NewClient(secretKey), paymentMethod: paymentMethod}
}

func (p *Processor) Name() string { return "stripe" }

func (p *Processor) SupportsPartialCapture() bool { return false }

func (p *Processor) Authorize(ctx context.Context, paymentID string, amount int64, currency string) (string, error) {
	params := &stripego.PaymentIntentCreateParams{
		Amount:        stripego.Int64(amount),
		Currency:      stripego.String(currency),
		CaptureMethod: stripego.String("manual"),
		Confirm:       stripego.Bool(true),
		PaymentMethod: stripego.String(p.paymentMethod),
		Description:   stripego.String(fmt.Sprintf("Payment %s", paymentID)),
		AutomaticPaymentMethods: &stripego.PaymentIntentCreateAutomaticPaymentMethodsParams{
			Enabled:        stripego.Bool(true),
			AllowRedirects: stripego.String("never"),
		},
	}
	params.AddMetadata("payment_id", paymentID)
	intent, err := p.client.V1PaymentIntents.Create(ctx, params)
	if err != nil {
		return "", err
	}
	if intent.Status != stripego.PaymentIntentStatusRequiresCapture {
		return "", fmt.Errorf("stripe payment intent %s has status %s, expected requires_capture", intent.ID, intent.Status)
	}
	return intent.ID, nil
}

func (p *Processor) Capture(ctx context.Context, paymentRef string, amount int64, key string) (string, error) {
	return p.capture(ctx, paymentRef, amount, key)
}

func (p *Processor) CaptureFinal(ctx context.Context, paymentRef string, amount int64, key string, final bool) (string, error) {
	return p.capture(ctx, paymentRef, amount, key)
}

func (p *Processor) capture(ctx context.Context, paymentRef string, amount int64, key string) (string, error) {
	params := &stripego.PaymentIntentCaptureParams{AmountToCapture: stripego.Int64(amount)}
	params.SetIdempotencyKey(key)
	intent, err := p.client.V1PaymentIntents.Capture(ctx, paymentRef, params)
	if err != nil {
		return "", err
	}
	return intent.ID, nil
}

var _ payment.Processor = (*Processor)(nil)
