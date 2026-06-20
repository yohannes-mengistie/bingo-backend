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
	"github.com/bingo/backend/pkg/telegram"
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
	walletUseCase := usecase.NewWalletUseCase(walletRepo, transactionRepo, userRepo, gameRepo, db)
	authUseCase := usecase.NewAuthUseCase(userRepo, jwtService, cfg.Admin.SecretCode, cfg.Telegram.BotToken)
	gameUseCase := usecase.NewGameUseCase(gameRepo, walletRepo, transactionRepo, userRepo, db, gameStateService)

	// Initialize handlers
	userHandler := handler.NewUserHandler(userUseCase)
	walletHandler := handler.NewWalletHandler(walletUseCase)
	authHandler := handler.NewAuthHandler(authUseCase)
	gameHandler := handler.NewGameHandler(gameUseCase)
	wsHandler := handler.NewWebSocketHandler(redisClient.GetClient(), gameStateService, gameUseCase)

	// Telegram bot: registration gateway + Mini App launcher (webhook-driven).
	telegramBot := telegram.NewBot(cfg.Telegram.BotToken)
	telegramHandler := handler.NewTelegramHandler(userUseCase, telegramBot, cfg.Telegram.WebhookSecret, cfg.Telegram.MiniAppURL)

	// Setup router
	router := setupRouter(userHandler, walletHandler, authHandler, gameHandler, wsHandler, telegramHandler, jwtService)

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

func setupRouter(userHandler *handler.UserHandler, walletHandler *handler.WalletHandler, authHandler *handler.AuthHandler, gameHandler *handler.GameHandler, wsHandler *handler.WebSocketHandler, telegramHandler *handler.TelegramHandler, jwtService *jwt.Service) *gin.Engine {
	// Set Gin to release mode in production
	if os.Getenv("GIN_MODE") == "release" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()

	// CORS middleware
	router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:3000", "http://localhost:3001", "http://localhost:3002", "https://bingo-frontend-production-7ee9.up.railway.app", "https://biruh-bingo-admin.vercel.app", "https://biruh-bingo-frontend.vercel.app", "https://winner.up.railway.app", "https://biruh-bingo-admin-production.up.railway.app", "https://biruh-bingo-frontend-production.up.railway.app", "https://bingo-miniapp-gold.vercel.app"},
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

	// Telegram bot webhook (set via the Bot API setWebhook method).
	router.POST("/telegram/webhook", telegramHandler.Webhook)

	// API routes
	api := router.Group("/api/v1")
	{
		// Public auth endpoints
		auth := api.Group("/auth")
		{
			auth.POST("/login", authHandler.Login)              // admin login (telegram_id + password)
			auth.POST("/create-admin", authHandler.CreateAdmin) // secret-code gated
			auth.POST("/telegram", authHandler.TelegramLogin)   // website: verify Mini App initData -> JWT
		}

		// Bot-facing user endpoints (server-to-server; called by the trusted Telegram bot)
		user := api.Group("/user")
		{
			user.POST("/register", userHandler.Register)
			user.GET("/telegram/:telegram_id", userHandler.FindByTelegramID)
			user.GET("/phone", userHandler.FindByPhone)
			user.GET("/referral/:referral_code", userHandler.FindByReferralCode)
		}

		// Bot-facing wallet reads (server-to-server)
		wallet := api.Group("/wallet")
		{
			wallet.GET("/telegram/:telegram_id", walletHandler.GetWalletByTelegramID)
			wallet.GET("/:user_id", walletHandler.GetWallet)
			wallet.GET("/:user_id/deposits", walletHandler.GetDepositHistory)
			wallet.GET("/:user_id/withdrawals", walletHandler.GetWithdrawalHistory)
			wallet.GET("/:user_id/transfers", walletHandler.GetTransferHistory)
		}

		// Public game reads (spectating / lobby — no auth needed)
		games := api.Group("/games")
		{
			games.GET("", gameHandler.GetGames)
			games.GET("/user/:user_id/history", gameHandler.GetGameHistory)
			games.GET("/:gameId/state", gameHandler.GetGameState)
			games.GET("/:gameId/players/:userId", gameHandler.GetPlayerInGame)
		}

		// Authenticated website endpoints (JWT required; user_id comes from the token)
		authed := api.Group("")
		authed.Use(middleware.AuthMiddleware(jwtService))
		{
			// Profile
			authed.GET("/me", userHandler.GetMe)
			authed.PUT("/me/name", userHandler.UpdateMyName)

			// Wallet (self)
			authed.GET("/me/wallet", walletHandler.GetMyWallet)
			authed.GET("/me/wallet/deposits", walletHandler.GetMyDeposits)
			authed.GET("/me/wallet/withdrawals", walletHandler.GetMyWithdrawals)
			authed.GET("/me/wallet/transfers", walletHandler.GetMyTransfers)
			authed.POST("/wallet/deposit", walletHandler.Deposit)
			authed.POST("/wallet/withdraw", walletHandler.Withdraw)
			authed.POST("/wallet/transfer", walletHandler.Transfer)

			// Games (self)
			authed.GET("/me/games", gameHandler.GetMyGameHistory)
			authed.GET("/me/games/:gameId", gameHandler.GetMyPlayerInGame)
			authed.POST("/games/:gameId/join", gameHandler.JoinGame)
			authed.POST("/games/:gameId/leave", gameHandler.LeaveGame)
			authed.POST("/games/:gameId/bingo", gameHandler.ClaimBingo)
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
			admin.GET("/users/:user_id", userHandler.GetUserDetail)
			admin.POST("/users/:user_id/role", userHandler.SetUserRole)
			admin.POST("/users/:user_id/ban", userHandler.BanUser)
			admin.POST("/users/:user_id/unban", userHandler.UnbanUser)
			admin.POST("/users/:user_id/adjust-balance", walletHandler.AdjustBalance)

			// Dashboard stats
			stats := admin.Group("/stats")
			{
				stats.GET("/dashboard", walletHandler.GetDashboardStats)
			}

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
