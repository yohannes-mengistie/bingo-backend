package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

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

// AdjustBalance credits (amount > 0) or debits (amount < 0) a user's wallet as
// a manual admin action, recording a completed transaction for the audit trail.
// Debits are rejected if they would overdraw the wallet.
func (s *TransactionService) AdjustBalance(ctx context.Context, userID uuid.UUID, amount float64, reason string) (*domain.Transaction, error) {
	if amount == 0 {
		return nil, fmt.Errorf("amount must be non-zero")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	wallet, err := s.walletRepo.LockForUpdate(ctx, tx, userID)
	if err != nil {
		return nil, fmt.Errorf("wallet not found: %w", err)
	}

	if amount < 0 && wallet.Balance+amount < 0 {
		return nil, fmt.Errorf("insufficient balance: cannot debit %.2f from %.2f", -amount, wallet.Balance)
	}

	if err := s.walletRepo.UpdateBalance(ctx, tx, userID, amount); err != nil {
		return nil, fmt.Errorf("failed to update balance: %w", err)
	}

	txType := domain.TransactionTypeDeposit
	category := domain.TransactionCategoryAdminCredit
	if amount < 0 {
		txType = domain.TransactionTypeWithdraw
		category = domain.TransactionCategoryAdminDebit
	}
	note := reason
	if note == "" {
		note = "admin balance adjustment"
	}
	record := &domain.Transaction{
		UserID:    userID,
		Type:      txType,
		Category:  category,
		Amount:    absFloat(amount),
		Status:    domain.TransactionStatusCompleted,
		Reference: &note,
	}
	if err := s.transactionRepo.Create(ctx, tx, record); err != nil {
		return nil, fmt.Errorf("failed to record adjustment: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return record, nil
}

// PayReferralReward credits the referrer of referredUserID exactly once, when
// that user's first deposit is approved. Atomic and idempotent: the
// referral_rewarded flag is claimed and the referrer credited in a SINGLE
// transaction, so concurrent deposit approvals can never pay twice, and a
// failure to credit rolls the flag back. Returns the referrer's id when a
// reward was paid, or nil when there was nothing to pay (no referrer, or
// already rewarded).
func (s *TransactionService) PayReferralReward(ctx context.Context, referredUserID uuid.UUID, amount float64) (*uuid.UUID, error) {
	if amount <= 0 {
		return nil, fmt.Errorf("referral reward must be positive")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Claim the reward atomically: only the first caller to flip the flag for a
	// referred user gets a row back, and only when a referrer is actually set.
	var referrerID uuid.UUID
	err = tx.QueryRowContext(ctx, `
		UPDATE users
		   SET referral_rewarded = true, updated_at = CURRENT_TIMESTAMP
		 WHERE id = $1 AND referred_by IS NOT NULL AND referral_rewarded = false
		RETURNING referred_by`, referredUserID).Scan(&referrerID)
	if errors.Is(err, sql.ErrNoRows) {
		// No referrer, or already rewarded — nothing to do.
		return nil, tx.Commit()
	}
	if err != nil {
		return nil, fmt.Errorf("failed to claim referral reward: %w", err)
	}

	// Credit the referrer's wallet within the same transaction.
	if _, err := s.walletRepo.LockForUpdate(ctx, tx, referrerID); err != nil {
		return nil, fmt.Errorf("referrer wallet not found: %w", err)
	}
	if err := s.walletRepo.UpdateBalance(ctx, tx, referrerID, amount); err != nil {
		return nil, fmt.Errorf("failed to credit referrer: %w", err)
	}
	note := "Referral reward"
	record := &domain.Transaction{
		UserID:    referrerID,
		Type:      domain.TransactionTypeDeposit,
		Category:  domain.TransactionCategoryReferralReward,
		Amount:    amount,
		Status:    domain.TransactionStatusCompleted,
		Reference: &note,
	}
	if err := s.transactionRepo.Create(ctx, tx, record); err != nil {
		return nil, fmt.Errorf("failed to record referral reward: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit referral reward: %w", err)
	}
	return &referrerID, nil
}

func absFloat(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
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
func (s *TransactionService) ProcessWithdrawal(ctx context.Context, userID uuid.UUID, amount float64, accountNumber string, accountType domain.PaymentMethod) (*domain.Transaction, error) {
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

	// Check if user has at least one completed *real* cash-in deposit. Genuine
	// deposits have a NULL reference; system credits that share the deposit type
	// (game prizes "GAME_PRIZE", refunds "GAME_REFUND", admin balance adjustments)
	// all carry a reference, so `reference IS NULL` excludes them — a player who
	// only ever won/was-refunded/was-credited still can't withdraw without
	// having put real money in.
	var depositCount int
	checkDepositQuery := `
		SELECT COUNT(*)
		FROM transactions
		WHERE user_id = $1
		  AND type = $2
		  AND status = $3
		  AND reference IS NULL
	`
	err = tx.QueryRowContext(ctx, checkDepositQuery, userID, domain.TransactionTypeDeposit, domain.TransactionStatusCompleted).Scan(&depositCount)
	if err != nil {
		return nil, fmt.Errorf("failed to check deposit history: %w", err)
	}

	if depositCount == 0 {
		return nil, fmt.Errorf("withdrawal not allowed: user must have at least one completed deposit")
	}

	// Check balance
	if wallet.Balance < amount {
		return nil, fmt.Errorf("insufficient balance")
	}

	// Remaining balance floor: the player must keep at least this much.
	remainingBalance := wallet.Balance - amount
	if remainingBalance < domain.MinBalanceAfterWithdrawal {
		return nil, fmt.Errorf("withdrawal not allowed: remaining balance must be at least %.0f", domain.MinBalanceAfterWithdrawal)
	}

	// Subtract balance immediately
	if err := s.walletRepo.UpdateBalance(ctx, tx, userID, -amount); err != nil {
		return nil, fmt.Errorf("failed to update balance: %w", err)
	}

	// Create transaction with pending status
	transaction := &domain.Transaction{
		UserID:          userID,
		Type:            domain.TransactionTypeWithdraw,
		Category:        domain.TransactionCategoryWithdrawal,
		Amount:          amount,
		Status:          domain.TransactionStatusPending,
		TransactionType: &accountType,
		TransactionID:   &accountNumber,
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

	// Lock both wallets in a consistent order (by UUID) to avoid deadlocks when
	// two reciprocal transfers (A->B and B->A) run concurrently.
	firstID, secondID := senderID, receiverID
	if receiverID.String() < senderID.String() {
		firstID, secondID = receiverID, senderID
	}
	firstWallet, err := s.walletRepo.LockForUpdate(ctx, tx, firstID)
	if err != nil {
		return nil, nil, fmt.Errorf("wallet not found: %w", err)
	}
	secondWallet, err := s.walletRepo.LockForUpdate(ctx, tx, secondID)
	if err != nil {
		return nil, nil, fmt.Errorf("wallet not found: %w", err)
	}

	// Identify the sender's wallet for the balance check.
	senderWallet := firstWallet
	if firstID != senderID {
		senderWallet = secondWallet
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
		Category:  domain.TransactionCategoryTransferOut,
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
		Category:  domain.TransactionCategoryTransferIn,
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

// ethiopianDayStart returns midnight of the current Ethiopian calendar day
// (UTC+3), expressed in the SAME wall-clock frame that timestamps are written
// in — which is why it takes `local` rather than assuming one.
//
// transactions.created_at is `timestamp without time zone` and is written by
// the application (transactionRepository.Create uses time.Now()), so the driver
// discards the offset and the column stores the app's LOCAL wall clock. A
// day-boundary computed in a different frame than the stored values silently
// slides the window.
//
// This previously ended in .UTC(), which is correct only while the app process
// happens to run in UTC — true in production today purely because the runtime
// image is bare alpine with no tzdata and no TZ set, so Go falls back to UTC.
// Setting TZ, or adding tzdata for some unrelated reason, would have shifted
// the daily withdrawal window by three hours with nothing to indicate it: the
// window would have started at 21:00 the previous evening, so a player's late
// withdrawals from yesterday would count against today's cap and block them
// early. Converting into `local` instead is correct whatever the process
// timezone is, and makes a development machine behave like production.
func ethiopianDayStart(now time.Time, local *time.Location) time.Time {
	eat := time.FixedZone("EAT", 3*60*60)
	nowEAT := now.In(eat)
	midnightEAT := time.Date(nowEAT.Year(), nowEAT.Month(), nowEAT.Day(), 0, 0, 0, 0, eat)
	return midnightEAT.In(local)
}
