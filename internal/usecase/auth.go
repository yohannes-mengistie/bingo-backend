package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/bingo/backend/internal/domain"
	"github.com/bingo/backend/pkg/auth"
	"github.com/bingo/backend/pkg/jwt"
)

type AuthUseCase struct {
	userRepo   domain.UserRepository
	jwtService *jwt.Service
}

// NewAuthUseCase creates a new auth use case
func NewAuthUseCase(userRepo domain.UserRepository, jwtService *jwt.Service) *AuthUseCase {
	return &AuthUseCase{
		userRepo:   userRepo,
		jwtService: jwtService,
	}
}

// Login authenticates an admin user and returns a JWT token
func (uc *AuthUseCase) Login(ctx context.Context, req domain.LoginRequest) (*domain.LoginResponse, error) {
	// Find user by telegram ID
	user, err := uc.userRepo.FindByTelegramID(ctx, req.TelegramID)
	if err != nil {
		return nil, errors.New("invalid credentials")
	}

	// Check if user has admin role
	if user.Role != "admin" {
		return nil, errors.New("admin access required")
	}

	// Check if user has a password set
	if user.Password == nil || *user.Password == "" {
		return nil, errors.New("password not set for this user")
	}

	// Verify password
	if !auth.CheckPasswordHash(req.Password, *user.Password) {
		return nil, errors.New("invalid credentials")
	}

	// Generate JWT token
	token, err := uc.jwtService.GenerateToken(user.ID, user.Role)
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	// Clear password from response
	user.Password = nil

	return &domain.LoginResponse{
		Token: token,
		User:  user,
	}, nil
}

// CreateAdmin promotes an existing user to admin and sets admin password.
func (uc *AuthUseCase) CreateAdmin(ctx context.Context, req domain.CreateAdminRequest) (*domain.User, error) {
	_, err := uc.userRepo.FindByTelegramID(ctx, req.TelegramID)
	if err != nil {
		return nil, errors.New("user not found")
	}

	hashedPassword, err := auth.HashPassword(req.Password)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	if err := uc.userRepo.SetAdminCredentialsByTelegramID(ctx, req.TelegramID, hashedPassword); err != nil {
		return nil, err
	}

	updatedUser, err := uc.userRepo.FindByTelegramID(ctx, req.TelegramID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch updated user: %w", err)
	}

	updatedUser.Password = nil
	return updatedUser, nil
}
