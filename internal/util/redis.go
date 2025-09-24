package util

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

func GenerateRedisURL(host string, port int, password string, db int) string {
	if password != "" {
		return fmt.Sprintf("redis://:%s@%s:%d/%d", password, host, port, db)
	}
	return fmt.Sprintf("redis://%s:%d/%d", host, port, db)
}

func TestRedisConnection(ctx context.Context, host string, port int, password string, db int) error {
	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", host, port),
		Password: password,
		DB:       db,
	})

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err := client.Ping(ctx).Result()
	if err != nil {
		return fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return nil
}
