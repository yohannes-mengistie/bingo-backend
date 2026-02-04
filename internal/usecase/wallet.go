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
	gameRepo           domain.GameRepository
	transactionService *postgres.TransactionService
	db                 *sql.DB
}

// NewWalletUseCase creates a new wallet use case
func NewWalletUseCase(
	walletRepo domain.WalletRepository,
	transactionRepo domain.TransactionRepository,
	userRepo domain.UserRepository,
	gameRepo domain.GameRepository,
	db *sql.DB,
) *WalletUseCase {
	transactionService := postgres.NewTransactionService(db, walletRepo, transactionRepo)
	return &WalletUseCase{
		walletRepo:         walletRepo,
		transactionRepo:    transactionRepo,
		userRepo:           userRepo,
		gameRepo:           gameRepo,
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

	// Validate account type
	if req.AccountType != domain.PaymentMethodCBE && req.AccountType != domain.PaymentMethodTelebirr {
		return nil, errors.New("account_type must be either CBE or Telebirr")
	}

	// Validate account number is not empty
	if req.AccountNumber == "" {
		return nil, errors.New("account_number is required")
	}

	// Verify user exists
	_, err := uc.userRepo.FindByID(ctx, req.UserID)
	if err != nil {
		return nil, errors.New("user not found")
	}

	// Process withdrawal (database operations in repository)
	return uc.transactionService.ProcessWithdrawal(ctx, req.UserID, req.Amount, req.AccountNumber, req.AccountType)
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

// GetDepositHistory returns deposit transactions for a user
// If limit is 0 or negative, uses default limit of 10
func (uc *WalletUseCase) GetDepositHistory(ctx context.Context, userID uuid.UUID, limit int) ([]*domain.Transaction, error) {
	if limit <= 0 {
		limit = domain.DefaultTransactionHistoryLimit
	}
	return uc.transactionRepo.FindByUserIDAndType(ctx, userID, domain.TransactionTypeDeposit, limit)
}

// GetWithdrawalHistory returns withdrawal transactions for a user
// If limit is 0 or negative, uses default limit of 10
func (uc *WalletUseCase) GetWithdrawalHistory(ctx context.Context, userID uuid.UUID, limit int) ([]*domain.Transaction, error) {
	if limit <= 0 {
		limit = domain.DefaultTransactionHistoryLimit
	}
	return uc.transactionRepo.FindByUserIDAndType(ctx, userID, domain.TransactionTypeWithdraw, limit)
}

// GetTransferHistory returns transfer transactions (both in and out) for a user
// Includes user information for the other party in the transfer
// If limit is 0 or negative, uses default limit of 10
func (uc *WalletUseCase) GetTransferHistory(ctx context.Context, userID uuid.UUID, limit int) ([]*domain.TransferHistoryEntry, error) {
	if limit <= 0 {
		limit = domain.DefaultTransactionHistoryLimit
	}
	transactions, err := uc.transactionRepo.FindByUserIDAndTypes(ctx, userID, []domain.TransactionType{
		domain.TransactionTypeTransferIn,
		domain.TransactionTypeTransferOut,
	}, limit)
	if err != nil {
		return nil, err
	}

	// Collect unique user IDs from references
	userIDs := make(map[uuid.UUID]bool)
	for _, tx := range transactions {
		if tx.Reference != nil {
			otherUserID, err := uuid.Parse(*tx.Reference)
			if err == nil {
				userIDs[otherUserID] = true
			}
		}
	}

	// Fetch all users in one batch
	usersMap := make(map[uuid.UUID]*domain.User)
	for otherUserID := range userIDs {
		user, err := uc.userRepo.FindByID(ctx, otherUserID)
		if err == nil && user != nil {
			usersMap[otherUserID] = user
		}
	}

	// Build response with user information
	entries := make([]*domain.TransferHistoryEntry, 0, len(transactions))
	for _, tx := range transactions {
		entry := &domain.TransferHistoryEntry{
			Transaction: tx,
		}

		// For transfer_out: reference is receiver's ID
		// For transfer_in: reference is sender's ID
		if tx.Reference != nil {
			otherUserID, err := uuid.Parse(*tx.Reference)
			if err == nil {
				if user, exists := usersMap[otherUserID]; exists {
					entry.To = user
				}
			}
		}

		entries = append(entries, entry)
	}

	return entries, nil
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

// GetDashboardStats returns dashboard statistics for admin
func (uc *WalletUseCase) GetDashboardStats(ctx context.Context) (*domain.DashboardStats, error) {
	stats := &domain.DashboardStats{
		GamesByType: make(map[domain.GameType]int),
	}

	// Get pending deposits count
	pendingDeposits, err := uc.transactionRepo.CountByStatusAndType(ctx, domain.TransactionStatusPending, domain.TransactionTypeDeposit)
	if err != nil {
		return nil, fmt.Errorf("failed to count pending deposits: %w", err)
	}
	stats.PendingDeposits = pendingDeposits

	// Get pending withdrawals count
	pendingWithdrawals, err := uc.transactionRepo.CountByStatusAndType(ctx, domain.TransactionStatusPending, domain.TransactionTypeWithdraw)
	if err != nil {
		return nil, fmt.Errorf("failed to count pending withdrawals: %w", err)
	}
	stats.PendingWithdrawals = pendingWithdrawals

	// Get total users count
	totalUsers, err := uc.userRepo.CountAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to count users: %w", err)
	}
	stats.TotalUsers = totalUsers

	// Get total transactions count
	totalTransactions, err := uc.transactionRepo.CountAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to count transactions: %w", err)
	}
	stats.TotalTransactions = totalTransactions

	// Get total balance
	totalBalance, err := uc.walletRepo.GetTotalBalance(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get total balance: %w", err)
	}
	stats.TotalBalance = totalBalance

	// Get games by type
	gamesByType, err := uc.gameRepo.CountGamesByType(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to count games by type: %w", err)
	}
	stats.GamesByType = gamesByType

	// Get total house cut
	totalHouseCut, err := uc.gameRepo.GetTotalHouseCut(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get total house cut: %w", err)
	}
	stats.TotalHouseCut = totalHouseCut

	return stats, nil
}
