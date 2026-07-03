package usecase

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"sort"
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
	// Reject unsupported tiers (only REGULAR and VIP are offered)
	if !gameType.IsValid() {
		return nil, fmt.Errorf("invalid game type %q: must be REGULAR or VIP", gameType)
	}

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

	// Idempotent reconnect for THIS card: if the user already holds this exact
	// card, return that row instead of charging again. This lets a player who
	// dropped off (closed the app / lost their WebSocket) tap an already-owned
	// card and get back in, even after drawing has started. A *different* card
	// is a new purchase and falls through to the checks below.
	if existingCard, _ := uc.gameRepo.FindPlayerCard(ctx, gameID, req.UserID, req.CardID); existingCard != nil {
		return existingCard, nil
	}

	// A new card can only be bought while the game hasn't started yet.
	if game.State != domain.GameStateWaiting && game.State != domain.GameStateCountdown {
		return nil, errors.New("game is not accepting new players")
	}

	// Reject if this card is already held by someone (active). The DB also has a
	// UNIQUE(game_id, card_id) constraint as a hard safety net against the rare
	// check-then-insert race.
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

	// Serialize concurrent joins to THIS game by locking its row before touching
	// the live counters. Without it, two players joining at once both read the
	// same player_count/prize_pool and one update is lost — the pool ends up
	// undercounted (winner underpaid) and the count can stay below MinPlayers so
	// the game never starts. Lock order is game-then-wallet everywhere.
	game, err = uc.gameRepo.LockForUpdate(ctx, tx, gameID)
	if err != nil {
		return nil, fmt.Errorf("game not found: %w", err)
	}

	// Re-check under the lock: the game may have started since the pre-lock read,
	// and a concurrent join may have just taken this card.
	if game.State != domain.GameStateWaiting && game.State != domain.GameStateCountdown {
		return nil, errors.New("game is not accepting new players")
	}
	if takenCards, terr := uc.gameRepo.GetTakenCards(ctx, gameID); terr == nil {
		for _, taken := range takenCards {
			if taken == req.CardID {
				return nil, errors.New("card is already taken")
			}
		}
	}

	// Lock wallet
	wallet, err := uc.walletRepo.LockForUpdate(ctx, tx, req.UserID)
	if err != nil {
		return nil, fmt.Errorf("wallet not found: %w", err)
	}

	// Enforce the per-player card cap. Read after locking the wallet so a user's
	// concurrent joins (which serialize on that lock) can't both slip past it.
	// count == 0 means this is the player's first card in the game.
	existingCardCount, err := uc.gameRepo.CountActiveCardsForUser(ctx, gameID, req.UserID)
	if err != nil {
		return nil, fmt.Errorf("failed to count player cards: %w", err)
	}
	if existingCardCount >= domain.MaxCardsPerPlayer {
		return nil, fmt.Errorf("maximum %d cards per game", domain.MaxCardsPerPlayer)
	}

	// Check balance (one card = one bet)
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

	// Recompute the live counters from the authoritative set of active cards in
	// this game (visible within this locked transaction, including the card just
	// added) rather than incrementing in memory — so concurrent joins can't lose
	// an update. player_count is DISTINCT people (the start rule needs 2 real
	// players); the prize pool is (cards × stake), excluding the house cut.
	active, err := uc.gameRepo.GetActivePlayersTx(ctx, tx, gameID)
	if err != nil {
		return nil, fmt.Errorf("failed to read players: %w", err)
	}
	distinct := make(map[uuid.UUID]bool, len(active))
	for _, p := range active {
		distinct[p.UserID] = true
	}
	prevCount := game.PlayerCount
	game.PlayerCount = len(distinct)
	game.PrizePool = float64(len(active)) * game.BetAmount * (1 - game.HouseCut)

	if err := uc.gameRepo.UpdateTx(ctx, tx, game); err != nil {
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

	// Start the countdown on the join that brings the game up to the minimum
	// distinct players (the WAITING → ready transition), and only from WAITING so
	// extra cards from existing players don't re-trigger it.
	if game.State == domain.GameStateWaiting && prevCount < domain.MinPlayers && game.PlayerCount >= domain.MinPlayers {
		go uc.startCountdown(context.Background(), gameID)
	}

	// Publish event. Carry the live prize_pool and player_count so already-
	// connected clients update — otherwise their pool stays frozen at whatever
	// it was when they first received INITIAL_STATE.
	uc.redisService.PublishEvent(ctx, gameID, domain.WebSocketEventPlayerJoined, map[string]interface{}{
		"user_id":      req.UserID.String(),
		"card_id":      req.CardID,
		"prize_pool":   game.PrizePool,
		"player_count": game.PlayerCount,
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

	// Figure out which of the user's cards to drop. req.CardID > 0 drops just
	// that one card; req.CardID == 0 means leave the game entirely (all cards).
	userCards, err := uc.gameRepo.FindPlayersByUser(ctx, gameID, req.UserID)
	if err != nil || len(userCards) == 0 {
		return errors.New("user is not in this game")
	}

	var toDrop []*domain.GamePlayer
	if req.CardID > 0 {
		for _, p := range userCards {
			if p.CardID == req.CardID {
				toDrop = append(toDrop, p)
				break
			}
		}
		if len(toDrop) == 0 {
			return errors.New("you do not hold that card")
		}
	} else {
		toDrop = userCards
	}

	// Start transaction
	tx, err := uc.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Lock the game row to read the latest state and serialize against concurrent
	// joins/leaves that also recompute the live counters (same lock as JoinGame).
	game, err = uc.gameRepo.LockForUpdate(ctx, tx, gameID)
	if err != nil {
		return fmt.Errorf("game not found: %w", err)
	}

	// Check if game is already finished/cancelled/closed
	if game.State == domain.GameStateFinished || game.State == domain.GameStateClosed || game.State == domain.GameStateCancelled {
		return errors.New("game is no longer active")
	}

	// Once the draw has started, stakes are committed: leaving is refused. A
	// mid-draw leave gives no refund AND would recompute (shrink) the prize pool
	// for the remaining players, so it's pure downside / a griefing vector.
	// Cards play automatically to the end — a player who navigates away stays in.
	if game.State == domain.GameStateDrawing {
		return errors.New("cannot leave after the game has started")
	}

	// Only WAITING/COUNTDOWN remain here, so every leave at this point is refundable.
	refundable := game.State == domain.GameStateWaiting || game.State == domain.GameStateCountdown
	if refundable {
		// Lock the wallet once for all of this user's refunds.
		if _, err := uc.walletRepo.LockForUpdate(ctx, tx, req.UserID); err != nil {
			return fmt.Errorf("wallet not found: %w", err)
		}
	}

	gameRefundRef := "GAME_REFUND"
	for _, p := range toDrop {
		// Drop this card.
		if err := uc.gameRepo.RemovePlayerCard(ctx, tx, gameID, req.UserID, p.CardID); err != nil {
			return fmt.Errorf("failed to remove card: %w", err)
		}

		// Refund this card's stake (only before the game starts).
		if refundable {
			if err := uc.walletRepo.UpdateBalance(ctx, tx, req.UserID, game.BetAmount); err != nil {
				return fmt.Errorf("failed to refund: %w", err)
			}
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
		}
	}

	// Recompute the live counters from the authoritative remaining active cards
	// (this tx's removals are already visible) instead of decrementing in memory,
	// so a concurrent join/leave can't lose an update. player_count is DISTINCT
	// people; the pool is (remaining cards × stake), excluding the house cut.
	remaining, err := uc.gameRepo.GetActivePlayersTx(ctx, tx, gameID)
	if err != nil {
		return fmt.Errorf("failed to read players: %w", err)
	}
	distinct := make(map[uuid.UUID]bool, len(remaining))
	for _, p := range remaining {
		distinct[p.UserID] = true
	}
	leftEntirely := !distinct[req.UserID]
	game.PlayerCount = len(distinct)
	game.PrizePool = float64(len(remaining)) * game.BetAmount * (1 - game.HouseCut)

	// If distinct players drop below minimum during countdown, revert to WAITING.
	// Remaining players stay; the countdown restarts when a 2nd player returns.
	didRevert := false
	if game.State == domain.GameStateCountdown && game.PlayerCount < domain.MinPlayers {
		game.State = domain.GameStateWaiting
		game.CountdownEnds = nil
		didRevert = true
	}

	if err := uc.gameRepo.UpdateTx(ctx, tx, game); err != nil {
		return fmt.Errorf("failed to update game: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Update Redis - only drop the player from the live set if they fully left.
	if leftEntirely {
		uc.redisService.RemovePlayer(ctx, gameID, req.UserID)
	}

	// Free each dropped card if no remaining active player holds it.
	players, err := uc.gameRepo.GetPlayers(ctx, gameID)
	if err == nil {
		held := make(map[int]bool, len(players))
		for _, p := range players {
			held[p.CardID] = true
		}
		for _, p := range toDrop {
			if !held[p.CardID] {
				uc.redisService.RemoveTakenCard(ctx, gameID, p.CardID)
			}
		}
	}

	// If the countdown was reverted to WAITING, clear countdown state and notify.
	if didRevert {
		uc.redisService.ClearCountdown(ctx, gameID)
		uc.redisService.PublishEvent(ctx, gameID, domain.WebSocketEventGameStatus, map[string]interface{}{
			"status": string(domain.GameStateWaiting),
		})
	}

	uc.redisService.SaveGameState(ctx, game)

	// Publish a PLAYER_LEFT per dropped card so clients can free the card in the
	// UI. Carry the live prize_pool and player_count so every client updates.
	for _, p := range toDrop {
		uc.redisService.PublishEvent(ctx, gameID, domain.WebSocketEventPlayerLeft, map[string]interface{}{
			"user_id":      req.UserID.String(),
			"card_id":      p.CardID,
			"prize_pool":   game.PrizePool,
			"player_count": game.PlayerCount,
		})
	}

	return nil
}

// startCountdown starts the countdown for a game
func (uc *GameUseCase) startCountdown(ctx context.Context, gameID uuid.UUID) {
	// Transition WAITING → COUNTDOWN under the game-row lock and re-read, so this
	// only changes state/timing and never clobbers the player_count/prize_pool
	// that concurrent joins are still recomputing (a stale full-row write here
	// was reverting the live pool back to its 2-player value).
	tx, err := uc.db.BeginTx(ctx, nil)
	if err != nil {
		return
	}
	game, err := uc.gameRepo.LockForUpdate(ctx, tx, gameID)
	if err != nil || game.State != domain.GameStateWaiting {
		tx.Rollback()
		return
	}

	countdownEnds := time.Now().Add(domain.CountdownDuration)
	game.State = domain.GameStateCountdown
	game.CountdownEnds = &countdownEnds

	if err := uc.gameRepo.UpdateTx(ctx, tx, game); err != nil {
		tx.Rollback()
		return
	}
	if err := tx.Commit(); err != nil {
		return
	}

	uc.redisService.SetCountdown(ctx, gameID, countdownEnds)
	uc.redisService.SaveGameState(ctx, game)

	// Publish countdown start. Include prize_pool/player_count so every client
	// (including the player who joined first) shows the final live pool.
	uc.redisService.PublishEvent(ctx, gameID, domain.WebSocketEventGameStatus, map[string]interface{}{
		"status":       string(domain.GameStateCountdown),
		"secondsLeft":  int(domain.CountdownDuration.Seconds()),
		"prize_pool":   game.PrizePool,
		"player_count": game.PlayerCount,
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
	// Transition COUNTDOWN → DRAWING under the game-row lock and re-read, so a
	// late join finishing during the last tick of the countdown can't have its
	// counter update clobbered by a stale full-row write here.
	tx, err := uc.db.BeginTx(ctx, nil)
	if err != nil {
		return
	}
	game, err := uc.gameRepo.LockForUpdate(ctx, tx, gameID)
	if err != nil || game.State != domain.GameStateCountdown {
		tx.Rollback()
		return
	}

	game.State = domain.GameStateDrawing
	now := time.Now()
	game.StartedAt = &now

	if err := uc.gameRepo.UpdateTx(ctx, tx, game); err != nil {
		tx.Rollback()
		return
	}
	if err := tx.Commit(); err != nil {
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

		// Auto-declare a winner the instant a card completes a pattern with the
		// number just drawn — players no longer need to press a Bingo button.
		if uc.checkAutoBingo(ctx, gameID, game) {
			return
		}
	}
}

// winnerCard is one card that has completed a valid bingo, together with the
// auto-daubbed marks that prove it.
type winnerCard struct {
	UserID uuid.UUID
	CardID int
	Marked []int
}

// checkAutoBingo scans every active card after a number is drawn and finalizes
// the game the moment one or more cards complete a valid pattern. Because
// auto-daub marks every drawn number on every card, a card wins exactly when its
// marks (the free center plus every drawn number on the card) form a bingo over
// the current draw set — no manual claim required. When several cards complete
// on the same draw, they are all co-winners and split the prize pool.
//
// Returns true once the game has been resolved, so the draw loop can stop.
func (uc *GameUseCase) checkAutoBingo(ctx context.Context, gameID uuid.UUID, game *domain.Game) bool {
	drawnSet, err := uc.drawnNumberSet(ctx, gameID)
	if err != nil {
		return false
	}

	winners, err := uc.collectWinners(ctx, gameID, drawnSet)
	if err != nil || len(winners) == 0 {
		return false
	}

	if _, err := uc.finalizeWinners(ctx, game, winners); err != nil {
		fmt.Printf("Warning: auto-bingo finalize failed for game %s: %v\n", gameID, err)
		return false
	}
	// The game is resolved (won here, or already claimed elsewhere) — stop drawing.
	return true
}

// drawnNumberSet returns the set of numbers drawn so far for quick membership tests.
func (uc *GameUseCase) drawnNumberSet(ctx context.Context, gameID uuid.UUID) (map[int]bool, error) {
	drawnNumbers, err := uc.redisService.GetDrawnNumbers(ctx, gameID)
	if err != nil {
		return nil, err
	}
	drawnSet := make(map[int]bool, len(drawnNumbers))
	for _, dn := range drawnNumbers {
		drawnSet[dn.Number] = true
	}
	return drawnSet, nil
}

// collectWinners returns every active card that currently forms a valid bingo
// over the drawn set, sorted deterministically (earliest joiner, then lowest
// card ID). The order fixes the "primary" winner (games.winner_id) and the
// recipient of any rounding remainder when the pot is split.
func (uc *GameUseCase) collectWinners(ctx context.Context, gameID uuid.UUID, drawnSet map[int]bool) ([]winnerCard, error) {
	players, err := uc.gameRepo.GetPlayers(ctx, gameID)
	if err != nil {
		return nil, err
	}

	sort.Slice(players, func(i, j int) bool {
		if players[i].JoinedAt.Equal(players[j].JoinedAt) {
			return players[i].CardID < players[j].CardID
		}
		return players[i].JoinedAt.Before(players[j].JoinedAt)
	})

	winners := make([]winnerCard, 0)
	for _, p := range players {
		if p.IsEliminated {
			continue
		}
		card := bingo.GenerateCard(p.CardID)
		if card == nil {
			continue
		}
		marked := autoDaubMarks(card, drawnSet)
		if bingo.ValidateBingo(card, marked) {
			winners = append(winners, winnerCard{UserID: p.UserID, CardID: p.CardID, Marked: marked})
		}
	}
	return winners, nil
}

// autoDaubMarks returns the marked numbers of a card under auto-daub: the free
// center plus every drawn number present on the card.
func autoDaubMarks(card *bingo.BingoCard, drawnSet map[int]bool) []int {
	marked := make([]int, 0, domain.CardTotalPositions)
	for row := 0; row < domain.CardGridSize; row++ {
		for col := 0; col < domain.CardGridSize; col++ {
			n := card.Numbers[row][col]
			if n == domain.CardCenterValue || drawnSet[n] {
				marked = append(marked, n)
			}
		}
	}
	return marked
}

// round2 rounds a birr amount to the santim (2 decimal places).
func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

// splitPot divides a prize pool into n even shares rounded to the santim. Any
// rounding remainder is folded into the first share so the shares always sum to
// exactly the pool (never paying out more than was staked).
func splitPot(pool float64, n int) []float64 {
	shares := make([]float64, n)
	if n <= 0 {
		return shares
	}
	share := round2(pool / float64(n))
	for i := range shares {
		shares[i] = share
	}
	remainder := round2(pool - share*float64(n))
	shares[0] = round2(share + remainder)
	return shares
}

// winnerDisplayName resolves a user's display name for winner events.
func (uc *GameUseCase) winnerDisplayName(ctx context.Context, userID uuid.UUID) string {
	name := "Unknown"
	if u, err := uc.userRepo.FindByID(ctx, userID); err == nil && u != nil {
		if u.LastName != nil && *u.LastName != "" {
			name = fmt.Sprintf("%s %s", u.FirstName, *u.LastName)
		} else {
			name = u.FirstName
		}
	}
	return name
}

// finalizeWinners closes the game and splits the prize pool evenly across every
// winning card. It is the single winner-resolution path shared by manual bingo
// claims and automatic bingo detection.
//
// The conditional ClaimWinner UPDATE is an atomic single-resolution guard: if
// two resolutions race (a manual claim racing the auto-check), only the first
// transitions DRAWING→FINISHED and pays out; the rest observe the game is
// already claimed and return (false, nil) — no error, no double payout. The
// winners slice must be pre-sorted deterministically; winners[0] becomes the
// primary winner (games.winner_id) and absorbs any rounding remainder so the
// credited total exactly equals the prize pool.
func (uc *GameUseCase) finalizeWinners(ctx context.Context, game *domain.Game, winners []winnerCard) (bool, error) {
	if len(winners) == 0 {
		return false, nil
	}

	gameID := game.ID
	primary := winners[0].UserID

	tx, err := uc.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Atomic single-resolution guard: claim the game only if it is still DRAWING.
	claimed, err := uc.gameRepo.ClaimWinner(ctx, tx, gameID, primary)
	if err != nil {
		return false, fmt.Errorf("failed to claim win: %w", err)
	}
	if !claimed {
		// The game was already resolved elsewhere — nothing to do.
		return false, nil
	}

	// Split the pot evenly across every winning card (santim-rounded, remainder
	// to the first winner) so the credited total is exactly the prize pool.
	n := len(winners)
	amounts := splitPot(game.PrizePool, n)

	// Reflect the claimed state in memory for the events/Redis updates below.
	game.State = domain.GameStateFinished
	game.WinnerID = &primary
	now := time.Now()
	game.FinishedAt = &now

	gamePrizeRef := "GAME_PRIZE"
	for i, w := range winners {
		amount := amounts[i]

		// Lock and credit the winner's wallet. Multiple cards may belong to the
		// same user; each card's share is credited separately to that wallet.
		if _, err := uc.walletRepo.LockForUpdate(ctx, tx, w.UserID); err != nil {
			return false, fmt.Errorf("wallet not found: %w", err)
		}
		if err := uc.walletRepo.UpdateBalance(ctx, tx, w.UserID, amount); err != nil {
			return false, fmt.Errorf("failed to add prize: %w", err)
		}

		// Record the prize as a completed transaction (GAME_PRIZE excludes it
		// from deposit history).
		if err := uc.transactionRepo.Create(ctx, tx, &domain.Transaction{
			UserID:    w.UserID,
			Type:      domain.TransactionTypeDeposit,
			Amount:    amount,
			Status:    domain.TransactionStatusCompleted,
			Reference: &gamePrizeRef,
		}); err != nil {
			return false, fmt.Errorf("failed to create prize transaction: %w", err)
		}

		// Flag the winning card and record its share.
		if err := uc.gameRepo.MarkCardWinner(ctx, tx, gameID, w.UserID, w.CardID, amount); err != nil {
			return false, fmt.Errorf("failed to mark winning card: %w", err)
		}
	}

	// Commit transaction (ClaimWinner already persisted the FINISHED state).
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Update Redis
	uc.redisService.SaveGameState(ctx, game)

	// Build the winners payload (each card's owner, share, and proving marks).
	winnersPayload := make([]map[string]interface{}, n)
	for i, w := range winners {
		winnersPayload[i] = map[string]interface{}{
			"user_id":        w.UserID.String(),
			"winner_name":    uc.winnerDisplayName(ctx, w.UserID),
			"prize":          amounts[i],
			"card_id":        w.CardID,
			"marked_numbers": w.Marked,
		}
	}

	// Publish the winner event. Top-level fields describe the primary winner for
	// backward compatibility with older clients; `winners` carries every
	// co-winner and their individual share, and `prize_pool` is the full pot.
	primaryPayload := winnersPayload[0]
	uc.redisService.PublishEvent(ctx, gameID, domain.WebSocketEventWinner, map[string]interface{}{
		"user_id":        primaryPayload["user_id"],
		"winner_name":    primaryPayload["winner_name"],
		"prize":          primaryPayload["prize"],
		"card_id":        primaryPayload["card_id"],
		"marked_numbers": primaryPayload["marked_numbers"],
		"prize_pool":     game.PrizePool,
		"split":          n > 1,
		"winners":        winnersPayload,
	})

	// Publish game finished status
	uc.redisService.PublishEvent(ctx, gameID, domain.WebSocketEventGameStatus, map[string]interface{}{
		"status": string(domain.GameStateFinished),
	})

	// Create a new game of the same type and notify clients
	go func() {
		newGame, err := uc.CreateOrGetGame(context.Background(), game.GameType)
		if err == nil && newGame != nil {
			// Publish new game available event to the same channel so clients can
			// reconnect to this new game.
			uc.redisService.PublishEvent(context.Background(), gameID, domain.WebSocketEventNewGameAvailable, map[string]interface{}{
				"gameId":   newGame.ID.String(),
				"gameType": string(newGame.GameType),
			})
		}
	}()

	return true, nil
}

// ClaimBingo validates and processes a manual bingo claim. With automatic bingo
// detection now resolving games as soon as a card completes (see checkAutoBingo),
// this endpoint is a fallback/no-op in most games, but is kept working so an
// explicit claim still pays out correctly — splitting the pot with any co-winners
// through the shared finalizeWinners path.
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

	// Resolve the specific card being claimed. It must belong to this user and
	// still be active in the game.
	player, err := uc.gameRepo.FindPlayerCard(ctx, gameID, req.UserID, req.CardID)
	if err != nil {
		return false, errors.New("you do not hold that card in this game")
	}

	if player.IsEliminated {
		return false, errors.New("this card is already eliminated")
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
		// Valid claim — resolve the game through the shared finalizeWinners path,
		// splitting the pot with any other cards that completed on the same draw.
		// Auto-daub marks are a superset of the client's marked positions, so the
		// claimant's card is guaranteed to be among the collected winners. The
		// atomic guard inside finalizeWinners prevents a double payout if a manual
		// claim and the auto-check race.
		winners, werr := uc.collectWinners(ctx, gameID, drawnSet)
		if werr != nil {
			return false, fmt.Errorf("failed to collect winners: %w", werr)
		}
		return uc.finalizeWinners(ctx, game, winners)
	} else {
		// Invalid claim - eliminate only this card; the player's other cards live on.
		if err := uc.gameRepo.EliminatePlayerCard(ctx, tx, gameID, req.UserID, req.CardID); err != nil {
			return false, fmt.Errorf("failed to eliminate card: %w", err)
		}

		// Count cards still in play. GetPlayers reads committed state, which does
		// not yet reflect this transaction's elimination, so skip the just-killed
		// card explicitly.
		players, _ := uc.gameRepo.GetPlayers(ctx, gameID)
		activeCards := 0
		for _, p := range players {
			if p.IsEliminated {
				continue
			}
			if p.UserID == req.UserID && p.CardID == req.CardID {
				continue // eliminated in this tx
			}
			activeCards++
		}

		if activeCards == 0 {
			// Every card is out - cancel the game and refund each card's stake
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

		if err := uc.gameRepo.UpdateTx(ctx, tx, game); err != nil {
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

// GetGameWinners returns the winning card(s) of a finished game — each with its
// prize share and the marked numbers reconstructed from the drawn set via
// auto-daub. There may be several (co-winners who split the pot). Used to render
// the post-game winner card(s), including for clients that connect after the
// transient live winner event. Returns an empty slice for games with no winner.
func (uc *GameUseCase) GetGameWinners(ctx context.Context, gameID uuid.UUID) ([]domain.GameWinner, error) {
	rows, err := uc.gameRepo.FindWinningCards(ctx, gameID)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []domain.GameWinner{}, nil
	}

	// Reconstruct each winner's marks from the drawn set (nil-safe if the draw
	// history is no longer cached — the card still renders, just unhighlighted).
	drawnSet, _ := uc.drawnNumberSet(ctx, gameID)

	winners := make([]domain.GameWinner, 0, len(rows))
	for _, w := range rows {
		out := *w
		if card := bingo.GenerateCard(w.CardID); card != nil {
			out.MarkedNumbers = autoDaubMarks(card, drawnSet)
		}
		winners = append(winners, out)
	}
	return winners, nil
}

// GetPlayerInGame checks if a player is in a game (for WebSocket validation)
func (uc *GameUseCase) GetPlayerInGame(ctx context.Context, gameID, userID uuid.UUID) (*domain.GamePlayer, error) {
	return uc.gameRepo.FindPlayer(ctx, gameID, userID)
}

// GetMyCardsInGame returns all of the authenticated user's active cards in a
// game (a player may hold up to MaxCardsPerPlayer).
func (uc *GameUseCase) GetMyCardsInGame(ctx context.Context, gameID, userID uuid.UUID) ([]*domain.GamePlayer, error) {
	return uc.gameRepo.FindPlayersByUser(ctx, gameID, userID)
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

		// Mark this card as having left so it can't be refunded again.
		if err := uc.gameRepo.RemovePlayerCard(ctx, tx, gameID, p.UserID, p.CardID); err != nil {
			return nil, 0, 0, fmt.Errorf("failed to remove player card: %w", err)
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

// GetRecentWinners returns the most recent game winners for the public lobby feed.
func (uc *GameUseCase) GetRecentWinners(ctx context.Context, limit int) ([]*domain.RecentWinner, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	winners, err := uc.gameRepo.FindRecentWinners(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get recent winners: %w", err)
	}
	return winners, nil
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
