package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
	
	"github.com/redis/go-redis/v9"
)

// APIKeyInfo API Key 缓存信息
type APIKeyInfo struct {
	ID                 uint
	Enabled            bool
	ExpiresAt          *time.Time
	ChannelRestriction *string
}

// APIKeyCache API Key 缓存层
type APIKeyCache struct {
	client *redis.Client
	ttl    time.Duration
}

// NewAPIKeyCache 创建 API Key 缓存实例
func NewAPIKeyCache(client *redis.Client, ttl time.Duration) *APIKeyCache {
	if ttl == 0 {
		ttl = 5 * time.Minute // 默认 5 分钟
	}
	return &APIKeyCache{
		client: client,
		ttl:    ttl,
	}
}

// Get 从缓存获取 API Key 信息
func (c *APIKeyCache) Get(ctx context.Context, key string) (*APIKeyInfo, error) {
	data, err := c.client.Get(ctx, c.cacheKey(key)).Bytes()
	if err != nil {
		return nil, err
	}
	
	var info APIKeyInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, fmt.Errorf("failed to unmarshal api key info: %w", err)
	}
	
	return &info, nil
}

// Set 将 API Key 信息写入缓存
func (c *APIKeyCache) Set(ctx context.Context, key string, info *APIKeyInfo) error {
	data, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("failed to marshal api key info: %w", err)
	}
	
	return c.client.Set(ctx, c.cacheKey(key), data, c.ttl).Err()
}

// Delete 删除缓存
func (c *APIKeyCache) Delete(ctx context.Context, key string) error {
	return c.client.Del(ctx, c.cacheKey(key)).Err()
}

// IsAvailable 检查 Redis 是否可用
func (c *APIKeyCache) IsAvailable(ctx context.Context) bool {
	return c.client.Ping(ctx).Err() == nil
}

func (c *APIKeyCache) cacheKey(key string) string {
	return "apikey:" + key
}
