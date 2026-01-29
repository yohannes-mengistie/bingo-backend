package domain

import "time"

// Game Configuration Constants

// MinPlayers is the minimum number of players required to start a game
const MinPlayers = 2

// HouseCut is the percentage of each bet that goes to the house (5%)
const HouseCut = 0.05

// CountdownDuration is the duration of the countdown before a game starts
const CountdownDuration = 60 * time.Second

// CountdownTickerInterval is the interval at which countdown updates are sent
const CountdownTickerInterval = 1 * time.Second

// Card Configuration Constants

// MinCardID is the minimum valid card ID
const MinCardID = 1

// MaxCardID is the maximum valid card ID
const MaxCardID = 100

// TotalCards is the total number of available bingo cards
const TotalCards = 100

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
