package handler

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/bingo/backend/internal/domain"
	"github.com/bingo/backend/internal/middleware"
	"github.com/bingo/backend/internal/usecase"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type WalletHandler struct {
	walletUseCase *usecase.WalletUseCase
}

// NewWalletHandler creates a new wallet handler
func NewWalletHandler(walletUseCase *usecase.WalletUseCase) *WalletHandler {
	return &WalletHandler{
		walletUseCase: walletUseCase,
	}
}

// Deposit handles the POST /wallet/deposit endpoint (authenticated)
func (h *WalletHandler) Deposit(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	var req domain.DepositRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request data",
			"details": err.Error(),
		})
		return
	}
	req.UserID = userID

	transaction, err := h.walletUseCase.Deposit(c.Request.Context(), req)
	if err != nil {
		statusCode := http.StatusInternalServerError
		if err.Error() == "amount must be greater than 0" ||
			err.Error() == "transaction_type must be one of Telebirr, CBEBirr, Mpesa" ||
			err.Error() == "transaction_id is required" ||
			err.Error() == "payment provider does not match transaction_type" ||
			strings.HasPrefix(err.Error(), "minimum deposit is") {
			statusCode = http.StatusBadRequest
		} else if strings.HasPrefix(err.Error(), "payment verification failed:") ||
			strings.HasPrefix(err.Error(), "verified payment amount") {
			statusCode = http.StatusBadRequest
		} else if err.Error() == "user not found" {
			statusCode = http.StatusNotFound
		} else if err.Error() == "this transaction reference was already used" {
			statusCode = http.StatusConflict
		}

		c.JSON(statusCode, gin.H{
			"error": err.Error(),
		})
		return
	}

	message := "Deposit request created successfully"
	if transaction.Status == domain.TransactionStatusCompleted {
		message = "Deposit verified and completed successfully"
	}

	c.JSON(http.StatusCreated, gin.H{
		"message":     message,
		"transaction": transaction,
	})
}

// Withdraw handles the POST /wallet/withdraw endpoint (authenticated)
func (h *WalletHandler) Withdraw(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	var req domain.WithdrawRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request data",
			"details": err.Error(),
		})
		return
	}
	req.UserID = userID

	transaction, err := h.walletUseCase.Withdraw(c.Request.Context(), req)
	if err != nil {
		statusCode := http.StatusInternalServerError
		if err.Error() == "amount must be greater than 0" ||
			err.Error() == "account_type must be one of Telebirr, CBEBirr, Mpesa" ||
			err.Error() == "withdrawal account must be a valid Ethiopian phone number" ||
			strings.HasPrefix(err.Error(), "minimum withdrawal") ||
			strings.HasPrefix(err.Error(), "no verified phone number") {
			statusCode = http.StatusBadRequest
		} else if err.Error() == "user not found" || err.Error() == "wallet not found" {
			statusCode = http.StatusNotFound
		} else if err.Error() == "insufficient balance" ||
			// Match on prefix rather than the whole string. These messages
			// interpolate constants (the balance floor, the daily cap), so an
			// exact comparison goes stale the moment a limit moves and silently
			// turns a plain validation rejection into a 500. That is precisely
			// what had happened here: the text still said "at least 10" while
			// MinBalanceAfterWithdrawal had become 50, so every player who hit
			// the floor got a server error instead of a readable 400.
			strings.HasPrefix(err.Error(), "withdrawal not allowed:") {
			statusCode = http.StatusBadRequest
		}

		c.JSON(statusCode, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":     "Withdrawal processed successfully",
		"transaction": transaction,
	})
}

// Transfer handles the POST /wallet/transfer endpoint (authenticated).
// The sender is always the authenticated user; only the receiver comes from the body.
func (h *WalletHandler) Transfer(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	var req domain.TransferRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request data",
			"details": err.Error(),
		})
		return
	}
	req.SenderID = userID

	senderTx, receiverTx, err := h.walletUseCase.Transfer(c.Request.Context(), req)
	if err != nil {
		statusCode := http.StatusInternalServerError
		if err.Error() == "amount must be greater than 0" {
			statusCode = http.StatusBadRequest
		} else if err.Error() == "cannot transfer to yourself" {
			statusCode = http.StatusBadRequest
		} else if err.Error() == "sender not found" || err.Error() == "receiver not found" ||
			err.Error() == "sender wallet not found" || err.Error() == "receiver wallet not found" {
			statusCode = http.StatusNotFound
		} else if err.Error() == "insufficient balance" {
			statusCode = http.StatusBadRequest
		}

		c.JSON(statusCode, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":     "Transfer completed successfully",
		"sender_tx":   senderTx,
		"receiver_tx": receiverTx,
	})
}

// historyLimit returns the limit for a history query (?all=true returns everything).
func historyLimit(c *gin.Context) int {
	if c.Query("all") == "true" {
		return 10000
	}
	return 10
}

// GetMyWallet handles GET /me/wallet — the authenticated user's wallet.
func (h *WalletHandler) GetMyWallet(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	wallet, err := h.walletUseCase.GetWallet(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Wallet not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"wallet": wallet})
}

// GetMyDeposits handles GET /me/wallet/deposits.
func (h *WalletHandler) GetMyDeposits(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	transactions, err := h.walletUseCase.GetDepositHistory(c.Request.Context(), userID, historyLimit(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch deposit history"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"deposits": transactions, "count": len(transactions)})
}

// GetMyWithdrawals handles GET /me/wallet/withdrawals.
func (h *WalletHandler) GetMyWithdrawals(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	transactions, err := h.walletUseCase.GetWithdrawalHistory(c.Request.Context(), userID, historyLimit(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch withdrawal history"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"withdrawals": transactions, "count": len(transactions)})
}

// GetMyTransfers handles GET /me/wallet/transfers.
func (h *WalletHandler) GetMyTransfers(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	transactions, err := h.walletUseCase.GetTransferHistory(c.Request.Context(), userID, historyLimit(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch transfer history"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"transfers": transactions, "count": len(transactions)})
}

// GetWallet handles the GET /wallet/:user_id endpoint
func (h *WalletHandler) GetWallet(c *gin.Context) {
	userIDStr := c.Param("user_id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid user ID",
		})
		return
	}

	wallet, err := h.walletUseCase.GetWallet(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Wallet not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"wallet": wallet,
	})
}

// GetWalletByTelegramID handles the GET /wallet/telegram/:telegram_id endpoint
func (h *WalletHandler) GetWalletByTelegramID(c *gin.Context) {
	var uri struct {
		TelegramID int64 `uri:"telegram_id" binding:"required"`
	}

	if err := c.ShouldBindUri(&uri); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid telegram ID",
		})
		return
	}

	wallet, err := h.walletUseCase.GetWalletByTelegramID(c.Request.Context(), uri.TelegramID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Wallet not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"wallet": wallet,
	})
}

// GetDepositHistory handles the GET /wallet/:user_id/deposits endpoint
// Query parameter: ?all=true to get all deposits (default: 10)
func (h *WalletHandler) GetDepositHistory(c *gin.Context) {
	userIDStr := c.Param("user_id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid user ID",
		})
		return
	}

	// Check if all parameter is provided
	limit := 10 // default limit
	if c.Query("all") == "true" {
		limit = 10000 // large limit to fetch all
	}

	transactions, err := h.walletUseCase.GetDepositHistory(c.Request.Context(), userID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to fetch deposit history",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"deposits": transactions,
		"count":    len(transactions),
	})
}

// GetWithdrawalHistory handles the GET /wallet/:user_id/withdrawals endpoint
// Query parameter: ?all=true to get all withdrawals (default: 10)
func (h *WalletHandler) GetWithdrawalHistory(c *gin.Context) {
	userIDStr := c.Param("user_id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid user ID",
		})
		return
	}

	// Check if all parameter is provided
	limit := 10 // default limit
	if c.Query("all") == "true" {
		limit = 10000 // large limit to fetch all
	}

	transactions, err := h.walletUseCase.GetWithdrawalHistory(c.Request.Context(), userID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to fetch withdrawal history",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"withdrawals": transactions,
		"count":       len(transactions),
	})
}

// GetTransferHistory handles the GET /wallet/:user_id/transfers endpoint
// Query parameter: ?all=true to get all transfers (default: 10)
func (h *WalletHandler) GetTransferHistory(c *gin.Context) {
	userIDStr := c.Param("user_id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid user ID",
		})
		return
	}

	// Check if all parameter is provided
	limit := 10 // default limit
	if c.Query("all") == "true" {
		limit = 10000 // large limit to fetch all
	}

	transactions, err := h.walletUseCase.GetTransferHistory(c.Request.Context(), userID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to fetch transfer history",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"transfers": transactions,
		"count":     len(transactions),
	})
}

// ApproveDeposit handles the POST /admin/transactions/:id/approve endpoint
func (h *WalletHandler) ApproveDeposit(c *gin.Context) {
	transactionIDStr := c.Param("id")
	transactionID, err := uuid.Parse(transactionIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid transaction ID",
		})
		return
	}

	transaction, err := h.walletUseCase.ApproveDeposit(c.Request.Context(), transactionID)
	if err != nil {
		statusCode := http.StatusInternalServerError
		if err.Error() == "transaction not found" {
			statusCode = http.StatusNotFound
		} else if err.Error() == "transaction is not a deposit" ||
			err.Error() == "transaction is not pending (current status: completed)" ||
			err.Error() == "transaction is not pending (current status: failed)" ||
			err.Error() == "transaction is not pending (current status: cancelled)" {
			statusCode = http.StatusBadRequest
		}

		c.JSON(statusCode, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":     "Deposit approved successfully",
		"transaction": transaction,
	})
}

// RejectDeposit handles the POST /admin/transactions/:id/reject-deposit endpoint
func (h *WalletHandler) RejectDeposit(c *gin.Context) {
	transactionIDStr := c.Param("id")
	transactionID, err := uuid.Parse(transactionIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid transaction ID",
		})
		return
	}

	transaction, err := h.walletUseCase.RejectDeposit(c.Request.Context(), transactionID)
	if err != nil {
		statusCode := http.StatusInternalServerError
		if err.Error() == "transaction not found" {
			statusCode = http.StatusNotFound
		} else if err.Error() == "transaction is not a deposit" ||
			err.Error() == "transaction is not pending (current status: completed)" ||
			err.Error() == "transaction is not pending (current status: failed)" ||
			err.Error() == "transaction is not pending (current status: cancelled)" {
			statusCode = http.StatusBadRequest
		}

		c.JSON(statusCode, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":     "Deposit rejected successfully",
		"transaction": transaction,
	})
}

// ApproveWithdrawal handles the POST /admin/transactions/:id/approve-withdrawal endpoint
func (h *WalletHandler) ApproveWithdrawal(c *gin.Context) {
	transactionIDStr := c.Param("id")
	transactionID, err := uuid.Parse(transactionIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid transaction ID",
		})
		return
	}

	transaction, err := h.walletUseCase.ApproveWithdrawal(c.Request.Context(), transactionID)
	if err != nil {
		statusCode := http.StatusInternalServerError
		if err.Error() == "transaction not found" {
			statusCode = http.StatusNotFound
		} else if err.Error() == "transaction is not a withdrawal" ||
			err.Error() == "transaction is not pending (current status: completed)" ||
			err.Error() == "transaction is not pending (current status: failed)" ||
			err.Error() == "transaction is not pending (current status: cancelled)" {
			statusCode = http.StatusBadRequest
		}

		c.JSON(statusCode, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":     "Withdrawal approved successfully",
		"transaction": transaction,
	})
}

// RejectWithdrawal handles the POST /admin/transactions/:id/reject-withdrawal endpoint
func (h *WalletHandler) RejectWithdrawal(c *gin.Context) {
	transactionIDStr := c.Param("id")
	transactionID, err := uuid.Parse(transactionIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid transaction ID",
		})
		return
	}

	transaction, err := h.walletUseCase.RejectWithdrawal(c.Request.Context(), transactionID)
	if err != nil {
		statusCode := http.StatusInternalServerError
		if err.Error() == "transaction not found" {
			statusCode = http.StatusNotFound
		} else if err.Error() == "transaction is not a withdrawal" ||
			err.Error() == "transaction is not pending (current status: completed)" ||
			err.Error() == "transaction is not pending (current status: failed)" ||
			err.Error() == "transaction is not pending (current status: cancelled)" {
			statusCode = http.StatusBadRequest
		}

		c.JSON(statusCode, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":     "Withdrawal rejected and balance refunded",
		"transaction": transaction,
	})
}

// RejectWithdrawalToBonus handles POST /admin/transactions/:id/reject-to-bonus —
// rolls back a pending withdrawal, returning the genuine (deposit/winnings-backed)
// portion to withdrawable cash and the rest as play-only bonus.
func (h *WalletHandler) RejectWithdrawalToBonus(c *gin.Context) {
	transactionID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid transaction ID"})
		return
	}
	res, err := h.walletUseCase.RejectWithdrawalToBonus(c.Request.Context(), transactionID)
	if err != nil {
		statusCode := http.StatusInternalServerError
		if strings.HasPrefix(err.Error(), "transaction not found") {
			statusCode = http.StatusNotFound
		} else if err.Error() == "transaction is not a withdrawal" ||
			strings.HasPrefix(err.Error(), "transaction is not pending") {
			statusCode = http.StatusBadRequest
		}
		c.JSON(statusCode, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message": fmt.Sprintf("Rolled back: %.0f to balance, %.0f to bonus", res.RealRefunded, res.BonusGranted),
		"result":  res,
	})
}

// CancelTransaction handles the POST /admin/transactions/:id/cancel endpoint
func (h *WalletHandler) CancelTransaction(c *gin.Context) {
	transactionIDStr := c.Param("id")
	transactionID, err := uuid.Parse(transactionIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid transaction ID",
		})
		return
	}

	transaction, err := h.walletUseCase.CancelTransaction(c.Request.Context(), transactionID)
	if err != nil {
		statusCode := http.StatusInternalServerError
		if err.Error() == "transaction not found" {
			statusCode = http.StatusNotFound
		} else if err.Error() == "transaction is not pending (current status: completed)" ||
			err.Error() == "transaction is not pending (current status: failed)" ||
			err.Error() == "transaction is not pending (current status: cancelled)" {
			statusCode = http.StatusBadRequest
		}

		c.JSON(statusCode, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":     "Transaction cancelled successfully",
		"transaction": transaction,
	})
}

// Admin transaction query handlers

// GetPendingDeposits handles GET /admin/transactions/pending/deposits
func (h *WalletHandler) GetPendingDeposits(c *gin.Context) {
	limit, offset := getPaginationParams(c)

	transactions, err := h.walletUseCase.GetPendingDeposits(c.Request.Context(), limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to fetch pending deposits",
		})
		return
	}

	total, _ := h.walletUseCase.CountByStatusAndType(c.Request.Context(), domain.TransactionStatusPending, domain.TransactionTypeDeposit)
	c.JSON(http.StatusOK, gin.H{
		"transactions": transactions,
		"count":        len(transactions),
		"total":        total,
		"limit":        limit,
		"offset":       offset,
	})
}

// GetPendingWithdrawals handles GET /admin/transactions/pending/withdrawals
func (h *WalletHandler) GetPendingWithdrawals(c *gin.Context) {
	limit, offset := getPaginationParams(c)

	transactions, err := h.walletUseCase.GetPendingWithdrawals(c.Request.Context(), limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to fetch pending withdrawals",
		})
		return
	}

	total, _ := h.walletUseCase.CountWithdrawalsByStatus(c.Request.Context(), domain.TransactionStatusPending)
	c.JSON(http.StatusOK, gin.H{
		"transactions": transactions,
		"count":        len(transactions),
		"total":        total,
		"limit":        limit,
		"offset":       offset,
	})
}

// GetCompletedDeposits handles GET /admin/transactions/completed/deposits
func (h *WalletHandler) GetCompletedDeposits(c *gin.Context) {
	limit, offset := getPaginationParams(c)

	transactions, err := h.walletUseCase.GetCompletedDeposits(c.Request.Context(), limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to fetch completed deposits",
		})
		return
	}

	total, _ := h.walletUseCase.CountByStatusAndType(c.Request.Context(), domain.TransactionStatusCompleted, domain.TransactionTypeDeposit)
	c.JSON(http.StatusOK, gin.H{
		"transactions": transactions,
		"count":        len(transactions),
		"total":        total,
		"limit":        limit,
		"offset":       offset,
	})
}

// GetCompletedWithdrawals handles GET /admin/transactions/completed/withdrawals
func (h *WalletHandler) GetCompletedWithdrawals(c *gin.Context) {
	limit, offset := getPaginationParams(c)

	transactions, err := h.walletUseCase.GetCompletedWithdrawals(c.Request.Context(), limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to fetch completed withdrawals",
		})
		return
	}

	total, _ := h.walletUseCase.CountWithdrawalsByStatus(c.Request.Context(), domain.TransactionStatusCompleted)
	c.JSON(http.StatusOK, gin.H{
		"transactions": transactions,
		"count":        len(transactions),
		"total":        total,
		"limit":        limit,
		"offset":       offset,
	})
}

// GetFailedTransactions handles GET /admin/transactions/failed
func (h *WalletHandler) GetFailedTransactions(c *gin.Context) {
	limit, offset := getPaginationParams(c)

	transactions, err := h.walletUseCase.GetFailedTransactions(c.Request.Context(), limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to fetch failed transactions",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"transactions": transactions,
		"count":        len(transactions),
		"limit":        limit,
		"offset":       offset,
	})
}

// GetTransferTransactions handles GET /admin/transactions/transfers
func (h *WalletHandler) GetTransferTransactions(c *gin.Context) {
	limit, offset := getPaginationParams(c)

	transactions, err := h.walletUseCase.GetTransferTransactions(c.Request.Context(), limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to fetch transfer transactions",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"transactions": transactions,
		"count":        len(transactions),
		"limit":        limit,
		"offset":       offset,
	})
}

// GetAllTransactions handles GET /admin/transactions
func (h *WalletHandler) GetAllTransactions(c *gin.Context) {
	limit, offset := getPaginationParams(c)

	transactions, err := h.walletUseCase.GetAllTransactions(c.Request.Context(), limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to fetch transactions",
		})
		return
	}

	// total is the grand count of all transactions, so the admin can page through
	// them; count stays the page size for backward compatibility.
	total, _ := h.walletUseCase.CountAllTransactions(c.Request.Context())

	c.JSON(http.StatusOK, gin.H{
		"transactions": transactions,
		"count":        len(transactions),
		"total":        total,
		"limit":        limit,
		"offset":       offset,
	})
}

// GetUserTransactions handles GET /admin/users/:user_id/transactions — one
// player's full transaction history (paginated) for the player-detail view.
func (h *WalletHandler) GetUserTransactions(c *gin.Context) {
	userID, err := uuid.Parse(c.Param("user_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}
	limit, offset := getPaginationParams(c)
	transactions, total, err := h.walletUseCase.GetUserTransactions(c.Request.Context(), userID, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch transactions"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"transactions": transactions,
		"count":        len(transactions),
		"total":        total,
		"limit":        limit,
		"offset":       offset,
	})
}

// GetHouseCutDetail handles GET /admin/dashboard/house-cut — the drill-down
// behind the dashboard house-cut figure (per tier, per day, and real-player P&L).
func (h *WalletHandler) GetHouseCutDetail(c *gin.Context) {
	detail, err := h.walletUseCase.GetHouseCutDetail(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load house-cut detail"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"detail": detail})
}

// GetSettings handles GET /admin/settings — operator-tunable app settings.
func (h *WalletHandler) GetSettings(c *gin.Context) {
	s, err := h.walletUseCase.GetSettings(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load settings"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"settings": s})
}

// MaintenanceStatus reports whether maintenance mode is on and its message. Used
// by the maintenance middleware. Fails open (false) on error so a settings read
// failure can never lock players out.
func (h *WalletHandler) MaintenanceStatus(ctx context.Context) (bool, string) {
	s, err := h.walletUseCase.GetSettings(ctx)
	if err != nil {
		return false, ""
	}
	return s.MaintenanceMode, s.MaintenanceMessage
}

// GetPublicStatus handles GET /status — an UNAUTHENTICATED endpoint the player
// Mini App polls to learn whether it should show a maintenance screen. It exposes
// only the maintenance flag and message, never any operator/financial settings.
func (h *WalletHandler) GetPublicStatus(c *gin.Context) {
	s, err := h.walletUseCase.GetSettings(c.Request.Context())
	if err != nil {
		// Fail open: never take the app down just because this read failed. Use the
		// default minimum deposit so the deposit form still shows a sane figure.
		c.JSON(http.StatusOK, gin.H{"maintenance": false, "message": "", "min_deposit": domain.DefaultMinDeposit})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"maintenance": s.MaintenanceMode,
		"message":     s.MaintenanceMessage,
		"min_deposit": s.MinDeposit,
	})
}

// UpdateSettings handles PUT /admin/settings — change app settings (e.g. minimum deposit).
func (h *WalletHandler) UpdateSettings(c *gin.Context) {
	var req domain.UpdateAppSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}
	s, err := h.walletUseCase.UpdateSettings(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"settings": s})
}

// GetRealPlayerWinnings handles GET /admin/transactions/winners — winnings paid
// to real (non-bot) players, paginated, so an admin can review genuine winners.
func (h *WalletHandler) GetRealPlayerWinnings(c *gin.Context) {
	limit, offset := getPaginationParams(c)

	transactions, total, err := h.walletUseCase.GetRealPlayerWinnings(c.Request.Context(), limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch winners"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"transactions": transactions,
		"count":        len(transactions),
		"total":        total,
		"limit":        limit,
		"offset":       offset,
	})
}

// getPaginationParams extracts limit and offset from query parameters
func getPaginationParams(c *gin.Context) (int, int) {
	limit := 50 // default limit
	offset := 0 // default offset

	if limitStr := c.Query("limit"); limitStr != "" {
		if parsedLimit := parseInt(limitStr); parsedLimit > 0 {
			limit = parsedLimit
		}
	}

	if offsetStr := c.Query("offset"); offsetStr != "" {
		if parsedOffset := parseInt(offsetStr); parsedOffset >= 0 {
			offset = parsedOffset
		}
	}

	return limit, offset
}

// parseInt safely parses a string to int
func parseInt(s string) int {
	var result int
	fmt.Sscanf(s, "%d", &result)
	return result
}

// GetDashboardStats handles GET /admin/stats/dashboard
func (h *WalletHandler) GetDashboardStats(c *gin.Context) {
	stats, err := h.walletUseCase.GetDashboardStats(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to fetch dashboard stats",
		})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// AdjustBalance handles POST /admin/users/:user_id/adjust-balance — manually
// credit (positive amount) or debit (negative amount) a user's wallet.
func (h *WalletHandler) AdjustBalance(c *gin.Context) {
	userID, err := uuid.Parse(c.Param("user_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	var req domain.AdjustBalanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request data", "details": err.Error()})
		return
	}

	txn, err := h.walletUseCase.AdjustBalance(c.Request.Context(), userID, req.Amount, req.Reason)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Balance adjusted", "transaction": txn})
}
