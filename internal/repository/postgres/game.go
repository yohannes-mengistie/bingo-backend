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

	return games, nil
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

// AddPlayer adds a player to a game
func (r *gameRepository) AddPlayer(ctx context.Context, tx *sql.Tx, player *domain.GamePlayer) error {
	query := `
		INSERT INTO game_players (id, game_id, user_id, card_id, is_eliminated, joined_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`

	player.JoinedAt = time.Now()

	var err error
	if tx != nil {
		_, err = tx.ExecContext(ctx, query, player.ID, player.GameID, player.UserID, player.CardID, player.IsEliminated, player.JoinedAt)
	} else {
		_, err = r.db.ExecContext(ctx, query, player.ID, player.GameID, player.UserID, player.CardID, player.IsEliminated, player.JoinedAt)
	}

	if err != nil {
		return fmt.Errorf("failed to add player: %w", err)
	}

	return nil
}

// RemovePlayer removes a player from a game
func (r *gameRepository) RemovePlayer(ctx context.Context, tx *sql.Tx, gameID, userID uuid.UUID) error {
	query := `
		UPDATE game_players
		SET left_at = $3
		WHERE game_id = $1 AND user_id = $2
	`

	now := time.Now()

	var err error
	if tx != nil {
		_, err = tx.ExecContext(ctx, query, gameID, userID, now)
	} else {
		_, err = r.db.ExecContext(ctx, query, gameID, userID, now)
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
