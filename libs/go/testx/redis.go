package testx

import (
	"fmt"
	"strings"
	"testing"

	"github.com/testcontainers/testcontainers-go/modules/redis"
)

var rds started

// Redis khoi dong (mot lan cho ca tien trinh) redis:7-alpine va tra ve "host:port".
func Redis(t testing.TB) string {
	t.Helper()
	return rds.get(t, RedisImage, startRedis)
}

func startRedis() (string, error) {
	ctx, cancel := startCtx()
	defer cancel()

	ctr, err := redis.Run(ctx, RedisImage)
	if ctr != nil {
		terminate(ctr)
	}
	if err != nil {
		return "", err
	}

	uri, err := ctr.ConnectionString(ctx)
	if err != nil {
		return "", fmt.Errorf("lay dia chi: %w", err)
	}
	return strings.TrimPrefix(uri, "redis://"), nil
}
