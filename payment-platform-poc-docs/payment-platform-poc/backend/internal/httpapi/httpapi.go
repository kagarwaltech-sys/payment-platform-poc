package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"payment-platform/backend/internal/payment"
	"payment-platform/backend/internal/store"
)

type API struct {
	Service *store.Service
	Logger  *slog.Logger
}

func (a *API) Router() http.Handler {
	r := chi.NewRouter()
	r.Post("/api/v1/payments", a.create)
	r.Post("/api/v1/payments/pay", a.pay)
	r.Get("/api/v1/payments/{id}", a.get)
	r.Post("/api/v1/payments/{id}/authorize", a.authorize)
	r.Post("/api/v1/payments/{id}/capture", a.capture)
	return r
}
func (a *API) pay(w http.ResponseWriter, r *http.Request) {
	k, ok := key(w, r)
	if !ok {
		return
	}
	var q store.CreateRequest
	if err := decode(r, &q); err != nil {
		errorResponse(w, 400, err)
		return
	}
	p, processorID, code, err := a.Service.Pay(r.Context(), k, q)
	if err != nil {
		errorResponse(w, statusFor(err), err)
		return
	}
	write(w, code, struct {
		payment.Payment
		ProcessorCaptureID string `json:"processor_capture_id"`
	}{p, processorID})
}
func write(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}
func errorResponse(w http.ResponseWriter, code int, err error) {
	c := "INTERNAL_ERROR"
	switch {
	case errors.Is(err, store.ErrIdempotencyConflict):
		c = "IDEMPOTENCY_KEY_REUSED"
	case errors.Is(err, payment.ErrInvalidTransition):
		c = "INVALID_STATE_TRANSITION"
	case errors.Is(err, payment.ErrOverCapture), errors.Is(err, payment.ErrInvalidAmount):
		c = "INVALID_REQUEST"
	case errors.Is(err, store.ErrPartialCaptureUnsupported):
		c = "PARTIAL_CAPTURE_UNSUPPORTED"
	case errors.Is(err, pgx.ErrNoRows):
		c = "PAYMENT_NOT_FOUND"
	case errors.Is(err, store.ErrInProgress):
		c = "IDEMPOTENCY_IN_PROGRESS"
	}
	write(w, code, map[string]any{"error": map[string]string{"code": c, "message": err.Error()}})
}
func key(w http.ResponseWriter, r *http.Request) (string, bool) {
	k := r.Header.Get("Idempotency-Key")
	if k == "" {
		errorResponse(w, 400, errors.New("Idempotency-Key header is required"))
		return "", false
	}
	return k, true
}
func decode(r *http.Request, v any) error {
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	return d.Decode(v)
}
func statusFor(err error) int {
	switch {
	case errors.Is(err, store.ErrIdempotencyConflict), errors.Is(err, payment.ErrInvalidTransition), errors.Is(err, store.ErrInProgress):
		return 409
	case errors.Is(err, payment.ErrOverCapture), errors.Is(err, payment.ErrInvalidAmount):
		return 400
	case errors.Is(err, store.ErrPartialCaptureUnsupported):
		return 400
	case errors.Is(err, pgx.ErrNoRows):
		return 404
	default:
		return 500
	}
}
func (a *API) create(w http.ResponseWriter, r *http.Request) {
	k, ok := key(w, r)
	if !ok {
		return
	}
	var q store.CreateRequest
	if err := decode(r, &q); err != nil {
		errorResponse(w, 400, err)
		return
	}
	p, c, err := a.Service.Create(r.Context(), k, q)
	if err != nil {
		errorResponse(w, statusFor(err), err)
		return
	}
	write(w, c, p)
}
func (a *API) get(w http.ResponseWriter, r *http.Request) {
	p, err := a.Service.Store.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		errorResponse(w, statusFor(err), err)
		return
	}
	write(w, 200, p)
}
func (a *API) authorize(w http.ResponseWriter, r *http.Request) {
	k, ok := key(w, r)
	if !ok {
		return
	}
	var ignored map[string]any
	if err := decode(r, &ignored); err != nil {
		errorResponse(w, 400, err)
		return
	}
	p, c, err := a.Service.Authorize(r.Context(), chi.URLParam(r, "id"), k)
	if err != nil {
		errorResponse(w, statusFor(err), err)
		return
	}
	write(w, c, p)
}
func (a *API) capture(w http.ResponseWriter, r *http.Request) {
	k, ok := key(w, r)
	if !ok {
		return
	}
	var q store.CaptureRequest
	if err := decode(r, &q); err != nil {
		errorResponse(w, 400, err)
		return
	}
	p, cap, c, err := a.Service.Capture(r.Context(), chi.URLParam(r, "id"), k, q)
	if err != nil {
		errorResponse(w, statusFor(err), err)
		return
	}
	write(w, c, struct {
		payment.Payment
		ProcessorCaptureID string `json:"processor_capture_id"`
	}{p, cap})
}
