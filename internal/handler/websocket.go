package handler

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/bingo/backend/internal/domain"
	"github.com/bingo/backend/internal/usecase"
	redisPkg "github.com/bingo/backend/pkg/redis"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// Allow all origins for now (adjust for production)
		return true
	},
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type WebSocketHandler struct {
	redisClient *redis.Client
	gameService *redisPkg.GameStateService
	gameUseCase *usecase.GameUseCase                // Add game use case for database checks
	clients     map[string]map[*websocket.Conn]bool // gameID -> connections
	mu          sync.RWMutex
}

// NewWebSocketHandler creates a new WebSocket handler
func NewWebSocketHandler(redisClient *redis.Client, gameService *redisPkg.GameStateService, gameUseCase *usecase.GameUseCase) *WebSocketHandler {
	return &WebSocketHandler{
		redisClient: redisClient,
		gameService: gameService,
		gameUseCase: gameUseCase,
		clients:     make(map[string]map[*websocket.Conn]bool),
	}
}

// HandleWebSocket handles WebSocket connections for game updates
func (h *WebSocketHandler) HandleWebSocket(c *gin.Context) {
	gameIDStr := c.Param("gameId")
	gameID, err := uuid.Parse(gameIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid game ID",
		})
		return
	}

	// Get user ID from query or header (you may want to use JWT here)
	userIDStr := c.Query("user_id")
	if userIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "user_id is required",
		})
		return
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid user ID",
		})
		return
	}

	// Verify user is in the game
	// First try Redis, then fallback to database
	isInGame := false
	if h.gameService != nil && h.redisClient != nil {
		var err error
		isInGame, err = h.gameService.IsPlayerInGame(c.Request.Context(), gameID, userID)
		if err != nil {
			log.Printf("Warning: Redis check failed for user %s in game %s: %v", userID, gameID, err)
		}
	}

	// Fallback to database check if Redis check failed or Redis not available
	if !isInGame && h.gameUseCase != nil {
		// Check via game repository
		player, err := h.gameUseCase.GetPlayerInGame(c.Request.Context(), gameID, userID)
		if err == nil && player != nil {
			isInGame = true
		}
	}

	if !isInGame {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "User is not in this game. Please join the game first via POST /api/v1/games/:gameId/join",
		})
		return
	}

	// Upgrade connection
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	// Add client
	h.mu.Lock()
	if h.clients[gameIDStr] == nil {
		h.clients[gameIDStr] = make(map[*websocket.Conn]bool)
	}
	h.clients[gameIDStr][conn] = true
	h.mu.Unlock()

	// Remove client on disconnect
	defer func() {
		h.mu.Lock()
		delete(h.clients[gameIDStr], conn)
		if len(h.clients[gameIDStr]) == 0 {
			delete(h.clients, gameIDStr)
		}
		h.mu.Unlock()
	}()

	// Subscribe to Redis pub/sub (Redis is already verified above)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pubsub := h.redisClient.Subscribe(ctx, redisPkg.GameChannel(gameIDStr))
	defer pubsub.Close()

	// Send initial game state
	h.sendInitialState(conn, gameID)

	// Channel for messages from Redis
	redisMessages := make(chan *redis.Message, 10)

	// Goroutine to receive Redis messages
	go func() {
		if pubsub == nil {
			return
		}
		for {
			msg, err := pubsub.ReceiveMessage(ctx)
			if err != nil {
				return
			}
			select {
			case redisMessages <- msg:
			case <-ctx.Done():
				return
			}
		}
	}()

	// Set read deadline
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	// Main loop
	ticker := time.NewTicker(54 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case msg := <-redisMessages:
			// Forward Redis message to WebSocket
			if err := conn.WriteMessage(websocket.TextMessage, []byte(msg.Payload)); err != nil {
				return
			}

		case <-ticker.C:
			// Send ping
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}

		default:
			// Check for client messages (read-only, but we handle pong)
			conn.SetReadDeadline(time.Now().Add(1 * time.Second))
			_, _, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Printf("WebSocket error: %v", err)
				}
				return
			}
		}
	}
}

// sendInitialState sends the initial game state to the client
func (h *WebSocketHandler) sendInitialState(conn *websocket.Conn, gameID uuid.UUID) {
	if h.gameService == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	game, err := h.gameService.GetGameState(ctx, gameID)
	if err != nil {
		return
	}

	drawnNumbers, _ := h.gameService.GetDrawnNumbers(ctx, gameID)
	takenCards, _ := h.gameService.GetTakenCards(ctx, gameID)

	// Get player count
	playerCount, _ := h.gameService.GetPlayerCount(ctx, gameID)

	// Get countdown if in countdown state
	var secondsLeft int
	if game.State == domain.GameStateCountdown {
		countdownEnds, err := h.gameService.GetCountdown(ctx, gameID)
		if err == nil {
			secondsLeft = int(time.Until(countdownEnds).Seconds())
			if secondsLeft < 0 {
				secondsLeft = 0
			}
		}
	}

	initialState := map[string]interface{}{
		"event": "INITIAL_STATE",
		"data": map[string]interface{}{
			"game":         game,
			"drawnNumbers": drawnNumbers,
			"takenCards":   takenCards,
			"playerCount":  playerCount,
			"secondsLeft":  secondsLeft,
		},
	}

	data, _ := json.Marshal(initialState)
	conn.WriteMessage(websocket.TextMessage, data)
}
