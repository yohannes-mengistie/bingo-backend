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

// A real Telebirr receipt carries both settledAmount (net, what the house got)
// and totalPaidAmount (settled + service fee). We must use settledAmount so the
// player isn't charged for the fee.
func TestVerifierTelebirrUsesSettledAmountNotTotalPaid(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"success": true,
			"provider": "telebirr",
			"data": {
				"transactionStatus": "Completed",
				"receiptNo": "DFU3F35PH3",
				"settledAmount": "20 Birr",
				"serviceFee": "0.87 Birr",
				"serviceFeeVAT": "0.13 Birr",
				"totalPaidAmount": "21 Birr"
			}
		}`))
	}))
	defer server.Close()

	verifier := NewVerifier(config.PaymentVerifierConfig{BaseURL: server.URL, APIKey: "test-key"})
	result, err := verifier.Verify(context.Background(), domain.PaymentMethodTelebirr, "DFU3F35PH3")
	if err != nil {
		t.Fatalf("Verify returned error: %v", err)
	}
	if result.Amount != 20 {
		t.Fatalf("amount = %v, want 20 (settledAmount, not totalPaidAmount 21)", result.Amount)
	}
}

func TestVerifierRejectsCBEMethod(t *testing.T) {
	verifier := NewVerifier(config.PaymentVerifierConfig{BaseURL: "http://unused", APIKey: "test-key"})
	if _, err := verifier.Verify(context.Background(), domain.PaymentMethod("CBE"), "FT253089F68Z"); err == nil {
		t.Fatal("expected unsupported payment method error for CBE")
	}
}

func TestVerifierAccountBinding(t *testing.T) {
	body := `{
		"success": true,
		"provider": "telebirr",
		"data": {
			"transactionStatus": "Completed",
			"receiptNo": "DFU3F35PH3",
			"creditedPartyAccountNo": "2519****9691",
			"settledAmount": "20 Birr"
		}
	}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	// Matching house account → accepted.
	ok := NewVerifier(config.PaymentVerifierConfig{BaseURL: server.URL, APIKey: "k", TelebirrAccount: "0997709691"})
	if _, err := ok.Verify(context.Background(), domain.PaymentMethodTelebirr, "DFU3F35PH3"); err != nil {
		t.Fatalf("matching account should verify, got %v", err)
	}

	// Receipt credited to a different account → rejected.
	bad := NewVerifier(config.PaymentVerifierConfig{BaseURL: server.URL, APIKey: "k", TelebirrAccount: "0911112222"})
	if _, err := bad.Verify(context.Background(), domain.PaymentMethodTelebirr, "DFU3F35PH3"); err == nil {
		t.Fatal("receipt paid to a different account should be rejected")
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

	verifier := NewVerifier(config.PaymentVerifierConfig{BaseURL: server.URL, APIKey: "test-key"})
	if _, err := verifier.Verify(context.Background(), domain.PaymentMethodTelebirr, "CE626EJRNS"); err == nil {
		t.Fatal("expected unsupported provider error")
	}
}
