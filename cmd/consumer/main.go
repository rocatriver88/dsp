package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strings"

	"github.com/heartgryphon/dsp/internal/config"
	"github.com/heartgryphon/dsp/internal/events"
	"github.com/heartgryphon/dsp/internal/reporting"
)

func main() {
	cfg := config.Load()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	store, err := reporting.NewStore(cfg.ClickHouseAddr, cfg.ClickHouseUser, cfg.ClickHousePassword)
	if err != nil {
		log.Fatalf("connect clickhouse: %v", err)
	}
	defer store.Close()
	log.Println("Connected to ClickHouse")

	brokers := strings.Split(cfg.KafkaBrokers, ",")
	dlqProducer := events.NewProducer(brokers, "/tmp/dsp-kafka-buffer-consumer")
	defer dlqProducer.Close()

	deps := RunnerDeps{
		Brokers:     brokers,
		Topics:      []string{"dsp.bids", "dsp.impressions"},
		GroupID:     "dsp-clickhouse-consumer",
		Store:       store,
		DLQProducer: dlqProducer,
	}

	log.Println("Kafka → ClickHouse consumer running. Press Ctrl+C to stop.")
	RunConsumer(ctx, deps)
	log.Println("Shutting down consumer...")
}
