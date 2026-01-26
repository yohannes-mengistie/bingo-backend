package usecase

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/bingo/backend/internal/domain"
	"github.com/bingo/backend/pkg/bingo"
	redisGame "github.com/bingo/backend/pkg/redis"
	"github.com/google/uuid"
)

type GameUseCase struct {
	gameRepo        domain.GameRepository
	walletRepo      domain.WalletRepository
	transactionRepo domain.TransactionRepository
	userRepo        domain.UserRepository
	db              *sql.DB
	redisService    *redisGame.GameStateService
}

// NewGameUseCase creates a new game use case
func NewGameUseCase(
	gameRepo domain.GameRepository,
	walletRepo domain.WalletRepository,
	transactionRepo domain.TransactionRepository,
	userRepo domain.UserRepository,
	db *sql.DB,
	redisService *redisGame.GameStateService,
) *GameUseCase {
	return &GameUseCase{
		gameRepo:        gameRepo,
		walletRepo:      walletRepo,
		transactionRepo: transactionRepo,
		userRepo:        userRepo,
		db:              db,
		redisService:    redisService,
	}
}

// GetAvailableGames gets available games (WAITING or COUNTDOWN state)
func (uc *GameUseCase) GetAvailableGames(ctx context.Context, gameType *domain.GameType) ([]*domain.Game, error) {
	games, err := uc.gameRepo.FindAvailable(ctx, gameType, 50)
	if err != nil {
		return nil, fmt.Errorf("failed to find available games: %w", err)
	}

	return games, nil
}

// CreateOrGetGame creates a new game or returns an existing available game
func (uc *GameUseCase) CreateOrGetGame(ctx context.Context, gameType domain.GameType) (*domain.Game, error) {
	// Try to find an available game first
	games, err := uc.gameRepo.FindAvailable(ctx, &gameType, 1)
	if err == nil && len(games) > 0 {
		// Return existing game
		return games[0], nil
	}

	// Create new game
	game := &domain.Game{
		ID:          uuid.New(),
		GameType:    gameType,
		State:       domain.GameStateWaiting,
		BetAmount:   gameType.GetBetAmount(),
		MinPlayers:  2,
		PlayerCount: 0,
		PrizePool:   0,
		HouseCut:    0.05, // 5% house cut
	}

	if err := uc.gameRepo.Create(ctx, game); err != nil {
		return nil, fmt.Errorf("failed to create game: %w", err)
	}

	// Save to Redis (if available)
	if uc.redisService != nil {
		if err := uc.redisService.SaveGameState(ctx, game); err != nil {
			// Log error but don't fail
			fmt.Printf("Warning: failed to save game state to Redis: %v\n", err)
		}
	}

	return game, nil
}

// JoinGame allows a user to join a game
func (uc *GameUseCase) JoinGame(ctx context.Context, gameID uuid.UUID, req domain.JoinGameRequest) (*domain.GamePlayer, error) {
	// Validate card ID
	if req.CardID < 1 || req.CardID > 100 {
		return nil, errors.New("card ID must be between 1 and 100")
	}

	// Get game
	game, err := uc.gameRepo.FindByID(ctx, gameID)
	if err != nil {
		return nil, fmt.Errorf("game not found: %w", err)
	}

	// Check game state
	if game.State != domain.GameStateWaiting {
		return nil, errors.New("game is not accepting new players")
	}

	// Check if user is already in the game
	existingPlayer, _ := uc.gameRepo.FindPlayer(ctx, gameID, req.UserID)
	if existingPlayer != nil {
		return nil, errors.New("user is already in this game")
	}

	// Check if card is already taken
	takenCards, err := uc.gameRepo.GetTakenCards(ctx, gameID)
	if err != nil {
		return nil, fmt.Errorf("failed to get taken cards: %w", err)
	}

	for _, takenCardID := range takenCards {
		if takenCardID == req.CardID {
			return nil, errors.New("card is already taken")
		}
	}

	// Start transaction
	tx, err := uc.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Lock wallet
	wallet, err := uc.walletRepo.LockForUpdate(ctx, tx, req.UserID)
	if err != nil {
		return nil, fmt.Errorf("wallet not found: %w", err)
	}

	// Check balance
	if wallet.Balance < game.BetAmount {
		return nil, errors.New("insufficient balance")
	}

	// Deduct bet amount
	if err := uc.walletRepo.UpdateBalance(ctx, tx, req.UserID, -game.BetAmount); err != nil {
		return nil, fmt.Errorf("failed to deduct bet: %w", err)
	}

	// Create transaction record
	transaction := &domain.Transaction{
		UserID: req.UserID,
		Type:   domain.TransactionTypeWithdraw, // Bet is treated as withdrawal
		Amount: game.BetAmount,
		Status: domain.TransactionStatusCompleted, // Bet is immediately deducted
	}

	if err := uc.transactionRepo.Create(ctx, tx, transaction); err != nil {
		return nil, fmt.Errorf("failed to create transaction: %w", err)
	}

	// Add player
	player := &domain.GamePlayer{
		ID:           uuid.New(),
		GameID:       gameID,
		UserID:       req.UserID,
		CardID:       req.CardID,
		IsEliminated: false,
	}

	if err := uc.gameRepo.AddPlayer(ctx, tx, player); err != nil {
		return nil, fmt.Errorf("failed to add player: %w", err)
	}

	// Update game player count and prize pool
	game.PlayerCount++
	game.PrizePool += game.BetAmount * (1 - game.HouseCut) // Prize pool excludes house cut

	if err := uc.gameRepo.Update(ctx, game); err != nil {
		return nil, fmt.Errorf("failed to update game: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Update Redis
	uc.redisService.AddPlayer(ctx, gameID, req.UserID)
	uc.redisService.AddTakenCard(ctx, gameID, req.CardID)
	uc.redisService.SaveGameState(ctx, game)

	// If this is the 2nd player, start countdown
	if game.PlayerCount == 2 {
		go uc.startCountdown(context.Background(), gameID)
	}

	// Publish event
	uc.redisService.PublishEvent(ctx, gameID, "PLAYER_JOINED", map[string]interface{}{
		"user_id": req.UserID.String(),
		"card_id": req.CardID,
	})

	return player, nil
}

// LeaveGame allows a user to leave a game
func (uc *GameUseCase) LeaveGame(ctx context.Context, gameID uuid.UUID, req domain.LeaveGameRequest) error {
	// Get game
	game, err := uc.gameRepo.FindByID(ctx, gameID)
	if err != nil {
		return fmt.Errorf("game not found: %w", err)
	}

	// Check if user is in the game
	_, err = uc.gameRepo.FindPlayer(ctx, gameID, req.UserID)
	if err != nil {
		return errors.New("user is not in this game")
	}

	// Check game state
	if game.State == domain.GameStateDrawing {
		return errors.New("cannot leave during drawing phase")
	}

	// Start transaction
	tx, err := uc.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Remove player
	if err := uc.gameRepo.RemovePlayer(ctx, tx, gameID, req.UserID); err != nil {
		return fmt.Errorf("failed to remove player: %w", err)
	}

	// Refund bet (only if in WAITING or COUNTDOWN)
	if game.State == domain.GameStateWaiting || game.State == domain.GameStateCountdown {
		// Lock wallet
		_, err := uc.walletRepo.LockForUpdate(ctx, tx, req.UserID)
		if err != nil {
			return fmt.Errorf("wallet not found: %w", err)
		}

		// Refund
		if err := uc.walletRepo.UpdateBalance(ctx, tx, req.UserID, game.BetAmount); err != nil {
			return fmt.Errorf("failed to refund: %w", err)
		}

		// Create refund transaction
		transaction := &domain.Transaction{
			UserID: req.UserID,
			Type:   domain.TransactionTypeDeposit, // Refund is treated as deposit
			Amount: game.BetAmount,
			Status: domain.TransactionStatusCompleted,
		}

		if err := uc.transactionRepo.Create(ctx, tx, transaction); err != nil {
			return fmt.Errorf("failed to create refund transaction: %w", err)
		}

		// Update game prize pool
		game.PrizePool -= game.BetAmount * (1 - game.HouseCut)
	}

	// Update game player count
	game.PlayerCount--

	// If players drop below 2 during countdown, cancel game
	if game.State == domain.GameStateCountdown && game.PlayerCount < 2 {
		game.State = domain.GameStateCancelled
		// Refund all remaining players
		players, _ := uc.gameRepo.GetPlayers(ctx, gameID)
		for _, p := range players {
			_, err := uc.walletRepo.LockForUpdate(ctx, tx, p.UserID)
			if err == nil {
				uc.walletRepo.UpdateBalance(ctx, tx, p.UserID, game.BetAmount)
			}
		}
	}

	if err := uc.gameRepo.Update(ctx, game); err != nil {
		return fmt.Errorf("failed to update game: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Update Redis
	uc.redisService.RemovePlayer(ctx, gameID, req.UserID)
	uc.redisService.SaveGameState(ctx, game)

	// Publish event
	uc.redisService.PublishEvent(ctx, gameID, "PLAYER_LEFT", map[string]interface{}{
		"user_id": req.UserID.String(),
	})

	return nil
}

// startCountdown starts the countdown for a game
func (uc *GameUseCase) startCountdown(ctx context.Context, gameID uuid.UUID) {
	game, err := uc.gameRepo.FindByID(ctx, gameID)
	if err != nil || game.State != domain.GameStateWaiting {
		return
	}

	// Update game state to COUNTDOWN
	game.State = domain.GameStateCountdown
	countdownEnds := time.Now().Add(60 * time.Second)
	game.CountdownEnds = &countdownEnds

	if err := uc.gameRepo.Update(ctx, game); err != nil {
		return
	}

	uc.redisService.SetCountdown(ctx, gameID, countdownEnds)
	uc.redisService.SaveGameState(ctx, game)

	// Publish countdown start
	uc.redisService.PublishEvent(ctx, gameID, "GAME_STATUS", map[string]interface{}{
		"status":      "COUNTDOWN",
		"secondsLeft": 60,
	})

	// Countdown ticker
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for i := 60; i > 0; i-- {
		<-ticker.C

		// Check if game still exists and is in countdown
		game, err := uc.gameRepo.FindByID(ctx, gameID)
		if err != nil || game.State != domain.GameStateCountdown {
			return
		}

		// Check if players dropped below 2
		if game.PlayerCount < 2 {
			// Cancel game
			game.State = domain.GameStateCancelled
			uc.gameRepo.Update(ctx, game)
			uc.redisService.PublishEvent(ctx, gameID, "GAME_CANCELLED", map[string]interface{}{})
			return
		}

		// Publish countdown update
		uc.redisService.PublishEvent(ctx, gameID, "COUNTDOWN", map[string]interface{}{
			"secondsLeft": i - 1,
		})
	}

	// Start drawing phase
	uc.startDrawing(ctx, gameID)
}

// startDrawing starts the drawing phase
func (uc *GameUseCase) startDrawing(ctx context.Context, gameID uuid.UUID) {
	game, err := uc.gameRepo.FindByID(ctx, gameID)
	if err != nil || game.State != domain.GameStateCountdown {
		return
	}

	game.State = domain.GameStateDrawing
	now := time.Now()
	game.StartedAt = &now

	if err := uc.gameRepo.Update(ctx, game); err != nil {
		return
	}

	uc.redisService.SaveGameState(ctx, game)

	uc.redisService.PublishEvent(ctx, gameID, "GAME_STATUS", map[string]interface{}{
		"status": "DRAWING",
	})

	// Start drawing numbers periodically
	go uc.drawNumbers(ctx, gameID)
}

// drawNumbers draws numbers periodically
func (uc *GameUseCase) drawNumbers(ctx context.Context, gameID uuid.UUID) {
	ticker := time.NewTicker(5 * time.Second) // Draw every 5 seconds
	defer ticker.Stop()

	for {
		<-ticker.C

		// Check if game is still in drawing state
		game, err := uc.gameRepo.FindByID(ctx, gameID)
		if err != nil || game.State != domain.GameStateDrawing {
			return
		}

		// Get drawn numbers
		drawnNumbers, _ := uc.redisService.GetDrawnNumbers(ctx, gameID)
		numbers := make([]int, len(drawnNumbers))
		for i, dn := range drawnNumbers {
			numbers[i] = dn.Number
		}

		// Draw next number
		letter, number, err := bingo.DrawNextNumber(numbers)
		if err != nil || number == 0 {
			// All numbers drawn or error
			continue
		}

		// Save drawn number
		drawnNumber := domain.DrawnNumber{
			Letter:  domain.BingoLetter(letter),
			Number:  number,
			DrawnAt: time.Now(),
		}

		uc.redisService.AddDrawnNumber(ctx, gameID, drawnNumber)
		uc.gameRepo.SaveDrawnNumber(ctx, gameID, letter, number)

		// Publish event
		uc.redisService.PublishEvent(ctx, gameID, "NUMBER_DRAWN", map[string]interface{}{
			"letter": letter,
			"number": number,
		})
	}
}

// ClaimBingo validates and processes a bingo claim
func (uc *GameUseCase) ClaimBingo(ctx context.Context, gameID uuid.UUID, req domain.ClaimBingoRequest) (bool, error) {
	// Get game
	game, err := uc.gameRepo.FindByID(ctx, gameID)
	if err != nil {
		return false, fmt.Errorf("game not found: %w", err)
	}

	// Check game state
	if game.State != domain.GameStateDrawing {
		return false, errors.New("game is not in drawing phase")
	}

	// Check if user is in the game
	player, err := uc.gameRepo.FindPlayer(ctx, gameID, req.UserID)
	if err != nil {
		return false, errors.New("user is not in this game")
	}

	if player.IsEliminated {
		return false, errors.New("player is already eliminated")
	}

	// Get drawn numbers
	drawnNumbers, err := uc.redisService.GetDrawnNumbers(ctx, gameID)
	if err != nil {
		return false, fmt.Errorf("failed to get drawn numbers: %w", err)
	}

	// Convert to list of numbers
	drawn := make([]int, len(drawnNumbers))
	for i, dn := range drawnNumbers {
		drawn[i] = dn.Number
	}

	// Generate card
	card := bingo.GenerateCard(player.CardID)

	// Validate bingo
	isValid := bingo.ValidateBingo(card, drawn)

	// Start transaction
	tx, err := uc.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	if isValid {
		// Winner!
		game.State = domain.GameStateFinished
		game.WinnerID = &req.UserID
		now := time.Now()
		game.FinishedAt = &now

		// Distribute prize
		_, err := uc.walletRepo.LockForUpdate(ctx, tx, req.UserID)
		if err != nil {
			return false, fmt.Errorf("wallet not found: %w", err)
		}

		// Add prize to winner's wallet
		if err := uc.walletRepo.UpdateBalance(ctx, tx, req.UserID, game.PrizePool); err != nil {
			return false, fmt.Errorf("failed to add prize: %w", err)
		}

		// Create transaction
		transaction := &domain.Transaction{
			UserID: req.UserID,
			Type:   domain.TransactionTypeDeposit,
			Amount: game.PrizePool,
			Status: domain.TransactionStatusCompleted,
		}

		if err := uc.transactionRepo.Create(ctx, tx, transaction); err != nil {
			return false, fmt.Errorf("failed to create transaction: %w", err)
		}

		if err := uc.gameRepo.Update(ctx, game); err != nil {
			return false, fmt.Errorf("failed to update game: %w", err)
		}

		// Commit transaction
		if err := tx.Commit(); err != nil {
			return false, fmt.Errorf("failed to commit transaction: %w", err)
		}

		// Update Redis
		uc.redisService.SaveGameState(ctx, game)

		// Publish winner event
		uc.redisService.PublishEvent(ctx, gameID, "WINNER", map[string]interface{}{
			"userId": req.UserID.String(),
			"prize":  game.PrizePool,
		})

		return true, nil
	} else {
		// Invalid claim - eliminate player
		if err := uc.gameRepo.EliminatePlayer(ctx, tx, gameID, req.UserID); err != nil {
			return false, fmt.Errorf("failed to eliminate player: %w", err)
		}

		// Check if all players are eliminated
		players, _ := uc.gameRepo.GetPlayers(ctx, gameID)
		activePlayers := 0
		for _, p := range players {
			if !p.IsEliminated {
				activePlayers++
			}
		}

		if activePlayers == 0 {
			// All players eliminated - cancel game and refund
			game.State = domain.GameStateCancelled
			for _, p := range players {
				_, err := uc.walletRepo.LockForUpdate(ctx, tx, p.UserID)
				if err == nil {
					uc.walletRepo.UpdateBalance(ctx, tx, p.UserID, game.BetAmount)
				}
			}
		}

		if err := uc.gameRepo.Update(ctx, game); err != nil {
			return false, fmt.Errorf("failed to update game: %w", err)
		}

		// Commit transaction
		if err := tx.Commit(); err != nil {
			return false, fmt.Errorf("failed to commit transaction: %w", err)
		}

		// Update Redis
		uc.redisService.SaveGameState(ctx, game)

		// Publish elimination event
		uc.redisService.PublishEvent(ctx, gameID, "PLAYER_ELIMINATED", map[string]interface{}{
			"userId": req.UserID.String(),
		})

		return false, nil
	}
}

// GetGameState gets the current game state
func (uc *GameUseCase) GetGameState(ctx context.Context, gameID uuid.UUID) (*domain.Game, []domain.DrawnNumber, []int, error) {
	game, err := uc.gameRepo.FindByID(ctx, gameID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("game not found: %w", err)
	}

	var drawnNumbers []domain.DrawnNumber
	var takenCards []int

	if uc.redisService != nil {
		drawnNumbers, _ = uc.redisService.GetDrawnNumbers(ctx, gameID)
		takenCards, _ = uc.redisService.GetTakenCards(ctx, gameID)
	} else {
		// Fallback to database
		takenCards, _ = uc.gameRepo.GetTakenCards(ctx, gameID)
		// drawnNumbers would need DB implementation
	}

	return game, drawnNumbers, takenCards, nil
}

// GetPlayerInGame checks if a player is in a game (for WebSocket validation)
func (uc *GameUseCase) GetPlayerInGame(ctx context.Context, gameID, userID uuid.UUID) (*domain.GamePlayer, error) {
	return uc.gameRepo.FindPlayer(ctx, gameID, userID)
}

// GetCardData returns the card data for a given card ID
func (uc *GameUseCase) GetCardData(ctx context.Context, cardID int) (*bingo.BingoCard, error) {
	// Validate card ID
	if cardID < 1 || cardID > 100 {
		return nil, errors.New("card ID must be between 1 and 100")
	}

	// Generate card (deterministic - same card_id always generates same card)
	card := bingo.GenerateCard(cardID)
	return card, nil
}
