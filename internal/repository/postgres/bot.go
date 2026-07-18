package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/bingo/backend/internal/domain"
	"github.com/google/uuid"
)

// botRepository serves bot-specific reads and the auto-fill policy row.
type botRepository struct {
	db *sql.DB
}

// NewBotRepository creates a new bot repository.
func NewBotRepository(db *sql.DB) domain.BotRepository {
	return &botRepository{db: db}
}

// ListBots returns bot users (is_bot = true) in RANDOM order, up to limit.
//
// The order is deliberately random rather than oldest-first: FillGame walks
// this list and takes the first N that fit, so a stable order meant the same
// oldest bots joined every single game in the same sequence — a recognisable
// roster to any regular player — while bots past the target count never played
// at all. Randomising spreads play across the whole pool and varies the lineup
// per round. Sorting only touches the few hundred rows matching is_bot, so the
// cost is negligible even at the 1s sweep interval.
func (r *botRepository) ListBots(ctx context.Context, limit int) ([]*domain.User, error) {
	query := `
		SELECT id, telegram_id, first_name, last_name, phone_number, referal_code, role, banned, is_bot, created_at, updated_at
		FROM users
		WHERE is_bot = true
		ORDER BY RANDOM()
		LIMIT $1
	`
	rows, err := r.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list bots: %w", err)
	}
	defer rows.Close()

	bots := make([]*domain.User, 0)
	for rows.Next() {
		user := &domain.User{}
		var lastName sql.NullString
		if err := rows.Scan(
			&user.ID,
			&user.TelegramID,
			&user.FirstName,
			&lastName,
			&user.PhoneNumber,
			&user.ReferalCode,
			&user.Role,
			&user.Banned,
			&user.IsBot,
			&user.CreatedAt,
			&user.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan bot: %w", err)
		}
		if lastName.Valid {
			user.LastName = &lastName.String
		}
		bots = append(bots, user)
	}
	return bots, rows.Err()
}

// CountBots returns how many bot accounts exist.
func (r *botRepository) CountBots(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE is_bot = true`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count bots: %w", err)
	}
	return count, nil
}

// CountRealPlayersInGame counts distinct non-bot users still active in a game.
func (r *botRepository) CountRealPlayersInGame(ctx context.Context, gameID uuid.UUID) (int, error) {
	return r.countPlayersInGame(ctx, gameID, false)
}

// CountBotsInGame counts distinct bot users still active in a game.
func (r *botRepository) CountBotsInGame(ctx context.Context, gameID uuid.UUID) (int, error) {
	return r.countPlayersInGame(ctx, gameID, true)
}

// SecondsSinceFirstRealPlayer returns the age, in seconds, of the earliest
// still-active real player's join. The bool is false when the game has no real
// players, which callers must treat as "not eligible" rather than "zero".
//
// The reference time is passed in from the APPLICATION rather than taken from
// the database's now(). That is deliberate: game_players.joined_at is
// `timestamp without time zone` and is written by the app (see
// gameRepository.AddPlayer), so it carries the app's wall clock with the offset
// discarded. Comparing it against the database's now() therefore measures the
// gap between two different clocks — on a host in EAT (UTC+3) against a UTC
// Postgres, every age came out about -10800s, which would have held the bots
// back forever. Passing the app's own clock keeps both sides of the
// subtraction consistent no matter what timezone either process runs in.
func (r *botRepository) SecondsSinceFirstRealPlayer(ctx context.Context, gameID uuid.UUID) (float64, bool, error) {
	query := `
		SELECT EXTRACT(EPOCH FROM ($2::timestamp - MIN(gp.joined_at)))
		FROM game_players gp
		JOIN users u ON u.id = gp.user_id
		WHERE gp.game_id = $1 AND gp.left_at IS NULL AND u.is_bot = false
	`
	var secs sql.NullFloat64
	if err := r.db.QueryRowContext(ctx, query, gameID, time.Now()).Scan(&secs); err != nil {
		return 0, false, fmt.Errorf("failed to age first real player: %w", err)
	}
	if !secs.Valid {
		return 0, false, nil // no real players in this game
	}
	return secs.Float64, true, nil
}

func (r *botRepository) countPlayersInGame(ctx context.Context, gameID uuid.UUID, isBot bool) (int, error) {
	query := `
		SELECT COUNT(DISTINCT gp.user_id)
		FROM game_players gp
		JOIN users u ON u.id = gp.user_id
		WHERE gp.game_id = $1 AND gp.left_at IS NULL AND u.is_bot = $2
	`
	var count int
	if err := r.db.QueryRowContext(ctx, query, gameID, isBot).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to count players: %w", err)
	}
	return count, nil
}

// GetConfig returns the single policy row (id = 1).
func (r *botRepository) GetConfig(ctx context.Context) (*domain.BotConfig, error) {
	query := `
		SELECT enabled, min_real_players, target_bots, tiers, updated_at
		FROM bot_config
		WHERE id = 1
	`
	cfg := &domain.BotConfig{}
	err := r.db.QueryRowContext(ctx, query).Scan(
		&cfg.Enabled,
		&cfg.MinRealPlayers,
		&cfg.TargetBots,
		&cfg.Tiers,
		&cfg.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		// Row missing (e.g. migration not yet applied) — fall back to safe
		// defaults with auto-fill OFF rather than erroring.
		return &domain.BotConfig{Enabled: false, MinRealPlayers: 1, TargetBots: 30, Tiers: "REGULAR,VIP"}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read bot config: %w", err)
	}
	return cfg, nil
}

// UpdateConfig persists the single policy row, creating it if absent.
func (r *botRepository) UpdateConfig(ctx context.Context, cfg *domain.BotConfig) error {
	query := `
		INSERT INTO bot_config (id, enabled, min_real_players, target_bots, tiers, updated_at)
		VALUES (1, $1, $2, $3, $4, CURRENT_TIMESTAMP)
		ON CONFLICT (id) DO UPDATE SET
			enabled = EXCLUDED.enabled,
			min_real_players = EXCLUDED.min_real_players,
			target_bots = EXCLUDED.target_bots,
			tiers = EXCLUDED.tiers,
			updated_at = CURRENT_TIMESTAMP
	`
	if _, err := r.db.ExecContext(ctx, query, cfg.Enabled, cfg.MinRealPlayers, cfg.TargetBots, cfg.Tiers); err != nil {
		return fmt.Errorf("failed to update bot config: %w", err)
	}
	return nil
}
