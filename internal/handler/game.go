package handler

import (
	"fmt"
	"net/http"

	"github.com/bingo/backend/internal/domain"
	"github.com/bingo/backend/internal/usecase"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type GameHandler struct {
	gameUseCase *usecase.GameUseCase
}

// NewGameHandler creates a new game handler
func NewGameHandler(gameUseCase *usecase.GameUseCase) *GameHandler {
	return &GameHandler{
		gameUseCase: gameUseCase,
	}
}

// GetGames handles GET /games?type={betAmount}
// Returns available games. If no games exist for a game type, creates one automatically.
func (h *GameHandler) GetGames(c *gin.Context) {
	var req domain.GetGamesRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid query parameters",
			"details": err.Error(),
		})
		return
	}

	// Get available games
	games, err := h.gameUseCase.GetAvailableGames(c.Request.Context(), req.GameType)
	if err != nil {
		// Log the error for debugging
		fmt.Printf("[GetGames] Error getting available games (type: %v): %v\n", req.GameType, err)
		// Ensure we send a proper JSON response even if there's an error
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to get available games",
			"details": err.Error(),
		})
		return
	}

	// If filtering by game type and no games found, create one
	if req.GameType != nil && len(games) == 0 {
		game, err := h.gameUseCase.CreateOrGetGame(c.Request.Context(), *req.GameType)
		if err != nil {
			fmt.Printf("[GetGames] Error creating/getting game (type: %v): %v\n", *req.GameType, err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to create or get game",
				"details": err.Error(),
			})
			return
		}
		games = []*domain.Game{game}
	}

	c.JSON(http.StatusOK, gin.H{
		"games": games,
	})
}

// JoinGame handles POST /games/:gameId/join
func (h *GameHandler) JoinGame(c *gin.Context) {
	gameIDStr := c.Param("gameId")
	gameID, err := uuid.Parse(gameIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid game ID",
		})
		return
	}

	var req domain.JoinGameRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request data",
			"details": err.Error(),
		})
		return
	}

	player, err := h.gameUseCase.JoinGame(c.Request.Context(), gameID, req)
	if err != nil {
		statusCode := http.StatusInternalServerError
		if err.Error() == "game is not accepting new players" ||
			err.Error() == "user is already in this game" ||
			err.Error() == "card is already taken" ||
			err.Error() == "insufficient balance" {
			statusCode = http.StatusBadRequest
		}

		c.JSON(statusCode, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"player": player,
	})
}

// LeaveGame handles POST /games/:gameId/leave
func (h *GameHandler) LeaveGame(c *gin.Context) {
	gameIDStr := c.Param("gameId")
	gameID, err := uuid.Parse(gameIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid game ID",
		})
		return
	}

	var req domain.LeaveGameRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request data",
			"details": err.Error(),
		})
		return
	}

	if err := h.gameUseCase.LeaveGame(c.Request.Context(), gameID, req); err != nil {
		statusCode := http.StatusInternalServerError
		if err.Error() == "user is not in this game" ||
			err.Error() == "cannot leave during drawing phase" {
			statusCode = http.StatusBadRequest
		}

		c.JSON(statusCode, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Successfully left the game",
	})
}

// ClaimBingo handles POST /games/:gameId/bingo
func (h *GameHandler) ClaimBingo(c *gin.Context) {
	gameIDStr := c.Param("gameId")
	gameID, err := uuid.Parse(gameIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid game ID",
		})
		return
	}

	var req domain.ClaimBingoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request data",
			"details": err.Error(),
		})
		return
	}

	isWinner, err := h.gameUseCase.ClaimBingo(c.Request.Context(), gameID, req)
	if err != nil {
		statusCode := http.StatusInternalServerError
		if err.Error() == "game is not in drawing phase" ||
			err.Error() == "user is not in this game" ||
			err.Error() == "player is already eliminated" {
			statusCode = http.StatusBadRequest
		}

		c.JSON(statusCode, gin.H{
			"error": err.Error(),
		})
		return
	}

	if isWinner {
		c.JSON(http.StatusOK, gin.H{
			"winner":  true,
			"message": "Congratulations! You won!",
		})
	} else {
		c.JSON(http.StatusOK, gin.H{
			"winner":  false,
			"message": "Invalid bingo claim. You have been eliminated.",
		})
	}
}

// GetGameState handles GET /games/:gameId/state
func (h *GameHandler) GetGameState(c *gin.Context) {
	gameIDStr := c.Param("gameId")
	gameID, err := uuid.Parse(gameIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid game ID",
		})
		return
	}

	game, drawnNumbers, takenCards, err := h.gameUseCase.GetGameState(c.Request.Context(), gameID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"game":         game,
		"drawnNumbers": drawnNumbers,
		"takenCards":   takenCards,
	})
}

// GetCardData handles GET /cards/:cardId
// Returns the bingo card data (5x5 grid) for a given card ID
func (h *GameHandler) GetCardData(c *gin.Context) {
	cardIDStr := c.Param("cardId")

	// Parse card ID
	var cardID int
	if _, err := fmt.Sscanf(cardIDStr, "%d", &cardID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid card ID",
		})
		return
	}

	card, err := h.gameUseCase.GetCardData(c.Request.Context(), cardID)
	if err != nil {
		statusCode := http.StatusBadRequest
		if err.Error() != "card ID must be between 1 and 100" {
			statusCode = http.StatusInternalServerError
		}

		c.JSON(statusCode, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"card": card,
	})
}
