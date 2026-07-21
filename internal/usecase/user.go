package usecase

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/bingo/backend/internal/domain"
	"github.com/bingo/backend/pkg/auth"
	"github.com/bingo/backend/pkg/referral"
	"github.com/bingo/backend/pkg/utils"
	"github.com/google/uuid"
)

type UserUseCase struct {
	userRepo        domain.UserRepository
	walletRepo      domain.WalletRepository
	transactionRepo domain.TransactionRepository
	db              *sql.DB
	// referralNotifier tells a referrer on Telegram that they earned a reward.
	// Optional and set after construction (the bot is built later); nil just
	// means the referrer isn't messaged — they still see the money.
	referralNotifier domain.BroadcastSender
}

// NewUserUseCase creates a new user use case
func NewUserUseCase(userRepo domain.UserRepository, walletRepo domain.WalletRepository, transactionRepo domain.TransactionRepository, db *sql.DB) *UserUseCase {
	return &UserUseCase{
		userRepo:        userRepo,
		walletRepo:      walletRepo,
		transactionRepo: transactionRepo,
		db:              db,
	}
}

// SetReferralNotifier wires the Telegram sender used to congratulate a referrer
// when their reward lands. Called from main after the bot is constructed.
func (uc *UserUseCase) SetReferralNotifier(n domain.BroadcastSender) {
	uc.referralNotifier = n
}

// CreateUser creates a new user and wallet together in a transaction
func (uc *UserUseCase) CreateUser(ctx context.Context, req domain.CreateUserRequest) (*domain.User, *domain.Wallet, error) {
	// Store the phone in the canonical 251XXXXXXXXX form. This must match what
	// Login and the withdrawal payout path use (CanonicalEthiopianPhone), or a
	// number registered as 0911... is stored as 911... and never matches the
	// 251911... a login looks up — the account becomes unreachable by phone and
	// the duplicate check silently misses it. Validation is centralized here
	// rather than left to each caller: the Telegram handler already checked,
	// but the plain HTTP registration endpoint did not.
	if !utils.IsEthiopianMobile(req.Phone) {
		return nil, nil, errors.New("phone must be a valid Ethiopian mobile number")
	}
	normalizedPhone := utils.CanonicalEthiopianPhone(req.Phone)

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

	// Resolve the invite link's referral code to a referrer, if one came in.
	// Best-effort: an unknown/blank code just means no referrer.
	var referredBy *uuid.UUID
	var referrer *domain.User
	if code := strings.TrimSpace(req.ReferrerCode); code != "" {
		if r, rerr := uc.userRepo.FindByReferralCode(ctx, code); rerr == nil && r != nil {
			// Self-referral guard: you cannot refer yourself. Reject a code that
			// resolves to the same person who is registering (same Telegram ID or
			// same phone), so nobody can pay themselves the reward with their own
			// link. A genuinely different account is a real referral.
			if r.TelegramID == req.TelegramID || r.PhoneNumber == normalizedPhone {
				log.Printf("[referral] ignoring self-referral by tg_id=%d", req.TelegramID)
			} else {
				referredBy = &r.ID
				referrer = r
			}
		}
	}

	// Create user. If they were referred, the referrer is paid right below in
	// this same transaction and referral_rewarded is flipped there.
	user := &domain.User{
		TelegramID:  req.TelegramID,
		FirstName:   req.FirstName,
		LastName:    req.LastName,
		PhoneNumber: normalizedPhone,
		ReferalCode: referralCode,
		ReferredBy:  referredBy,
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

	// Pay the referrer their reward NOW, at signup — no longer gated on a first
	// deposit. Done in the same transaction as the signup so the credit and the
	// account are created atomically (never a referred user without the reward,
	// nor a reward without the user). referral_rewarded is flipped so nothing can
	// ever pay a second time for this user.
	if referredBy != nil {
		if _, err := uc.walletRepo.LockForUpdate(ctx, tx, *referredBy); err != nil {
			return nil, nil, fmt.Errorf("referrer wallet not found: %w", err)
		}
		if err := uc.walletRepo.UpdateBalance(ctx, tx, *referredBy, domain.ReferralRewardAmount); err != nil {
			return nil, nil, fmt.Errorf("failed to credit referrer: %w", err)
		}
		note := "Referral reward"
		reward := &domain.Transaction{
			UserID:    *referredBy,
			Type:      domain.TransactionTypeDeposit,
			Category:  domain.TransactionCategoryReferralReward,
			Amount:    domain.ReferralRewardAmount,
			Status:    domain.TransactionStatusCompleted,
			Reference: &note,
		}
		if err := uc.transactionRepo.Create(ctx, tx, reward); err != nil {
			return nil, nil, fmt.Errorf("failed to record referral reward: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE users SET referral_rewarded = true WHERE id = $1`, user.ID); err != nil {
			return nil, nil, fmt.Errorf("failed to mark referral rewarded: %w", err)
		}
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return nil, nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Congratulate the referrer on Telegram (best-effort, after commit — the
	// money is already theirs whether or not the message goes through).
	if referrer != nil && uc.referralNotifier != nil && referrer.TelegramID > 0 {
		msg := fmt.Sprintf(
			"🎉 %0.f ብር የግብዣ ሽልማት አግኝተዋል!\nየጋበዙት ሰው አካውንት ከፍቷል።\n\n"+
				"You earned a %0.f birr referral reward — someone you invited just signed up! 💰",
			domain.ReferralRewardAmount, domain.ReferralRewardAmount)
		if serr := uc.referralNotifier.SendMessage(referrer.TelegramID, msg); serr != nil {
			log.Printf("[referral] rewarded %s but the Telegram notice failed: %v", referrer.ID, serr)
		}
	}

	return user, wallet, nil
}

// GetUserByID returns a user by their ID (password stripped)
func (uc *UserUseCase) GetUserByID(ctx context.Context, userID uuid.UUID) (*domain.User, error) {
	user, err := uc.userRepo.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	user.Password = nil
	return user, nil
}

// FindUserByTelegramID finds a user by their Telegram ID
func (uc *UserUseCase) FindUserByTelegramID(ctx context.Context, telegramID int64) (*domain.User, error) {
	user, err := uc.userRepo.FindByTelegramID(ctx, telegramID)
	if err != nil {
		return nil, err
	}
	return user, nil
}

// GetUserDetail returns a user together with their wallet (for admin detail view).
func (uc *UserUseCase) GetUserDetail(ctx context.Context, userID uuid.UUID) (*domain.UserWithWallet, error) {
	user, err := uc.userRepo.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	user.Password = nil

	uw := &domain.UserWithWallet{User: user}
	if wallet, err := uc.walletRepo.FindByUserID(ctx, userID); err == nil && wallet != nil {
		uw.Wallet = wallet
	}
	return uw, nil
}

// SetUserRole changes a user's role (admin action). Role is validated by the
// handler's request binding (oneof=user admin).
func (uc *UserUseCase) SetUserRole(ctx context.Context, userID uuid.UUID, role string) error {
	return uc.userRepo.UpdateRole(ctx, userID, role)
}

// SetUserBanned bans or unbans a user (admin action).
func (uc *UserUseCase) SetUserBanned(ctx context.Context, userID uuid.UUID, banned bool) error {
	return uc.userRepo.SetBanned(ctx, userID, banned)
}

// DeleteUser permanently removes a user and (via FK cascade) their wallet,
// transactions, and game participation (admin action). Admin accounts are
// refused as a foot-gun guard — demote to a regular user first if you really
// mean to remove one.
func (uc *UserUseCase) DeleteUser(ctx context.Context, userID uuid.UUID) error {
	user, err := uc.userRepo.FindByID(ctx, userID)
	if err != nil {
		return errors.New("user not found")
	}
	if user.Role == "admin" {
		return errors.New("cannot delete an admin account; demote it first")
	}
	return uc.userRepo.Delete(ctx, userID)
}

// MakeAdmin promotes a user to admin and sets their dashboard password
// (admin action). The password is hashed before storage.
func (uc *UserUseCase) MakeAdmin(ctx context.Context, userID uuid.UUID, password string) error {
	if len(password) < 8 {
		return errors.New("password must be at least 8 characters")
	}
	hashed, err := auth.HashPassword(password)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}
	return uc.userRepo.SetAdminCredentialsByID(ctx, userID, hashed)
}

// FindUserByPhone finds a user by their phone number, canonicalizing it first
// so any accepted input shape (0911..., 251911..., +251 911..., 911...)
// resolves to the single stored form.
func (uc *UserUseCase) FindUserByPhone(ctx context.Context, phone string) (*domain.User, error) {
	normalizedPhone := utils.CanonicalEthiopianPhone(phone)
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

// GetAllUsersWithWallets returns all users with their wallets (for admin)
func (uc *UserUseCase) GetAllUsersWithWallets(ctx context.Context, limit, offset int) ([]*domain.UserWithWallet, int, error) {
	// Get users
	users, err := uc.userRepo.FindAll(ctx, limit, offset)
	if err != nil {
		return nil, 0, err
	}

	// Get total count
	totalCount, err := uc.userRepo.CountAll(ctx)
	if err != nil {
		return nil, 0, err
	}

	// Fetch wallets for each user
	usersWithWallets := make([]*domain.UserWithWallet, 0, len(users))
	for _, user := range users {
		uw := &domain.UserWithWallet{
			User: user,
		}

		// Try to fetch wallet (may not exist for some users)
		wallet, err := uc.walletRepo.FindByUserID(ctx, user.ID)
		if err == nil && wallet != nil {
			uw.Wallet = wallet
		}

		usersWithWallets = append(usersWithWallets, uw)
	}

	return usersWithWallets, totalCount, nil
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
