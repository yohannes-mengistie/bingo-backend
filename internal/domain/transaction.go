package domain

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// ErrVerifierUnavailable means the payment verifier could not produce a verdict
// (network failure, timeout, 5xx, auth, or rate-limit). Callers should fall back
// to manual admin approval rather than rejecting the deposit. It does NOT mean
// the receipt was rejected — a definitive "receipt invalid" is a plain error.
var ErrVerifierUnavailable = errors.New("payment verifier unavailable")

// TransactionType represents the type of transaction
type TransactionType string

const (
	TransactionTypeDeposit     TransactionType = "deposit"
	TransactionTypeWithdraw    TransactionType = "withdraw"
	TransactionTypeTransferIn  TransactionType = "transfer_in"
	TransactionTypeTransferOut TransactionType = "transfer_out"
)

// TransactionStatus represents the status of a transaction
type TransactionStatus string

const (
	TransactionStatusPending   TransactionStatus = "pending"
	TransactionStatusCompleted TransactionStatus = "completed"
	TransactionStatusFailed    TransactionStatus = "failed"
	TransactionStatusCancelled TransactionStatus = "cancelled"
)

// PaymentMethod represents the payment method
type PaymentMethod string

const (
	PaymentMethodTelebirr PaymentMethod = "Telebirr"
)

// Note: CBE is no longer an accepted payment method. Historical transactions
// may still carry transaction_type = "CBE" in the database; that value reads
// back fine as a plain string but can no longer be submitted.

// PaymentVerificationResult contains normalized data returned by an external
// payment verifier.
type PaymentVerificationResult struct {
	Provider  PaymentMethod `json:"provider"`
	Reference string        `json:"reference"`
	Amount    float64       `json:"amount"`
	Status    string        `json:"status,omitempty"`
}

type PaymentVerifier interface {
	Verify(ctx context.Context, method PaymentMethod, reference string) (*PaymentVerificationResult, error)
}

// Transaction represents a transaction entity in the domain
type Transaction struct {
	ID              uuid.UUID         `json:"id" db:"id"`
	UserID          uuid.UUID         `json:"user_id" db:"user_id"`
	Type            TransactionType   `json:"type" db:"type"`
	Amount          float64           `json:"amount" db:"amount"`
	Status          TransactionStatus `json:"status" db:"status"`
	TransactionType *PaymentMethod    `json:"transaction_type,omitempty" db:"transaction_type"`
	TransactionID   *string           `json:"transaction_id,omitempty" db:"transaction_id"`
	Reference       *string           `json:"reference,omitempty" db:"reference"`
	CreatedAt       time.Time         `json:"created_at" db:"created_at"`
}

// DepositRequest represents the data needed to create a deposit.
// UserID is populated from the authenticated JWT, not the request body.
type DepositRequest struct {
	UserID          uuid.UUID     `json:"-"`
	Amount          float64       `json:"amount" binding:"required,gt=0"`
	TransactionType PaymentMethod `json:"transaction_type" binding:"required"`
	TransactionID   string        `json:"transaction_id" binding:"required"`
}

// WithdrawRequest represents the data needed to create a withdrawal.
// UserID is populated from the authenticated JWT, not the request body.
type WithdrawRequest struct {
	UserID        uuid.UUID     `json:"-"`
	Amount        float64       `json:"amount" binding:"required,gt=0"`
	AccountNumber string        `json:"account_number" binding:"required"`
	AccountType   PaymentMethod `json:"account_type" binding:"required"`
}

// TransferRequest represents the data needed to create a transfer.
// SenderID is populated from the authenticated JWT, not the request body.
type TransferRequest struct {
	SenderID   uuid.UUID `json:"-"`
	ReceiverID uuid.UUID `json:"receiver_id" binding:"required"`
	Amount     float64   `json:"amount" binding:"required,gt=0"`
}

// TransferHistoryEntry represents a transfer transaction with user information
type TransferHistoryEntry struct {
	Transaction *Transaction `json:"transaction"`
	To          *User        `json:"to,omitempty"` // User info for transfer_out (receiver) or transfer_in (sender)
}
