package cache

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	DefaultExpiration      = 24 * time.Hour * 7 // 7 days
	IndexerComandoTorrents = "indexer:comando_torrents"
)

type Redis struct {
	client *redis.Client
}

func NewRedis() *Redis {
	redisHost := os.Getenv("REDIS_HOST")
	if redisHost == "" {
		redisHost = "localhost"
	}
	return &Redis{
		client: redis.NewClient(&redis.Options{
			Addr:     fmt.Sprintf("%s:6379", redisHost),
			Password: "",
		}),
	}
}

func (r *Redis) Get(ctx context.Context, key string) ([]byte, error) {
	return r.client.Get(ctx, key).Bytes()
}

func (r *Redis) Set(ctx context.Context, key string, value []byte) error {
	return r.client.Set(ctx, key, value, DefaultExpiration).Err()
}

func (r *Redis) SetWithExpiration(ctx context.Context, key string, value []byte, expiration time.Duration) error {
	return r.client.Set(ctx, key, value, expiration).Err()
}
