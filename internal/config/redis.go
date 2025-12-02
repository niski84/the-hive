package config

import (
	"context"
	"log"
	"os"
	"strconv"

	"github.com/redis/go-redis/v9"
)

// NewRedisClient creates a new Redis client from environment variables.
// Reads REDIS_ADDR (default: 127.0.0.1:6379), REDIS_DB (default: 0), and REDIS_PASSWORD (optional).
// Returns a ready-to-use Redis client or an error.
func NewRedisClient(ctx context.Context) (*redis.Client, error) {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "127.0.0.1:6379"
	}

	dbStr := os.Getenv("REDIS_DB")
	if dbStr == "" {
		dbStr = "0"
	}
	db, err := strconv.Atoi(dbStr)
	if err != nil {
		log.Printf("NewRedisClient: invalid REDIS_DB value '%s', using default 0", dbStr)
		db = 0
	}

	password := os.Getenv("REDIS_PASSWORD")

	log.Printf("NewRedisClient: addr=%s db=%d passwordSet=%v", addr, db, password != "")

	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		DB:       db,
		Password: password,
	})

	// Test connection
	if err := client.Ping(ctx).Err(); err != nil {
		log.Printf("NewRedisClient: failed to ping Redis: %v", err)
		return nil, err
	}

	log.Printf("NewRedisClient: successfully connected to Redis")
	return client, nil
}

