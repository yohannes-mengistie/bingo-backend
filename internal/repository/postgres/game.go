package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/bingo/backend/internal/domain"
	"github.com/google/uuid"
)

type gameRepository struct {
	db *sql.DB
}

// NewGameRepository creates a new PostgreSQL game repository
func NewGameRepository(db *sql.DB) domain.GameRepository {
	return &gameRepository{db: db}
}

// Create creates a new game
func (r *gameRepository) Create(ctx context.Context, game *domain.Game) error {
	query := `
		INSERT INTO games (id, game_type, state, bet_amount, min_players, player_count, prize_pool, house_cut, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`

	now := time.Now()
	game.CreatedAt = now
	game.UpdatedAt = now

	_, err := r.db.ExecContext(ctx, query,
		game.ID,
		game.GameType,
		game.State,
		game.BetAmount,
		game.MinPlayers,
		game.PlayerCount,
		game.PrizePool,
		game.HouseCut,
		game.CreatedAt,
		game.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to create game: %w", err)
	}

	return nil
}

// FindByID finds a game by ID
func (r *gameRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.Game, error) {
	query := `
		SELECT id, game_type, state, bet_amount, min_players, player_count, prize_pool, house_cut,
		       winner_id, countdown_ends, started_at, finished_at, created_at, updated_at
		FROM games
		WHERE id = $1
	`

	game := &domain.Game{}
	var winnerID sql.NullString
	var countdownEnds sql.NullTime
	var startedAt sql.NullTime
	var finishedAt sql.NullTime

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&game.ID,
		&game.GameType,
		&game.State,
		&game.BetAmount,
		&game.MinPlayers,
		&game.PlayerCount,
		&game.PrizePool,
		&game.HouseCut,
		&winnerID,
		&countdownEnds,
		&startedAt,
		&finishedAt,
		&game.CreatedAt,
		&game.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("game not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find game: %w", err)
	}

	// Handle nullable fields
	if winnerID.Valid {
		parsedID, err := uuid.Parse(winnerID.String)
		if err != nil {
			return nil, fmt.Errorf("failed to parse winner_id: %w", err)
		}
		game.WinnerID = &parsedID
	}
	if countdownEnds.Valid {
		game.CountdownEnds = &countdownEnds.Time
	}
	if startedAt.Valid {
		game.StartedAt = &startedAt.Time
	}
	if finishedAt.Valid {
		game.FinishedAt = &finishedAt.Time
	}

	return game, nil
}

// FindAvailable finds available games (WAITING or COUNTDOWN state)
func (r *gameRepository) FindAvailable(ctx context.Context, gameType *domain.GameType, limit int) ([]*domain.Game, error) {
	query := `
		SELECT id, game_type, state, bet_amount, min_players, player_count, prize_pool, house_cut,
		       winner_id, countdown_ends, started_at, finished_at, created_at, updated_at
		FROM games
		WHERE state IN ('WAITING', 'COUNTDOWN')
	`

	args := []interface{}{}
	argPos := 1

	if gameType != nil {
		query += fmt.Sprintf(" AND game_type = $%d", argPos)
		args = append(args, *gameType)
		argPos++
	}

	query += " ORDER BY created_at ASC LIMIT $" + fmt.Sprintf("%d", argPos)
	args = append(args, limit)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to find available games: %w", err)
	}
	defer rows.Close()

	games := []*domain.Game{}
	for rows.Next() {
		game := &domain.Game{}
		var winnerID sql.NullString
		var countdownEnds sql.NullTime
		var startedAt sql.NullTime
		var finishedAt sql.NullTime

		err := rows.Scan(
			&game.ID,
			&game.GameType,
			&game.State,
			&game.BetAmount,
			&game.MinPlayers,
			&game.PlayerCount,
			&game.PrizePool,
			&game.HouseCut,
			&winnerID,
			&countdownEnds,
			&startedAt,
			&finishedAt,
			&game.CreatedAt,
			&game.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan game: %w", err)
		}

		// Handle nullable fields
		if winnerID.Valid {
			parsedID, err := uuid.Parse(winnerID.String)
			if err != nil {
				return nil, fmt.Errorf("failed to parse winner_id: %w", err)
			}
			game.WinnerID = &parsedID
		}
		if countdownEnds.Valid {
			game.CountdownEnds = &countdownEnds.Time
		}
		if startedAt.Valid {
			game.StartedAt = &startedAt.Time
		}
		if finishedAt.Valid {
			game.FinishedAt = &finishedAt.Time
		}

		games = append(games, game)
	}

	// Check for errors from iterating over rows
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating games: %w", err)
	}

	return games, nil
}

// scanGame scans a single game row from a *sql.Row-like Scan into a domain.Game,
// handling the nullable columns. The column order must match the SELECT below.
func scanGame(scan func(dest ...interface{}) error) (*domain.Game, error) {
	game := &domain.Game{}
	var winnerID sql.NullString
	var countdownEnds sql.NullTime
	var startedAt sql.NullTime
	var finishedAt sql.NullTime

	if err := scan(
		&game.ID,
		&game.GameType,
		&game.State,
		&game.BetAmount,
		&game.MinPlayers,
		&game.PlayerCount,
		&game.PrizePool,
		&game.HouseCut,
		&winnerID,
		&countdownEnds,
		&startedAt,
		&finishedAt,
		&game.CreatedAt,
		&game.UpdatedAt,
	); err != nil {
		return nil, err
	}

	if winnerID.Valid {
		parsedID, err := uuid.Parse(winnerID.String)
		if err != nil {
			return nil, fmt.Errorf("failed to parse winner_id: %w", err)
		}
		game.WinnerID = &parsedID
	}
	if countdownEnds.Valid {
		game.CountdownEnds = &countdownEnds.Time
	}
	if startedAt.Valid {
		game.StartedAt = &startedAt.Time
	}
	if finishedAt.Valid {
		game.FinishedAt = &finishedAt.Time
	}

	return game, nil
}

// FindAll returns games filtered by optional state and type, newest first.
func (r *gameRepository) FindAll(ctx context.Context, state *domain.GameState, gameType *domain.GameType, limit, offset int) ([]*domain.Game, error) {
	query := `
		SELECT id, game_type, state, bet_amount, min_players, player_count, prize_pool, house_cut,
		       winner_id, countdown_ends, started_at, finished_at, created_at, updated_at
		FROM games
		WHERE 1=1
	`

	args := []interface{}{}
	argPos := 1

	if state != nil {
		query += fmt.Sprintf(" AND state = $%d", argPos)
		args = append(args, *state)
		argPos++
	}
	if gameType != nil {
		query += fmt.Sprintf(" AND game_type = $%d", argPos)
		args = append(args, *gameType)
		argPos++
	}

	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", argPos, argPos+1)
	args = append(args, limit, offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to find games: %w", err)
	}
	defer rows.Close()

	games := []*domain.Game{}
	for rows.Next() {
		game, err := scanGame(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("failed to scan game: %w", err)
		}
		games = append(games, game)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating games: %w", err)
	}

	return games, nil
}

// CountAll counts games matching the optional state and type filters.
func (r *gameRepository) CountAll(ctx context.Context, state *domain.GameState, gameType *domain.GameType) (int, error) {
	query := `SELECT COUNT(*) FROM games WHERE 1=1`

	args := []interface{}{}
	argPos := 1

	if state != nil {
		query += fmt.Sprintf(" AND state = $%d", argPos)
		args = append(args, *state)
		argPos++
	}
	if gameType != nil {
		query += fmt.Sprintf(" AND game_type = $%d", argPos)
		args = append(args, *gameType)
		argPos++
	}

	var count int
	if err := r.db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to count games: %w", err)
	}
	return count, nil
}

// LockForUpdate locks a game row FOR UPDATE inside a transaction.
func (r *gameRepository) LockForUpdate(ctx context.Context, tx *sql.Tx, id uuid.UUID) (*domain.Game, error) {
	query := `
		SELECT id, game_type, state, bet_amount, min_players, player_count, prize_pool, house_cut,
		       winner_id, countdown_ends, started_at, finished_at, created_at, updated_at
		FROM games
		WHERE id = $1
		FOR UPDATE
	`

	game, err := scanGame(tx.QueryRowContext(ctx, query, id).Scan)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("game not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to lock game: %w", err)
	}
	return game, nil
}

// UpdateTx updates a game row inside an existing transaction.
func (r *gameRepository) UpdateTx(ctx context.Context, tx *sql.Tx, game *domain.Game) error {
	query := `
		UPDATE games
		SET state = $2, player_count = $3, prize_pool = $4, house_cut = $5,
		    winner_id = $6, countdown_ends = $7, started_at = $8, finished_at = $9, updated_at = $10
		WHERE id = $1
	`

	game.UpdatedAt = time.Now()

	result, err := tx.ExecContext(ctx, query,
		game.ID,
		game.State,
		game.PlayerCount,
		game.PrizePool,
		game.HouseCut,
		game.WinnerID,
		game.CountdownEnds,
		game.StartedAt,
		game.FinishedAt,
		game.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to update game: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("game not found")
	}
	return nil
}

// GetActivePlayersTx returns players still in the game (left_at IS NULL), read
// inside an existing transaction.
func (r *gameRepository) GetActivePlayersTx(ctx context.Context, tx *sql.Tx, gameID uuid.UUID) ([]*domain.GamePlayer, error) {
	query := `
		SELECT id, game_id, user_id, card_id, is_eliminated, joined_at, left_at
		FROM game_players
		WHERE game_id = $1 AND left_at IS NULL
	`

	rows, err := tx.QueryContext(ctx, query, gameID)
	if err != nil {
		return nil, fmt.Errorf("failed to get active players: %w", err)
	}
	defer rows.Close()

	players := []*domain.GamePlayer{}
	for rows.Next() {
		player := &domain.GamePlayer{}
		if err := rows.Scan(
			&player.ID,
			&player.GameID,
			&player.UserID,
			&player.CardID,
			&player.IsEliminated,
			&player.JoinedAt,
			&player.LeftAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan player: %w", err)
		}
		players = append(players, player)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating players: %w", err)
	}

	return players, nil
}

// Update updates a game
func (r *gameRepository) Update(ctx context.Context, game *domain.Game) error {
	query := `
		UPDATE games
		SET state = $2, player_count = $3, prize_pool = $4, house_cut = $5,
		    winner_id = $6, countdown_ends = $7, started_at = $8, finished_at = $9, updated_at = $10
		WHERE id = $1
	`

	game.UpdatedAt = time.Now()

	result, err := r.db.ExecContext(ctx, query,
		game.ID,
		game.State,
		game.PlayerCount,
		game.PrizePool,
		game.HouseCut,
		game.WinnerID,
		game.CountdownEnds,
		game.StartedAt,
		game.FinishedAt,
		game.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to update game: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("game not found")
	}

	return nil
}

// ClaimWinner atomically marks the game FINISHED with the given winner, but only
// if it is still in DRAWING state. The WHERE clause makes this a single-winner
// guard: with concurrent claims, exactly one UPDATE affects a row (returns true);
// the others affect zero rows (return false), so no double winner or double payout.
func (r *gameRepository) ClaimWinner(ctx context.Context, tx *sql.Tx, gameID, winnerID uuid.UUID) (bool, error) {
	query := `
		UPDATE games
		SET state = 'FINISHED', winner_id = $2, finished_at = $3, updated_at = $3
		WHERE id = $1 AND state = 'DRAWING'
	`

	now := time.Now()

	var result sql.Result
	var err error
	if tx != nil {
		result, err = tx.ExecContext(ctx, query, gameID, winnerID, now)
	} else {
		result, err = r.db.ExecContext(ctx, query, gameID, winnerID, now)
	}
	if err != nil {
		return false, fmt.Errorf("failed to claim winner: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("failed to get rows affected: %w", err)
	}

	return rowsAffected == 1, nil
}

// AddPlayer adds a player to a game
// If player previously left (left_at is set), updates the existing record instead of inserting
func (r *gameRepository) AddPlayer(ctx context.Context, tx *sql.Tx, player *domain.GamePlayer) error {
	player.JoinedAt = time.Now()

	// First, try to update if player record exists with left_at set (rejoining)
	updateQuery := `
		UPDATE game_players
		SET card_id = $1, is_eliminated = $2, joined_at = $3, left_at = NULL
		WHERE game_id = $4 AND user_id = $5 AND left_at IS NOT NULL
	`

	var err error
	var result sql.Result
	if tx != nil {
		result, err = tx.ExecContext(ctx, updateQuery, player.CardID, player.IsEliminated, player.JoinedAt, player.GameID, player.UserID)
	} else {
		result, err = r.db.ExecContext(ctx, updateQuery, player.CardID, player.IsEliminated, player.JoinedAt, player.GameID, player.UserID)
	}

	if err != nil {
		return fmt.Errorf("failed to update player: %w", err)
	}

	// Check if update affected any rows (player was rejoining)
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	// If update didn't affect any rows, insert new record
	if rowsAffected == 0 {
		insertQuery := `
			INSERT INTO game_players (id, game_id, user_id, card_id, is_eliminated, joined_at)
			VALUES ($1, $2, $3, $4, $5, $6)
		`

		if tx != nil {
			_, err = tx.ExecContext(ctx, insertQuery, player.ID, player.GameID, player.UserID, player.CardID, player.IsEliminated, player.JoinedAt)
		} else {
			_, err = r.db.ExecContext(ctx, insertQuery, player.ID, player.GameID, player.UserID, player.CardID, player.IsEliminated, player.JoinedAt)
		}

		if err != nil {
			return fmt.Errorf("failed to add player: %w", err)
		}
	}

	return nil
}

// RemovePlayer removes a player from a game
// Sets left_at timestamp to mark the player as having left (soft delete)
func (r *gameRepository) RemovePlayer(ctx context.Context, tx *sql.Tx, gameID, userID uuid.UUID) error {
	query := `
		UPDATE game_players
		SET left_at = $3
		WHERE game_id = $1 AND user_id = $2 AND left_at IS NULL
	`

	leftAt := time.Now()
	var err error
	if tx != nil {
		_, err = tx.ExecContext(ctx, query, gameID, userID, leftAt)
	} else {
		_, err = r.db.ExecContext(ctx, query, gameID, userID, leftAt)
	}

	if err != nil {
		return fmt.Errorf("failed to remove player: %w", err)
	}

	return nil
}

// FindPlayer finds a player in a game
func (r *gameRepository) FindPlayer(ctx context.Context, gameID, userID uuid.UUID) (*domain.GamePlayer, error) {
	query := `
		SELECT id, game_id, user_id, card_id, is_eliminated, joined_at, left_at
		FROM game_players
		WHERE game_id = $1 AND user_id = $2 AND left_at IS NULL
	`

	player := &domain.GamePlayer{}
	err := r.db.QueryRowContext(ctx, query, gameID, userID).Scan(
		&player.ID,
		&player.GameID,
		&player.UserID,
		&player.CardID,
		&player.IsEliminated,
		&player.JoinedAt,
		&player.LeftAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("player not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find player: %w", err)
	}

	return player, nil
}

// GetPlayers gets all players in a game
func (r *gameRepository) GetPlayers(ctx context.Context, gameID uuid.UUID) ([]*domain.GamePlayer, error) {
	query := `
		SELECT id, game_id, user_id, card_id, is_eliminated, joined_at, left_at
		FROM game_players
		WHERE game_id = $1 AND left_at IS NULL
	`

	rows, err := r.db.QueryContext(ctx, query, gameID)
	if err != nil {
		return nil, fmt.Errorf("failed to get players: %w", err)
	}
	defer rows.Close()

	players := []*domain.GamePlayer{}
	for rows.Next() {
		player := &domain.GamePlayer{}
		err := rows.Scan(
			&player.ID,
			&player.GameID,
			&player.UserID,
			&player.CardID,
			&player.IsEliminated,
			&player.JoinedAt,
			&player.LeftAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan player: %w", err)
		}
		players = append(players, player)
	}

	return players, nil
}

// EliminatePlayer marks a player as eliminated
func (r *gameRepository) EliminatePlayer(ctx context.Context, tx *sql.Tx, gameID, userID uuid.UUID) error {
	query := `
		UPDATE game_players
		SET is_eliminated = TRUE
		WHERE game_id = $1 AND user_id = $2
	`

	var err error
	if tx != nil {
		_, err = tx.ExecContext(ctx, query, gameID, userID)
	} else {
		_, err = r.db.ExecContext(ctx, query, gameID, userID)
	}

	if err != nil {
		return fmt.Errorf("failed to eliminate player: %w", err)
	}

	return nil
}

// GetTakenCards gets all taken card IDs for a game
func (r *gameRepository) GetTakenCards(ctx context.Context, gameID uuid.UUID) ([]int, error) {
	query := `
		SELECT card_id
		FROM game_players
		WHERE game_id = $1 AND left_at IS NULL
	`

	rows, err := r.db.QueryContext(ctx, query, gameID)
	if err != nil {
		return nil, fmt.Errorf("failed to get taken cards: %w", err)
	}
	defer rows.Close()

	cards := []int{}
	for rows.Next() {
		var cardID int
		if err := rows.Scan(&cardID); err != nil {
			return nil, fmt.Errorf("failed to scan card ID: %w", err)
		}
		cards = append(cards, cardID)
	}

	return cards, nil
}

// SaveDrawnNumber saves a drawn number to the database
func (r *gameRepository) SaveDrawnNumber(ctx context.Context, gameID uuid.UUID, letter string, number int) error {
	query := `
		INSERT INTO drawn_numbers (game_id, letter, number, drawn_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (game_id, letter, number) DO NOTHING
	`

	_, err := r.db.ExecContext(ctx, query, gameID, letter, number, time.Now())
	if err != nil {
		return fmt.Errorf("failed to save drawn number: %w", err)
	}

	return nil
}

// FindGamesByUserID finds all games a user has participated in
func (r *gameRepository) FindGamesByUserID(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.GameHistoryEntry, error) {
	query := `
		SELECT 
			g.id, g.game_type, g.state, g.bet_amount, g.min_players, g.player_count, 
			g.prize_pool, g.house_cut, g.winner_id, g.countdown_ends, g.started_at, 
			g.finished_at, g.created_at, g.updated_at,
			gp.card_id, gp.is_eliminated, gp.joined_at, gp.left_at
		FROM game_players gp
		INNER JOIN games g ON gp.game_id = g.id
		WHERE gp.user_id = $1
		ORDER BY gp.joined_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.db.QueryContext(ctx, query, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to find games by user ID: %w", err)
	}
	defer rows.Close()

	entries := []*domain.GameHistoryEntry{}
	for rows.Next() {
		entry := &domain.GameHistoryEntry{
			Game: &domain.Game{},
		}
		var winnerID sql.NullString
		var countdownEnds sql.NullTime
		var startedAt sql.NullTime
		var finishedAt sql.NullTime
		var leftAt sql.NullTime

		err := rows.Scan(
			&entry.Game.ID,
			&entry.Game.GameType,
			&entry.Game.State,
			&entry.Game.BetAmount,
			&entry.Game.MinPlayers,
			&entry.Game.PlayerCount,
			&entry.Game.PrizePool,
			&entry.Game.HouseCut,
			&winnerID,
			&countdownEnds,
			&startedAt,
			&finishedAt,
			&entry.Game.CreatedAt,
			&entry.Game.UpdatedAt,
			&entry.CardID,
			&entry.IsEliminated,
			&entry.JoinedAt,
			&leftAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan game history entry: %w", err)
		}

		// Handle nullable fields
		if winnerID.Valid {
			parsedID, err := uuid.Parse(winnerID.String)
			if err != nil {
				return nil, fmt.Errorf("failed to parse winner_id: %w", err)
			}
			entry.Game.WinnerID = &parsedID
			entry.IsWinner = parsedID == userID
		}
		if countdownEnds.Valid {
			entry.Game.CountdownEnds = &countdownEnds.Time
		}
		if startedAt.Valid {
			entry.Game.StartedAt = &startedAt.Time
		}
		if finishedAt.Valid {
			entry.Game.FinishedAt = &finishedAt.Time
		}
		if leftAt.Valid {
			entry.LeftAt = &leftAt.Time
		}

		entries = append(entries, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating game history entries: %w", err)
	}

	return entries, nil
}

// CountGamesByType counts games by type (only completed games: FINISHED or CLOSED)
func (r *gameRepository) CountGamesByType(ctx context.Context) (map[domain.GameType]int, error) {
	query := `
		SELECT game_type, COUNT(*) as count
		FROM games
		WHERE state IN ('FINISHED', 'CLOSED')
		GROUP BY game_type
	`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to count games by type: %w", err)
	}
	defer rows.Close()

	result := make(map[domain.GameType]int)
	for rows.Next() {
		var gameType domain.GameType
		var count int
		if err := rows.Scan(&gameType, &count); err != nil {
			return nil, fmt.Errorf("failed to scan game count: %w", err)
		}
		result[gameType] = count
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating game counts: %w", err)
	}

	return result, nil
}

// GetTotalHouseCut calculates the total house cut from all games
func (r *gameRepository) GetTotalHouseCut(ctx context.Context) (float64, error) {
	query := `
		SELECT COALESCE(SUM(prize_pool * house_cut / (1 - house_cut)), 0) as total_house_cut
		FROM games
		WHERE state IN ('FINISHED', 'CLOSED')
	`

	var totalHouseCut float64
	err := r.db.QueryRowContext(ctx, query).Scan(&totalHouseCut)
	if err != nil {
		return 0, fmt.Errorf("failed to get total house cut: %w", err)
	}

	return totalHouseCut, nil
}
