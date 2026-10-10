package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type Client interface {
	SetOTP(ctx context.Context, identifier string, otp string, ttl time.Duration) error
	GetOTP(ctx context.Context, identifier string) (string, error)
	DeleteOTP(ctx context.Context, identifier string) error
	IncrementRateLimit(ctx context.Context, key string, window time.Duration) (int64, error)
}

type redisClient struct {
	rdb *redis.Client
}

func NewRedisClient(rdb *redis.Client) Client {
	return &redisClient{rdb: rdb}
}

func (c *redisClient) SetOTP(ctx context.Context, identifier string, otp string, ttl time.Duration) error {
	key := fmt.Sprintf("otp:%s", identifier)
	return c.rdb.Set(ctx, key, otp, ttl).Err()
}

func (c *redisClient) GetOTP(ctx context.Context, identifier string) (string, error) {
	key := fmt.Sprintf("otp:%s", identifier)
	return c.rdb.Get(ctx, key).Result()
}

func (c *redisClient) DeleteOTP(ctx context.Context, identifier string) error {
	key := fmt.Sprintf("otp:%s", identifier)
	return c.rdb.Del(ctx, key).Err()
}

// IncrementRateLimit increments the counter for a given key.
// If the key doesn't exist, it sets the TTL to the provided window.
// Returns the current count.
func (c *redisClient) IncrementRateLimit(ctx context.Context, key string, window time.Duration) (int64, error) {
	pipe := c.rdb.TxPipeline()
	incrCmd := pipe.Incr(ctx, key)
	// We only want to set expiry if the key is newly created, but for simplicity in high concurrency, 
	// we just let INCR do its job and set EXPIRE only if count == 1 (handled after execution).
	// An atomic approach using Lua is better, but this is a simple sliding window simulation.
	
	_, err := pipe.Exec(ctx)
	if err != nil {
		return 0, err
	}

	count := incrCmd.Val()
	if count == 1 {
		c.rdb.Expire(ctx, key, window)
	}

	return count, nil
}
