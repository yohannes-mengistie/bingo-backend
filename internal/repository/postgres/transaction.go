package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/bingo/backend/internal/domain"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

type transactionRepository struct {
	db *sql.DB
}

// NewTransactionRepository creates a new PostgreSQL transaction repository
func NewTransactionRepository(db *sql.DB) domain.TransactionRepository {
	return &transactionRepository{db: db}
}

// Create inserts a new transaction into the database
func (r *transactionRepository) Create(ctx context.Context, tx *sql.Tx, transaction *domain.Transaction) error {
	query := `
		INSERT INTO transactions (id, user_id, type, amount, status, transaction_type, transaction_id, reference, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`

	now := time.Now()
	transaction.CreatedAt = now

	if transaction.ID == uuid.Nil {
		transaction.ID = uuid.New()
	}

	var err error
	if tx != nil {
		_, err = tx.ExecContext(
			ctx,
			query,
			transaction.ID,
			transaction.UserID,
			transaction.Type,
			transaction.Amount,
			transaction.Status,
			transaction.TransactionType,
			transaction.TransactionID,
			transaction.Reference,
			transaction.CreatedAt,
		)
	} else {
		_, err = r.db.ExecContext(
			ctx,
			query,
			transaction.ID,
			transaction.UserID,
			transaction.Type,
			transaction.Amount,
			transaction.Status,
			transaction.TransactionType,
			transaction.TransactionID,
			transaction.Reference,
			transaction.CreatedAt,
		)
	}

	if err != nil {
		return fmt.Errorf("failed to create transaction: %w", err)
	}

	return nil
}

// FindByID finds a transaction by ID
func (r *transactionRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.Transaction, error) {
	query := `
		SELECT id, user_id, type, amount, status, transaction_type, transaction_id, reference, created_at
		FROM transactions
		WHERE id = $1
	`

	transaction := &domain.Transaction{}
	var transactionType sql.NullString
	var transactionID sql.NullString
	var reference sql.NullString

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&transaction.ID,
		&transaction.UserID,
		&transaction.Type,
		&transaction.Amount,
		&transaction.Status,
		&transactionType,
		&transactionID,
		&reference,
		&transaction.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("transaction not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find transaction: %w", err)
	}

	if transactionType.Valid {
		pm := domain.PaymentMethod(transactionType.String)
		transaction.TransactionType = &pm
	}
	if transactionID.Valid {
		transaction.TransactionID = &transactionID.String
	}
	if reference.Valid {
		transaction.Reference = &reference.String
	}

	return transaction, nil
}

// FindByUserID finds transactions by user ID
func (r *transactionRepository) FindByUserID(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.Transaction, error) {
	query := `
		SELECT id, user_id, type, amount, status, transaction_type, transaction_id, reference, created_at
		FROM transactions
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.db.QueryContext(ctx, query, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to find transactions: %w", err)
	}
	defer rows.Close()

	var transactions []*domain.Transaction
	for rows.Next() {
		transaction := &domain.Transaction{}
		var transactionType sql.NullString
		var transactionID sql.NullString
		var reference sql.NullString

		err := rows.Scan(
			&transaction.ID,
			&transaction.UserID,
			&transaction.Type,
			&transaction.Amount,
			&transaction.Status,
			&transactionType,
			&transactionID,
			&reference,
			&transaction.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan transaction: %w", err)
		}

		if transactionType.Valid {
			pm := domain.PaymentMethod(transactionType.String)
			transaction.TransactionType = &pm
		}
		if transactionID.Valid {
			transaction.TransactionID = &transactionID.String
		}
		if reference.Valid {
			transaction.Reference = &reference.String
		}

		transactions = append(transactions, transaction)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate transactions: %w", err)
	}

	return transactions, nil
}

// FindByUserIDAndType finds transactions by user ID and transaction type
func (r *transactionRepository) FindByUserIDAndType(ctx context.Context, userID uuid.UUID, transactionType domain.TransactionType, limit int) ([]*domain.Transaction, error) {
	query := `
		SELECT id, user_id, type, amount, status, transaction_type, transaction_id, reference, created_at
		FROM transactions
		WHERE user_id = $1 AND type = $2
		ORDER BY created_at DESC
		LIMIT $3
	`

	rows, err := r.db.QueryContext(ctx, query, userID, transactionType, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to find transactions: %w", err)
	}
	defer rows.Close()

	return r.scanTransactions(rows)
}

// FindByUserIDAndTypes finds transactions by user ID and multiple transaction types
func (r *transactionRepository) FindByUserIDAndTypes(ctx context.Context, userID uuid.UUID, transactionTypes []domain.TransactionType, limit int) ([]*domain.Transaction, error) {
	if len(transactionTypes) == 0 {
		return []*domain.Transaction{}, nil
	}

	// Build query with IN clause
	query := `
		SELECT id, user_id, type, amount, status, transaction_type, transaction_id, reference, created_at
		FROM transactions
		WHERE user_id = $1 AND type = ANY($2)
		ORDER BY created_at DESC
		LIMIT $3
	`

	// Convert []TransactionType to []string for PostgreSQL array
	typeStrings := make([]string, len(transactionTypes))
	for i, t := range transactionTypes {
		typeStrings[i] = string(t)
	}

	rows, err := r.db.QueryContext(ctx, query, userID, pq.Array(typeStrings), limit)
	if err != nil {
		return nil, fmt.Errorf("failed to find transactions: %w", err)
	}
	defer rows.Close()

	return r.scanTransactions(rows)
}

// scanTransactions is a helper function to scan transaction rows
func (r *transactionRepository) scanTransactions(rows *sql.Rows) ([]*domain.Transaction, error) {
	var transactions []*domain.Transaction
	for rows.Next() {
		transaction := &domain.Transaction{}
		var transactionType sql.NullString
		var transactionID sql.NullString
		var reference sql.NullString

		err := rows.Scan(
			&transaction.ID,
			&transaction.UserID,
			&transaction.Type,
			&transaction.Amount,
			&transaction.Status,
			&transactionType,
			&transactionID,
			&reference,
			&transaction.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan transaction: %w", err)
		}

		if transactionType.Valid {
			pm := domain.PaymentMethod(transactionType.String)
			transaction.TransactionType = &pm
		}
		if transactionID.Valid {
			transaction.TransactionID = &transactionID.String
		}
		if reference.Valid {
			transaction.Reference = &reference.String
		}

		transactions = append(transactions, transaction)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate transactions: %w", err)
	}

	return transactions, nil
}

// FindByStatusAndType finds transactions by status and type
func (r *transactionRepository) FindByStatusAndType(ctx context.Context, status domain.TransactionStatus, transactionType domain.TransactionType, limit, offset int) ([]*domain.Transaction, error) {
	query := `
		SELECT id, user_id, type, amount, status, transaction_type, transaction_id, reference, created_at
		FROM transactions
		WHERE status = $1 AND type = $2
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4
	`

	rows, err := r.db.QueryContext(ctx, query, status, transactionType, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to find transactions: %w", err)
	}
	defer rows.Close()

	return r.scanTransactions(rows)
}

// FindByStatus finds transactions by status
func (r *transactionRepository) FindByStatus(ctx context.Context, status domain.TransactionStatus, limit, offset int) ([]*domain.Transaction, error) {
	query := `
		SELECT id, user_id, type, amount, status, transaction_type, transaction_id, reference, created_at
		FROM transactions
		WHERE status = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.db.QueryContext(ctx, query, status, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to find transactions: %w", err)
	}
	defer rows.Close()

	return r.scanTransactions(rows)
}

// FindByTypes finds transactions by multiple types
func (r *transactionRepository) FindByTypes(ctx context.Context, transactionTypes []domain.TransactionType, limit, offset int) ([]*domain.Transaction, error) {
	if len(transactionTypes) == 0 {
		return []*domain.Transaction{}, nil
	}

	query := `
		SELECT id, user_id, type, amount, status, transaction_type, transaction_id, reference, created_at
		FROM transactions
		WHERE type = ANY($1)
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	typeStrings := make([]string, len(transactionTypes))
	for i, t := range transactionTypes {
		typeStrings[i] = string(t)
	}

	rows, err := r.db.QueryContext(ctx, query, pq.Array(typeStrings), limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to find transactions: %w", err)
	}
	defer rows.Close()

	return r.scanTransactions(rows)
}

// FindAll finds all transactions with pagination
func (r *transactionRepository) FindAll(ctx context.Context, limit, offset int) ([]*domain.Transaction, error) {
	query := `
		SELECT id, user_id, type, amount, status, transaction_type, transaction_id, reference, created_at
		FROM transactions
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := r.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to find transactions: %w", err)
	}
	defer rows.Close()

	return r.scanTransactions(rows)
}

// UpdateStatus updates the status of a transaction
func (r *transactionRepository) UpdateStatus(ctx context.Context, tx *sql.Tx, id uuid.UUID, status domain.TransactionStatus) error {
	query := `
		UPDATE transactions
		SET status = $2
		WHERE id = $1
	`

	result, err := tx.ExecContext(ctx, query, id, status)
	if err != nil {
		return fmt.Errorf("failed to update transaction status: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("transaction not found")
	}

	return nil
}
