package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/bingo/backend/internal/domain"
	"github.com/google/uuid"
)

type TransactionService struct {
	db              *sql.DB
	walletRepo      domain.WalletRepository
	transactionRepo domain.TransactionRepository
}

// NewTransactionService creates a new transaction service
func NewTransactionService(
	db *sql.DB,
	walletRepo domain.WalletRepository,
	transactionRepo domain.TransactionRepository,
) *TransactionService {
	return &TransactionService{
		db:              db,
		walletRepo:      walletRepo,
		transactionRepo: transactionRepo,
	}
}

// ApproveDeposit approves a deposit transaction and updates wallet balance
func (s *TransactionService) ApproveDeposit(ctx context.Context, transactionID uuid.UUID) (*domain.Transaction, error) {
	// Get transaction
	transaction, err := s.transactionRepo.FindByID(ctx, transactionID)
	if err != nil {
		return nil, fmt.Errorf("transaction not found: %w", err)
	}

	// Validate transaction type and status
	if transaction.Type != domain.TransactionTypeDeposit {
		return nil, fmt.Errorf("transaction is not a deposit")
	}

	if transaction.Status != domain.TransactionStatusPending {
		return nil, fmt.Errorf("transaction is not pending (current status: %s)", transaction.Status)
	}

	// Start database transaction
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Lock wallet for update
	_, err = s.walletRepo.LockForUpdate(ctx, tx, transaction.UserID)
	if err != nil {
		return nil, fmt.Errorf("wallet not found: %w", err)
	}

	// Update wallet balance
	if err := s.walletRepo.UpdateBalance(ctx, tx, transaction.UserID, transaction.Amount); err != nil {
		return nil, fmt.Errorf("failed to update balance: %w", err)
	}

	// Update transaction status to completed
	if err := s.transactionRepo.UpdateStatus(ctx, tx, transactionID, domain.TransactionStatusCompleted); err != nil {
		return nil, fmt.Errorf("failed to update transaction status: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Fetch updated transaction
	transaction, err = s.transactionRepo.FindByID(ctx, transactionID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch updated transaction: %w", err)
	}

	return transaction, nil
}

// RejectDeposit rejects a deposit transaction (no balance change)
func (s *TransactionService) RejectDeposit(ctx context.Context, transactionID uuid.UUID) (*domain.Transaction, error) {
	// Get transaction
	transaction, err := s.transactionRepo.FindByID(ctx, transactionID)
	if err != nil {
		return nil, fmt.Errorf("transaction not found: %w", err)
	}

	// Validate transaction type and status
	if transaction.Type != domain.TransactionTypeDeposit {
		return nil, fmt.Errorf("transaction is not a deposit")
	}

	if transaction.Status != domain.TransactionStatusPending {
		return nil, fmt.Errorf("transaction is not pending (current status: %s)", transaction.Status)
	}

	// Start database transaction
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Update transaction status to failed (no balance change for rejected deposits)
	if err := s.transactionRepo.UpdateStatus(ctx, tx, transactionID, domain.TransactionStatusFailed); err != nil {
		return nil, fmt.Errorf("failed to update transaction status: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Fetch updated transaction
	transaction, err = s.transactionRepo.FindByID(ctx, transactionID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch updated transaction: %w", err)
	}

	return transaction, nil
}

// ApproveWithdrawal approves a withdrawal transaction (balance already subtracted)
func (s *TransactionService) ApproveWithdrawal(ctx context.Context, transactionID uuid.UUID) (*domain.Transaction, error) {
	// Get transaction
	transaction, err := s.transactionRepo.FindByID(ctx, transactionID)
	if err != nil {
		return nil, fmt.Errorf("transaction not found: %w", err)
	}

	// Validate transaction type and status
	if transaction.Type != domain.TransactionTypeWithdraw {
		return nil, fmt.Errorf("transaction is not a withdrawal")
	}

	if transaction.Status != domain.TransactionStatusPending {
		return nil, fmt.Errorf("transaction is not pending (current status: %s)", transaction.Status)
	}

	// Start database transaction
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Update transaction status to completed (balance already subtracted when withdrawal was created)
	if err := s.transactionRepo.UpdateStatus(ctx, tx, transactionID, domain.TransactionStatusCompleted); err != nil {
		return nil, fmt.Errorf("failed to update transaction status: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Fetch updated transaction
	transaction, err = s.transactionRepo.FindByID(ctx, transactionID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch updated transaction: %w", err)
	}

	return transaction, nil
}

// RejectWithdrawal rejects a withdrawal transaction and refunds the balance
func (s *TransactionService) RejectWithdrawal(ctx context.Context, transactionID uuid.UUID) (*domain.Transaction, error) {
	// Get transaction
	transaction, err := s.transactionRepo.FindByID(ctx, transactionID)
	if err != nil {
		return nil, fmt.Errorf("transaction not found: %w", err)
	}

	// Validate transaction type and status
	if transaction.Type != domain.TransactionTypeWithdraw {
		return nil, fmt.Errorf("transaction is not a withdrawal")
	}

	if transaction.Status != domain.TransactionStatusPending {
		return nil, fmt.Errorf("transaction is not pending (current status: %s)", transaction.Status)
	}

	// Start database transaction
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Lock wallet for update
	_, err = s.walletRepo.LockForUpdate(ctx, tx, transaction.UserID)
	if err != nil {
		return nil, fmt.Errorf("wallet not found: %w", err)
	}

	// Refund balance (add it back)
	if err := s.walletRepo.UpdateBalance(ctx, tx, transaction.UserID, transaction.Amount); err != nil {
		return nil, fmt.Errorf("failed to refund balance: %w", err)
	}

	// Update transaction status to failed
	if err := s.transactionRepo.UpdateStatus(ctx, tx, transactionID, domain.TransactionStatusFailed); err != nil {
		return nil, fmt.Errorf("failed to update transaction status: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Fetch updated transaction
	transaction, err = s.transactionRepo.FindByID(ctx, transactionID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch updated transaction: %w", err)
	}

	return transaction, nil
}

// CancelTransaction cancels a pending transaction
func (s *TransactionService) CancelTransaction(ctx context.Context, transactionID uuid.UUID) (*domain.Transaction, error) {
	// Get transaction
	transaction, err := s.transactionRepo.FindByID(ctx, transactionID)
	if err != nil {
		return nil, fmt.Errorf("transaction not found: %w", err)
	}

	// Validate status
	if transaction.Status != domain.TransactionStatusPending {
		return nil, fmt.Errorf("transaction is not pending (current status: %s)", transaction.Status)
	}

	// Start database transaction
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// If it's a withdrawal, refund the balance
	if transaction.Type == domain.TransactionTypeWithdraw {
		// Lock wallet for update
		_, err = s.walletRepo.LockForUpdate(ctx, tx, transaction.UserID)
		if err != nil {
			return nil, fmt.Errorf("wallet not found: %w", err)
		}

		// Refund balance
		if err := s.walletRepo.UpdateBalance(ctx, tx, transaction.UserID, transaction.Amount); err != nil {
			return nil, fmt.Errorf("failed to refund balance: %w", err)
		}
	}

	// Update transaction status to cancelled
	if err := s.transactionRepo.UpdateStatus(ctx, tx, transactionID, domain.TransactionStatusCancelled); err != nil {
		return nil, fmt.Errorf("failed to update transaction status: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Fetch updated transaction
	transaction, err = s.transactionRepo.FindByID(ctx, transactionID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch updated transaction: %w", err)
	}

	return transaction, nil
}

// ProcessWithdrawal processes a withdrawal request (subtracts balance and creates transaction)
func (s *TransactionService) ProcessWithdrawal(ctx context.Context, userID uuid.UUID, amount float64) (*domain.Transaction, error) {
	// Start database transaction
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Lock wallet for update
	wallet, err := s.walletRepo.LockForUpdate(ctx, tx, userID)
	if err != nil {
		return nil, fmt.Errorf("wallet not found: %w", err)
	}

	// Check balance
	if wallet.Balance < amount {
		return nil, fmt.Errorf("insufficient balance")
	}

	// Check if remaining balance would be less than 10
	remainingBalance := wallet.Balance - amount
	if remainingBalance < 10 {
		return nil, fmt.Errorf("withdrawal not allowed: remaining balance must be at least 10")
	}

	// Subtract balance immediately
	if err := s.walletRepo.UpdateBalance(ctx, tx, userID, -amount); err != nil {
		return nil, fmt.Errorf("failed to update balance: %w", err)
	}

	// Create transaction with pending status
	transaction := &domain.Transaction{
		UserID: userID,
		Type:   domain.TransactionTypeWithdraw,
		Amount: amount,
		Status: domain.TransactionStatusPending,
	}

	if err := s.transactionRepo.Create(ctx, tx, transaction); err != nil {
		return nil, fmt.Errorf("failed to create withdrawal transaction: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return transaction, nil
}

// ProcessTransfer processes a transfer between two users
func (s *TransactionService) ProcessTransfer(ctx context.Context, senderID, receiverID uuid.UUID, amount float64) (*domain.Transaction, *domain.Transaction, error) {
	// Start database transaction
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Lock sender wallet
	senderWallet, err := s.walletRepo.LockForUpdate(ctx, tx, senderID)
	if err != nil {
		return nil, nil, fmt.Errorf("sender wallet not found: %w", err)
	}

	// Lock receiver wallet
	_, err = s.walletRepo.LockForUpdate(ctx, tx, receiverID)
	if err != nil {
		return nil, nil, fmt.Errorf("receiver wallet not found: %w", err)
	}

	// Check sender balance
	if senderWallet.Balance < amount {
		return nil, nil, fmt.Errorf("insufficient balance")
	}

	// Subtract from sender
	if err := s.walletRepo.UpdateBalance(ctx, tx, senderID, -amount); err != nil {
		return nil, nil, fmt.Errorf("failed to update sender balance: %w", err)
	}

	// Add to receiver
	if err := s.walletRepo.UpdateBalance(ctx, tx, receiverID, amount); err != nil {
		return nil, nil, fmt.Errorf("failed to update receiver balance: %w", err)
	}

	// Create transfer_out transaction for sender
	receiverIDStr := receiverID.String()
	senderTransaction := &domain.Transaction{
		UserID:    senderID,
		Type:      domain.TransactionTypeTransferOut,
		Amount:    amount,
		Status:    domain.TransactionStatusCompleted,
		Reference: &receiverIDStr,
	}

	if err := s.transactionRepo.Create(ctx, tx, senderTransaction); err != nil {
		return nil, nil, fmt.Errorf("failed to create sender transaction: %w", err)
	}

	// Create transfer_in transaction for receiver
	senderIDStr := senderID.String()
	receiverTransaction := &domain.Transaction{
		UserID:    receiverID,
		Type:      domain.TransactionTypeTransferIn,
		Amount:    amount,
		Status:    domain.TransactionStatusCompleted,
		Reference: &senderIDStr,
	}

	if err := s.transactionRepo.Create(ctx, tx, receiverTransaction); err != nil {
		return nil, nil, fmt.Errorf("failed to create receiver transaction: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return nil, nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return senderTransaction, receiverTransaction, nil
}
