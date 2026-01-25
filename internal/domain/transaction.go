package domain

import (
	"time"

	"github.com/google/uuid"
)

// TransactionType represents the type of transaction
type TransactionType string

const (
	TransactionTypeDeposit      TransactionType = "deposit"
	TransactionTypeWithdraw     TransactionType = "withdraw"
	TransactionTypeTransferIn   TransactionType = "transfer_in"
	TransactionTypeTransferOut  TransactionType = "transfer_out"
)

// TransactionStatus represents the status of a transaction
type TransactionStatus string

const (
	TransactionStatusPending    TransactionStatus = "pending"
	TransactionStatusCompleted  TransactionStatus = "completed"
	TransactionStatusFailed     TransactionStatus = "failed"
	TransactionStatusCancelled  TransactionStatus = "cancelled"
)

// PaymentMethod represents the payment method
type PaymentMethod string

const (
	PaymentMethodCBE      PaymentMethod = "CBE"
	PaymentMethodTelebirr PaymentMethod = "Telebirr"
)

// Transaction represents a transaction entity in the domain
type Transaction struct {
	ID              uuid.UUID        `json:"id" db:"id"`
	UserID          uuid.UUID        `json:"user_id" db:"user_id"`
	Type            TransactionType   `json:"type" db:"type"`
	Amount          float64           `json:"amount" db:"amount"`
	Status          TransactionStatus `json:"status" db:"status"`
	TransactionType *PaymentMethod   `json:"transaction_type,omitempty" db:"transaction_type"`
	TransactionID   *string           `json:"transaction_id,omitempty" db:"transaction_id"`
	Reference       *string           `json:"reference,omitempty" db:"reference"`
	CreatedAt       time.Time         `json:"created_at" db:"created_at"`
}

// DepositRequest represents the data needed to create a deposit
type DepositRequest struct {
	UserID          uuid.UUID     `json:"user_id" binding:"required"`
	Amount          float64       `json:"amount" binding:"required,gt=0"`
	TransactionType PaymentMethod `json:"transaction_type" binding:"required"`
	TransactionID   string        `json:"transaction_id" binding:"required"`
}

// WithdrawRequest represents the data needed to create a withdrawal
type WithdrawRequest struct {
	UserID uuid.UUID `json:"user_id" binding:"required"`
	Amount float64   `json:"amount" binding:"required,gt=0"`
}

// TransferRequest represents the data needed to create a transfer
type TransferRequest struct {
	SenderID   uuid.UUID `json:"sender_id" binding:"required"`
	ReceiverID uuid.UUID `json:"receiver_id" binding:"required"`
	Amount     float64   `json:"amount" binding:"required,gt=0"`
}

