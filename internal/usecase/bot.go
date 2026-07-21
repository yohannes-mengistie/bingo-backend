package usecase

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"time"

	"github.com/bingo/backend/internal/domain"
	"github.com/bingo/backend/pkg/referral"
	redisGame "github.com/bingo/backend/pkg/redis"
	"github.com/google/uuid"
)

// botTelegramIDBase is the offset for synthetic bot Telegram IDs. Real Telegram
// IDs are positive, so large negative IDs can never collide with a real user.
const botTelegramIDBase = -1_000_000_000

// botFirstNames and botLastNames are combined to give filler bots plausible
// full names. The DB only requires telegram_id / phone / referral_code to be
// unique, so names are free to repeat — but they should not.
//
// Where bot names actually reach players: NOT the game room, which renders a
// bare participant count and no roster at all (bingo-frontend
// src/screens/GameRoom.tsx). They surface through winner_name — the winner
// overlay and the recent-winners list — built by winnerDisplayName
// (usecase/game.go) and the recent-winners query (repository/postgres/game.go),
// both of which emit "First Last" whenever a surname is set. Because bots
// outnumber real players by design, bots take most wins, so that list is
// largely bot names: with the old 30-entry first-name-only pool it repeated the
// same handful of names constantly. The admin game-detail page is the one place
// a full roster with names is shown.
//
// The two lengths are deliberately COPRIME (120 and 49 share no factor), which
// makes the pair (index%120, index%49) unique for the first 120*49 = 5880 bots
// by the Chinese remainder theorem, while both halves advance on every index.
// Keep them coprime if you edit these lists — appending one name to either can
// silently collapse the space (e.g. 120 and 50 share a factor and repeat every
// 600). A first name recurs only every 120 bots, which at a 300 pool means a
// handful of repeats paired with different surnames: realistic for Ethiopian
// given names, where common ones genuinely recur in any real crowd.
var botFirstNames = []string{
	"Abel", "Abenezer", "Abiy", "Addis", "Amanuel", "Anteneh", "Ashenafi",
	"Bereket", "Berhanu", "Biruk", "Bruk", "Chala", "Dagim", "Dagmawi",
	"Daniel", "Dawit", "Dereje", "Desalegn", "Elias", "Endale", "Ephrem",
	"Eyasu", "Eyob", "Ezra", "Fasil", "Fikru", "Fitsum", "Getachew", "Girma",
	"Habtamu", "Hailu", "Henok", "Kaleb", "Kalab", "Kebede", "Kidus",
	"Kirubel", "Leul", "Mekonnen", "Melaku", "Mesfin", "Michael", "Mulugeta",
	"Nahom", "Natnael", "Nebiyu", "Robel", "Samson", "Samuel", "Solomon",
	"Surafel", "Tadesse", "Tamirat", "Tesfaye", "Tewodros", "Tsegaye", "Yared",
	"Yeabsira", "Yohannes", "Yonas", "Zelalem", "Zerihun", "Alemayehu",
	"Bekele", "Belay", "Ermias", "Getnet", "Gizachew", "Kassahun", "Mengistu",
	"Sisay", "Wondwosen", "Yilma", "Abera", "Endalkachew", "Abraham", "Amare",
	"Asrat", "Assefa", "Ayele", "Bahiru", "Behailu", "Binyam", "Chalachew", "Dagne",
	"Damtew", "Demeke", "Endeshaw", "Eskinder", "Fekadu", "Feyisa", "Gebre", "Gemechu",
	"Getahun", "Habte", "Haile", "Kefyalew", "Lemma", "Mamo", "Melese",
	"Mekuria", "Mitiku", "Moges", "Muluken", "Negash", "Nigatu", "Nigus", "Petros",
	"Sahle", "Seyoum", "Shimelis", "Sintayehu", "Tariku", "Tekle", "Temesgen",
	"Teshome", "Tilahun", "Worku", "Yidnekachew", "Zenebe",
}

// Ethiopian surnames are patronymics — the father's given name — so the overlap
// with botFirstNames below is authentic, not an oversight.
var botLastNames = []string{
	"Tesfaye", "Bekele", "Alemu", "Girma", "Hailu", "Kebede", "Mengistu",
	"Assefa", "Tadesse", "Wolde", "Gebre", "Haile", "Desta", "Abebe",
	"Mulugeta", "Getachew", "Negash", "Teshome", "Worku", "Yimer", "Ayele",
	"Berhe", "Demissie", "Fikadu", "Gizaw", "Kassa", "Lemma", "Mamo",
	"Regassa", "Shiferaw", "Tilahun", "Wondimu", "Zeleke", "Abera", "Adugna",
	"Bayissa", "Emiru", "Feyisa", "Jemal", "Kumsa", "Melaku", "Nigussie",
	"Olana", "Tola", "Urgessa", "Chane", "Dida", "Sori", "Gonfa",
}

// BotSettings holds the operator-tunable knobs supplied from config/env.
type BotSettings struct {
	PoolSize        int           // how many bot accounts to seed
	WalletFloat     float64       // balance each bot wallet is topped up to
	MaxJoinsPerTick int           // bots added per game per sweep (spaces out joins)
	JoinDelay       time.Duration // hold bots back this long after the first real player joins
	CheckInterval   time.Duration // how often the auto-filler sweeps
}

// BotUseCase seeds a pool of house-owned filler bots and joins them into games
// that are short on real players. It reuses GameUseCase.JoinGame for every join,
// so all wallet locking, prize-pool math and payout logic are unchanged — a bot
// is just another player whose money belongs to the house.
type BotUseCase struct {
	botRepo         domain.BotRepository
	userRepo        domain.UserRepository
	walletRepo      domain.WalletRepository
	transactionRepo domain.TransactionRepository
	gameRepo        domain.GameRepository
	gameUC          *GameUseCase
	gameState       *redisGame.GameStateService // reads the per-tier "recently browsed" marker
	db              *sql.DB
	settings        BotSettings
}

// NewBotUseCase wires the bot use case.
func NewBotUseCase(
	botRepo domain.BotRepository,
	userRepo domain.UserRepository,
	walletRepo domain.WalletRepository,
	transactionRepo domain.TransactionRepository,
	gameRepo domain.GameRepository,
	gameUC *GameUseCase,
	gameState *redisGame.GameStateService,
	db *sql.DB,
	settings BotSettings,
) *BotUseCase {
	return &BotUseCase{
		botRepo:         botRepo,
		userRepo:        userRepo,
		walletRepo:      walletRepo,
		transactionRepo: transactionRepo,
		gameRepo:        gameRepo,
		gameUC:          gameUC,
		gameState:       gameState,
		db:              db,
		settings:        settings,
	}
}

// ---- Config passthrough (admin dashboard) ----

// GetConfig returns the current auto-fill policy.
func (uc *BotUseCase) GetConfig(ctx context.Context) (*domain.BotConfig, error) {
	return uc.botRepo.GetConfig(ctx)
}

// UpdateConfig applies a partial policy update from the admin dashboard.
func (uc *BotUseCase) UpdateConfig(ctx context.Context, req domain.UpdateBotConfigRequest) (*domain.BotConfig, error) {
	cfg, err := uc.botRepo.GetConfig(ctx)
	if err != nil {
		return nil, err
	}
	if req.Enabled != nil {
		cfg.Enabled = *req.Enabled
	}
	if req.MinRealPlayers != nil {
		if *req.MinRealPlayers < 0 {
			return nil, fmt.Errorf("min_real_players cannot be negative")
		}
		cfg.MinRealPlayers = *req.MinRealPlayers
	}
	if req.TargetBots != nil {
		if *req.TargetBots < 0 {
			return nil, fmt.Errorf("target_bots cannot be negative")
		}
		cfg.TargetBots = *req.TargetBots
	}
	if req.Tiers != nil {
		cfg.Tiers = *req.Tiers
	}
	if err := uc.botRepo.UpdateConfig(ctx, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// ---- Seeding ----

// SeedPool ensures the bot pool exists, defaulting to the configured pool size
// when count <= 0. Exposed to the admin dashboard.
func (uc *BotUseCase) SeedPool(ctx context.Context, count int) error {
	if count <= 0 {
		count = uc.settings.PoolSize
	}
	return uc.EnsureBotPool(ctx, count)
}

// EnsureBotPool makes sure `size` bot accounts exist (idempotent) and tops each
// bot wallet up to the configured float. Safe to call on every boot.
func (uc *BotUseCase) EnsureBotPool(ctx context.Context, size int) error {
	for i := 1; i <= size; i++ {
		telegramID := int64(botTelegramIDBase - i)

		existing, err := uc.userRepo.FindByTelegramID(ctx, telegramID)
		if err == nil && existing != nil {
			// Already seeded — just keep its wallet funded.
			if ferr := uc.fundBotWallet(ctx, existing.ID); ferr != nil {
				return fmt.Errorf("fund existing bot %d: %w", i, ferr)
			}
			continue
		}

		if err := uc.createBot(ctx, i, telegramID); err != nil {
			return fmt.Errorf("create bot %d: %w", i, err)
		}
	}
	return nil
}

// createBot inserts one bot user + a funded wallet in a single transaction.
func (uc *BotUseCase) createBot(ctx context.Context, index int, telegramID int64) error {
	code, err := uc.uniqueReferralCode(ctx)
	if err != nil {
		return err
	}

	name := botFirstNames[(index-1)%len(botFirstNames)]
	lastName := botLastNames[(index-1)%len(botLastNames)]
	phone := fmt.Sprintf("BOT-%08d", index)

	tx, err := uc.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	user := &domain.User{
		ID:          uuid.New(),
		TelegramID:  telegramID,
		FirstName:   name,
		LastName:    &lastName,
		PhoneNumber: phone,
		ReferalCode: code,
		Role:        "user",
		IsBot:       true,
	}
	if err := uc.userRepo.Create(ctx, tx, user); err != nil {
		return err
	}

	wallet := &domain.Wallet{UserID: user.ID, Balance: uc.settings.WalletFloat}
	if err := uc.walletRepo.Create(ctx, tx, wallet); err != nil {
		return err
	}

	// Record the house money injected to bankroll this bot.
	if err := uc.recordBotFunding(ctx, tx, user.ID, uc.settings.WalletFloat); err != nil {
		return err
	}

	return tx.Commit()
}

func (uc *BotUseCase) uniqueReferralCode(ctx context.Context) (string, error) {
	for i := 0; i < domain.MaxReferralCodeGenerationAttempts; i++ {
		code, err := referral.GenerateReferralCode()
		if err != nil {
			return "", fmt.Errorf("failed to generate referral code: %w", err)
		}
		if _, err := uc.userRepo.FindByReferralCode(ctx, code); err != nil {
			return code, nil // not found → free to use
		}
	}
	return "", fmt.Errorf("failed to generate unique referral code")
}

// fundBotWallet tops a bot wallet back up to the float if it has dropped below
// it, recording the injected house money. No-op when already at/above float.
func (uc *BotUseCase) fundBotWallet(ctx context.Context, userID uuid.UUID) error {
	tx, err := uc.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	wallet, err := uc.walletRepo.LockForUpdate(ctx, tx, userID)
	if err != nil {
		return err
	}
	topUp := uc.settings.WalletFloat - wallet.Balance
	if topUp <= 0 {
		return tx.Commit() // already funded
	}
	if err := uc.walletRepo.UpdateBalance(ctx, tx, userID, topUp); err != nil {
		return err
	}
	if err := uc.recordBotFunding(ctx, tx, userID, topUp); err != nil {
		return err
	}
	return tx.Commit()
}

func (uc *BotUseCase) recordBotFunding(ctx context.Context, tx *sql.Tx, userID uuid.UUID, amount float64) error {
	ref := "BOT_FUNDING"
	return uc.transactionRepo.Create(ctx, tx, &domain.Transaction{
		UserID:    userID,
		Type:      domain.TransactionTypeDeposit,
		Category:  domain.TransactionCategoryBotFunding,
		Amount:    amount,
		Status:    domain.TransactionStatusCompleted,
		Reference: &ref,
	})
}

// ---- Filling ----

// FillGame adds up to `requested` bots to one game, reusing JoinGame for each.
// It enforces the core rule (never join a game with zero real players) and
// respects free-card availability. Returns how many were actually added.
//
// This is the guarded, human-facing entry point (admin "add bots" button). The
// automatic sweeper calls fill directly and may pass allowEmpty when policy
// permits bot-only games — see sweep.
func (uc *BotUseCase) FillGame(ctx context.Context, gameID uuid.UUID, requested int) (*domain.BotFillResult, error) {
	return uc.fill(ctx, gameID, requested, false)
}

// fill is the shared implementation behind FillGame and the auto-sweeper. When
// allowEmpty is false it keeps the classic guard (never seed a game with zero
// real players); when true it may seed a bot-only game to keep the lobby alive.
func (uc *BotUseCase) fill(ctx context.Context, gameID uuid.UUID, requested int, allowEmpty bool) (*domain.BotFillResult, error) {
	game, err := uc.gameRepo.FindByID(ctx, gameID)
	if err != nil {
		return nil, fmt.Errorf("game not found: %w", err)
	}
	if game.State != domain.GameStateWaiting && game.State != domain.GameStateCountdown {
		return nil, fmt.Errorf("game is not accepting new players")
	}

	realPlayers, err := uc.botRepo.CountRealPlayersInGame(ctx, gameID)
	if err != nil {
		return nil, err
	}
	botPlayers, err := uc.botRepo.CountBotsInGame(ctx, gameID)
	if err != nil {
		return nil, err
	}

	result := &domain.BotFillResult{GameID: gameID, Requested: requested, RealPlayers: realPlayers, BotPlayers: botPlayers}

	// Guard: never let bots play a game with no real player in it — unless the
	// caller explicitly opted into bot-only games (allowEmpty).
	if !allowEmpty && realPlayers < 1 {
		return result, nil
	}

	// Which bots are already in this game, and which cards are taken.
	inGame, err := uc.playersInGame(ctx, gameID)
	if err != nil {
		return nil, err
	}
	freeCards, err := uc.freeCards(ctx, gameID)
	if err != nil {
		return nil, err
	}

	bots, err := uc.botRepo.ListBots(ctx, uc.settings.PoolSize*2+requested+10)
	if err != nil {
		return nil, err
	}

	added := 0
	for _, bot := range bots {
		if added >= requested || len(freeCards) == 0 {
			break
		}
		if inGame[bot.ID] {
			continue
		}

		// Ensure the bot can afford the stake; top up from the house if not.
		if err := uc.ensureAffordable(ctx, bot.ID, game.BetAmount); err != nil {
			continue // skip this bot, try the next
		}

		cardID := freeCards[len(freeCards)-1]
		freeCards = freeCards[:len(freeCards)-1]

		if _, err := uc.gameUC.JoinGame(ctx, gameID, domain.JoinGameRequest{UserID: bot.ID, CardID: cardID}); err != nil {
			// Card raced, game just started, etc. — stop if the game closed.
			if err.Error() == "game is not accepting new players" {
				break
			}
			continue
		}
		inGame[bot.ID] = true
		added++
	}

	result.Added = added
	result.BotPlayers = botPlayers + added
	return result, nil
}

// ensureAffordable tops a bot wallet up to the float if it cannot cover a stake.
// This is a best-effort pre-check; JoinGame re-verifies the balance under a row
// lock, so a race here only means the join is skipped, never an overdraft.
func (uc *BotUseCase) ensureAffordable(ctx context.Context, userID uuid.UUID, bet float64) error {
	wallet, err := uc.walletRepo.FindByUserID(ctx, userID)
	if err == nil && wallet.Balance >= bet {
		return nil
	}
	return uc.fundBotWallet(ctx, userID)
}

func (uc *BotUseCase) playersInGame(ctx context.Context, gameID uuid.UUID) (map[uuid.UUID]bool, error) {
	players, err := uc.gameRepo.GetPlayers(ctx, gameID)
	if err != nil {
		return nil, err
	}
	set := make(map[uuid.UUID]bool, len(players))
	for _, p := range players {
		set[p.UserID] = true
	}
	return set, nil
}

func (uc *BotUseCase) freeCards(ctx context.Context, gameID uuid.UUID) ([]int, error) {
	taken, err := uc.gameRepo.GetTakenCards(ctx, gameID)
	if err != nil {
		return nil, err
	}
	takenSet := make(map[int]bool, len(taken))
	for _, c := range taken {
		takenSet[c] = true
	}
	free := make([]int, 0, domain.MaxCardID)
	for c := domain.MinCardID; c <= domain.MaxCardID; c++ {
		if !takenSet[c] {
			free = append(free, c)
		}
	}
	// Shuffle so bots don't always grab the same cards.
	rand.Shuffle(len(free), func(i, j int) { free[i], free[j] = free[j], free[i] })
	return free, nil
}

// ---- Auto-fill sweeper ----

// joinDelayFor returns how long bots must hold off after the first real player
// sat down in this game.
//
// Without it the sweeper fills the moment someone joins, so a player watches
// five strangers appear within a second of picking their card — nobody arrives
// that promptly, and it is the clearest tell that the room is padded.
//
// The wait is jittered per game rather than fixed, because "exactly N seconds,
// every time" is itself a recognisable pattern to anyone who plays more than a
// few rounds. The jitter is derived from the game id, so it is stable across
// sweeps of the same game (the delay cannot wobble tick to tick) while
// differing between games. Range is [delay, delay*1.5).
func (uc *BotUseCase) joinDelayFor(gameID uuid.UUID) time.Duration {
	base := uc.settings.JoinDelay
	if base <= 0 {
		return 0
	}
	spread := float64(gameID[0]) / 256.0 // 0.0 .. ~1.0, fixed per game
	return base + time.Duration(float64(base)*0.5*spread)
}

// Run drives the automatic filler until ctx is cancelled. Each tick it reads the
// admin policy and, for every WAITING/COUNTDOWN game in the configured tiers
// that has at least one real player but fewer than min_real_players, adds bots
// toward target_bots — at most MaxJoinsPerTick per game per tick, so bots
// trickle in rather than appearing all at once.
func (uc *BotUseCase) Run(ctx context.Context) {
	ticker := time.NewTicker(uc.settings.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			uc.sweep(ctx)
		}
	}
}

func (uc *BotUseCase) sweep(ctx context.Context) {
	cfg, err := uc.botRepo.GetConfig(ctx)
	if err != nil || !cfg.Enabled || cfg.TargetBots <= 0 {
		return
	}

	for _, tier := range cfg.TierList() {
		t := tier
		games, err := uc.gameRepo.FindAvailable(ctx, &t, domain.MaxAvailableGamesLimit)
		if err != nil {
			continue
		}
		for _, game := range games {
			realPlayers, err := uc.botRepo.CountRealPlayersInGame(ctx, game.ID)
			if err != nil {
				continue
			}

			// MinRealPlayers is a FLOOR: only add bots once a game holds at least
			// this many real players. No upper ceiling — bots always top the game
			// up to TargetBots regardless of how many real players join. Set the
			// floor to 0 to let bots seed and run BOT-ONLY games (0 real players),
			// which keeps the lobby looking alive to attract visitors.
			if realPlayers < cfg.MinRealPlayers {
				continue
			}

			if realPlayers == 0 {
				// Bot-only game. Run these only while a real player has recently
				// browsed this tier, so the lobby stays alive around visitors and
				// quietly idles when nobody is around (e.g. overnight). Redis
				// unavailable or nothing browsed → skip and let the game idle.
				if uc.gameState == nil {
					continue
				}
				recent, err := uc.gameState.TierBrowsedRecently(ctx, string(tier))
				if err != nil || !recent {
					continue
				}
				// No real player to pace against, so joinDelayFor (keyed on the
				// first real arrival) does not apply — bots may seed immediately.
			} else {
				// Let the room breathe before anyone "arrives". See joinDelayFor.
				if delay := uc.joinDelayFor(game.ID); delay > 0 {
					age, hasReal, err := uc.botRepo.SecondsSinceFirstRealPlayer(ctx, game.ID)
					if err != nil || !hasReal {
						continue
					}
					if time.Duration(age*float64(time.Second)) < delay {
						continue // still too soon — try again on a later tick
					}
				}
			}

			botPlayers, err := uc.botRepo.CountBotsInGame(ctx, game.ID)
			if err != nil {
				continue
			}
			need := cfg.TargetBots - botPlayers
			if need <= 0 {
				continue
			}
			if need > uc.settings.MaxJoinsPerTick {
				need = uc.settings.MaxJoinsPerTick
			}
			// allowEmpty mirrors the bot-only decision above: a 0-real game only
			// reaches here when the throttle passed, so it may be seeded.
			if _, err := uc.fill(ctx, game.ID, need, realPlayers == 0); err != nil {
				continue
			}
		}
	}
}
