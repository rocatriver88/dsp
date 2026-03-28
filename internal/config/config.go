package config

import "os"

type Config struct {
	DBHost         string
	DBPort         string
	DBUser         string
	DBPassword     string
	DBName         string
	RedisAddr      string
	KafkaBrokers   string
	ClickHouseAddr string
	APIPort        string
	BidderPort     string
}

func Load() *Config {
	return &Config{
		DBHost:         getEnv("DB_HOST", "localhost"),
		DBPort:         getEnv("DB_PORT", "5432"),
		DBUser:         getEnv("DB_USER", "dsp"),
		DBPassword:     getEnv("DB_PASSWORD", "dsp_dev_password"),
		DBName:         getEnv("DB_NAME", "dsp"),
		RedisAddr:      getEnv("REDIS_ADDR", "localhost:6380"),
		KafkaBrokers:   getEnv("KAFKA_BROKERS", "localhost:9094"),
		ClickHouseAddr: getEnv("CLICKHOUSE_ADDR", "localhost:9001"),
		APIPort:        getEnv("API_PORT", "8181"),
		BidderPort:     getEnv("BIDDER_PORT", "8180"),
	}
}

func (c *Config) DSN() string {
	return "postgres://" + c.DBUser + ":" + c.DBPassword + "@" + c.DBHost + ":" + c.DBPort + "/" + c.DBName + "?sslmode=disable"
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
