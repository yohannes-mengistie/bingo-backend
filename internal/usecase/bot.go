package usecase

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/bingo/backend/internal/domain"
	redisGame "github.com/bingo/backend/pkg/redis"
	"github.com/bingo/backend/pkg/referral"
	"github.com/google/uuid"
)

// botTelegramIDBase is the offset for synthetic bot Telegram IDs. Real Telegram
// IDs are positive, so large negative IDs can never collide with a real user.
const botTelegramIDBase = -1_000_000_000

// botDisplayNames are Telegram-style display names used for house-controlled
// filler bots. They intentionally mirror the mixed formatting real Telegram
// users often choose, because these names are visible in winner overlays and
// recent-winner feeds. Keep this list in sync with migration
// 048_bot_telegram_display_names.sql so existing and newly seeded bots match.
var botDisplayNames = []string{
	"`N`a`t`i", "b e k i .", "𝑩", "b e k k k", "𝘚𝘰𝘭𝘪𝘵𝘶𝘥𝘦",
	"God's property", "_m.a.k.i_", "B e k a", "ኪያ", "B a b i",
	"`Y`a`d`i", "n a t i .", "• 𝘕 •", "n a t i i i i", "𝘌𝘤𝘭𝘪𝘱𝘴𝘦",
	"𝘠𝘢 𝘈𝘭𝘭𝘢𝘩", "✞ 𝘕 𝘢 𝘵 𝘪 ✞", "E n j a", "ማክ", "M a m u s h",
	"`F`i`k`i`r`", "y a b s", "♔ 𝘚 ♔", "e d u u u", "𝘗𝘦𝘢𝘤𝘦",
	"His", "☾ 𝘉 ☽", "A b e t", "ኑ", "K i k i",
	"`A`b`i`", "m a k i _", "𝙅 .", "h a n i i", "𝘊𝘩𝘢𝘰𝘴",
	"𝘎𝘳𝘢𝘤𝘦", "☠︎︎ 𝘚 𝘢 𝘮 ☠︎︎", "T e w", "ቤካ", "M i m i",
	"`H`a`b`i", "e d u", "𝒦", "y a b s s s", "𝘓𝘰𝘯𝘦𝘳",
	"𝘈𝘮𝘦𝘯", "❀ 𝘌 𝘥 𝘶 ❀", "M i n", "ሳሚ", "N a n i",
	"`M`e`l`a", "h a n i .", "𝕯", "s a m m y y", "𝘝𝘪𝘣𝘦𝘴",
	"𝘊𝘩𝘰𝘴𝘦𝘯", "✰ 𝘒 𝘪 𝘳 𝘢 ✰", "A w o", "ናቲ", "T u t i",
	"`T`s`e`d`a`", "s a m i", "𝔐", "m a k k k", "𝘈𝘶𝘳𝘢",
	"𝘎𝘰𝘴𝘱𝘦𝘭", "• 𝘺 𝘢 𝘣 𝘴 •", "I s h i", "ሃኒ", "J i j i",
	"`B`e`k`i`", "b e t i .", "ℋ", "a b i i i", "𝘔𝘰𝘰𝘯𝘤𝘩𝘪𝘭𝘥",
	"𝘑𝘦𝘴𝘶𝘴", "~ 𝘮 𝘪 𝘬 𝘪 ~", "K i y a", "ኤዱ", "F i f i",
	"`R`o`b`i", "c h a l a", "𝒴", "f i k i r r r", "𝘚𝘰𝘶𝘭",
	"𝘍𝘢𝘪𝘵𝘩", "[ 𝘩 𝘢 𝘯 𝘪 ]", "E r e", "ዮኒ", "C h u c h u",
	"`D`a`w`i`t", "m u n i", "ℰ", "l i l i i", "𝘉𝘳𝘰𝘬𝘦𝘯",
	"𝘔𝘢𝘳𝘺'𝘴", "{ 𝘣 𝘦 𝘵 𝘪 }", "G i d a", "ዳኒ", "D i d i",
	"`Y`o`n`i`", "n a m i", "♚ 𝘼 ♚", "d a n i i", "𝘍𝘢𝘥𝘦𝘥",
	"𝘈𝘭𝘩𝘢𝘮𝘥𝘶𝘭𝘪𝘭𝘭𝘢𝘩", "♔ 𝘫 𝘰 ♔", "E b a k", "ፍቅር", "B o b o",
	"`K`a`l`e`b", "r o z i", "✞ 𝘛 ✞", "r o b b", "𝘎𝘩𝘰𝘴𝘵",
	"𝘚𝘢𝘣𝘳", "☼ 𝘴 𝘰 𝘭 ☼", "T i k", "ሰላም", "M o m o",
	"`F`e`n`a", "m e r o n", "𝘙", "j e r r y y", "𝘚𝘪𝘭𝘦𝘯𝘤𝘦",
	"𝘛𝘢𝘸𝘢𝘬𝘬𝘶𝘭", "✧ 𝘦 𝘻 𝘪 ✧", "F e n", "ፀጋ", "J o j o",
	"`E`z`i`", "y e r u s", "𝘓", "t u t u u", "𝘌𝘮𝘱𝘵𝘺",
	"𝘉𝘭𝘦𝘴𝘴𝘦𝘥", "♤ 𝘥 𝘢 𝘯 ♤", "L e m e n", "ጌታ", "N o n o",
	"`B`r`u`k", "m i n a", "𝘗", "e z i i i", "𝘝𝘰𝘪𝘥",
	"𝘙𝘦𝘥𝘦𝘦𝘮𝘦𝘥", "♧ 𝘳 𝘰 𝘣 ♧", "M a n", "ራህመት", "P i p i",
	"`S`a`f`i", "t s e d", "𝘊", "f i o o o", "𝘕𝘰𝘵𝘩𝘪𝘯𝘨",
	"𝘚𝘢𝘷𝘦𝘥", "♢ 𝘧 𝘪 𝘬 ♢", "H u l", "ጁማ", "T o t o",
	"`E`l`u", "y o f e", "𝘝", "s o l l", "𝘚𝘩𝘢𝘥𝘰𝘸",
	"𝘏𝘰𝘭𝘺", "♡ 𝘭 𝘪 𝘥 ♡", "C h i l", "ህይወት", "L o l o",
	"`J`e`r`i", "a b i . .", "𝘡", "b a b b", "𝘉𝘭𝘪𝘴𝘴",
	"𝘗𝘳𝘢𝘺", "♛ 𝘯 𝘢 ♛", "W e y", "ተስፋ", "Y o y o",
	"`R`e`d`i", "m e z e", "𝘎", "g a r i i", "𝘚𝘦𝘳𝘦𝘯𝘪𝘵𝘺",
	"𝘓𝘰𝘳𝘥", "⚡︎ 𝘬 𝘢 𝘭 ⚡︎", "G e d", "ብርሃን", "G o g o",
	"`B`o`g`i", "z e d", "𝘍", "n o a h h", "𝘓𝘰𝘴𝘵",
	"𝘊𝘩𝘳𝘪𝘴𝘵", "⚓︎ 𝘣 𝘳 𝘶 ⚓︎", "Z i m", "ቃል", "S h u s h u",
	"`M`i`k`i", "j a p p y", "𝘛", "s i d d", "𝘛𝘪𝘳𝘦𝘥",
	"𝘚𝘢𝘪𝘯𝘵", "☁︎ 𝘴 𝘦 𝘭 ☁︎", "B e s m", "እምነት", "K u k u",
	"`S`e`l`i", "l e l a", "𝘞", "y e m i i", "𝘌𝘯𝘪𝘨𝘮𝘢",
	"𝘗𝘦𝘢𝘤𝘦", "✈︎ 𝘮 𝘦 𝘭 ✈︎", "K e f", "ፀሀይ", "M a c a",
	"`L`i`n`a", "e b a", "𝘘", "f a r r", "𝘈𝘣𝘺𝘴𝘴",
	"𝘔𝘦𝘳𝘤𝘺", "✌︎ 𝘵 𝘴 𝘦 ✌︎", "A r e", "ጨረቃ", "P a p a",
	"`M`i`k`y", "t o k a", "𝘟", "k a l l", "𝘔𝘪𝘳𝘢𝘨𝘦",
	"𝘏𝘦𝘢𝘷𝘦𝘯", "✍︎ 𝘢 𝘣 ✍︎", "B e q", "ኪያዬ", "D a d a",
	"`D`a`g`i", "j o c c h a", "𝘠", "k i r a a", "𝘌𝘤𝘩𝘰",
	"𝘈𝘯𝘨𝘦𝘭", "☘︎ 𝘧 𝘢 ☘︎", "E n d e", "ሰማይ", "C o c o",
	"`F`a`s`i`l", "f r a y", "𝘖", "l u c c", "𝘊𝘩𝘪𝘭𝘭",
	"𝘗𝘳𝘰𝘱𝘩𝘦𝘵", "☂︎ 𝘨 𝘢 ☂︎", "D e r", "ምድር", "Z i z i",
	"`H`e`n`i", "h i w i", "𝘐", "t i t i i", "𝘋𝘢𝘸𝘯",
	"𝘕𝘰𝘶𝘳", "☕︎ 𝘫 𝘦 ☕︎", "T e l", "ዝምታ", "R i r i",
	"`A`m`u", "m a c k", "𝘜", "a l x x", "𝘋𝘶𝘴𝘬",
	"𝘋𝘦𝘦𝘯", "♈︎ 𝘢 𝘮 ♈︎", "M e c h", "እውነት", "V i v i",
	"`T`a`r`i", "y o d i", "𝘒", "m a r i i", "𝘓𝘶𝘯𝘢",
	"𝘎𝘭𝘰𝘳𝘺", "☯︎ 𝘳 𝘦 ☯︎", "S e w", "ፍቅር☥", "L i l i",
	"`N`a`h`o`m`", "n e n a .", "𝘏 .", "y u m i i i", "𝘚𝘵𝘢𝘳 .",
	"𝘡𝘪𝘰𝘯", "☽ 𝘺 𝘰 ☾", "F a r", "ኑሮ .", "W u w u",
}

// BotSettings holds the operator-tunable knobs supplied from config/env.
type BotSettings struct {
	PoolSize        int           // how many bot accounts to seed
	WalletFloat     float64       // balance each bot wallet is topped up to
	MaxJoinsPerTick int           // bots added per game per sweep (spaces out joins)
	JoinDelay       time.Duration // hold bots back this long after the first real player joins
	CheckInterval   time.Duration // how often the auto-filler sweeps
	WinRate         float64       // probability (0-1) that bots win when they have a valid bingo alongside humans
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
	if req.WinRate != nil {
		if *req.WinRate < 0 || *req.WinRate > 1 {
			return nil, fmt.Errorf("win_rate must be between 0 and 1")
		}
		cfg.WinRate = *req.WinRate
	}
	if req.BotAlwaysWin != nil {
		cfg.BotAlwaysWin = *req.BotAlwaysWin
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

	name := botDisplayNames[(index-1)%len(botDisplayNames)]
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

// perGameTarget varies the per-game bot count so rooms differ round to round —
// a fixed count in every game (always exactly 50) is itself a tell — but NEVER
// below the configured target. cfgTarget is the FLOOR; each game adds up to ~30%
// more on top. Derived from the game id, so it is stable across sweeps of the
// same game (the target can't wobble tick to tick) while differing between games.
// Range is [cfgTarget, cfgTarget*1.3].
func (uc *BotUseCase) perGameTarget(gameID uuid.UUID, cfgTarget int) int {
	if cfgTarget <= 0 {
		return 0
	}
	extra := float64(cfgTarget) * 0.30 * (float64(gameID[1]) / 255.0) // 0 .. 30% of target
	t := cfgTarget + int(math.Round(extra))
	if t < domain.MinPlayers {
		t = domain.MinPlayers
	}
	// Never target more bots than the pool holds — otherwise the room can never
	// reach the target (it runs out of distinct bots) and the count just stalls.
	// So configuring the target AT the pool size defeats the upward variation:
	// leave headroom (e.g. target 200 with a 300 pool) for counts to vary.
	if uc.settings.PoolSize > 0 && t > uc.settings.PoolSize {
		t = uc.settings.PoolSize
	}
	return t
}

// desiredBotsNow returns how many bots SHOULD be present in this game right now.
// Instead of rushing to the full target and then sitting frozen (the count hits
// 50 at countdown-20s and never moves — obviously fake), it seeds just enough to
// cross MinPlayers and kick off the countdown, then RAMPS the count up across the
// countdown, reaching the (per-game, varied) target a little before 0s, with a
// small bursty wobble. The sweeper turns this into per-tick joins:
// need = desiredBotsNow - botsInGame.
//
// remaining is the countdown time left (only meaningful when inCountdown). It is
// derived from the Redis countdown, stored as a Unix epoch, so it is immune to
// the app-vs-Postgres timezone skew that makes DB timestamps unusable for
// app-side clock math here.
func (uc *BotUseCase) desiredBotsNow(gameID uuid.UUID, cfgTarget int, inCountdown bool, remaining time.Duration) int {
	target := uc.perGameTarget(gameID, cfgTarget)
	if target <= 0 {
		return 0
	}

	if !inCountdown {
		// Pre-countdown: seed just enough to cross MinPlayers and start it; the
		// rest arrive paced during the countdown.
		if target < domain.MinPlayers {
			return target
		}
		return domain.MinPlayers
	}

	if remaining <= 0 {
		return target
	}
	// progress 0→1 across the countdown. Reach the full target a touch early
	// (÷0.85) so the count isn't still visibly climbing in the final seconds.
	progress := 1 - remaining.Seconds()/domain.CountdownDuration.Seconds()
	if progress < 0 {
		progress = 0
	}
	scaled := progress / 0.85
	if scaled > 1 {
		scaled = 1
	}
	desired := int(math.Ceil(float64(target) * scaled))
	desired += rand.Intn(3) - 1 // -1..+1: arrivals come in small bursts, not a line
	if desired < domain.MinPlayers {
		desired = domain.MinPlayers // never below the starter that kicked the countdown
	}
	if desired > target {
		desired = target
	}
	return desired
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
			// Pace toward a per-game, varied target instead of snapping to a fixed
			// TargetBots — see desiredBotsNow. This is what makes the room fill
			// gradually over the countdown and land on a different count each game.
			// Countdown time left comes from Redis (epoch-based), not the DB
			// timestamps, which are in a different timezone frame here.
			inCountdown := game.State == domain.GameStateCountdown
			var remaining time.Duration
			if inCountdown && uc.gameState != nil {
				if end, gerr := uc.gameState.GetCountdown(ctx, game.ID); gerr == nil {
					remaining = time.Until(end)
				} else {
					remaining = domain.CountdownDuration // key missing → treat as fresh
				}
			}
			need := uc.desiredBotsNow(game.ID, cfg.TargetBots, inCountdown, remaining) - botPlayers
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
