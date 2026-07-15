package domain

import "time"

// Game Configuration Constants

// MinPlayers is the minimum number of players required to start a game
const MinPlayers = 2

// HouseCut is the percentage of each bet that goes to the house (20%)
const HouseCut = 0.2

// CountdownDuration is the duration of the countdown before a game starts
const CountdownDuration = 40 * time.Second

// CountdownTickerInterval is the interval at which countdown updates are sent
const CountdownTickerInterval = 1 * time.Second

// Card Configuration Constants

// MinCardID is the minimum valid card ID
const MinCardID = 1

// MaxCardID is the maximum valid card ID
const MaxCardID = 200

// TotalCards is the total number of available bingo cards
const TotalCards = 200

// MaxCardsPerPlayer is the maximum number of cards a single player may hold in
// one game. Each card costs one full bet (e.g. 4 cards on a 10-birr table = 40).
const MaxCardsPerPlayer = 4

// MinWithdrawalAmount is the smallest withdrawal a player may request (birr).
const MinWithdrawalAmount = 10.0

// MaxDailyWithdrawal caps total withdrawals per Ethiopian calendar day (birr).
const MaxDailyWithdrawal = 2000.0

// MinBalanceAfterWithdrawal is the balance a player must keep after withdrawing.
const MinBalanceAfterWithdrawal = 50.0

// CardGridSize is the size of the bingo card grid (5x5)
const CardGridSize = 5

// WebSocket Configuration Constants

// WebSocketReadDeadline is the read deadline for WebSocket connections
const WebSocketReadDeadline = 60 * time.Second

// WebSocketPingInterval is the interval at which ping messages are sent to keep the connection alive
const WebSocketPingInterval = 54 * time.Second

// Bingo Number Range Constants

// BingoNumberRangeB is the range for column B (1-15)
const (
	BingoNumberMinB = 1
	BingoNumberMaxB = 15
)

// BingoNumberRangeI is the range for column I (16-30)
const (
	BingoNumberMinI = 16
	BingoNumberMaxI = 30
)

// BingoNumberRangeN is the range for column N (31-45)
const (
	BingoNumberMinN = 31
	BingoNumberMaxN = 45
)

// BingoNumberRangeG is the range for column G (46-60)
const (
	BingoNumberMinG = 46
	BingoNumberMaxG = 60
)

// BingoNumberRangeO is the range for column O (61-75)
const (
	BingoNumberMinO = 61
	BingoNumberMaxO = 75
)

// Card Position Constants

// CardTotalPositions is the total number of positions on a bingo card (5x5 = 25)
const CardTotalPositions = CardGridSize * CardGridSize

// CardCenterValue is the value that should be in the center cell (free space)
const CardCenterValue = 0

// CardCenterRow is the row index of the center cell (0-indexed)
const CardCenterRow = 2

// CardCenterCol is the column index of the center cell (0-indexed)
const CardCenterCol = 2

// NumbersPerLetter is the maximum number of numbers per bingo letter (15)
const NumbersPerLetter = 15

// Game Type Bet Amount Constants

// BetAmountRegular is the bet amount for the standard game (10 birr)
const BetAmountRegular = 10.0

// BetAmountVIP is the bet amount for the VIP game (50 birr)
const BetAmountVIP = 50.0

// WebSocket Event Name Constants

// WebSocketEventPlayerJoined is the event name when a player joins a game
const WebSocketEventPlayerJoined = "PLAYER_JOINED"

// WebSocketEventPlayerLeft is the event name when a player leaves a game
const WebSocketEventPlayerLeft = "PLAYER_LEFT"

// WebSocketEventGameStatus is the event name for game status updates
const WebSocketEventGameStatus = "GAME_STATUS"

// WebSocketEventCountdown is the event name for countdown updates
const WebSocketEventCountdown = "COUNTDOWN"

// WebSocketEventNumberDrawn is the event name when a number is drawn
const WebSocketEventNumberDrawn = "NUMBER_DRAWN"

// WebSocketEventWinner is the event name when a winner is declared
const WebSocketEventWinner = "WINNER"

// WebSocketEventPlayerEliminated is the event name when a player is eliminated
const WebSocketEventPlayerEliminated = "PLAYER_ELIMINATED"

// WebSocketEventNewGameAvailable is the event name when a new game becomes available
const WebSocketEventNewGameAvailable = "NEW_GAME_AVAILABLE"

// WebSocketEventInitialState is the event name for the initial game state
const WebSocketEventInitialState = "INITIAL_STATE"

// Game Query Constants

// MaxAvailableGamesLimit is the maximum number of available games to return in a query
const MaxAvailableGamesLimit = 50

// Drawing Constants

// DrawInterval is the interval between drawing numbers
const DrawInterval = 3000 * time.Millisecond

// Empty-game cleanup: how often the sweeper runs, and how long an empty
// WAITING/COUNTDOWN game (0 players) may sit untouched before it is cancelled
// and dropped from the lobby/active list. The grace period gives a freshly
// spawned or just-served lobby game time to get its first player.
const EmptyGameCleanupInterval = 60 * time.Second
const EmptyGameGracePeriod = 120 * time.Second

// WebSocket Handler Constants

// WebSocketInitialStateTimeout is the timeout for fetching initial state
const WebSocketInitialStateTimeout = 5 * time.Second

// User Constants

// DefaultUserBalance is the welcome credit given to a new user's wallet on
// signup. It must cover at least one regular game (BetAmountRegular) so a new
// player can play immediately without depositing first.
const DefaultUserBalance = 10.0

// MaxReferralCodeGenerationAttempts is the maximum number of attempts to generate a unique referral code
const MaxReferralCodeGenerationAttempts = 10

// Transaction History Constants

// DefaultTransactionHistoryLimit is the default limit for transaction history queries
const DefaultTransactionHistoryLimit = 10
