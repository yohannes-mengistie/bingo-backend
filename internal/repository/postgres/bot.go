package postgres

import (
	"context"
	"database/sql"
	"fmt"

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

// ListBots returns bot users (is_bot = true), oldest first, up to limit.
func (r *botRepository) ListBots(ctx context.Context, limit int) ([]*domain.User, error) {
	query := `
		SELECT id, telegram_id, first_name, last_name, phone_number, referal_code, role, banned, is_bot, created_at, updated_at
		FROM users
		WHERE is_bot = true
		ORDER BY created_at ASC
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
