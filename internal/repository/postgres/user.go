package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/bingo/backend/internal/domain"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

type userRepository struct {
	db *sql.DB
}

// NewUserRepository creates a new PostgreSQL user repository
func NewUserRepository(db *sql.DB) domain.UserRepository {
	return &userRepository{db: db}
}

// Create inserts a new user into the database
func (r *userRepository) Create(ctx context.Context, tx *sql.Tx, user *domain.User) error {
	query := `
		INSERT INTO users (id, telegram_id, first_name, last_name, phone_number, referal_code, role, password, is_bot, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`

	now := time.Now()
	user.CreatedAt = now
	user.UpdatedAt = now

	if user.ID == uuid.Nil {
		user.ID = uuid.New()
	}

	// Set default role if not provided
	if user.Role == "" {
		user.Role = "user"
	}

	var err error
	if tx != nil {
		_, err = tx.ExecContext(
			ctx,
			query,
			user.ID,
			user.TelegramID,
			user.FirstName,
			user.LastName,
			user.PhoneNumber,
			user.ReferalCode,
			user.Role,
			user.Password,
			user.IsBot,
			user.CreatedAt,
			user.UpdatedAt,
		)
	} else {
		_, err = r.db.ExecContext(
			ctx,
			query,
			user.ID,
			user.TelegramID,
			user.FirstName,
			user.LastName,
			user.PhoneNumber,
			user.ReferalCode,
			user.Role,
			user.Password,
			user.IsBot,
			user.CreatedAt,
			user.UpdatedAt,
		)
	}

	if err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}

	return nil
}

// FindByTelegramID finds a user by their Telegram ID
func (r *userRepository) FindByTelegramID(ctx context.Context, telegramID int64) (*domain.User, error) {
	query := `
		SELECT id, telegram_id, first_name, last_name, phone_number, referal_code, role, banned, password, created_at, updated_at
		FROM users
		WHERE telegram_id = $1
	`

	user := &domain.User{}
	var lastName sql.NullString
	var password sql.NullString

	err := r.db.QueryRowContext(ctx, query, telegramID).Scan(
		&user.ID,
		&user.TelegramID,
		&user.FirstName,
		&lastName,
		&user.PhoneNumber,
		&user.ReferalCode,
		&user.Role,
		&user.Banned,
		&password,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find user by telegram_id: %w", err)
	}

	if lastName.Valid {
		user.LastName = &lastName.String
	}
	if password.Valid {
		user.Password = &password.String
	}

	return user, nil
}

// FindByPhone finds a user by their phone number
func (r *userRepository) FindByPhone(ctx context.Context, phone string) (*domain.User, error) {
	query := `
		SELECT id, telegram_id, first_name, last_name, phone_number, referal_code, role, password, created_at, updated_at
		FROM users
		WHERE phone_number = $1
	`

	user := &domain.User{}
	var lastName sql.NullString
	var password sql.NullString

	err := r.db.QueryRowContext(ctx, query, phone).Scan(
		&user.ID,
		&user.TelegramID,
		&user.FirstName,
		&lastName,
		&user.PhoneNumber,
		&user.ReferalCode,
		&user.Role,
		&password,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find user by phone: %w", err)
	}

	if lastName.Valid {
		user.LastName = &lastName.String
	}
	if password.Valid {
		user.Password = &password.String
	}

	return user, nil
}

// FindByReferralCode finds a user by their referral code
func (r *userRepository) FindByReferralCode(ctx context.Context, referralCode string) (*domain.User, error) {
	query := `
		SELECT id, telegram_id, first_name, last_name, phone_number, referal_code, role, password, created_at, updated_at
		FROM users
		WHERE referal_code = $1
	`

	user := &domain.User{}
	var lastName sql.NullString
	var password sql.NullString

	err := r.db.QueryRowContext(ctx, query, referralCode).Scan(
		&user.ID,
		&user.TelegramID,
		&user.FirstName,
		&lastName,
		&user.PhoneNumber,
		&user.ReferalCode,
		&user.Role,
		&password,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find user by referral_code: %w", err)
	}

	if lastName.Valid {
		user.LastName = &lastName.String
	}
	if password.Valid {
		user.Password = &password.String
	}

	return user, nil
}

// FindByID finds a user by their ID
func (r *userRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	query := `
		SELECT id, telegram_id, first_name, last_name, phone_number, referal_code, role, banned, password, created_at, updated_at
		FROM users
		WHERE id = $1
	`

	user := &domain.User{}
	var lastName sql.NullString
	var password sql.NullString

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&user.ID,
		&user.TelegramID,
		&user.FirstName,
		&lastName,
		&user.PhoneNumber,
		&user.ReferalCode,
		&user.Role,
		&user.Banned,
		&password,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find user by id: %w", err)
	}

	if lastName.Valid {
		user.LastName = &lastName.String
	}
	if password.Valid {
		user.Password = &password.String
	}

	return user, nil
}

// FindAll finds all users with pagination
func (r *userRepository) FindAll(ctx context.Context, limit, offset int) ([]*domain.User, error) {
	query := `
		SELECT id, telegram_id, first_name, last_name, phone_number, referal_code, role, banned, password, created_at, updated_at
		FROM users
		WHERE is_bot = false
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := r.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to find users: %w", err)
	}
	defer rows.Close()

	var users []*domain.User
	for rows.Next() {
		user := &domain.User{}
		var lastName sql.NullString
		var password sql.NullString

		err := rows.Scan(
			&user.ID,
			&user.TelegramID,
			&user.FirstName,
			&lastName,
			&user.PhoneNumber,
			&user.ReferalCode,
			&user.Role,
			&user.Banned,
			&password,
			&user.CreatedAt,
			&user.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan user: %w", err)
		}

		if lastName.Valid {
			user.LastName = &lastName.String
		}
		// Never expose password in list responses
		user.Password = nil

		users = append(users, user)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate users: %w", err)
	}

	return users, nil
}

// SetAdminCredentialsByTelegramID promotes a user to admin and stores password hash.
func (r *userRepository) SetAdminCredentialsByTelegramID(ctx context.Context, telegramID int64, hashedPassword string) error {
	query := `
		UPDATE users
		SET role = 'admin', password = $2, updated_at = $3
		WHERE telegram_id = $1
	`

	result, err := r.db.ExecContext(ctx, query, telegramID, hashedPassword, time.Now())
	if err != nil {
		return fmt.Errorf("failed to set admin credentials: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("user not found")
	}

	return nil
}

// Update updates an existing user
func (r *userRepository) Update(ctx context.Context, user *domain.User) error {
	query := `
		UPDATE users
		SET first_name = $2, last_name = $3, phone_number = $4, updated_at = $5
		WHERE id = $1
	`

	user.UpdatedAt = time.Now()

	result, err := r.db.ExecContext(
		ctx,
		query,
		user.ID,
		user.FirstName,
		user.LastName,
		user.PhoneNumber,
		user.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("user not found")
	}

	return nil
}

// UpdateRole sets a user's role (e.g. promote to admin / demote to user).
func (r *userRepository) UpdateRole(ctx context.Context, id uuid.UUID, role string) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE users SET role = $2, updated_at = $3 WHERE id = $1`,
		id, role, time.Now())
	if err != nil {
		return fmt.Errorf("failed to update role: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("user not found")
	}
	return nil
}

// SetAdminCredentialsByID promotes a user to admin and sets their password hash
// (by user ID). Used by the admin dashboard's "make admin" action.
func (r *userRepository) SetAdminCredentialsByID(ctx context.Context, id uuid.UUID, hashedPassword string) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE users SET role = 'admin', password = $2, updated_at = $3 WHERE id = $1`,
		id, hashedPassword, time.Now())
	if err != nil {
		return fmt.Errorf("failed to set admin credentials: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("user not found")
	}
	return nil
}

// SetBanned bans or unbans a user.
func (r *userRepository) SetBanned(ctx context.Context, id uuid.UUID, banned bool) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE users SET banned = $2, updated_at = $3 WHERE id = $1`,
		id, banned, time.Now())
	if err != nil {
		return fmt.Errorf("failed to update banned: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("user not found")
	}
	return nil
}

// Delete permanently removes a user. Foreign keys with ON DELETE CASCADE
// (wallets, transactions, game_players) remove the user's attached rows, and
// games.winner_id is set NULL — so no orphan rows are left behind.
func (r *userRepository) Delete(ctx context.Context, id uuid.UUID) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("user not found")
	}
	return nil
}

// CountAll counts all real users (filler bots are excluded so dashboard counts
// reflect actual accounts, not house-controlled players).
func (r *userRepository) CountAll(ctx context.Context) (int, error) {
	query := `SELECT COUNT(*) FROM users WHERE is_bot = false`

	var count int
	err := r.db.QueryRowContext(ctx, query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count users: %w", err)
	}

	return count, nil
}
