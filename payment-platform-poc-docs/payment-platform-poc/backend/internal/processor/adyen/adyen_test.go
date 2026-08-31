package adyen

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthorizeAndCapture(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != "key" {
			t.Fatalf("missing API key")
		}
		if r.URL.Path == "/payments" {
			var request map[string]any
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatal(err)
			}
			if request["captureDelay"] != "manual" || request["merchantAccount"] != "merchant" {
				t.Fatalf("unexpected authorize request: %#v", request)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"resultCode":"Authorised","pspReference":"adyen-auth-1"}`))
			return
		}
		if r.URL.Path == "/payments/adyen-auth-1/captures" && r.Header.Get("Idempotency-Key") == "capture-1" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"pspReference":"adyen-capture-1"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	processor, err := New("key", "merchant", server.URL, `{"type":"scheme"}`)
	if err != nil {
		t.Fatal(err)
	}
	authReference, err := processor.Authorize(context.Background(), "payment-1", 1000, "USD")
	if err != nil || authReference != "adyen-auth-1" {
		t.Fatalf("authorize = %q, %v", authReference, err)
	}
	captureReference, err := processor.Capture(context.Background(), authReference, 1000, "capture-1")
	if err != nil || captureReference != "adyen-capture-1" {
		t.Fatalf("capture = %q, %v", captureReference, err)
	}
}
