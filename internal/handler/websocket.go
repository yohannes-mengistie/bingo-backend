package handler

import (
	"context"
	"encoding/json"
	"fmt"
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

// isValidGameType checks if the game type is valid
func isValidGameType(gameType domain.GameType) bool {
	validTypes := []domain.GameType{
		domain.GameTypeG1,
		domain.GameTypeG2,
		domain.GameTypeG3,
		domain.GameTypeG4,
		domain.GameTypeG5,
		domain.GameTypeG6,
		domain.GameTypeG7,
	}
	for _, vt := range validTypes {
		if gameType == vt {
			return true
		}
	}
	return false
}

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
// Public viewing - anyone can connect by game type (e.g., ?type=G5) or game ID
func (h *WebSocketHandler) HandleWebSocket(c *gin.Context) {
	// Log request details for debugging
	log.Printf("[WebSocket] Request received - Method: %s, Path: %s, Query: %s, Headers: %v",
		c.Request.Method, c.Request.URL.Path, c.Request.URL.RawQuery, c.Request.Header)
	var gameID uuid.UUID
	var err error
	var errorReason string

	// Check if connecting by game type (public viewing) or game ID
	gameTypeStr := c.Query("type")
	gameIDParam := c.Param("gameId")

	log.Printf("[WebSocket] ===== Connection attempt ===== Path: %s, Query: %s, gameId param: '%s'",
		c.Request.URL.Path, c.Request.URL.RawQuery, gameIDParam)

	if gameTypeStr != "" {
		// Connect by game type - find or create an available game
		gameType := domain.GameType(gameTypeStr)
		if !isValidGameType(gameType) {
			errorReason = fmt.Sprintf("Invalid game type '%s'. Must be one of: G1, G2, G3, G4, G5, G6, G7", gameTypeStr)
			log.Printf("[WebSocket] ERROR: %s", errorReason)
			c.JSON(http.StatusBadRequest, gin.H{
				"error":  errorReason,
				"reason": errorReason,
			})
			return
		}

		// Find or create a game of this type
		if h.gameUseCase == nil {
			errorReason = "Game use case service is not initialized (nil pointer)"
			log.Printf("[WebSocket] ERROR: %s", errorReason)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":  errorReason,
				"reason": errorReason,
			})
			return
		}

		log.Printf("[WebSocket] Creating or getting game of type %s", gameType)
		game, err := h.gameUseCase.CreateOrGetGame(c.Request.Context(), gameType)
		if err != nil {
			errorReason = fmt.Sprintf("Failed to create or get game of type %s: %v", gameType, err)
			log.Printf("[WebSocket] ERROR: %s", errorReason)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":  errorReason,
				"reason": errorReason,
			})
			return
		}
		if game == nil {
			errorReason = fmt.Sprintf("Game service returned nil game for type %s", gameType)
			log.Printf("[WebSocket] ERROR: %s", errorReason)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":  errorReason,
				"reason": errorReason,
			})
			return
		}
		gameID = game.ID
		log.Printf("[WebSocket] ✓ Found/created game %s (type: %s, state: %s, players: %d)",
			gameID, gameType, game.State, game.PlayerCount)
	} else if gameIDParam != "" {
		// Connect by game ID (from path parameter)
		gameID, err = uuid.Parse(gameIDParam)
		if err != nil {
			errorReason = fmt.Sprintf("Invalid game ID format '%s': %v", gameIDParam, err)
			log.Printf("[WebSocket] ERROR: %s", errorReason)
			c.JSON(http.StatusBadRequest, gin.H{
				"error":  errorReason,
				"reason": errorReason,
			})
			return
		}
		log.Printf("[WebSocket] ✓ Parsed game ID: %s", gameID)
	} else {
		errorReason = "No game type or game ID provided. Use ?type=G5 or /ws/game/:gameId"
		log.Printf("[WebSocket] ERROR: %s", errorReason)
		c.JSON(http.StatusBadRequest, gin.H{
			"error":  errorReason,
			"reason": errorReason,
		})
		return
	}

	// Check Redis availability first (before upgrading connection)
	if h.redisClient == nil {
		errorReason = "Redis client is nil. WebSocket requires Redis for real-time updates."
		log.Printf("[WebSocket] ERROR: %s", errorReason)
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":  errorReason,
			"reason": errorReason,
		})
		return
	}

	// Verify Redis connection
	ctx := c.Request.Context()
	log.Printf("[WebSocket] Testing Redis connection...")
	if err := h.redisClient.Ping(ctx).Err(); err != nil {
		errorReason = fmt.Sprintf("Redis ping failed: %v. Check Redis connection and credentials.", err)
		log.Printf("[WebSocket] ERROR: %s", errorReason)
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":  errorReason,
			"reason": errorReason,
		})
		return
	}
	log.Printf("[WebSocket] ✓ Redis connection verified")

	gameIDStr := gameID.String()
	log.Printf("[WebSocket] All pre-checks passed. Attempting WebSocket upgrade for game %s", gameIDStr)

	// Upgrade connection
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		errorReason = fmt.Sprintf("WebSocket upgrade failed: %v. Check if connection is already upgraded or headers are correct.", err)
		log.Printf("[WebSocket] ERROR: %s", errorReason)
		// Try to send error response if connection not yet upgraded
		if !c.Writer.Written() {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":  errorReason,
				"reason": errorReason,
			})
		}
		return
	}
	log.Printf("[WebSocket] ✓ Connection upgraded successfully for game %s", gameIDStr)
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

	channel := redisPkg.GameChannel(gameIDStr)
	log.Printf("[WebSocket] Subscribing to Redis channel: %s", channel)
	pubsub := h.redisClient.Subscribe(ctx, channel)
	defer pubsub.Close()

	// Verify subscription
	_, err = pubsub.ReceiveTimeout(ctx, 2*time.Second)
	if err != nil {
		log.Printf("[WebSocket] Warning: Subscription verification failed: %v", err)
		// Continue anyway, subscription might still work
	} else {
		log.Printf("[WebSocket] Successfully subscribed to Redis channel")
	}

	// Send initial game state
	log.Printf("[WebSocket] Sending initial game state for game %s", gameIDStr)
	if err := h.sendInitialState(conn, gameID); err != nil {
		log.Printf("[WebSocket] ERROR: Failed to send initial state for game %s: %v", gameIDStr, err)
		return
	}
	log.Printf("[WebSocket] ✓ Initial state sent. Starting message loop for game %s", gameIDStr)

	// Channel for messages from Redis
	redisMessages := make(chan *redis.Message, 10)

	// Goroutine to receive Redis messages
	go func() {
		if pubsub == nil {
			log.Printf("[WebSocket] ERROR: PubSub is nil, cannot receive messages for game %s", gameIDStr)
			return
		}
		log.Printf("[WebSocket] Starting Redis message receiver for game %s", gameIDStr)
		for {
			msg, err := pubsub.ReceiveMessage(ctx)
			if err != nil {
				if err == context.Canceled {
					log.Printf("[WebSocket] Redis subscription context canceled for game %s", gameIDStr)
				} else {
					log.Printf("[WebSocket] Redis receive error for game %s: %v", gameIDStr, err)
				}
				return
			}
			log.Printf("[WebSocket] Received Redis message for game %s: %s", gameIDStr, msg.Payload)
			select {
			case redisMessages <- msg:
			case <-ctx.Done():
				log.Printf("[WebSocket] Context done, stopping Redis receiver for game %s", gameIDStr)
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
				log.Printf("[WebSocket] Error writing Redis message to client for game %s: %v", gameIDStr, err)
				return
			}
			log.Printf("[WebSocket] ✓ Forwarded Redis message to client for game %s", gameIDStr)

		case <-ticker.C:
			// Send ping
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Printf("[WebSocket] Error sending ping for game %s: %v", gameIDStr, err)
				return
			}

		default:
			// Check for client messages (read-only, but we handle pong)
			conn.SetReadDeadline(time.Now().Add(1 * time.Second))
			_, _, err := conn.ReadMessage()
			if err != nil {
				// Check if it's a timeout/deadline error (expected)
				if netErr, ok := err.(interface{ Timeout() bool }); ok && netErr.Timeout() {
					// Expected timeout, continue the loop
					continue
				}
				// Check if it's a WebSocket close error
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Printf("[WebSocket] Unexpected close error for game %s: %v", gameIDStr, err)
					return
				}
				// Check if connection was closed normally
				if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
					log.Printf("[WebSocket] Connection closed normally for game %s", gameIDStr)
					return
				}
				// Other errors - log and continue (might be temporary)
				log.Printf("[WebSocket] Read error for game %s (continuing): %v", gameIDStr, err)
				continue
			}
		}
	}
}

// sendInitialState sends the initial game state to the client
// Returns an error if the message cannot be sent
func (h *WebSocketHandler) sendInitialState(conn *websocket.Conn, gameID uuid.UUID) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Try Redis first, fallback to database
	var game *domain.Game
	var err error

	if h.gameService != nil && h.redisClient != nil {
		game, err = h.gameService.GetGameState(ctx, gameID)
	}

	// Fallback to database if Redis fails or not available
	if game == nil && h.gameUseCase != nil {
		game, _, _, err = h.gameUseCase.GetGameState(ctx, gameID)
	}

	if err != nil || game == nil {
		log.Printf("Warning: Could not get game state for %s: %v", gameID, err)
		// Send minimal state
		initialState := map[string]interface{}{
			"event": "INITIAL_STATE",
			"data": map[string]interface{}{
				"game":         nil,
				"drawnNumbers": []interface{}{},
				"takenCards":   []int{},
				"playerCount":  0,
				"secondsLeft":  0,
			},
		}
		data, err := json.Marshal(initialState)
		if err != nil {
			log.Printf("[WebSocket] ERROR: Failed to marshal minimal initial state for game %s: %v", gameID, err)
			return err
		}
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			log.Printf("[WebSocket] ERROR: Failed to write minimal initial state message for game %s: %v", gameID, err)
			return err
		}
		log.Printf("[WebSocket] ✓ Sent minimal initial state for game %s", gameID)
		return nil
	}

	var drawnNumbers []domain.DrawnNumber
	var takenCards []int
	var playerCount int64

	if h.gameService != nil && h.redisClient != nil {
		drawnNumbers, _ = h.gameService.GetDrawnNumbers(ctx, gameID)
		takenCards, _ = h.gameService.GetTakenCards(ctx, gameID)
		playerCount, _ = h.gameService.GetPlayerCount(ctx, gameID)
	} else if h.gameUseCase != nil {
		// Fallback to database
		_, drawnNumbers, takenCards, _ = h.gameUseCase.GetGameState(ctx, gameID)
		// Player count from game (game is already checked to be non-nil above)
		playerCount = int64(game.PlayerCount)
	}

	// Get countdown if in countdown state
	var secondsLeft int
	if game.State == domain.GameStateCountdown {
		if h.gameService != nil && h.redisClient != nil {
			countdownEnds, err := h.gameService.GetCountdown(ctx, gameID)
			if err == nil {
				secondsLeft = int(time.Until(countdownEnds).Seconds())
				if secondsLeft < 0 {
					secondsLeft = 0
				}
			}
		} else if game.CountdownEnds != nil {
			// Fallback to database
			secondsLeft = int(time.Until(*game.CountdownEnds).Seconds())
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

	data, err := json.Marshal(initialState)
	if err != nil {
		log.Printf("[WebSocket] ERROR: Failed to marshal initial state for game %s: %v", gameID, err)
		return err
	}
	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		log.Printf("[WebSocket] ERROR: Failed to write initial state message for game %s: %v", gameID, err)
		return err
	}
	log.Printf("[WebSocket] ✓ Initial state message sent successfully for game %s", gameID)
	return nil
}
