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
// Defaults match main's docker-compose.yml:
//
//	postgres   → 15432
//	redis      → 16380
//	clickhouse → 19001 (native)
//	kafka      → 19094
//	bidder     → 18180
//
// Pre-merge these defaults pointed at the engine worktree stack
// (17432/18380/21001/21094/20180) which no longer exists on main.
// Override via QA_POSTGRES_DSN / QA_REDIS_ADDR / etc. when pointing
// at a different stack. Passwords default to the dev defaults.
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
		PostgresDSN:     getenv("QA_POSTGRES_DSN", "postgres://dsp:dsp_dev_password@localhost:15432/dsp?sslmode=disable"),
		RedisAddr:       getenv("QA_REDIS_ADDR", "localhost:16380"),
		RedisPassword:   getenv("QA_REDIS_PASSWORD", "dsp_dev_password"),
		RedisDB:         getenvInt("QA_REDIS_DB", 15),
		KafkaBrokers:    []string{getenv("QA_KAFKA_BROKERS", "localhost:19094")},
		ClickHouseAddr:  getenv("QA_CLICKHOUSE_ADDR", "localhost:19001"),
		ClickHouseUser:  getenv("QA_CLICKHOUSE_USER", "default"),
		ClickHousePass:  getenv("QA_CLICKHOUSE_PASSWORD", "dsp_dev_password"),
		BidderPublicURL: getenv("QA_BIDDER_PUBLIC_URL", "http://localhost:18180"),
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
