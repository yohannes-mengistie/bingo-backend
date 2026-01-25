package domain

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
)

// UserRepository defines the interface for user data operations
type UserRepository interface {
	Create(ctx context.Context, tx *sql.Tx, user *User) error
	FindByTelegramID(ctx context.Context, telegramID int64) (*User, error)
	FindByPhone(ctx context.Context, phone string) (*User, error)
	FindByReferralCode(ctx context.Context, referralCode string) (*User, error)
	FindByID(ctx context.Context, id uuid.UUID) (*User, error)
	FindAll(ctx context.Context, limit, offset int) ([]*User, error)
	Update(ctx context.Context, user *User) error
}

// WalletRepository defines the interface for wallet data operations
type WalletRepository interface {
	Create(ctx context.Context, tx *sql.Tx, wallet *Wallet) error
	FindByUserID(ctx context.Context, userID uuid.UUID) (*Wallet, error)
	UpdateBalance(ctx context.Context, tx *sql.Tx, userID uuid.UUID, amount float64) error
	LockForUpdate(ctx context.Context, tx *sql.Tx, userID uuid.UUID) (*Wallet, error)
	Update(ctx context.Context, wallet *Wallet) error
}

// TransactionRepository defines the interface for transaction data operations
type TransactionRepository interface {
	Create(ctx context.Context, tx *sql.Tx, transaction *Transaction) error
	FindByID(ctx context.Context, id uuid.UUID) (*Transaction, error)
	FindByUserID(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*Transaction, error)
	FindByUserIDAndType(ctx context.Context, userID uuid.UUID, transactionType TransactionType, limit int) ([]*Transaction, error)
	FindByUserIDAndTypes(ctx context.Context, userID uuid.UUID, transactionTypes []TransactionType, limit int) ([]*Transaction, error)
	FindByStatusAndType(ctx context.Context, status TransactionStatus, transactionType TransactionType, limit, offset int) ([]*Transaction, error)
	FindByStatus(ctx context.Context, status TransactionStatus, limit, offset int) ([]*Transaction, error)
	FindByTypes(ctx context.Context, transactionTypes []TransactionType, limit, offset int) ([]*Transaction, error)
	FindAll(ctx context.Context, limit, offset int) ([]*Transaction, error)
	UpdateStatus(ctx context.Context, tx *sql.Tx, id uuid.UUID, status TransactionStatus) error
}
