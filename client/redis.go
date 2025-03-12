package client

import (
	"context"
	"log"
	"reacher-cron/config"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
)

var (
	rdb   *redis.Client
	rOnce sync.Once
	Ctx   = context.Background()
)

func ConnectRedis() *redis.Client {
	rOnce.Do(func() {
		opts, err := redis.ParseURL(config.AppConfig.RedisURI)
		if err != nil {
			log.Fatal("Failed to parse redis URL:", err)
		}
		rdb = redis.NewClient(opts)
		if err = rdb.Ping(Ctx).Err(); err != nil {
			log.Fatal("Redis connection failed:", err)
		}
		rdb.Options().DialTimeout = 5 * time.Second
		rdb.Options().ReadTimeout = 5 * time.Second
		rdb.Options().WriteTimeout = 5 * time.Second
	})
	return rdb
}
