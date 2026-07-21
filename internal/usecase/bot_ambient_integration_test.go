//go:build integration

package usecase

import (
	"context"
	"testing"

	"github.com/bingo/backend/internal/domain"
	"github.com/bingo/backend/internal/repository/postgres"
	redisPkg "github.com/bingo/backend/pkg/redis"
)

// botUC builds a BotUseCase wired to the same live Postgres + Redis the game
// harness uses, so the auto-sweeper runs against real JoinGame/countdown logic.
// targetBots is how full a game should get; the pool is seeded a little larger
// so distinct bots are always available.
func (h *harness) botUC(targetBots int) (*BotUseCase, domain.BotRepository) {
	h.t.Helper()
	botRepo := postgres.NewBotRepository(h.db)
	uc := NewBotUseCase(
		botRepo,
		postgres.NewUserRepository(h.db),
		postgres.NewWalletRepository(h.db),
		postgres.NewTransactionRepository(h.db),
		postgres.NewGameRepository(h.db),
		h.uc,              // real GameUseCase → real JoinGame path
		h.uc.redisService, // same GameStateService the browse marker is written to
		h.db,
		BotSettings{PoolSize: targetBots + 3, WalletFloat: 1000, MaxJoinsPerTick: 10},
	)
	if err := uc.EnsureBotPool(context.Background(), targetBots+3); err != nil {
		h.t.Fatalf("seed bot pool: %v", err)
	}
	// Enable bots and allow bot-only games (MinRealPlayers = 0).
	if err := botRepo.UpdateConfig(context.Background(), &domain.BotConfig{
		Enabled: true, MinRealPlayers: 0, TargetBots: targetBots, Tiers: "REGULAR",
	}); err != nil {
		h.t.Fatalf("set bot config: %v", err)
	}
	return uc, botRepo
}

// The whole point of the recently-browsed throttle: a game with zero real
// players is filled ONLY while a real player has recently opened that tier's
// lobby. With no browse marker, the sweep must leave the empty game alone so it
// idles (e.g. overnight) instead of running bot-only games forever.
func TestIntegration_Ambient_ThrottledWhenNotBrowsed(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()
	ctx := context.Background()

	uc, botRepo := h.botUC(3)
	gameID := h.seedWaitingGame() // REGULAR, 0 players

	// Ensure the tier is NOT marked as recently browsed.
	if err := h.rdb.Del(ctx, redisPkg.LobbyActivityKey("REGULAR")).Err(); err != nil {
		t.Fatalf("clear activity key: %v", err)
	}

	uc.sweep(ctx)

	got, err := botRepo.CountBotsInGame(ctx, gameID)
	if err != nil {
		t.Fatalf("count bots: %v", err)
	}
	if got != 0 {
		t.Fatalf("expected 0 bots in an un-browsed empty game, got %d", got)
	}
}

// The counterpart: once a real player has browsed the tier (the marker exists),
// the sweep seeds bots into the empty game so a visitor sees an active lobby.
func TestIntegration_Ambient_FillsAfterBrowse(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()
	ctx := context.Background()

	uc, botRepo := h.botUC(3)
	gameID := h.seedWaitingGame()

	// A real player just opened the REGULAR lobby.
	if err := h.uc.redisService.MarkTierBrowsed(ctx, "REGULAR", domain.LobbyActivityWindow); err != nil {
		t.Fatalf("mark browsed: %v", err)
	}

	uc.sweep(ctx)

	got, err := botRepo.CountBotsInGame(ctx, gameID)
	if err != nil {
		t.Fatalf("count bots: %v", err)
	}
	if got != 3 {
		t.Fatalf("expected 3 bots after a browse, got %d", got)
	}
	// No real player joined, yet the game reached the start threshold on bots
	// alone — proving a bot-only game can run.
	if got < domain.MinPlayers {
		t.Fatalf("bot-only game did not reach MinPlayers (%d): got %d", domain.MinPlayers, got)
	}
}

// FillGame (the admin "add bots" button) must KEEP the classic guard: it never
// seeds a game with zero real players, regardless of the browse marker. Only the
// automatic sweeper opts into bot-only games.
func TestIntegration_Ambient_ManualFillStillGuarded(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()
	ctx := context.Background()

	uc, botRepo := h.botUC(3)
	gameID := h.seedWaitingGame()

	// Even with the tier marked browsed, the guarded public entry point refuses.
	if err := h.uc.redisService.MarkTierBrowsed(ctx, "REGULAR", domain.LobbyActivityWindow); err != nil {
		t.Fatalf("mark browsed: %v", err)
	}

	res, err := uc.FillGame(ctx, gameID, 3)
	if err != nil {
		t.Fatalf("FillGame: %v", err)
	}
	if res.Added != 0 {
		t.Fatalf("FillGame must not seed a zero-real game, added %d", res.Added)
	}
	got, _ := botRepo.CountBotsInGame(ctx, gameID)
	if got != 0 {
		t.Fatalf("expected 0 bots after guarded FillGame, got %d", got)
	}
}
