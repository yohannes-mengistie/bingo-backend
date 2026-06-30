package payment

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bingo/backend/config"
	"github.com/bingo/backend/internal/domain"
)

func TestVerifierTelebirr(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "test-key" {
			t.Fatalf("missing api key header")
		}

		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload["reference"] != "CE626EJRNS" {
			t.Fatalf("reference = %q", payload["reference"])
		}
		if _, ok := payload["suffix"]; ok {
			t.Fatalf("telebirr request should not include suffix")
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"success": true,
			"provider": "telebirr",
			"data": {
				"transactionStatus": "Completed",
				"receiptNo": "CE626EJRNS",
				"settledAmount": "N/A",
				"totalPaidAmount": "101.00 Birr"
			}
		}`))
	}))
	defer server.Close()

	verifier := NewVerifier(config.PaymentVerifierConfig{BaseURL: server.URL, APIKey: "test-key"})
	result, err := verifier.Verify(context.Background(), domain.PaymentMethodTelebirr, "CE626EJRNS")
	if err != nil {
		t.Fatalf("Verify returned error: %v", err)
	}
	if result.Provider != domain.PaymentMethodTelebirr {
		t.Fatalf("provider = %q", result.Provider)
	}
	if result.Amount != 101 {
		t.Fatalf("amount = %v", result.Amount)
	}
}

func TestVerifierCBEIncludesSuffix(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload["suffix"] != "16825193" {
			t.Fatalf("suffix = %q", payload["suffix"])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"success": true,
			"provider": "cbe",
			"data": {
				"reference": "FT253089F68Z",
				"amount": "1,000.00 ETB"
			}
		}`))
	}))
	defer server.Close()

	verifier := NewVerifier(config.PaymentVerifierConfig{BaseURL: server.URL, APIKey: "test-key", CBESuffix: "16825193"})
	result, err := verifier.Verify(context.Background(), domain.PaymentMethodCBE, "FT253089F68Z")
	if err != nil {
		t.Fatalf("Verify returned error: %v", err)
	}
	if result.Provider != domain.PaymentMethodCBE {
		t.Fatalf("provider = %q", result.Provider)
	}
	if result.Amount != 1000 {
		t.Fatalf("amount = %v", result.Amount)
	}
}

func TestVerifierUnavailableOnServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("upstream down"))
	}))
	defer server.Close()

	verifier := NewVerifier(config.PaymentVerifierConfig{BaseURL: server.URL, APIKey: "test-key"})
	_, err := verifier.Verify(context.Background(), domain.PaymentMethodTelebirr, "CE626EJRNS")
	if !errors.Is(err, domain.ErrVerifierUnavailable) {
		t.Fatalf("expected ErrVerifierUnavailable, got %v", err)
	}
}

func TestVerifierRejectsUnsupportedProvider(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"success": true,
			"provider": "dashen",
			"data": {"amount": 100}
		}`))
	}))
	defer server.Close()

	verifier := NewVerifier(config.PaymentVerifierConfig{BaseURL: server.URL, APIKey: "test-key", CBESuffix: "12345678"})
	if _, err := verifier.Verify(context.Background(), domain.PaymentMethodCBE, "FT253089F68Z"); err == nil {
		t.Fatal("expected unsupported provider error")
	}
}
