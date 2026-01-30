package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/bingo/backend/config"
	"github.com/bingo/backend/internal/handler"
	"github.com/bingo/backend/internal/middleware"
	"github.com/bingo/backend/internal/repository/postgres"
	"github.com/bingo/backend/internal/usecase"
	"github.com/bingo/backend/pkg/jwt"
	redisPkg "github.com/bingo/backend/pkg/redis"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize database connection
	db, err := sql.Open("postgres", cfg.Database.GetDSN())
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Test database connection
	if err := db.Ping(); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}

	// Set connection pool settings for high concurrency
	db.SetMaxOpenConns(100)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(time.Hour)

	// Initialize Redis client
	redisClient, err := redisPkg.NewClient(cfg)
	if err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	defer redisClient.Close()

	// Initialize repositories
	userRepo := postgres.NewUserRepository(db)
	walletRepo := postgres.NewWalletRepository(db)
	transactionRepo := postgres.NewTransactionRepository(db)
	gameRepo := postgres.NewGameRepository(db)

	// Initialize Redis services
	gameStateService := redisPkg.NewGameStateService(redisClient.GetClient())

	// Initialize JWT service
	jwtService := jwt.NewService(cfg)

	// Initialize use cases
	userUseCase := usecase.NewUserUseCase(userRepo, walletRepo, db)
	walletUseCase := usecase.NewWalletUseCase(walletRepo, transactionRepo, userRepo, db)
	authUseCase := usecase.NewAuthUseCase(userRepo, jwtService)
	gameUseCase := usecase.NewGameUseCase(gameRepo, walletRepo, transactionRepo, userRepo, db, gameStateService)

	// Initialize handlers
	userHandler := handler.NewUserHandler(userUseCase)
	walletHandler := handler.NewWalletHandler(walletUseCase)
	authHandler := handler.NewAuthHandler(authUseCase)
	gameHandler := handler.NewGameHandler(gameUseCase)
	wsHandler := handler.NewWebSocketHandler(redisClient.GetClient(), gameStateService, gameUseCase)

	// Setup router
	router := setupRouter(userHandler, walletHandler, authHandler, gameHandler, wsHandler, jwtService)

	// Create HTTP server
	srv := &http.Server{
		Addr:         fmt.Sprintf("%s:%s", cfg.Server.Host, cfg.Server.Port),
		Handler:      router,
		ReadTimeout:  time.Duration(cfg.Server.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(cfg.Server.WriteTimeout) * time.Second,
		IdleTimeout:  time.Duration(cfg.Server.IdleTimeout) * time.Second,
	}

	// Start server in a goroutine
	go func() {
		log.Printf("Server starting on %s:%s", cfg.Server.Host, cfg.Server.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited")
}

func setupRouter(userHandler *handler.UserHandler, walletHandler *handler.WalletHandler, authHandler *handler.AuthHandler, gameHandler *handler.GameHandler, wsHandler *handler.WebSocketHandler, jwtService *jwt.Service) *gin.Engine {
	// Set Gin to release mode in production
	if os.Getenv("GIN_MODE") == "release" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()

	// CORS middleware
	router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:3000", "https://bingo-frontend-production-7ee9.up.railway.app", "https://winner.up.railway.app"},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Length", "Content-Type", "Authorization", "Upgrade", "Connection", "Sec-WebSocket-Key", "Sec-WebSocket-Version", "Sec-WebSocket-Extensions", "Sec-WebSocket-Protocol"},
		ExposeHeaders:    []string{"Content-Length", "Upgrade", "Connection", "Sec-WebSocket-Accept"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// Middleware
	router.Use(gin.Logger())
	router.Use(gin.Recovery())

	// Debug middleware for WebSocket routes
	router.Use(func(c *gin.Context) {
		if c.Request.URL.Path == "/api/v1/ws/game" || strings.HasPrefix(c.Request.URL.Path, "/api/v1/ws/game") {
			log.Printf("[Middleware] WebSocket request - Method: %s, Path: %s, Query: %s, Upgrade: %s, Connection: %s",
				c.Request.Method, c.Request.URL.Path, c.Request.URL.RawQuery,
				c.Request.Header.Get("Upgrade"), c.Request.Header.Get("Connection"))
		}
		c.Next()
	})

	// Health check endpoint
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
		})
	})

	// API routes
	api := router.Group("/api/v1")
	{
		// Public auth endpoint
		auth := api.Group("/auth")
		{
			auth.POST("/login", authHandler.Login)
		}

		// Public user endpoints (for bot access)
		user := api.Group("/user")
		{
			user.POST("/register", userHandler.Register)
			user.GET("/telegram/:telegram_id", userHandler.FindByTelegramID)
			user.GET("/phone", userHandler.FindByPhone)
			user.GET("/referral/:referral_code", userHandler.FindByReferralCode)
			user.PUT("/:user_id/name", userHandler.UpdateUserName)
		}

		// Public wallet endpoints (for bot access)
		wallet := api.Group("/wallet")
		{
			wallet.POST("/deposit", walletHandler.Deposit)
			wallet.POST("/withdraw", walletHandler.Withdraw)
			wallet.POST("/transfer", walletHandler.Transfer)
			wallet.GET("/telegram/:telegram_id", walletHandler.GetWalletByTelegramID)
			wallet.GET("/:user_id", walletHandler.GetWallet)
			wallet.GET("/:user_id/deposits", walletHandler.GetDepositHistory)
			wallet.GET("/:user_id/withdrawals", walletHandler.GetWithdrawalHistory)
			wallet.GET("/:user_id/transfers", walletHandler.GetTransferHistory)
		}

		// Public game endpoints
		games := api.Group("/games")
		{
			games.GET("", gameHandler.GetGames)
			games.GET("/user/:user_id/history", gameHandler.GetGameHistory)
			games.GET("/:gameId/state", gameHandler.GetGameState)
			games.POST("/:gameId/join", gameHandler.JoinGame)
			games.POST("/:gameId/leave", gameHandler.LeaveGame)
			games.POST("/:gameId/bingo", gameHandler.ClaimBingo)
		}

		// Public card endpoints
		cards := api.Group("/cards")
		{
			cards.GET("/:cardId", gameHandler.GetCardData)
		}

		// WebSocket endpoints
		// Connect by game type (public viewing): /ws/game?type=G5
		// Connect by game ID: /ws/game/:gameId
		// Note: Order matters - more specific route first
		// Also handle OPTIONS for CORS preflight
		api.OPTIONS("/ws/game/:gameId", func(c *gin.Context) {
			c.Header("Access-Control-Allow-Origin", "*")
			c.Header("Access-Control-Allow-Methods", "GET, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization, Upgrade, Connection, Sec-WebSocket-Key, Sec-WebSocket-Version")
			c.Status(http.StatusOK)
		})
		api.OPTIONS("/ws/game", func(c *gin.Context) {
			c.Header("Access-Control-Allow-Origin", "*")
			c.Header("Access-Control-Allow-Methods", "GET, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization, Upgrade, Connection, Sec-WebSocket-Key, Sec-WebSocket-Version")
			c.Status(http.StatusOK)
		})
		api.GET("/ws/game/:gameId", wsHandler.HandleWebSocket)
		api.GET("/ws/game", wsHandler.HandleWebSocket)

		// Protected admin endpoints
		admin := api.Group("/admin")
		admin.Use(middleware.AuthMiddleware(jwtService))
		admin.Use(middleware.AdminMiddleware())
		{
			// User management
			admin.GET("/users", userHandler.GetAllUsers)

			// Transaction queries
			transactions := admin.Group("/transactions")
			{
				transactions.GET("", walletHandler.GetAllTransactions)
				transactions.GET("/pending/deposits", walletHandler.GetPendingDeposits)
				transactions.GET("/pending/withdrawals", walletHandler.GetPendingWithdrawals)
				transactions.GET("/completed/deposits", walletHandler.GetCompletedDeposits)
				transactions.GET("/completed/withdrawals", walletHandler.GetCompletedWithdrawals)
				transactions.GET("/failed", walletHandler.GetFailedTransactions)
				transactions.GET("/transfers", walletHandler.GetTransferTransactions)
			}

			// Deposit operations
			admin.POST("/transactions/:id/approve-deposit", walletHandler.ApproveDeposit)
			admin.POST("/transactions/:id/reject-deposit", walletHandler.RejectDeposit)

			// Withdrawal operations
			admin.POST("/transactions/:id/approve-withdrawal", walletHandler.ApproveWithdrawal)
			admin.POST("/transactions/:id/reject-withdrawal", walletHandler.RejectWithdrawal)

			// General operations
			admin.POST("/transactions/:id/cancel", walletHandler.CancelTransaction)
		}
	}

	return router
}
