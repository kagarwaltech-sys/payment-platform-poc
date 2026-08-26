package payment

import (
	"context"
	"errors"
	"fmt"
)

type Status string

const (
	Created             Status = "CREATED"
	Authorized          Status = "AUTHORIZED"
	PartiallyCaptured   Status = "PARTIALLY_CAPTURED"
	Captured            Status = "CAPTURED"
	AuthorizationFailed Status = "AUTHORIZATION_FAILED"
)

var (
	ErrInvalidTransition = errors.New("invalid payment state transition")
	ErrOverCapture       = errors.New("capture amount exceeds remaining authorized amount")
	ErrInvalidAmount     = errors.New("amount must be greater than zero")
)

type Payment struct {
	ID                 string `json:"id"`
	Amount             int64  `json:"amount"`
	Currency           string `json:"currency"`
	AuthorizedAmount   int64  `json:"authorized_amount"`
	CapturedAmount     int64  `json:"captured_amount"`
	Status             Status `json:"status"`
	CaptureMethod      string `json:"capture_method"`
	Reference          string `json:"reference,omitempty"`
	Processor          string `json:"processor,omitempty"`
	ProcessorPaymentID string `json:"processor_payment_id,omitempty"`
}

type Capture struct {
	ID                 string `json:"id"`
	PaymentID          string `json:"payment_id"`
	ProcessorCaptureID string `json:"processor_capture_id"`
	Amount             int64  `json:"amount"`
}

type Processor interface {
	Authorize(context.Context, string, int64, string) (string, error)
	Capture(context.Context, string, int64, string) (string, error)
}

func CaptureStatus(current Status, authorized, captured, amount int64) (Status, error) {
	if current != Authorized && current != PartiallyCaptured {
		return "", ErrInvalidTransition
	}
	if amount <= 0 {
		return "", ErrInvalidAmount
	}
	if amount > authorized-captured {
		return "", ErrOverCapture
	}
	if captured+amount == authorized {
		return Captured, nil
	}
	return PartiallyCaptured, nil
}

func CanAuthorize(status Status) error {
	if status != Created {
		return fmt.Errorf("%w: authorize requires CREATED", ErrInvalidTransition)
	}
	return nil
}
