package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/bingo/backend/config"
	"github.com/redis/go-redis/v9"
)

// Client wraps the Redis client
type Client struct {
	client *redis.Client
}

// NewClient creates a new Redis client
func NewClient(cfg *config.Config) (*Client, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.GetAddr(),
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return &Client{client: rdb}, nil
}

// GetClient returns the underlying Redis client
func (c *Client) GetClient() *redis.Client {
	return c.client
}

// Close closes the Redis connection
func (c *Client) Close() error {
	return c.client.Close()
}

// Game State Keys
func GameStateKey(gameID string) string {
	return fmt.Sprintf("game:%s:state", gameID)
}

func GamePlayersKey(gameID string) string {
	return fmt.Sprintf("game:%s:players", gameID)
}

func GameDrawnNumbersKey(gameID string) string {
	return fmt.Sprintf("game:%s:drawn", gameID)
}

func GameTakenCardsKey(gameID string) string {
	return fmt.Sprintf("game:%s:cards:taken", gameID)
}

func GameCountdownKey(gameID string) string {
	return fmt.Sprintf("game:%s:countdown", gameID)
}

// Pub/Sub Channels
func GameChannel(gameID string) string {
	return fmt.Sprintf("game:%s:events", gameID)
}

// BonusCampaignChannel carries live "first N players" giveaway events to
// subscribed admin dashboards. A single global channel, not one per campaign,
// because only one campaign is ever active at a time.
const BonusCampaignChannel = "admin:bonus_campaign:events"
