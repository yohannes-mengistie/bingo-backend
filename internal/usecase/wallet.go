package usecase

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/bingo/backend/internal/domain"
	"github.com/bingo/backend/internal/repository/postgres"
	"github.com/google/uuid"
)

type WalletUseCase struct {
	walletRepo         domain.WalletRepository
	transactionRepo    domain.TransactionRepository
	userRepo           domain.UserRepository
	transactionService *postgres.TransactionService
	db                 *sql.DB
}

// NewWalletUseCase creates a new wallet use case
func NewWalletUseCase(
	walletRepo domain.WalletRepository,
	transactionRepo domain.TransactionRepository,
	userRepo domain.UserRepository,
	db *sql.DB,
) *WalletUseCase {
	transactionService := postgres.NewTransactionService(db, walletRepo, transactionRepo)
	return &WalletUseCase{
		walletRepo:         walletRepo,
		transactionRepo:    transactionRepo,
		userRepo:           userRepo,
		transactionService: transactionService,
		db:                 db,
	}
}

// Deposit creates a deposit request (pending status, does not update balance)
func (uc *WalletUseCase) Deposit(ctx context.Context, req domain.DepositRequest) (*domain.Transaction, error) {
	// Validate amount
	if req.Amount <= 0 {
		return nil, errors.New("amount must be greater than 0")
	}

	// Verify user exists
	_, err := uc.userRepo.FindByID(ctx, req.UserID)
	if err != nil {
		return nil, errors.New("user not found")
	}

	// Create transaction with pending status
	transactionType := req.TransactionType
	transaction := &domain.Transaction{
		UserID:          req.UserID,
		Type:            domain.TransactionTypeDeposit,
		Amount:          req.Amount,
		Status:          domain.TransactionStatusPending,
		TransactionType: &transactionType,
		TransactionID:   &req.TransactionID,
	}

	// Save transaction (no balance update)
	if err := uc.transactionRepo.Create(ctx, nil, transaction); err != nil {
		return nil, fmt.Errorf("failed to create deposit transaction: %w", err)
	}

	return transaction, nil
}

// Withdraw creates a withdrawal and immediately subtracts from balance
func (uc *WalletUseCase) Withdraw(ctx context.Context, req domain.WithdrawRequest) (*domain.Transaction, error) {
	// Validate amount
	if req.Amount <= 0 {
		return nil, errors.New("amount must be greater than 0")
	}

	// Verify user exists
	_, err := uc.userRepo.FindByID(ctx, req.UserID)
	if err != nil {
		return nil, errors.New("user not found")
	}

	// Process withdrawal (database operations in repository)
	return uc.transactionService.ProcessWithdrawal(ctx, req.UserID, req.Amount)
}

// Transfer transfers money from one user to another (atomic operation)
func (uc *WalletUseCase) Transfer(ctx context.Context, req domain.TransferRequest) (*domain.Transaction, *domain.Transaction, error) {
	// Validate amount
	if req.Amount <= 0 {
		return nil, nil, errors.New("amount must be greater than 0")
	}

	// No self-transfer
	if req.SenderID == req.ReceiverID {
		return nil, nil, errors.New("cannot transfer to yourself")
	}

	// Verify sender exists
	_, err := uc.userRepo.FindByID(ctx, req.SenderID)
	if err != nil {
		return nil, nil, errors.New("sender not found")
	}

	// Verify receiver exists
	_, err = uc.userRepo.FindByID(ctx, req.ReceiverID)
	if err != nil {
		return nil, nil, errors.New("receiver not found")
	}

	// Process transfer (database operations in repository)
	return uc.transactionService.ProcessTransfer(ctx, req.SenderID, req.ReceiverID, req.Amount)
}

// GetWallet retrieves wallet by user ID
func (uc *WalletUseCase) GetWallet(ctx context.Context, userID uuid.UUID) (*domain.Wallet, error) {
	wallet, err := uc.walletRepo.FindByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	return wallet, nil
}

// GetWalletByTelegramID retrieves wallet by Telegram ID
func (uc *WalletUseCase) GetWalletByTelegramID(ctx context.Context, telegramID int64) (*domain.Wallet, error) {
	// Find user by telegram ID
	user, err := uc.userRepo.FindByTelegramID(ctx, telegramID)
	if err != nil {
		return nil, err
	}

	// Get wallet by user ID
	wallet, err := uc.walletRepo.FindByUserID(ctx, user.ID)
	if err != nil {
		return nil, err
	}

	return wallet, nil
}

// ApproveDeposit approves a pending deposit transaction and updates the wallet balance
func (uc *WalletUseCase) ApproveDeposit(ctx context.Context, transactionID uuid.UUID) (*domain.Transaction, error) {
	return uc.transactionService.ApproveDeposit(ctx, transactionID)
}

// RejectDeposit rejects a pending deposit transaction (no balance change)
func (uc *WalletUseCase) RejectDeposit(ctx context.Context, transactionID uuid.UUID) (*domain.Transaction, error) {
	return uc.transactionService.RejectDeposit(ctx, transactionID)
}

// ApproveWithdrawal approves a pending withdrawal transaction (balance already subtracted)
func (uc *WalletUseCase) ApproveWithdrawal(ctx context.Context, transactionID uuid.UUID) (*domain.Transaction, error) {
	return uc.transactionService.ApproveWithdrawal(ctx, transactionID)
}

// RejectWithdrawal rejects a pending withdrawal and refunds the balance
func (uc *WalletUseCase) RejectWithdrawal(ctx context.Context, transactionID uuid.UUID) (*domain.Transaction, error) {
	return uc.transactionService.RejectWithdrawal(ctx, transactionID)
}

// CancelTransaction cancels a pending transaction (for deposits, no balance change; for withdrawals, refund balance)
func (uc *WalletUseCase) CancelTransaction(ctx context.Context, transactionID uuid.UUID) (*domain.Transaction, error) {
	return uc.transactionService.CancelTransaction(ctx, transactionID)
}

// GetDepositHistory returns the top deposit transactions for a user
func (uc *WalletUseCase) GetDepositHistory(ctx context.Context, userID uuid.UUID) ([]*domain.Transaction, error) {
	return uc.transactionRepo.FindByUserIDAndType(ctx, userID, domain.TransactionTypeDeposit, domain.DefaultTransactionHistoryLimit)
}

// GetWithdrawalHistory returns the top withdrawal transactions for a user
func (uc *WalletUseCase) GetWithdrawalHistory(ctx context.Context, userID uuid.UUID) ([]*domain.Transaction, error) {
	return uc.transactionRepo.FindByUserIDAndType(ctx, userID, domain.TransactionTypeWithdraw, domain.DefaultTransactionHistoryLimit)
}

// GetTransferHistory returns the top transfer transactions (both in and out) for a user
func (uc *WalletUseCase) GetTransferHistory(ctx context.Context, userID uuid.UUID) ([]*domain.Transaction, error) {
	return uc.transactionRepo.FindByUserIDAndTypes(ctx, userID, []domain.TransactionType{
		domain.TransactionTypeTransferIn,
		domain.TransactionTypeTransferOut,
	}, domain.DefaultTransactionHistoryLimit)
}

// Admin transaction query methods

// GetPendingDeposits returns pending deposit transactions for admin
func (uc *WalletUseCase) GetPendingDeposits(ctx context.Context, limit, offset int) ([]*domain.Transaction, error) {
	return uc.transactionRepo.FindByStatusAndType(ctx, domain.TransactionStatusPending, domain.TransactionTypeDeposit, limit, offset)
}

// GetPendingWithdrawals returns pending withdrawal transactions for admin
func (uc *WalletUseCase) GetPendingWithdrawals(ctx context.Context, limit, offset int) ([]*domain.Transaction, error) {
	return uc.transactionRepo.FindByStatusAndType(ctx, domain.TransactionStatusPending, domain.TransactionTypeWithdraw, limit, offset)
}

// GetCompletedDeposits returns completed deposit transactions for admin
func (uc *WalletUseCase) GetCompletedDeposits(ctx context.Context, limit, offset int) ([]*domain.Transaction, error) {
	return uc.transactionRepo.FindByStatusAndType(ctx, domain.TransactionStatusCompleted, domain.TransactionTypeDeposit, limit, offset)
}

// GetCompletedWithdrawals returns completed withdrawal transactions for admin
func (uc *WalletUseCase) GetCompletedWithdrawals(ctx context.Context, limit, offset int) ([]*domain.Transaction, error) {
	return uc.transactionRepo.FindByStatusAndType(ctx, domain.TransactionStatusCompleted, domain.TransactionTypeWithdraw, limit, offset)
}

// GetFailedTransactions returns all failed transactions for admin
func (uc *WalletUseCase) GetFailedTransactions(ctx context.Context, limit, offset int) ([]*domain.Transaction, error) {
	return uc.transactionRepo.FindByStatus(ctx, domain.TransactionStatusFailed, limit, offset)
}

// GetTransferTransactions returns all transfer transactions (in and out) for admin
func (uc *WalletUseCase) GetTransferTransactions(ctx context.Context, limit, offset int) ([]*domain.Transaction, error) {
	return uc.transactionRepo.FindByTypes(ctx, []domain.TransactionType{
		domain.TransactionTypeTransferIn,
		domain.TransactionTypeTransferOut,
	}, limit, offset)
}

// GetAllTransactions returns all transactions with optional filters for admin
func (uc *WalletUseCase) GetAllTransactions(ctx context.Context, limit, offset int) ([]*domain.Transaction, error) {
	return uc.transactionRepo.FindAll(ctx, limit, offset)
}
