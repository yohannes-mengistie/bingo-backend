package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// BotConfig is the single-row policy (bot_config table) that drives the
// automatic filler. It is read every sweep and edited from the admin dashboard.
type BotConfig struct {
	Enabled        bool      `json:"enabled" db:"enabled"`                   // master auto-fill switch
	MinRealPlayers int       `json:"min_real_players" db:"min_real_players"` // FLOOR: start adding bots once a game has at least this many real players (1 = as soon as one joins). No upper ceiling.
	TargetBots     int       `json:"target_bots" db:"target_bots"`           // add bots until the game holds this many
	Tiers          string    `json:"tiers" db:"tiers"`                       // comma-separated game types to fill, e.g. "REGULAR,VIP"
	UpdatedAt      time.Time `json:"updated_at" db:"updated_at"`
}

// TierList splits the stored CSV into game types, skipping blanks.
func (c BotConfig) TierList() []GameType {
	out := make([]GameType, 0, 2)
	start := 0
	s := c.Tiers + ","
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			part := s[start:i]
			// trim spaces
			for len(part) > 0 && part[0] == ' ' {
				part = part[1:]
			}
			for len(part) > 0 && part[len(part)-1] == ' ' {
				part = part[:len(part)-1]
			}
			if part != "" {
				out = append(out, GameType(part))
			}
			start = i + 1
		}
	}
	return out
}

// UpdateBotConfigRequest is the admin dashboard payload to change the policy.
// Pointers so an admin can update a single field without resetting the others.
type UpdateBotConfigRequest struct {
	Enabled        *bool   `json:"enabled,omitempty"`
	MinRealPlayers *int    `json:"min_real_players,omitempty"`
	TargetBots     *int    `json:"target_bots,omitempty"`
	Tiers          *string `json:"tiers,omitempty"`
}

// AddBotsRequest is the admin dashboard payload to manually inject bots into one
// game. Count is how many bots to add (capped by the config target and by free
// cards).
type AddBotsRequest struct {
	Count int `json:"count" binding:"required,min=1"`
}

// BotFillResult reports the outcome of a manual or automatic fill of one game.
type BotFillResult struct {
	GameID      uuid.UUID `json:"game_id"`
	Requested   int       `json:"requested"`
	Added       int       `json:"added"`
	RealPlayers int       `json:"real_players"`
	BotPlayers  int       `json:"bot_players"`
}

// BotRepository serves bot-specific reads and the auto-fill policy. Kept
// separate from UserRepository so the money engine and existing interfaces are
// untouched.
type BotRepository interface {
	// ListBots returns bot users in random order, up to limit. Callers must not
	// rely on a stable order — it varies per call so lobbies vary per round.
	ListBots(ctx context.Context, limit int) ([]*User, error)
	// CountBots returns how many bot accounts exist.
	CountBots(ctx context.Context) (int, error)
	// CountRealPlayersInGame counts distinct non-bot users still active in a game.
	CountRealPlayersInGame(ctx context.Context, gameID uuid.UUID) (int, error)
	// CountBotsInGame counts distinct bot users still active in a game.
	CountBotsInGame(ctx context.Context, gameID uuid.UUID) (int, error)
	// GetConfig returns the single policy row.
	GetConfig(ctx context.Context) (*BotConfig, error)
	// UpdateConfig persists the policy row.
	UpdateConfig(ctx context.Context, cfg *BotConfig) error
}
