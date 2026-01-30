package domain

import (
	"time"

	"github.com/google/uuid"
)

// GameType represents the type of game (G1-G7)
type GameType string

const (
	GameTypeG1 GameType = "G1" // Bet: 5
	GameTypeG2 GameType = "G2" // Bet: 7
	GameTypeG3 GameType = "G3" // Bet: 10
	GameTypeG4 GameType = "G4" // Bet: 20
	GameTypeG5 GameType = "G5" // Bet: 50
	GameTypeG6 GameType = "G6" // Bet: 100
	GameTypeG7 GameType = "G7" // Bet: 200
)

// GetBetAmount returns the bet amount for a game type
func (gt GameType) GetBetAmount() float64 {
	switch gt {
	case GameTypeG1:
		return BetAmountG1
	case GameTypeG2:
		return BetAmountG2
	case GameTypeG3:
		return BetAmountG3
	case GameTypeG4:
		return BetAmountG4
	case GameTypeG5:
		return BetAmountG5
	case GameTypeG6:
		return BetAmountG6
	case GameTypeG7:
		return BetAmountG7
	default:
		return 0
	}
}

// GameState represents the state of a game
type GameState string

const (
	GameStateWaiting   GameState = "WAITING"
	GameStateCountdown GameState = "COUNTDOWN"
	GameStateDrawing   GameState = "DRAWING"
	GameStateFinished  GameState = "FINISHED"
	GameStateClosed    GameState = "CLOSED"
	GameStateCancelled GameState = "CANCELLED"
)

// BingoLetter represents the column letter in BINGO
type BingoLetter string

const (
	BingoLetterB BingoLetter = "B"
	BingoLetterI BingoLetter = "I"
	BingoLetterN BingoLetter = "N"
	BingoLetterG BingoLetter = "G"
	BingoLetterO BingoLetter = "O"
)

// Game represents a bingo game instance
type Game struct {
	ID            uuid.UUID `json:"id" db:"id"`
	GameType      GameType  `json:"game_type" db:"game_type"`
	State         GameState `json:"state" db:"state"`
	BetAmount     float64   `json:"bet_amount" db:"bet_amount"`
	MinPlayers    int       `json:"min_players" db:"min_players"`
	PlayerCount   int       `json:"player_count" db:"player_count"`
	PrizePool     float64   `json:"prize_pool" db:"prize_pool"`
	HouseCut      float64   `json:"house_cut" db:"house_cut"`
	WinnerID      *uuid.UUID `json:"winner_id,omitempty" db:"winner_id"`
	CountdownEnds *time.Time `json:"countdown_ends,omitempty" db:"countdown_ends"`
	StartedAt     *time.Time `json:"started_at,omitempty" db:"started_at"`
	FinishedAt    *time.Time `json:"finished_at,omitempty" db:"finished_at"`
	CreatedAt     time.Time `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time `json:"updated_at" db:"updated_at"`
}

// GamePlayer represents a player in a game
type GamePlayer struct {
	ID        uuid.UUID  `json:"id" db:"id"`
	GameID    uuid.UUID  `json:"game_id" db:"game_id"`
	UserID    uuid.UUID  `json:"user_id" db:"user_id"`
	CardID    int        `json:"card_id" db:"card_id"` // 1-100
	IsEliminated bool    `json:"is_eliminated" db:"is_eliminated"`
	JoinedAt  time.Time  `json:"joined_at" db:"joined_at"`
	LeftAt    *time.Time `json:"left_at,omitempty" db:"left_at"`
}

// BingoCard represents a 5x5 bingo card
type BingoCard struct {
	ID     int       `json:"id"`     // 1-100
	Numbers [5][5]int `json:"numbers"` // 5x5 grid
}

// DrawnNumber represents a number drawn in the game
type DrawnNumber struct {
	Letter BingoLetter `json:"letter"`
	Number int         `json:"number"`
	DrawnAt time.Time  `json:"drawn_at"`
}

// JoinGameRequest represents the request to join a game
type JoinGameRequest struct {
	UserID uuid.UUID `json:"user_id" binding:"required"`
	CardID int       `json:"card_id" binding:"required,min=1,max=100"` // min=MinCardID, max=MaxCardID (see constants.go)
}

// LeaveGameRequest represents the request to leave a game
type LeaveGameRequest struct {
	UserID uuid.UUID `json:"user_id" binding:"required"`
}

// ClaimBingoRequest represents the request to claim bingo
type ClaimBingoRequest struct {
	UserID uuid.UUID `json:"user_id" binding:"required"`
	MarkedNumbers []int `json:"marked_numbers" binding:"required"` // Array of marked number positions (0-24)
}

// GetGamesRequest represents the request to get available games
type GetGamesRequest struct {
	GameType *GameType `form:"type"` // Optional filter by game type
}

