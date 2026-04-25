package cache

import (
	"context"
	"fmt"
	"log/slog"
	
	"github.com/redis/go-redis/v9"
)

// NewRedisClient 创建 Redis 客户端
func NewRedisClient(redisURL string) (*redis.Client, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse redis url: %w", err)
	}
	
	client := redis.NewClient(opts)
	
	// 测试连接
	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}
	
	slog.Info("redis connected successfully")
	return client, nil
}
