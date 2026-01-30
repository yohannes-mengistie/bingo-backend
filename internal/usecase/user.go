package usecase

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/bingo/backend/internal/domain"
	"github.com/bingo/backend/pkg/referral"
	"github.com/bingo/backend/pkg/utils"
	"github.com/google/uuid"
)

type UserUseCase struct {
	userRepo   domain.UserRepository
	walletRepo domain.WalletRepository
	db         *sql.DB
}

// NewUserUseCase creates a new user use case
func NewUserUseCase(userRepo domain.UserRepository, walletRepo domain.WalletRepository, db *sql.DB) *UserUseCase {
	return &UserUseCase{
		userRepo:   userRepo,
		walletRepo: walletRepo,
		db:         db,
	}
}

// CreateUser creates a new user and wallet together in a transaction
func (uc *UserUseCase) CreateUser(ctx context.Context, req domain.CreateUserRequest) (*domain.User, *domain.Wallet, error) {
	// Normalize phone number
	normalizedPhone := utils.NormalizePhoneNumber(req.Phone)

	// Check if user with this telegram ID already exists
	existingUser, err := uc.userRepo.FindByTelegramID(ctx, req.TelegramID)
	if err == nil && existingUser != nil {
		return nil, nil, errors.New("user with this telegram ID already exists")
	}

	// Check if user with this phone already exists
	existingUserByPhone, err := uc.userRepo.FindByPhone(ctx, normalizedPhone)
	if err == nil && existingUserByPhone != nil {
		return nil, nil, errors.New("user with this phone number already exists")
	}

	// Generate unique referral code
	var referralCode string
	maxAttempts := domain.MaxReferralCodeGenerationAttempts
	for i := 0; i < maxAttempts; i++ {
		code, err := referral.GenerateReferralCode()
		if err != nil {
			return nil, nil, fmt.Errorf("failed to generate referral code: %w", err)
		}

		// Check if referral code already exists
		_, err = uc.userRepo.FindByReferralCode(ctx, code)
		if err != nil {
			// Code doesn't exist, we can use it
			referralCode = code
			break
		}

		if i == maxAttempts-1 {
			return nil, nil, errors.New("failed to generate unique referral code after multiple attempts")
		}
	}

	// Start transaction
	tx, err := uc.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Create user
	user := &domain.User{
		TelegramID:  req.TelegramID,
		FirstName:   req.FirstName,
		LastName:    req.LastName,
		PhoneNumber: normalizedPhone,
		ReferalCode: referralCode,
	}

	if err := uc.userRepo.Create(ctx, tx, user); err != nil {
		return nil, nil, fmt.Errorf("failed to create user: %w", err)
	}

	// Create wallet with default balance
	wallet := &domain.Wallet{
		UserID:      user.ID,
		Balance:     domain.DefaultUserBalance,
		DemoBalance: 0.0,
	}

	if err := uc.walletRepo.Create(ctx, tx, wallet); err != nil {
		return nil, nil, fmt.Errorf("failed to create wallet: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return nil, nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return user, wallet, nil
}

// FindUserByTelegramID finds a user by their Telegram ID
func (uc *UserUseCase) FindUserByTelegramID(ctx context.Context, telegramID int64) (*domain.User, error) {
	user, err := uc.userRepo.FindByTelegramID(ctx, telegramID)
	if err != nil {
		return nil, err
	}
	return user, nil
}

// FindUserByPhone finds a user by their phone number (normalizes the phone first)
func (uc *UserUseCase) FindUserByPhone(ctx context.Context, phone string) (*domain.User, error) {
	normalizedPhone := utils.NormalizePhoneNumber(phone)
	user, err := uc.userRepo.FindByPhone(ctx, normalizedPhone)
	if err != nil {
		return nil, err
	}
	return user, nil
}

// FindUserByReferralCode finds a user by their referral code
func (uc *UserUseCase) FindUserByReferralCode(ctx context.Context, referralCode string) (*domain.User, error) {
	user, err := uc.userRepo.FindByReferralCode(ctx, referralCode)
	if err != nil {
		return nil, err
	}
	return user, nil
}

// GetAllUsers returns all users with pagination (for admin)
func (uc *UserUseCase) GetAllUsers(ctx context.Context, limit, offset int) ([]*domain.User, error) {
	return uc.userRepo.FindAll(ctx, limit, offset)
}

// UpdateUserName updates a user's first and last name
func (uc *UserUseCase) UpdateUserName(ctx context.Context, userID uuid.UUID, req domain.UpdateUserNameRequest) (*domain.User, error) {
	// Find the user first
	user, err := uc.userRepo.FindByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	// Update the name fields
	user.FirstName = req.FirstName
	user.LastName = req.LastName

	// Update in database
	if err := uc.userRepo.Update(ctx, user); err != nil {
		return nil, fmt.Errorf("failed to update user: %w", err)
	}

	return user, nil
}
