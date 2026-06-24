package domain

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
)

// UserRepository defines the interface for user data operations
type UserRepository interface {
	Create(ctx context.Context, tx *sql.Tx, user *User) error
	FindByTelegramID(ctx context.Context, telegramID int64) (*User, error)
	FindByPhone(ctx context.Context, phone string) (*User, error)
	FindByReferralCode(ctx context.Context, referralCode string) (*User, error)
	FindByID(ctx context.Context, id uuid.UUID) (*User, error)
	FindAll(ctx context.Context, limit, offset int) ([]*User, error)
	SetAdminCredentialsByTelegramID(ctx context.Context, telegramID int64, hashedPassword string) error
	Update(ctx context.Context, user *User) error
	UpdateRole(ctx context.Context, id uuid.UUID, role string) error
	SetAdminCredentialsByID(ctx context.Context, id uuid.UUID, hashedPassword string) error
	SetBanned(ctx context.Context, id uuid.UUID, banned bool) error
	CountAll(ctx context.Context) (int, error)
}

// WalletRepository defines the interface for wallet data operations
type WalletRepository interface {
	Create(ctx context.Context, tx *sql.Tx, wallet *Wallet) error
	FindByUserID(ctx context.Context, userID uuid.UUID) (*Wallet, error)
	UpdateBalance(ctx context.Context, tx *sql.Tx, userID uuid.UUID, amount float64) error
	LockForUpdate(ctx context.Context, tx *sql.Tx, userID uuid.UUID) (*Wallet, error)
	Update(ctx context.Context, wallet *Wallet) error
	GetTotalBalance(ctx context.Context) (float64, error)
}

// TransactionRepository defines the interface for transaction data operations
type TransactionRepository interface {
	Create(ctx context.Context, tx *sql.Tx, transaction *Transaction) error
	FindByID(ctx context.Context, id uuid.UUID) (*Transaction, error)
	FindByUserID(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*Transaction, error)
	FindByUserIDAndType(ctx context.Context, userID uuid.UUID, transactionType TransactionType, limit int) ([]*Transaction, error)
	FindByUserIDAndTypes(ctx context.Context, userID uuid.UUID, transactionTypes []TransactionType, limit int) ([]*Transaction, error)
	FindByStatusAndType(ctx context.Context, status TransactionStatus, transactionType TransactionType, limit, offset int) ([]*Transaction, error)
	FindByStatus(ctx context.Context, status TransactionStatus, limit, offset int) ([]*Transaction, error)
	FindByTypes(ctx context.Context, transactionTypes []TransactionType, limit, offset int) ([]*Transaction, error)
	FindAll(ctx context.Context, limit, offset int) ([]*Transaction, error)
	UpdateStatus(ctx context.Context, tx *sql.Tx, id uuid.UUID, status TransactionStatus) error
	CountByStatusAndType(ctx context.Context, status TransactionStatus, transactionType TransactionType) (int, error)
	ExistsActiveDepositByTransactionID(ctx context.Context, transactionID string) (bool, error)
	CountAll(ctx context.Context) (int, error)
}

// GameHistoryEntry represents a game with user's participation details
type GameHistoryEntry struct {
	Game         *Game      `json:"game"`
	CardID       int        `json:"card_id"`
	IsEliminated bool       `json:"is_eliminated"`
	JoinedAt     time.Time  `json:"joined_at"`
	LeftAt       *time.Time `json:"left_at,omitempty"`
	IsWinner     bool       `json:"is_winner"`
}

// GameRepository defines the interface for game data operations
type GameRepository interface {
	Create(ctx context.Context, game *Game) error
	FindByID(ctx context.Context, id uuid.UUID) (*Game, error)
	FindAvailable(ctx context.Context, gameType *GameType, limit int) ([]*Game, error)
	// FindAll returns games filtered by optional state and type, newest first.
	FindAll(ctx context.Context, state *GameState, gameType *GameType, limit, offset int) ([]*Game, error)
	// CountAll counts games matching the optional state and type filters.
	CountAll(ctx context.Context, state *GameState, gameType *GameType) (int, error)
	Update(ctx context.Context, game *Game) error
	// LockForUpdate locks a game row FOR UPDATE inside a transaction. Used to
	// serialize force-cancel against winner claims and double-cancels.
	LockForUpdate(ctx context.Context, tx *sql.Tx, id uuid.UUID) (*Game, error)
	// UpdateTx updates a game row inside an existing transaction.
	UpdateTx(ctx context.Context, tx *sql.Tx, game *Game) error
	// GetActivePlayersTx returns players still in the game (left_at IS NULL),
	// read inside an existing transaction.
	GetActivePlayersTx(ctx context.Context, tx *sql.Tx, gameID uuid.UUID) ([]*GamePlayer, error)
	// ClaimWinner atomically marks the game FINISHED with the winner, but only if
	// it is still DRAWING. Returns true only for the single claim that succeeds.
	ClaimWinner(ctx context.Context, tx *sql.Tx, gameID, winnerID uuid.UUID) (bool, error)
	AddPlayer(ctx context.Context, tx *sql.Tx, player *GamePlayer) error
	// RemovePlayerCard marks one specific card (game_id, user_id, card_id) as
	// left. A player may hold several cards, so leaving is per-card.
	RemovePlayerCard(ctx context.Context, tx *sql.Tx, gameID, userID uuid.UUID, cardID int) error
	// FindPlayer returns any one active card row for the user (used to test
	// membership / reconnect). Prefer FindPlayersByUser when all cards matter.
	FindPlayer(ctx context.Context, gameID, userID uuid.UUID) (*GamePlayer, error)
	// FindPlayersByUser returns all of a user's active card rows in a game.
	FindPlayersByUser(ctx context.Context, gameID, userID uuid.UUID) ([]*GamePlayer, error)
	// FindPlayerCard returns the user's row for one specific card, if active.
	FindPlayerCard(ctx context.Context, gameID, userID uuid.UUID, cardID int) (*GamePlayer, error)
	// CountActiveCardsForUser counts a user's active cards in a game (cap check).
	CountActiveCardsForUser(ctx context.Context, gameID, userID uuid.UUID) (int, error)
	// CountDistinctPlayers counts distinct active users in a game (start rule).
	CountDistinctPlayers(ctx context.Context, gameID uuid.UUID) (int, error)
	GetPlayers(ctx context.Context, gameID uuid.UUID) ([]*GamePlayer, error)
	// EliminatePlayerCard eliminates one specific card after a wrong claim; the
	// player's other cards keep playing.
	EliminatePlayerCard(ctx context.Context, tx *sql.Tx, gameID, userID uuid.UUID, cardID int) error
	GetTakenCards(ctx context.Context, gameID uuid.UUID) ([]int, error)
	SaveDrawnNumber(ctx context.Context, gameID uuid.UUID, letter string, number int) error
	FindGamesByUserID(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*GameHistoryEntry, error)
	CountGamesByType(ctx context.Context) (map[GameType]int, error)
	GetTotalHouseCut(ctx context.Context) (float64, error)
	// FindRecentWinners returns the most recently finished games that had a
	// winner, with the winner's display name and prize, for the lobby feed.
	FindRecentWinners(ctx context.Context, limit int) ([]*RecentWinner, error)
}
