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
	"github.com/bingo/backend/pkg/utils"
)

var numberPattern = regexp.MustCompile(`[-+]?\d+(\.\d+)?`)

type Verifier struct {
	baseURL string
	apiKey  string
	// houseAccounts holds, per payment method, the digits of the house account
	// deposits must be credited to. When a method has a non-empty entry, receipts
	// credited to a different account are rejected; an empty/absent entry disables
	// the check for that method.
	houseAccounts map[domain.PaymentMethod]string
	client        *http.Client
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

	houseAccounts := map[domain.PaymentMethod]string{}
	if acct := onlyDigits(cfg.TelebirrAccount); acct != "" {
		houseAccounts[domain.PaymentMethodTelebirr] = acct
	}
	if acct := onlyDigits(cfg.CBEBirrAccount); acct != "" {
		houseAccounts[domain.PaymentMethodCBEBirr] = acct
	}
	if acct := onlyDigits(cfg.MpesaAccount); acct != "" {
		houseAccounts[domain.PaymentMethodMpesa] = acct
	}

	return &Verifier{
		baseURL:       baseURL,
		apiKey:        cfg.APIKey,
		houseAccounts: houseAccounts,
		client: &http.Client{
			Timeout: 20 * time.Second,
		},
	}
}

func (v *Verifier) Verify(ctx context.Context, req domain.PaymentVerificationRequest) (*domain.PaymentVerificationResult, error) {
	if v == nil {
		return nil, errors.New("payment verifier is not configured")
	}
	if !domain.IsSupportedPaymentMethod(req.Method) {
		return nil, errors.New("unsupported payment method")
	}

	reference := strings.TrimSpace(req.Reference)

	// Telebirr goes through the universal /verify endpoint, which auto-detects
	// the provider from the reference. CBE Birr and M-Pesa each have a dedicated
	// endpoint (the universal one does not auto-detect them) with a
	// {receiptNumber, phoneNumber} request shape, where phoneNumber is a phone
	// involved in the transaction. We always supply the HOUSE number of the
	// method — for a deposit the house is the receiver, so nothing extra is
	// needed from the player, and the lookup is inherently pinned to receipts
	// the house participated in.
	endpoint := "/verify"
	payload := map[string]string{"reference": reference}
	// dedicated marks the newer per-provider endpoints (/verify-cbebirr,
	// /verify-mpesa). Unlike the mature universal /verify, these have proven
	// flaky in production: a valid receipt can come back as a raw non-2xx (404/
	// 422) or with a provider string we don't recognise. For those methods we
	// therefore treat an AMBIGUOUS response (an HTTP error status, or an
	// unresolved provider) as "verifier unavailable" so a real player's deposit
	// falls back to manual admin review instead of being rejected outright.
	// EXPLICIT negatives (the body says the receipt was not verified, a wrong
	// status, or money paid to a different account) are still hard rejections.
	dedicated := req.Method == domain.PaymentMethodCBEBirr || req.Method == domain.PaymentMethodMpesa
	if dedicated {
		house := v.houseAccounts[req.Method]
		if house == "" {
			// Without the house number we cannot even look the receipt up —
			// fall back to manual admin review, never auto-credit.
			return nil, fmt.Errorf("%w: no house account configured for %s, deposit needs manual review", domain.ErrVerifierUnavailable, req.Method)
		}
		endpoint = "/verify-" + strings.ToLower(string(req.Method))
		payload = map[string]string{
			"receiptNumber": reference,
			"phoneNumber":   utils.CanonicalEthiopianPhone(house),
		}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to encode verifier request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, v.baseURL+endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to build verifier request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", v.apiKey)

	resp, err := v.client.Do(httpReq)
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
		// Remaining non-2xx (400/404/422, etc.). On the universal /verify
		// (Telebirr) this is a definitive negative verdict. On the flakier
		// dedicated M-Pesa/CBE endpoints it is too often a false negative on a
		// real receipt, so we route it to manual review instead of bouncing the
		// player — the admin can confirm, and nothing is auto-credited either way.
		if dedicated {
			return nil, fmt.Errorf("%w: %s verifier returned status %d: %s", domain.ErrVerifierUnavailable, req.Method, resp.StatusCode, responseMessage(decoded))
		}
		return nil, fmt.Errorf("verifier returned status %d: %s", resp.StatusCode, responseMessage(decoded))
	}

	if success, ok := boolValue(decoded["success"]); ok && !success {
		return nil, fmt.Errorf("receipt was not verified: %s", responseMessage(decoded))
	}

	data := mapValue(decoded["data"])
	if success, ok := boolValue(data["success"]); ok && !success {
		return nil, fmt.Errorf("receipt was not verified: %s", responseMessage(data))
	}
	// Some endpoints (e.g. /verify-cbebirr) return the receipt fields flat at
	// the top level instead of wrapped in {success, provider, data: {...}}.
	// Fall back to reading the root as the data object so the status, credited
	// account and amount lookups below work for both shapes.
	if len(data) == 0 {
		data = decoded
	}

	provider := normalizeProvider(stringValue(decoded["provider"]), req.Method)
	if provider == "" {
		// On the dedicated endpoints we already know the method from the URL we
		// called, so an unrecognised provider string is not a reason to bounce a
		// real receipt — fall back to manual review rather than a hard failure.
		if dedicated {
			return nil, fmt.Errorf("%w: %s response had an unrecognised provider, deposit needs manual review", domain.ErrVerifierUnavailable, req.Method)
		}
		return nil, errors.New("verifier response did not include a supported provider")
	}

	status := firstString(data, "transactionStatus", "status")
	if status != "" && !isCompletedStatus(status) {
		return nil, fmt.Errorf("receipt status is %s", status)
	}

	// Account binding: ensure the receipt was actually credited to the house
	// account, not to some third party. Receipts mask the middle digits
	// (e.g. "2519****9691"), so we compare the visible trailing digits. The
	// house account is looked up per resolved provider.
	//
	// Binding is MANDATORY for auto-approval: a valid receipt only proves money
	// moved somewhere, not that it reached us. So when we cannot prove the
	// credited party is the house — either the method has no configured house
	// account, or the response doesn't reveal the credited account — we return
	// ErrVerifierUnavailable so the deposit falls back to pending manual admin
	// review instead of being auto-credited (or wrongly rejected).
	houseAccount := v.houseAccounts[provider]
	if houseAccount == "" {
		return nil, fmt.Errorf("%w: no house account configured for %s, deposit needs manual review", domain.ErrVerifierUnavailable, provider)
	}
	// NOTE: "sender" must never be in this list — for a deposit it is the
	// PAYER, and a payer-side digit match would bind the receipt to the wrong
	// party. "receiver" is the documented M-Pesa field; when it carries only a
	// holder name (no digits) the hasAccountDigits guard below sends the
	// deposit to manual review rather than auto-crediting.
	credited := firstString(data,
		"creditedPartyAccountNo", "creditedAccountNo", "creditedPartyAccount",
		"creditAccount", "receiverAccount", "receiverPhone", "receiver",
		"payeeAccount", "to")
	if !hasAccountDigits(credited) {
		return nil, fmt.Errorf("%w: verifier response did not reveal the credited account, deposit needs manual review", domain.ErrVerifierUnavailable)
	}
	if !accountMatches(houseAccount, credited) {
		return nil, fmt.Errorf("receipt was paid to a different account (%s), not the house account", credited)
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

// amountKeys are the response fields that may carry the payment amount, in
// preference order. settledAmount (the net amount actually credited to the
// receiving account) comes FIRST so the Telebirr service fee baked into
// totalPaidAmount is never counted — a 20-birr transfer reads as 20, not the
// 21 the sender paid. totalPaidAmount is only a last-resort fallback for
// receipts that omit the net amount.
var amountKeys = []string{"settledAmount", "amount", "transactionAmount", "paidAmount", "total", "totalPaidAmount"}

func responseAmount(root, data map[string]any) (float64, error) {
	for _, key := range amountKeys {
		if amount, ok := parseAmount(data[key]); ok {
			return amount, nil
		}
	}
	for _, key := range amountKeys {
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
	normalized = strings.ReplaceAll(normalized, "-", "")
	switch normalized {
	case "telebirr", "telebirrmobilemoney":
		return domain.PaymentMethodTelebirr
	case "cbebirr":
		return domain.PaymentMethodCBEBirr
	case "mpesa", "safaricommpesa":
		return domain.PaymentMethodMpesa
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

var digitRuns = regexp.MustCompile(`\d+`)

// accountMatches reports whether a credited account/phone from a receipt
// belongs to the configured house account. Credited values come in several
// shapes across providers:
//
//	"2519****9691"                  masked — only leading/trailing digits visible
//	"251912345678 - FULL NAME"      CBE Birr — full phone followed by the holder name
//
// So we consider every digit run in the string. A run matches when it is a
// suffix of the house digits (masked case, ≥4 digits so "20" in a date can't
// match), or when both are full phone numbers whose Ethiopian 9-digit national
// significant numbers agree (handles 09XXXXXXXX vs 2519XXXXXXXX prefix forms).
func accountMatches(houseDigits, credited string) bool {
	for _, run := range digitRuns.FindAllString(credited, -1) {
		if len(run) >= 4 && len(run) <= len(houseDigits) && strings.HasSuffix(houseDigits, run) {
			return true
		}
		if len(run) >= 9 && len(houseDigits) >= 9 && run[len(run)-9:] == houseDigits[len(houseDigits)-9:] {
			return true
		}
	}
	return false
}

// hasAccountDigits reports whether s reveals enough of an account (any digit
// run of 4+) for accountMatches to give a meaningful verdict; anything less
// means the response did not disclose the credited account.
func hasAccountDigits(s string) bool {
	for _, run := range digitRuns.FindAllString(s, -1) {
		if len(run) >= 4 {
			return true
		}
	}
	return false
}

// onlyDigits strips everything but 0-9 from s.
func onlyDigits(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' {
			b.WriteByte(s[i])
		}
	}
	return b.String()
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
