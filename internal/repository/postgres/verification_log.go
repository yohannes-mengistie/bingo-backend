package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"github.com/bingo/backend/internal/domain"
	"github.com/google/uuid"
)

// verificationLogRepository persists external payment-verifier lookups for the
// admin audit view.
type verificationLogRepository struct {
	db *sql.DB
}

// NewVerificationLogRepository creates a new verification-log repository.
func NewVerificationLogRepository(db *sql.DB) domain.VerificationLogRepository {
	return &verificationLogRepository{db: db}
}

// Record inserts one verifier lookup. It is best-effort: a logging failure is
// logged and swallowed so it can never break the deposit that triggered it.
func (r *verificationLogRepository) Record(ctx context.Context, entry *domain.VerificationLog) {
	if entry == nil {
		return
	}
	if entry.ID == uuid.Nil {
		entry.ID = uuid.New()
	}
	query := `
		INSERT INTO verification_logs (id, user_id, method, reference, outcome, reason, amount, raw_response)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	if _, err := r.db.ExecContext(ctx, query,
		entry.ID, entry.UserID, entry.Method, entry.Reference,
		entry.Outcome, entry.Reason, entry.Amount, entry.RawResponse,
	); err != nil {
		log.Printf("[verify] failed to record verification log for ref %s: %v", entry.Reference, err)
	}
}

// LatestByReference returns the most recent lookup for a receipt reference
// (case-insensitive), or nil when there is none.
func (r *verificationLogRepository) LatestByReference(ctx context.Context, reference string) (*domain.VerificationLog, error) {
	query := `
		SELECT id, user_id, method, reference, outcome, reason, amount, raw_response, created_at
		FROM verification_logs
		WHERE UPPER(reference) = UPPER($1)
		ORDER BY created_at DESC
		LIMIT 1
	`
	entry := &domain.VerificationLog{}
	var amount sql.NullFloat64
	err := r.db.QueryRowContext(ctx, query, reference).Scan(
		&entry.ID, &entry.UserID, &entry.Method, &entry.Reference,
		&entry.Outcome, &entry.Reason, &amount, &entry.RawResponse, &entry.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load latest verification log: %w", err)
	}
	if amount.Valid {
		entry.Amount = &amount.Float64
	}
	return entry, nil
}

// List returns verification lookups newest-first, optionally filtered by receipt
// reference (case-insensitive), along with the total count for paging.
func (r *verificationLogRepository) List(ctx context.Context, reference string, limit, offset int) ([]*domain.VerificationLog, int, error) {
	// $1 is the optional reference filter: empty string keeps every row, otherwise
	// a case-insensitive substring match so an admin can paste a partial receipt.
	where := `WHERE ($1 = '' OR UPPER(reference) LIKE '%' || UPPER($1) || '%')`

	var total int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM verification_logs `+where, reference,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count verification logs: %w", err)
	}

	query := `
		SELECT id, user_id, method, reference, outcome, reason, amount, raw_response, created_at
		FROM verification_logs ` + where + `
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`
	rows, err := r.db.QueryContext(ctx, query, reference, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list verification logs: %w", err)
	}
	defer rows.Close()

	logs := make([]*domain.VerificationLog, 0)
	for rows.Next() {
		entry := &domain.VerificationLog{}
		var amount sql.NullFloat64
		if err := rows.Scan(
			&entry.ID,
			&entry.UserID,
			&entry.Method,
			&entry.Reference,
			&entry.Outcome,
			&entry.Reason,
			&amount,
			&entry.RawResponse,
			&entry.CreatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan verification log: %w", err)
		}
		if amount.Valid {
			entry.Amount = &amount.Float64
		}
		logs = append(logs, entry)
	}
	return logs, total, rows.Err()
}
