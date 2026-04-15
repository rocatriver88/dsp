package qaharness

import (
	"context"
	"os"
	"strconv"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
)

// Env holds connection parameters for the compose stack of this worktree.
// Defaults match .worktrees/engine/docker-compose.override.yml (+12000 offset).
// Passwords default to the compose stack's dev defaults (dsp_dev_password).
type Env struct {
	PostgresDSN     string
	RedisAddr       string
	RedisPassword   string
	RedisDB         int
	KafkaBrokers    []string
	ClickHouseAddr  string
	ClickHouseUser  string
	ClickHousePass  string
	BidderPublicURL string
}

func LoadEnv() *Env {
	return &Env{
		PostgresDSN:     getenv("QA_POSTGRES_DSN", "postgres://dsp:dsp_dev_password@localhost:17432/dsp?sslmode=disable"),
		RedisAddr:       getenv("QA_REDIS_ADDR", "localhost:18380"),
		RedisPassword:   getenv("QA_REDIS_PASSWORD", "dsp_dev_password"),
		RedisDB:         getenvInt("QA_REDIS_DB", 15),
		KafkaBrokers:    []string{getenv("QA_KAFKA_BROKERS", "localhost:21094")},
		ClickHouseAddr:  getenv("QA_CLICKHOUSE_ADDR", "localhost:21001"),
		ClickHouseUser:  getenv("QA_CLICKHOUSE_USER", "default"),
		ClickHousePass:  getenv("QA_CLICKHOUSE_PASSWORD", "dsp_dev_password"),
		BidderPublicURL: getenv("QA_BIDDER_PUBLIC_URL", "http://localhost:20180"),
	}
}

func (e *Env) OpenPostgres(ctx context.Context) (*pgxpool.Pool, error) {
	return pgxpool.New(ctx, e.PostgresDSN)
}

func (e *Env) OpenRedis() *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:     e.RedisAddr,
		Password: e.RedisPassword,
		DB:       e.RedisDB,
	})
}

func (e *Env) OpenClickHouse() (driver.Conn, error) {
	return clickhouse.Open(&clickhouse.Options{
		Addr: []string{e.ClickHouseAddr},
		Auth: clickhouse.Auth{Database: "default", Username: e.ClickHouseUser, Password: e.ClickHousePass},
	})
}

func (e *Env) KafkaDialer() *kafka.Dialer {
	return &kafka.Dialer{Timeout: 5 * time.Second}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
