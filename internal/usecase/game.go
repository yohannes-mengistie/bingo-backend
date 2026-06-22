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
	games, err := uc.gameRepo.FindAvailable(ctx, gameType, domain.MaxAvailableGamesLimit)
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
		MinPlayers:  domain.MinPlayers,
		PlayerCount: 0,
		PrizePool:   0,
		HouseCut:    domain.HouseCut,
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
	if req.CardID < domain.MinCardID || req.CardID > domain.MaxCardID {
		return nil, fmt.Errorf("card ID must be between %d and %d", domain.MinCardID, domain.MaxCardID)
	}

	// Get game
	game, err := uc.gameRepo.FindByID(ctx, gameID)
	if err != nil {
		return nil, fmt.Errorf("game not found: %w", err)
	}

	// Idempotent reconnect: if the user is already an active player in this
	// game, return their existing record instead of an error. This lets a
	// player who dropped off (closed the app / lost their WebSocket) tap their
	// already-selected card and get back in — even after drawing has started.
	// Their card cannot change, so we ignore req.CardID and return the truth.
	existingPlayer, _ := uc.gameRepo.FindPlayer(ctx, gameID, req.UserID)
	if existingPlayer != nil {
		return existingPlayer, nil
	}

	// Check game state - allow new players to join only in WAITING or COUNTDOWN
	if game.State != domain.GameStateWaiting && game.State != domain.GameStateCountdown {
		return nil, errors.New("game is not accepting new players")
	}

	// Enforce one card per player per game: reject if another active player
	// already holds this card. (The DB also has a UNIQUE(game_id, card_id)
	// constraint as a hard safety net against the rare check-then-insert race.)
	takenCards, err := uc.gameRepo.GetTakenCards(ctx, gameID)
	if err == nil {
		for _, taken := range takenCards {
			if taken == req.CardID {
				return nil, errors.New("card is already taken")
			}
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
	gameBetRef := "GAME_BET"
	transaction := &domain.Transaction{
		UserID:    req.UserID,
		Type:      domain.TransactionTypeWithdraw, // Bet is treated as withdrawal
		Amount:    game.BetAmount,
		Status:    domain.TransactionStatusCompleted, // Bet is immediately deducted
		Reference: &gameBetRef,                       // Mark as game bet to exclude from withdrawal history
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

	// If this is the minimum required player, start countdown
	if game.PlayerCount == domain.MinPlayers {
		go uc.startCountdown(context.Background(), gameID)
	}

	// Publish event
	uc.redisService.PublishEvent(ctx, gameID, domain.WebSocketEventPlayerJoined, map[string]interface{}{
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

	// Check if user is in the game and get player info (including card ID)
	player, err := uc.gameRepo.FindPlayer(ctx, gameID, req.UserID)
	if err != nil {
		return errors.New("user is not in this game")
	}
	cardID := player.CardID

	// Start transaction
	tx, err := uc.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Re-fetch game state to get latest state (prevents race condition if countdown ended)
	// Note: This happens right after transaction starts to minimize race window
	game, err = uc.gameRepo.FindByID(ctx, gameID)
	if err != nil {
		return fmt.Errorf("game not found: %w", err)
	}

	// Check if game is already finished/cancelled/closed
	if game.State == domain.GameStateFinished || game.State == domain.GameStateClosed || game.State == domain.GameStateCancelled {
		return errors.New("game is no longer active")
	}

	// Note: Players can leave during DRAWING phase, but won't get a refund

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
		gameRefundRef := "GAME_REFUND"
		transaction := &domain.Transaction{
			UserID:    req.UserID,
			Type:      domain.TransactionTypeDeposit, // Refund is treated as deposit
			Amount:    game.BetAmount,
			Status:    domain.TransactionStatusCompleted,
			Reference: &gameRefundRef, // Mark as game refund to exclude from deposit history
		}

		if err := uc.transactionRepo.Create(ctx, tx, transaction); err != nil {
			return fmt.Errorf("failed to create refund transaction: %w", err)
		}

		// Update game prize pool
		game.PrizePool -= game.BetAmount * (1 - game.HouseCut)
	}

	// Update game player count
	game.PlayerCount--

	// If players drop below minimum during countdown, revert to WAITING state
	// This allows the game to continue when more players join, rather than cancelling
	if game.State == domain.GameStateCountdown && game.PlayerCount < domain.MinPlayers {
		game.State = domain.GameStateWaiting
		game.CountdownEnds = nil // Clear countdown timestamp
		// Note: Remaining players stay in the game and will continue when minimum players join
		// The countdown will automatically restart when player count reaches MinPlayers again
	}

	if err := uc.gameRepo.Update(ctx, game); err != nil {
		return fmt.Errorf("failed to update game: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Update Redis - remove player
	uc.redisService.RemovePlayer(ctx, gameID, req.UserID)

	// Check if any other active players are using this card
	// Only remove card from taken cards if no other players have it
	// (GetPlayers already filters by left_at IS NULL, so leaving player won't be in the list)
	players, err := uc.gameRepo.GetPlayers(ctx, gameID)
	if err == nil {
		otherPlayersHaveCard := false
		for _, p := range players {
			if p.CardID == cardID {
				otherPlayersHaveCard = true
				break
			}
		}
		// Only remove card if no other active players are using it
		if !otherPlayersHaveCard {
			uc.redisService.RemoveTakenCard(ctx, gameID, cardID)
		}
	}

	// If game reverted to WAITING from COUNTDOWN, clear countdown state and notify
	revertedFromCountdown := game.State == domain.GameStateWaiting && game.CountdownEnds == nil
	if revertedFromCountdown {
		uc.redisService.ClearCountdown(ctx, gameID)
		uc.redisService.PublishEvent(ctx, gameID, domain.WebSocketEventGameStatus, map[string]interface{}{
			"status": string(domain.GameStateWaiting),
		})
	}

	uc.redisService.SaveGameState(ctx, game)

	// Publish event
	uc.redisService.PublishEvent(ctx, gameID, domain.WebSocketEventPlayerLeft, map[string]interface{}{
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
	countdownEnds := time.Now().Add(domain.CountdownDuration)
	game.CountdownEnds = &countdownEnds

	if err := uc.gameRepo.Update(ctx, game); err != nil {
		return
	}

	uc.redisService.SetCountdown(ctx, gameID, countdownEnds)
	uc.redisService.SaveGameState(ctx, game)

	// Publish countdown start
	uc.redisService.PublishEvent(ctx, gameID, domain.WebSocketEventGameStatus, map[string]interface{}{
		"status":      string(domain.GameStateCountdown),
		"secondsLeft": int(domain.CountdownDuration.Seconds()),
	})

	// Countdown ticker
	ticker := time.NewTicker(domain.CountdownTickerInterval)
	defer ticker.Stop()

	countdownSeconds := int(domain.CountdownDuration.Seconds())
	for i := countdownSeconds; i > 0; i-- {
		<-ticker.C

		// Check if game still exists and is in countdown
		game, err := uc.gameRepo.FindByID(ctx, gameID)
		if err != nil || game.State != domain.GameStateCountdown {
			return
		}

		// Check if players dropped below minimum
		// If so, the countdown ticker will exit and LeaveGame will handle reverting to WAITING
		// This check ensures we don't continue countdown with insufficient players
		if game.PlayerCount < domain.MinPlayers {
			// Exit countdown - LeaveGame will handle state transition to WAITING
			// The countdown ticker will stop, and when state changes, this goroutine exits
			return
		}

		// Publish countdown update
		uc.redisService.PublishEvent(ctx, gameID, domain.WebSocketEventCountdown, map[string]interface{}{
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

	uc.redisService.PublishEvent(ctx, gameID, domain.WebSocketEventGameStatus, map[string]interface{}{
		"status": string(domain.GameStateDrawing),
	})

	// Start drawing numbers periodically
	go uc.drawNumbers(ctx, gameID)
}

// drawNumbers draws numbers periodically
func (uc *GameUseCase) drawNumbers(ctx context.Context, gameID uuid.UUID) {
	ticker := time.NewTicker(domain.DrawInterval)
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
		if err != nil {
			// Transient draw error — try again on the next tick.
			continue
		}
		if number == 0 {
			// All 75 numbers have been drawn and nobody submitted a valid bingo
			// claim. The game can never resolve on its own, so cancel it and
			// refund every active player. Without this, the staked money would
			// stay locked in a perpetual DRAWING game.
			if _, _, _, cerr := uc.cancelGameAndRefund(ctx, gameID, "all numbers drawn, no winner"); cerr != nil {
				fmt.Printf("Warning: failed to auto-cancel exhausted game %s: %v\n", gameID, cerr)
				continue
			}

			// Spin up a fresh game of the same type for the lobby.
			go func() {
				if newGame, err := uc.CreateOrGetGame(context.Background(), game.GameType); err == nil && newGame != nil {
					uc.redisService.PublishEvent(context.Background(), gameID, domain.WebSocketEventNewGameAvailable, map[string]interface{}{
						"gameId":   newGame.ID.String(),
						"gameType": string(newGame.GameType),
					})
				}
			}()
			return
		}

		// Save drawn number
		drawnAt := time.Now()
		drawnNumber := domain.DrawnNumber{
			Letter:  domain.BingoLetter(letter),
			Number:  number,
			DrawnAt: drawnAt,
		}

		uc.redisService.AddDrawnNumber(ctx, gameID, drawnNumber)
		uc.gameRepo.SaveDrawnNumber(ctx, gameID, letter, number)

		// Publish event
		uc.redisService.PublishEvent(ctx, gameID, domain.WebSocketEventNumberDrawn, map[string]interface{}{
			"letter":   letter,
			"number":   number,
			"drawn_at": drawnAt.Format(time.RFC3339),
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

	// Convert to set for quick lookup
	drawnSet := make(map[int]bool)
	for _, dn := range drawnNumbers {
		drawnSet[dn.Number] = true
	}

	// Generate card
	card := bingo.GenerateCard(player.CardID)
	if card == nil {
		return false, fmt.Errorf("invalid card ID")
	}

	// Convert marked positions (0-24) to actual card numbers
	markedNumbers := make([]int, 0, len(req.MarkedNumbers))
	for _, pos := range req.MarkedNumbers {
		if pos < 0 || pos >= domain.CardTotalPositions {
			return false, fmt.Errorf("invalid position: %d (must be 0-%d)", pos, domain.CardTotalPositions-1)
		}

		// Convert position to row/col: position = row * CardGridSize + col
		row := pos / domain.CardGridSize
		col := pos % domain.CardGridSize
		cardNumber := card.Numbers[row][col]

		// Verify this number was actually drawn
		if cardNumber != domain.CardCenterValue && !drawnSet[cardNumber] {
			return false, fmt.Errorf("number %d at position %d was not drawn", cardNumber, pos)
		}

		// Add to marked numbers (include CardCenterValue for center cell)
		markedNumbers = append(markedNumbers, cardNumber)
	}

	// Validate bingo with the marked numbers
	isValid := bingo.ValidateBingo(card, markedNumbers)

	// Start transaction
	tx, err := uc.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	if isValid {
		// Atomic single-winner guard: claim the win only if the game is still in
		// DRAWING state. If two valid claims race, only the first conditional
		// UPDATE affects a row; the second is rejected here — so there can never
		// be two winners or a double payout of the prize pool.
		claimed, err := uc.gameRepo.ClaimWinner(ctx, tx, gameID, req.UserID)
		if err != nil {
			return false, fmt.Errorf("failed to claim win: %w", err)
		}
		if !claimed {
			return false, errors.New("game already has a winner")
		}

		// Reflect the claimed state in memory for the events/Redis updates below.
		game.State = domain.GameStateFinished
		game.WinnerID = &req.UserID
		now := time.Now()
		game.FinishedAt = &now

		// Distribute prize
		_, err = uc.walletRepo.LockForUpdate(ctx, tx, req.UserID)
		if err != nil {
			return false, fmt.Errorf("wallet not found: %w", err)
		}

		// Add prize to winner's wallet
		if err := uc.walletRepo.UpdateBalance(ctx, tx, req.UserID, game.PrizePool); err != nil {
			return false, fmt.Errorf("failed to add prize: %w", err)
		}

		// Create transaction
		gamePrizeRef := "GAME_PRIZE"
		transaction := &domain.Transaction{
			UserID:    req.UserID,
			Type:      domain.TransactionTypeDeposit,
			Amount:    game.PrizePool,
			Status:    domain.TransactionStatusCompleted,
			Reference: &gamePrizeRef, // Mark as game prize to exclude from deposit history
		}

		if err := uc.transactionRepo.Create(ctx, tx, transaction); err != nil {
			return false, fmt.Errorf("failed to create transaction: %w", err)
		}

		// Commit transaction (ClaimWinner already persisted the FINISHED state)
		if err := tx.Commit(); err != nil {
			return false, fmt.Errorf("failed to commit transaction: %w", err)
		}

		// Update Redis
		uc.redisService.SaveGameState(ctx, game)

		// Fetch winner's user information for the event
		winner, err := uc.userRepo.FindByID(ctx, req.UserID)
		winnerName := "Unknown"
		if err == nil && winner != nil {
			if winner.LastName != nil && *winner.LastName != "" {
				winnerName = fmt.Sprintf("%s %s", winner.FirstName, *winner.LastName)
			} else {
				winnerName = winner.FirstName
			}
		}

		// Publish winner event with full details
		uc.redisService.PublishEvent(ctx, gameID, domain.WebSocketEventWinner, map[string]interface{}{
			"user_id":        req.UserID.String(),
			"winner_name":    winnerName,
			"prize":          game.PrizePool,
			"card_id":        player.CardID,
			"marked_numbers": markedNumbers,
		})

		// Publish game finished status
		uc.redisService.PublishEvent(ctx, gameID, domain.WebSocketEventGameStatus, map[string]interface{}{
			"status": string(domain.GameStateFinished),
		})

		// Create a new game of the same type and notify clients
		go func() {
			newGame, err := uc.CreateOrGetGame(context.Background(), game.GameType)
			if err == nil && newGame != nil {
				// Publish new game available event to the same channel
				// Clients can reconnect to this new game
				uc.redisService.PublishEvent(context.Background(), gameID, domain.WebSocketEventNewGameAvailable, map[string]interface{}{
					"gameId":   newGame.ID.String(),
					"gameType": string(newGame.GameType),
				})
			}
		}()

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
			gameRefundRef := "GAME_REFUND"
			for _, p := range players {
				_, err := uc.walletRepo.LockForUpdate(ctx, tx, p.UserID)
				if err == nil {
					uc.walletRepo.UpdateBalance(ctx, tx, p.UserID, game.BetAmount)
					// Create refund transaction
					refundTx := &domain.Transaction{
						UserID:    p.UserID,
						Type:      domain.TransactionTypeDeposit,
						Amount:    game.BetAmount,
						Status:    domain.TransactionStatusCompleted,
						Reference: &gameRefundRef, // Mark as game refund
					}
					uc.transactionRepo.Create(ctx, tx, refundTx)
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
		uc.redisService.PublishEvent(ctx, gameID, domain.WebSocketEventPlayerEliminated, map[string]interface{}{
			"userId": req.UserID.String(),
		})

		// If game was cancelled, publish status and create new game
		if game.State == domain.GameStateCancelled {
			uc.redisService.PublishEvent(ctx, gameID, domain.WebSocketEventGameStatus, map[string]interface{}{
				"status": string(domain.GameStateCancelled),
			})

			// Create a new game of the same type and notify clients
			go func() {
				newGame, err := uc.CreateOrGetGame(context.Background(), game.GameType)
				if err == nil && newGame != nil {
					uc.redisService.PublishEvent(context.Background(), gameID, domain.WebSocketEventNewGameAvailable, map[string]interface{}{
						"gameId":   newGame.ID.String(),
						"gameType": string(newGame.GameType),
					})
				}
			}()
		}

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
	if cardID < domain.MinCardID || cardID > domain.MaxCardID {
		return nil, fmt.Errorf("card ID must be between %d and %d", domain.MinCardID, domain.MaxCardID)
	}

	// Generate card (deterministic - same card_id always generates same card)
	card := bingo.GenerateCard(cardID)
	return card, nil
}

// ListGames returns games for the admin dashboard, filtered by optional state
// and type, with pagination. Returns the games and the total matching count.
func (uc *GameUseCase) ListGames(ctx context.Context, state *domain.GameState, gameType *domain.GameType, limit, offset int) ([]*domain.Game, int, error) {
	if limit <= 0 {
		limit = domain.MaxAvailableGamesLimit
	}
	if offset < 0 {
		offset = 0
	}

	games, err := uc.gameRepo.FindAll(ctx, state, gameType, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list games: %w", err)
	}

	total, err := uc.gameRepo.CountAll(ctx, state, gameType)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count games: %w", err)
	}

	return games, total, nil
}

// GetGameDetail returns a game and its active players enriched with user info,
// for the admin dashboard.
func (uc *GameUseCase) GetGameDetail(ctx context.Context, gameID uuid.UUID) (*domain.AdminGameDetail, error) {
	game, err := uc.gameRepo.FindByID(ctx, gameID)
	if err != nil {
		return nil, fmt.Errorf("game not found: %w", err)
	}

	players, err := uc.gameRepo.GetPlayers(ctx, gameID)
	if err != nil {
		return nil, fmt.Errorf("failed to get players: %w", err)
	}

	detail := &domain.AdminGameDetail{
		Game:    game,
		Players: make([]*domain.AdminGamePlayer, 0, len(players)),
	}

	for _, p := range players {
		entry := &domain.AdminGamePlayer{
			UserID:       p.UserID,
			CardID:       p.CardID,
			IsEliminated: p.IsEliminated,
			JoinedAt:     p.JoinedAt,
		}
		// Best-effort user enrichment; a missing user shouldn't break the view.
		if user, err := uc.userRepo.FindByID(ctx, p.UserID); err == nil && user != nil {
			entry.FirstName = user.FirstName
			entry.LastName = user.LastName
			entry.PhoneNumber = user.PhoneNumber
			entry.TelegramID = user.TelegramID
		}
		detail.Players = append(detail.Players, entry)
	}

	return detail, nil
}

// CancelGame is the admin force-cancel: it cancels the game and refunds every
// active player's stake. Only games that are not yet resolved can be cancelled.
func (uc *GameUseCase) CancelGame(ctx context.Context, gameID uuid.UUID) (*domain.CancelGameResult, error) {
	game, count, amount, err := uc.cancelGameAndRefund(ctx, gameID, "cancelled by admin")
	if err != nil {
		return nil, err
	}
	return &domain.CancelGameResult{
		Game:           game,
		RefundedCount:  count,
		RefundedAmount: amount,
	}, nil
}

// cancelGameAndRefund cancels a game and refunds every active player's stake in
// a single transaction. It locks the game row first to serialize against winner
// claims and concurrent cancels. Returns the updated game, the number of players
// refunded, and the total amount refunded. Shared by the admin force-cancel and
// the automatic "numbers exhausted, no winner" path in drawNumbers.
func (uc *GameUseCase) cancelGameAndRefund(ctx context.Context, gameID uuid.UUID, reason string) (*domain.Game, int, float64, error) {
	tx, err := uc.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Lock the game row. This blocks a concurrent winner claim's conditional
	// UPDATE (WHERE state='DRAWING') until we commit, after which it affects
	// zero rows — so we can never both refund and pay out a prize.
	game, err := uc.gameRepo.LockForUpdate(ctx, tx, gameID)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("game not found: %w", err)
	}

	// Only games that are still in play can be cancelled.
	if game.State == domain.GameStateFinished ||
		game.State == domain.GameStateCancelled ||
		game.State == domain.GameStateClosed {
		return nil, 0, 0, fmt.Errorf("game is already resolved (state: %s)", game.State)
	}

	players, err := uc.gameRepo.GetActivePlayersTx(ctx, tx, gameID)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("failed to get active players: %w", err)
	}

	refundRef := "GAME_REFUND"
	refundedCount := 0
	var refundedAmount float64

	for _, p := range players {
		if _, err := uc.walletRepo.LockForUpdate(ctx, tx, p.UserID); err != nil {
			return nil, 0, 0, fmt.Errorf("failed to lock wallet for refund: %w", err)
		}
		if err := uc.walletRepo.UpdateBalance(ctx, tx, p.UserID, game.BetAmount); err != nil {
			return nil, 0, 0, fmt.Errorf("failed to refund stake: %w", err)
		}

		refundTx := &domain.Transaction{
			UserID:    p.UserID,
			Type:      domain.TransactionTypeDeposit, // Refund is treated as a deposit
			Amount:    game.BetAmount,
			Status:    domain.TransactionStatusCompleted,
			Reference: &refundRef, // Mark as game refund to exclude from deposit history
		}
		if err := uc.transactionRepo.Create(ctx, tx, refundTx); err != nil {
			return nil, 0, 0, fmt.Errorf("failed to record refund: %w", err)
		}

		// Mark the player as having left so they can't be refunded again.
		if err := uc.gameRepo.RemovePlayer(ctx, tx, gameID, p.UserID); err != nil {
			return nil, 0, 0, fmt.Errorf("failed to remove player: %w", err)
		}

		refundedCount++
		refundedAmount += game.BetAmount
	}

	// Mark the game cancelled and zero out its live counters.
	game.State = domain.GameStateCancelled
	game.PlayerCount = 0
	game.PrizePool = 0
	if err := uc.gameRepo.UpdateTx(ctx, tx, game); err != nil {
		return nil, 0, 0, fmt.Errorf("failed to cancel game: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, 0, 0, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Best-effort live-state sync + notification (outside the money transaction).
	if uc.redisService != nil {
		uc.redisService.SaveGameState(ctx, game)
		uc.redisService.PublishEvent(ctx, gameID, domain.WebSocketEventGameStatus, map[string]interface{}{
			"status": string(domain.GameStateCancelled),
			"reason": reason,
		})
	}

	return game, refundedCount, refundedAmount, nil
}

// GetGameHistory returns the game history for a user
func (uc *GameUseCase) GetGameHistory(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.GameHistoryEntry, error) {
	if limit <= 0 {
		limit = domain.DefaultTransactionHistoryLimit
	}
	if offset < 0 {
		offset = 0
	}

	history, err := uc.gameRepo.FindGamesByUserID(ctx, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to get game history: %w", err)
	}

	return history, nil
}
