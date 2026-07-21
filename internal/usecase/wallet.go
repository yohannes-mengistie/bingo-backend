package usecase

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math"
	"strings"

	"github.com/bingo/backend/internal/domain"
	"github.com/bingo/backend/internal/repository/postgres"
	"github.com/bingo/backend/pkg/utils"
	"github.com/google/uuid"
)

// errDuplicateReference is returned when a player submits a deposit whose
// payment reference (transaction_id) is already tied to an active deposit.
var errDuplicateReference = errors.New("this transaction reference was already used")

type WalletUseCase struct {
	walletRepo         domain.WalletRepository
	transactionRepo    domain.TransactionRepository
	userRepo           domain.UserRepository
	gameRepo           domain.GameRepository
	transactionService *postgres.TransactionService
	db                 *sql.DB
	paymentVerifier    domain.PaymentVerifier
}

// NewWalletUseCase creates a new wallet use case
func NewWalletUseCase(
	walletRepo domain.WalletRepository,
	transactionRepo domain.TransactionRepository,
	userRepo domain.UserRepository,
	gameRepo domain.GameRepository,
	db *sql.DB,
	paymentVerifier domain.PaymentVerifier,
) *WalletUseCase {
	transactionService := postgres.NewTransactionService(db, walletRepo, transactionRepo)
	return &WalletUseCase{
		walletRepo:         walletRepo,
		transactionRepo:    transactionRepo,
		userRepo:           userRepo,
		gameRepo:           gameRepo,
		transactionService: transactionService,
		db:                 db,
		paymentVerifier:    paymentVerifier,
	}
}

// Deposit creates or completes a deposit, depending on verifier configuration.
func (uc *WalletUseCase) Deposit(ctx context.Context, req domain.DepositRequest) (*domain.Transaction, error) {
	// Validate amount
	if req.Amount <= 0 {
		return nil, errors.New("amount must be greater than 0")
	}

	if !domain.IsSupportedPaymentMethod(req.TransactionType) {
		return nil, errors.New("transaction_type must be one of Telebirr, CBEBirr, Mpesa")
	}

	// Canonicalize the payment reference to uppercase. Provider references
	// (Telebirr/CBE/M-Pesa) are uppercase alphanumeric, and the external
	// verifier tolerates case variants — without this, "ce626ejrns" and
	// "CE626EJRNS" would slip past the duplicate check as two different
	// receipts and the same payment could be credited twice.
	req.TransactionID = strings.ToUpper(strings.TrimSpace(req.TransactionID))
	if req.TransactionID == "" {
		return nil, errors.New("transaction_id is required")
	}

	// Verify user exists
	_, err := uc.userRepo.FindByID(ctx, req.UserID)
	if err != nil {
		return nil, errors.New("user not found")
	}

	// Anti-fraud: block reuse of a payment reference. A transaction_id already
	// tied to a pending or approved deposit cannot be submitted again.
	dup, err := uc.transactionRepo.ExistsActiveDepositByTransactionID(ctx, req.TransactionID)
	if err != nil {
		return nil, err
	}
	if dup {
		return nil, errDuplicateReference
	}

	// verified stays false unless the verifier returns a positive verdict. A
	// verified deposit is auto-approved below; everything else is left pending
	// for manual admin approval. creditAmount is what actually hits the wallet —
	// for a verified deposit it is the net amount the house account received
	// (settledAmount), NOT what the player typed or the fee-inclusive total.
	verified := false
	creditAmount := req.Amount
	if uc.paymentVerifier != nil {
		verification, err := uc.paymentVerifier.Verify(ctx, domain.PaymentVerificationRequest{
			Method:    req.TransactionType,
			Reference: req.TransactionID,
		})
		switch {
		case err == nil:
			if verification.Provider != req.TransactionType {
				return nil, errors.New("payment provider does not match transaction_type")
			}
			// Guard against referencing a receipt for a wildly different amount.
			// The Telebirr service fee is already excluded (we read settledAmount),
			// so the player simply types the amount they sent; a small tolerance
			// absorbs rounding. The wallet is credited the verified net amount.
			if math.Abs(verification.Amount-req.Amount) > 1.0 {
				log.Printf("deposit %s: amount mismatch — verified %.2f, requested %.2f", req.TransactionID, verification.Amount, req.Amount)
				return nil, fmt.Errorf("verified payment amount (%.2f) does not match requested amount (%.2f)", verification.Amount, req.Amount)
			}
			verified = true
			creditAmount = verification.Amount
		case errors.Is(err, domain.ErrVerifierUnavailable):
			// Infrastructure failure (verifier down, timeout, 5xx, auth, rate
			// limit) — fall back to manual approval rather than reject the
			// deposit. The receipt was NOT judged invalid.
			log.Printf("deposit %s: payment verifier unavailable, falling back to manual approval: %v", req.TransactionID, err)
		default:
			// Definitive negative verdict (bad receipt, amount/provider mismatch).
			return nil, fmt.Errorf("payment verification failed: %w", err)
		}
	}

	// Create transaction with pending status. Verified deposits are immediately
	// approved below so balance updates still use the existing atomic path.
	transactionType := req.TransactionType
	transaction := &domain.Transaction{
		UserID:          req.UserID,
		Type:            domain.TransactionTypeDeposit,
		Category:        domain.TransactionCategoryDeposit,
		Amount:          creditAmount,
		Status:          domain.TransactionStatusPending,
		TransactionType: &transactionType,
		TransactionID:   &req.TransactionID,
	}

	// Save transaction (no balance update). The partial unique index catches a
	// concurrent duplicate that slipped past the check above (race).
	if err := uc.transactionRepo.Create(ctx, nil, transaction); err != nil {
		if strings.Contains(err.Error(), "uniq_active_deposit_transaction_id") {
			return nil, errDuplicateReference
		}
		return nil, fmt.Errorf("failed to create deposit transaction: %w", err)
	}

	if verified {
		return uc.transactionService.ApproveDeposit(ctx, transaction.ID)
	}

	return transaction, nil
}

// Withdraw creates a withdrawal and immediately subtracts from balance.
// The payout destination is always the user's verified registration phone — the
// client-supplied account number is ignored — so a withdrawal can never be
// redirected to a different account.
func (uc *WalletUseCase) Withdraw(ctx context.Context, req domain.WithdrawRequest) (*domain.Transaction, error) {
	// Validate amount
	if req.Amount <= 0 {
		return nil, errors.New("amount must be greater than 0")
	}

	// Enforce the minimum withdrawal.
	if req.Amount < domain.MinWithdrawalAmount {
		return nil, fmt.Errorf("minimum withdrawal is %.0f birr", domain.MinWithdrawalAmount)
	}

	// Validate account type
	if !domain.IsSupportedPaymentMethod(req.AccountType) {
		return nil, errors.New("account_type must be one of Telebirr, CBEBirr, Mpesa")
	}

	// Verify user exists
	user, err := uc.userRepo.FindByID(ctx, req.UserID)
	if err != nil {
		return nil, errors.New("user not found")
	}

	// Determine the payout destination. All supported methods (Telebirr, CBE
	// Birr, M-Pesa) are phone-based mobile money. By default the payout goes to
	// the user's verified registration phone (a real Ethiopian mobile shared
	// with the bot); if the player supplies a different number — because their
	// wallet is on another phone — we accept it only after validating it is a
	// real Ethiopian mobile, so a payout can never go to a typo'd account.
	var payoutAccount string
	if supplied := strings.TrimSpace(req.AccountNumber); supplied != "" {
		if !utils.IsEthiopianMobile(supplied) {
			return nil, errors.New("withdrawal account must be a valid Ethiopian phone number")
		}
		payoutAccount = utils.CanonicalEthiopianPhone(supplied)
	} else {
		if !utils.IsEthiopianMobile(user.PhoneNumber) {
			return nil, errors.New("no verified phone number on file; provide a phone number to withdraw to")
		}
		payoutAccount = utils.CanonicalEthiopianPhone(user.PhoneNumber)
	}

	// Process withdrawal (database operations in repository)
	return uc.transactionService.ProcessWithdrawal(ctx, req.UserID, req.Amount, payoutAccount, req.AccountType)
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

// AdjustBalance manually credits or debits a user's wallet (admin action).
func (uc *WalletUseCase) AdjustBalance(ctx context.Context, userID uuid.UUID, amount float64, reason string) (*domain.Transaction, error) {
	return uc.transactionService.AdjustBalance(ctx, userID, amount, reason)
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

// CountAllTransactions is the grand total (for page-by-page navigation).
func (uc *WalletUseCase) CountAllTransactions(ctx context.Context) (int, error) {
	return uc.transactionRepo.CountAll(ctx)
}

// GetRealPlayerWinnings lists winnings paid to real (non-bot) players, plus the
// total for pagination. Powers the admin "Winners" tab.
func (uc *WalletUseCase) GetRealPlayerWinnings(ctx context.Context, limit, offset int) ([]*domain.Transaction, int, error) {
	rows, err := uc.transactionRepo.FindRealPlayerWinnings(ctx, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	total, err := uc.transactionRepo.CountRealPlayerWinnings(ctx)
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
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

	// Real-player game P&L (stakes − winnings, bots excluded). Negative = real
	// cash exposure from bot-inflated pools real players won.
	realPnl, err := uc.transactionRepo.RealPlayerGamePnL(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to compute real-player game P&L: %w", err)
	}
	stats.RealPlayerGamePnl = realPnl

	return stats, nil
}
