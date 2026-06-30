package payment

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/bingo/backend/config"
	"github.com/bingo/backend/internal/domain"
)

var numberPattern = regexp.MustCompile(`[-+]?\d+(\.\d+)?`)

type Verifier struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

// NewVerifier returns a configured verifier, or a nil domain.PaymentVerifier
// interface when no API key is set. Returning the interface type (rather than
// *Verifier) is deliberate: a typed-nil *Verifier boxed into an interface is
// not == nil, which would defeat the "verifier configured?" checks at call sites.
func NewVerifier(cfg config.PaymentVerifierConfig) domain.PaymentVerifier {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil
	}

	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://verifyapi.leulzenebe.pro"
	}

	return &Verifier{
		baseURL: baseURL,
		apiKey:  cfg.APIKey,
		client: &http.Client{
			Timeout: 20 * time.Second,
		},
	}
}

func (v *Verifier) Verify(ctx context.Context, method domain.PaymentMethod, reference string) (*domain.PaymentVerificationResult, error) {
	if v == nil {
		return nil, errors.New("payment verifier is not configured")
	}
	if method != domain.PaymentMethodTelebirr {
		return nil, errors.New("unsupported payment method")
	}

	payload := map[string]string{"reference": strings.TrimSpace(reference)}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to encode verifier request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, v.baseURL+"/verify", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to build verifier request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", v.apiKey)

	resp, err := v.client.Do(req)
	if err != nil {
		// Network failure, timeout, DNS, connection refused — infrastructure,
		// not a verdict on the receipt. Let the caller fall back to manual review.
		return nil, fmt.Errorf("%w: verifier request failed: %v", domain.ErrVerifierUnavailable, err)
	}
	defer resp.Body.Close()

	// 5xx / auth / rate-limit / timeout mean the verifier gave us no usable
	// answer — treat as transient so the deposit falls back to manual approval
	// instead of being rejected as fraudulent.
	if isTransientStatus(resp.StatusCode) {
		return nil, fmt.Errorf("%w: verifier returned status %d", domain.ErrVerifierUnavailable, resp.StatusCode)
	}

	var decoded map[string]any
	decoder := json.NewDecoder(resp.Body)
	decoder.UseNumber()
	if err := decoder.Decode(&decoded); err != nil {
		// A response we can't parse is not a verdict either — fall back to manual.
		return nil, fmt.Errorf("%w: failed to decode verifier response: %v", domain.ErrVerifierUnavailable, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Remaining non-2xx (400/404/422, etc.) — the verifier rejected the
		// request or receipt. This IS a definitive negative verdict.
		return nil, fmt.Errorf("verifier returned status %d: %s", resp.StatusCode, responseMessage(decoded))
	}

	if success, ok := boolValue(decoded["success"]); ok && !success {
		return nil, fmt.Errorf("receipt was not verified: %s", responseMessage(decoded))
	}

	data := mapValue(decoded["data"])
	if success, ok := boolValue(data["success"]); ok && !success {
		return nil, fmt.Errorf("receipt was not verified: %s", responseMessage(data))
	}

	provider := normalizeProvider(stringValue(decoded["provider"]), method)
	if provider == "" {
		return nil, errors.New("verifier response did not include a supported provider")
	}

	status := firstString(data, "transactionStatus", "status")
	if status != "" && !isCompletedStatus(status) {
		return nil, fmt.Errorf("receipt status is %s", status)
	}

	amount, err := responseAmount(decoded, data)
	if err != nil {
		return nil, err
	}

	verifiedReference := firstString(data, "reference", "receiptNo", "receiptNumber", "transactionReference")
	if verifiedReference == "" {
		verifiedReference = firstString(decoded, "reference", "receiptNo", "receiptNumber", "transactionReference")
	}
	if verifiedReference == "" {
		verifiedReference = reference
	}

	return &domain.PaymentVerificationResult{
		Provider:  provider,
		Reference: verifiedReference,
		Amount:    amount,
		Status:    status,
	}, nil
}

func responseAmount(root, data map[string]any) (float64, error) {
	for _, key := range []string{"amount", "paidAmount", "totalPaidAmount", "transactionAmount", "total", "settledAmount"} {
		if amount, ok := parseAmount(data[key]); ok {
			return amount, nil
		}
	}
	for _, key := range []string{"amount", "paidAmount", "totalPaidAmount", "transactionAmount", "total", "settledAmount"} {
		if amount, ok := parseAmount(root[key]); ok {
			return amount, nil
		}
	}
	return 0, errors.New("payment verifier did not return amount")
}

func parseAmount(value any) (float64, bool) {
	switch v := value.(type) {
	case json.Number:
		amount, err := v.Float64()
		return amount, err == nil
	case float64:
		return v, true
	case int:
		return float64(v), true
	case string:
		cleaned := strings.ReplaceAll(v, ",", "")
		match := numberPattern.FindString(cleaned)
		if match == "" {
			return 0, false
		}
		amount, err := strconv.ParseFloat(match, 64)
		return amount, err == nil
	default:
		return 0, false
	}
}

func normalizeProvider(value string, fallback domain.PaymentMethod) domain.PaymentMethod {
	normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), " ", ""))
	switch normalized {
	case "telebirr", "telebirrmobilemoney":
		return domain.PaymentMethodTelebirr
	case "":
		return fallback
	}
	return ""
}

// isTransientStatus reports whether an HTTP status from the verifier means we
// got no usable verdict (so the caller should fall back to manual approval),
// rather than a definitive answer about the receipt. 401/403 (our own auth /
// misconfiguration) and 429 (rate limit) are included so a temporary key or
// quota problem doesn't block every player's deposit.
func isTransientStatus(code int) bool {
	switch code {
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusRequestTimeout, http.StatusTooManyRequests:
		return true
	}
	return code >= 500
}

func isCompletedStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "complete", "success", "successful":
		return true
	default:
		return false
	}
}

func mapValue(value any) map[string]any {
	if typed, ok := value.(map[string]any); ok {
		return typed
	}
	return map[string]any{}
}

func boolValue(value any) (bool, bool) {
	if typed, ok := value.(bool); ok {
		return typed, true
	}
	return false, false
}

func firstString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := stringValue(values[key]); value != "" {
			return value
		}
	}
	return ""
}

func stringValue(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case json.Number:
		return v.String()
	default:
		return ""
	}
}

func responseMessage(values map[string]any) string {
	for _, key := range []string{"error", "message", "details"} {
		if value := stringValue(values[key]); value != "" {
			return value
		}
	}
	return "unknown verifier response"
}
