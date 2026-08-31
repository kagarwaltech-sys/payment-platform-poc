package adyen

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"payment-platform/backend/internal/payment"
)

type Processor struct {
	apiKey          string
	merchantAccount string
	baseURL         string
	paymentMethod   map[string]any
	client          *http.Client
}

func New(apiKey, merchantAccount, baseURL, paymentMethodJSON string) (*Processor, error) {
	if baseURL == "" {
		baseURL = "https://checkout-test.adyen.com/v71"
	}
	if paymentMethodJSON == "" {
		paymentMethodJSON = `{"type":"scheme","number":"4111111111111111","expiryMonth":"03","expiryYear":"2030","cvc":"737"}`
	}
	var paymentMethod map[string]any
	if err := json.Unmarshal([]byte(paymentMethodJSON), &paymentMethod); err != nil {
		return nil, fmt.Errorf("invalid ADYEN_PAYMENT_METHOD_JSON: %w", err)
	}
	return &Processor{apiKey: apiKey, merchantAccount: merchantAccount, baseURL: strings.TrimRight(baseURL, "/"), paymentMethod: paymentMethod, client: http.DefaultClient}, nil
}

func (p *Processor) Name() string { return "adyen" }

func (p *Processor) SupportsPartialCapture() bool { return false }

func (p *Processor) Authorize(ctx context.Context, paymentID string, amount int64, currency string) (string, error) {
	body := map[string]any{
		"amount":          map[string]any{"currency": currency, "value": amount},
		"reference":       paymentID,
		"merchantAccount": p.merchantAccount,
		"captureDelay":    "manual",
		"paymentMethod":   p.paymentMethod,
	}
	var response struct {
		ResultCode   string `json:"resultCode"`
		PSPReference string `json:"pspReference"`
	}
	if err := p.post(ctx, "/payments", "", body, &response); err != nil {
		return "", err
	}
	if response.ResultCode != "Authorised" {
		return "", fmt.Errorf("adyen payment %s returned resultCode %q", paymentID, response.ResultCode)
	}
	return response.PSPReference, nil
}

func (p *Processor) Capture(ctx context.Context, paymentRef string, amount int64, key string) (string, error) {
	body := map[string]any{
		"amount":          map[string]any{"value": amount},
		"merchantAccount": p.merchantAccount,
		"reference":       paymentRef + "-capture",
	}
	var response struct {
		PSPReference string `json:"pspReference"`
	}
	if err := p.post(ctx, "/payments/"+paymentRef+"/captures", key, body, &response); err != nil {
		return "", err
	}
	return response.PSPReference, nil
}

func (p *Processor) post(ctx context.Context, path, idempotencyKey string, body any, response any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("X-API-Key", p.apiKey)
	req.Header.Set("Content-Type", "application/json")
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	res, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("adyen API returned HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(data)))
	}
	if err := json.Unmarshal(data, response); err != nil {
		return fmt.Errorf("decode Adyen response: %w", err)
	}
	return nil
}

var _ payment.Processor = (*Processor)(nil)
