package handler

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/bingo/backend/internal/domain"
	"github.com/bingo/backend/internal/middleware"
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

	userID, ok := middleware.GetUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
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
	req.UserID = userID

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

	userID, ok := middleware.GetUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	var req domain.LeaveGameRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Leave has no required body fields; ignore empty-body bind errors.
		req = domain.LeaveGameRequest{}
	}
	req.UserID = userID

	if err := h.gameUseCase.LeaveGame(c.Request.Context(), gameID, req); err != nil {
		statusCode := http.StatusInternalServerError
		if err.Error() == "user is not in this game" ||
			err.Error() == "game is no longer active" {
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

	userID, ok := middleware.GetUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
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
	req.UserID = userID

	isWinner, err := h.gameUseCase.ClaimBingo(c.Request.Context(), gameID, req)
	if err != nil {
		statusCode := http.StatusInternalServerError
		if err.Error() == "game is not in drawing phase" ||
			err.Error() == "user is not in this game" ||
			err.Error() == "player is already eliminated" {
			statusCode = http.StatusBadRequest
		} else if err.Error() == "game already has a winner" {
			// Lost the race to another simultaneous valid claim.
			statusCode = http.StatusConflict
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

// GetPlayerInGame handles GET /games/:gameId/players/:userId
// Returns the player data if the user is in the game, or 404 if not found
func (h *GameHandler) GetPlayerInGame(c *gin.Context) {
	gameIDStr := c.Param("gameId")
	gameID, err := uuid.Parse(gameIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid game ID",
		})
		return
	}

	userIDStr := c.Param("userId")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid user ID",
		})
		return
	}

	player, err := h.gameUseCase.GetPlayerInGame(c.Request.Context(), gameID, userID)
	if err != nil {
		if err.Error() == "player not found" {
			c.JSON(http.StatusNotFound, gin.H{
				"player": nil,
			})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"player": player,
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
		if err.Error() != fmt.Sprintf("card ID must be between %d and %d", domain.MinCardID, domain.MaxCardID) {
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

// GetMyGameHistory handles GET /me/games — the authenticated user's game history.
func (h *GameHandler) GetMyGameHistory(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	limit := 10
	offset := 0
	if limitStr := c.Query("limit"); limitStr != "" {
		if parsedLimit := parseInt(limitStr); parsedLimit > 0 {
			limit = parsedLimit
		}
	}
	if offsetStr := c.Query("offset"); offsetStr != "" {
		if parsedOffset := parseInt(offsetStr); parsedOffset >= 0 {
			offset = parsedOffset
		}
	}

	history, err := h.gameUseCase.GetGameHistory(c.Request.Context(), userID, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch game history"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"games":  history,
		"count":  len(history),
		"limit":  limit,
		"offset": offset,
	})
}

// GetMyPlayerInGame handles GET /me/games/:gameId — whether the authenticated
// user is in the given game (returns the player record or 404).
func (h *GameHandler) GetMyPlayerInGame(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	gameID, err := uuid.Parse(c.Param("gameId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid game ID"})
		return
	}

	player, err := h.gameUseCase.GetPlayerInGame(c.Request.Context(), gameID, userID)
	if err != nil {
		if err.Error() == "player not found" {
			c.JSON(http.StatusNotFound, gin.H{"player": nil})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"player": player})
}

// AdminListGames handles GET /admin/games
// Lists games for the admin dashboard with optional ?state= and ?type= filters
// and ?limit=/?offset= pagination.
func (h *GameHandler) AdminListGames(c *gin.Context) {
	var filter domain.AdminGameFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid query parameters",
			"details": err.Error(),
		})
		return
	}

	limit := domain.MaxAvailableGamesLimit
	offset := 0
	if limitStr := c.Query("limit"); limitStr != "" {
		if parsed := parseInt(limitStr); parsed > 0 {
			limit = parsed
		}
	}
	if offsetStr := c.Query("offset"); offsetStr != "" {
		if parsed := parseInt(offsetStr); parsed >= 0 {
			offset = parsed
		}
	}

	games, total, err := h.gameUseCase.ListGames(c.Request.Context(), filter.State, filter.GameType, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"games":  games,
		"total":  total,
		"count":  len(games),
		"limit":  limit,
		"offset": offset,
	})
}

// AdminGetGame handles GET /admin/games/:gameId
// Returns a game plus its active players (with user info) for the admin view.
func (h *GameHandler) AdminGetGame(c *gin.Context) {
	gameID, err := uuid.Parse(c.Param("gameId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid game ID"})
		return
	}

	detail, err := h.gameUseCase.GetGameDetail(c.Request.Context(), gameID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, detail)
}

// AdminCancelGame handles POST /admin/games/:gameId/cancel
// Force-cancels a game and refunds every active player's stake.
func (h *GameHandler) AdminCancelGame(c *gin.Context) {
	gameID, err := uuid.Parse(c.Param("gameId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid game ID"})
		return
	}

	result, err := h.gameUseCase.CancelGame(c.Request.Context(), gameID)
	if err != nil {
		statusCode := http.StatusInternalServerError
		if strings.Contains(err.Error(), "already resolved") {
			statusCode = http.StatusBadRequest
		} else if strings.Contains(err.Error(), "game not found") {
			statusCode = http.StatusNotFound
		}
		c.JSON(statusCode, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":         "Game cancelled and stakes refunded",
		"game":            result.Game,
		"refunded_count":  result.RefundedCount,
		"refunded_amount": result.RefundedAmount,
	})
}

// GetGameHistory handles GET /games/user/:user_id/history
// Returns the game history for a user
func (h *GameHandler) GetGameHistory(c *gin.Context) {
	userIDStr := c.Param("user_id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid user ID",
		})
		return
	}

	// Parse query parameters for pagination
	limit := 10 // default limit
	offset := 0 // default offset

	if limitStr := c.Query("limit"); limitStr != "" {
		if parsedLimit := parseInt(limitStr); parsedLimit > 0 {
			limit = parsedLimit
		}
	}

	if offsetStr := c.Query("offset"); offsetStr != "" {
		if parsedOffset := parseInt(offsetStr); parsedOffset >= 0 {
			offset = parsedOffset
		}
	}

	history, err := h.gameUseCase.GetGameHistory(c.Request.Context(), userID, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to fetch game history",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"games":  history,
		"count":  len(history),
		"limit":  limit,
		"offset": offset,
	})
}
